package handler

import (
	"database/sql"
	"errors"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"ewallet-payment/internal/auth"
	"ewallet-payment/internal/corekit"
)

type payRequest struct {
	MerchantID  string  `json:"merchantID" binding:"required"`
	Amount      float64 `json:"amount" binding:"required"`
	Description string  `json:"description"`
}

// CreatePayment opens a payment session valid for Cfg.PaymentTTL (10 min). No money moves yet.
func (h *Handler) CreatePayment(c *gin.Context) {
	var req payRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, http.StatusBadRequest, "merchantID and amount are required")
		return
	}
	req.MerchantID = strings.TrimSpace(req.MerchantID)
	if req.MerchantID == "" || !validAmount(req.Amount) {
		fail(c, http.StatusBadRequest, "merchantID is required; "+errAmount)
		return
	}
	userID := auth.UserID(c)
	ctx := c.Request.Context()

	balance, err := h.Core.Balance(ctx, userID)
	if errors.Is(err, corekit.ErrWalletNotFound) {
		fail(c, http.StatusUnauthorized, "wallet not found")
		return
	}
	if err != nil {
		log.Printf("core balance: %v", err)
		fail(c, http.StatusServiceUnavailable, "wallet service unavailable")
		return
	}
	if balance < req.Amount {
		fail(c, http.StatusBadRequest, "insufficient funds")
		return
	}

	id := uuid.NewString()
	var expiresAt time.Time
	err = h.DB.QueryRowContext(ctx,
		`INSERT INTO payment_sessions (id, user_id, merchant_id, description, amount, status, expires_at)
		 VALUES ($1, $2, $3, $4, $5, 'pending', now() + ($6::float8 * interval '1 second'))
		 RETURNING expires_at`,
		id, userID, req.MerchantID, req.Description, req.Amount, h.Cfg.PaymentTTL.Seconds()).Scan(&expiresAt)
	if err != nil {
		log.Printf("create session: %v", err)
		fail(c, http.StatusInternalServerError, "database error")
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"transactionID": id,
		"message":       "Payment session created. Confirm it before it expires.",
		"newBalance":    balance,
		"status":        "pending",
		"expiresAt":     expiresAt,
	})
}

func (h *Handler) GetPayment(c *gin.Context) {
	id := c.Param("id")
	if _, err := uuid.Parse(id); err != nil {
		fail(c, http.StatusBadRequest, "invalid payment id")
		return
	}
	var status, merchant string
	var amount float64
	var expiresAt time.Time
	var expired bool
	err := h.DB.QueryRowContext(c.Request.Context(),
		`SELECT status, merchant_id, amount, expires_at, expires_at <= now()
		 FROM payment_sessions WHERE id = $1 AND user_id = $2`,
		id, auth.UserID(c)).Scan(&status, &merchant, &amount, &expiresAt, &expired)
	if errors.Is(err, sql.ErrNoRows) {
		fail(c, http.StatusNotFound, "payment not found")
		return
	}
	if err != nil {
		fail(c, http.StatusInternalServerError, "database error")
		return
	}
	if status == "pending" && expired {
		status = "expired"
	}
	c.JSON(http.StatusOK, gin.H{
		"transactionID": id, "status": status, "merchantID": merchant,
		"amount": amount, "expiresAt": expiresAt,
	})
}

// ConfirmPayment: lock the session row, reject if expired, then ask core-service
// to debit (idempotent on the session ID) and mark the session completed.
func (h *Handler) ConfirmPayment(c *gin.Context) {
	id := c.Param("id")
	if _, err := uuid.Parse(id); err != nil {
		fail(c, http.StatusBadRequest, "invalid payment id")
		return
	}
	userID := auth.UserID(c)
	ctx := c.Request.Context()

	tx, err := h.DB.BeginTx(ctx, nil)
	if err != nil {
		fail(c, http.StatusInternalServerError, "database error")
		return
	}
	defer tx.Rollback()

	var merchant, description string
	var amount float64
	var status string
	var expired bool
	err = tx.QueryRowContext(ctx,
		`SELECT merchant_id, COALESCE(description, ''), amount, status, expires_at <= now()
		 FROM payment_sessions WHERE id = $1 AND user_id = $2 FOR UPDATE`,
		id, userID).Scan(&merchant, &description, &amount, &status, &expired)
	if errors.Is(err, sql.ErrNoRows) {
		fail(c, http.StatusNotFound, "payment not found")
		return
	}
	if err != nil {
		fail(c, http.StatusInternalServerError, "database error")
		return
	}

	switch status {
	case "completed":
		fail(c, http.StatusConflict, "payment already completed")
		return
	case "expired":
		fail(c, http.StatusGone, "payment session expired")
		return
	case "failed":
		fail(c, http.StatusConflict, "payment already failed")
		return
	}

	if expired {
		if _, err := tx.ExecContext(ctx,
			`UPDATE payment_sessions SET status = 'expired', completed_at = now() WHERE id = $1`, id); err != nil {
			fail(c, http.StatusInternalServerError, "database error")
			return
		}
		if err := tx.Commit(); err != nil {
			fail(c, http.StatusInternalServerError, "database error")
			return
		}
		fail(c, http.StatusGone, "payment session expired")
		return
	}

	newBalance, err := h.Core.Debit(ctx, corekit.DebitRequest{
		Reference: id, UserID: userID, MerchantID: merchant, Description: description, Amount: amount,
	})
	if errors.Is(err, corekit.ErrInsufficientFunds) {
		fail(c, http.StatusBadRequest, "insufficient funds") // stays pending; user may top up and retry
		return
	}
	if err != nil {
		log.Printf("core debit: %v", err)
		fail(c, http.StatusServiceUnavailable, "wallet service unavailable, please retry")
		return
	}

	if _, err := tx.ExecContext(ctx,
		`UPDATE payment_sessions SET status = 'completed', completed_at = now() WHERE id = $1`, id); err != nil {
		fail(c, http.StatusInternalServerError, "database error")
		return
	}
	if err := tx.Commit(); err != nil {
		// money already moved; a retry of /confirm is safe because the debit is idempotent
		fail(c, http.StatusInternalServerError, "database error, please retry")
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"transactionID": id,
		"message":       "Payment successful",
		"newBalance":    newBalance,
	})
}
