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

	"ewallet-monolith/internal/auth"
)

type payRequest struct {
	MerchantID  string  `json:"merchantID" binding:"required"`
	Amount      float64 `json:"amount" binding:"required"`
	Description string  `json:"description"`
}

// CreatePayment (POST /transactions/pay) opens a payment *session*.
// Nothing is deducted yet; the session is valid for Cfg.PaymentTTL (10 min).
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

	var balance float64
	var enough bool
	err := h.DB.QueryRowContext(ctx,
		`SELECT balance, balance >= $2::numeric FROM wallets WHERE user_id = $1`,
		userID, req.Amount).Scan(&balance, &enough)
	if errors.Is(err, sql.ErrNoRows) {
		fail(c, http.StatusUnauthorized, "wallet not found")
		return
	}
	if err != nil {
		fail(c, http.StatusInternalServerError, "database error")
		return
	}
	if !enough {
		fail(c, http.StatusBadRequest, "insufficient funds")
		return
	}

	id := uuid.NewString()
	var expiresAt time.Time
	err = h.DB.QueryRowContext(ctx,
		`INSERT INTO transactions (id, type, from_user, merchant_id, description, amount, status, expires_at)
		 VALUES ($1, 'payment', $2, $3, $4, $5, 'pending', now() + ($6::float8 * interval '1 second'))
		 RETURNING expires_at`,
		id, userID, req.MerchantID, req.Description, req.Amount, h.Cfg.PaymentTTL.Seconds()).Scan(&expiresAt)
	if err != nil {
		log.Printf("create payment: %v", err)
		fail(c, http.StatusInternalServerError, "database error")
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"transactionID": id,
		"message":       "Payment session created. Confirm it before it expires.",
		"newBalance":    balance, // unchanged until the session is confirmed
		"status":        "pending",
		"expiresAt":     expiresAt,
	})
}

// GetPayment returns the live status of a payment session.
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
		 FROM transactions WHERE id = $1 AND type = 'payment' AND from_user = $2`,
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
		status = "expired" // the sweeper hasn't persisted it yet
	}
	c.JSON(http.StatusOK, gin.H{
		"transactionID": id, "status": status, "merchantID": merchant,
		"amount": amount, "expiresAt": expiresAt,
	})
}

// ConfirmPayment settles a session. The expiry check and the deduction happen in
// ONE database transaction under a row lock, so an expired session can never be paid.
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

	var amount float64
	var status string
	var expired bool
	err = tx.QueryRowContext(ctx,
		`SELECT amount, status, expires_at <= now()
		 FROM transactions WHERE id = $1 AND type = 'payment' AND from_user = $2 FOR UPDATE`,
		id, userID).Scan(&amount, &status, &expired)
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
			`UPDATE transactions SET status = 'expired', completed_at = now() WHERE id = $1`, id); err != nil {
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

	var newBalance float64
	err = tx.QueryRowContext(ctx,
		`UPDATE wallets SET balance = balance - $1, updated_at = now()
		 WHERE user_id = $2 AND balance >= $1 RETURNING balance`, amount, userID).Scan(&newBalance)
	if errors.Is(err, sql.ErrNoRows) {
		// session stays pending: the user may top up and retry before it expires
		fail(c, http.StatusBadRequest, "insufficient funds")
		return
	}
	if err != nil {
		log.Printf("confirm payment: %v", err)
		fail(c, http.StatusInternalServerError, "database error")
		return
	}
	if _, err := tx.ExecContext(ctx,
		`UPDATE transactions SET status = 'completed', completed_at = now() WHERE id = $1`, id); err != nil {
		fail(c, http.StatusInternalServerError, "database error")
		return
	}
	if err := tx.Commit(); err != nil {
		fail(c, http.StatusInternalServerError, "database error")
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"transactionID": id,
		"message":       "Payment successful",
		"newBalance":    newBalance,
	})
}
