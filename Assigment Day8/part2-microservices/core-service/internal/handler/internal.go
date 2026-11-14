package handler

import (
	"crypto/subtle"
	"database/sql"
	"errors"
	"log"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
)

// internalAuth protects /internal/* with a shared secret (X-Internal-Token).
func internalAuth(token string) gin.HandlerFunc {
	return func(c *gin.Context) {
		got := c.GetHeader("X-Internal-Token")
		if subtle.ConstantTimeCompare([]byte(got), []byte(token)) != 1 {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "invalid internal token"})
			return
		}
		c.Next()
	}
}

// InternalBalance lets payment-service check funds before opening a session.
func (h *Handler) InternalBalance(c *gin.Context) {
	userID := c.Param("userID")
	if _, err := uuid.Parse(userID); err != nil {
		fail(c, http.StatusBadRequest, "invalid user id")
		return
	}
	var balance float64
	err := h.DB.QueryRowContext(c.Request.Context(),
		`SELECT balance FROM wallets WHERE user_id = $1`, userID).Scan(&balance)
	if errors.Is(err, sql.ErrNoRows) {
		fail(c, http.StatusNotFound, "wallet not found")
		return
	}
	if err != nil {
		fail(c, http.StatusInternalServerError, "database error")
		return
	}
	c.JSON(http.StatusOK, gin.H{"userID": userID, "balance": balance})
}

type debitRequest struct {
	Reference   string  `json:"reference" binding:"required"` // payment session ID (idempotency key)
	UserID      string  `json:"userID" binding:"required"`
	MerchantID  string  `json:"merchantID" binding:"required"`
	Description string  `json:"description"`
	Amount      float64 `json:"amount" binding:"required"`
}

// InternalDebit atomically debits a wallet for a payment and records it in the
// ledger (transactions table) so it shows up in /transactions/history.
// It is idempotent on `reference`: repeating a call never debits twice.
func (h *Handler) InternalDebit(c *gin.Context) {
	var req debitRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, http.StatusBadRequest, "reference, userID, merchantID and amount are required")
		return
	}
	req.MerchantID = strings.TrimSpace(req.MerchantID)
	if _, err := uuid.Parse(req.Reference); err != nil {
		fail(c, http.StatusBadRequest, "reference must be a UUID")
		return
	}
	if _, err := uuid.Parse(req.UserID); err != nil {
		fail(c, http.StatusBadRequest, "userID must be a UUID")
		return
	}
	if req.MerchantID == "" || !validAmount(req.Amount) {
		fail(c, http.StatusBadRequest, "merchantID is required; "+errAmount)
		return
	}
	ctx := c.Request.Context()

	tx, err := h.DB.BeginTx(ctx, nil)
	if err != nil {
		fail(c, http.StatusInternalServerError, "database error")
		return
	}
	defer tx.Rollback()

	// Already processed? Return the current state instead of debiting again.
	var existing string
	err = tx.QueryRowContext(ctx, `SELECT status FROM transactions WHERE id = $1`, req.Reference).Scan(&existing)
	if err == nil {
		var bal float64
		if err := tx.QueryRowContext(ctx,
			`SELECT balance FROM wallets WHERE user_id = $1`, req.UserID).Scan(&bal); err != nil {
			fail(c, http.StatusInternalServerError, "database error")
			return
		}
		c.JSON(http.StatusOK, gin.H{"transactionID": req.Reference, "newBalance": bal, "alreadyProcessed": true})
		return
	}
	if !errors.Is(err, sql.ErrNoRows) {
		fail(c, http.StatusInternalServerError, "database error")
		return
	}

	var newBalance float64
	err = tx.QueryRowContext(ctx,
		`UPDATE wallets SET balance = balance - $1, updated_at = now()
		 WHERE user_id = $2 AND balance >= $1 RETURNING balance`, req.Amount, req.UserID).Scan(&newBalance)
	if errors.Is(err, sql.ErrNoRows) {
		fail(c, http.StatusBadRequest, "insufficient funds")
		return
	}
	if err != nil {
		log.Printf("internal debit: %v", err)
		fail(c, http.StatusInternalServerError, "database error")
		return
	}

	if _, err := tx.ExecContext(ctx,
		`INSERT INTO transactions (id, type, from_user, merchant_id, description, amount, status, completed_at)
		 VALUES ($1, 'payment', $2, $3, $4, $5, 'completed', now())`,
		req.Reference, req.UserID, req.MerchantID, req.Description, req.Amount); err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			// concurrent duplicate: our debit is rolled back, caller should retry
			fail(c, http.StatusConflict, "duplicate reference, retry")
			return
		}
		log.Printf("internal debit insert: %v", err)
		fail(c, http.StatusInternalServerError, "database error")
		return
	}
	if err := tx.Commit(); err != nil {
		fail(c, http.StatusInternalServerError, "database error")
		return
	}
	c.JSON(http.StatusOK, gin.H{"transactionID": req.Reference, "newBalance": newBalance, "alreadyProcessed": false})
}
