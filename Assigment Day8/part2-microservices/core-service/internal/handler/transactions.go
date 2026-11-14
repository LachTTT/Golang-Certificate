package handler

import (
	"context"
	"database/sql"
	"errors"
	"log"
	"math"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"ewallet-core/internal/auth"
)

// ---------------------------------------------------------------- top-up

type topUpRequest struct {
	Amount float64 `json:"amount" binding:"required"`
}

func (h *Handler) TopUp(c *gin.Context) {
	var req topUpRequest
	if err := c.ShouldBindJSON(&req); err != nil || !validAmount(req.Amount) {
		fail(c, http.StatusBadRequest, errAmount)
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

	var newBalance float64
	err = tx.QueryRowContext(ctx,
		`UPDATE wallets SET balance = balance + $1, updated_at = now()
		 WHERE user_id = $2 RETURNING balance`, req.Amount, userID).Scan(&newBalance)
	if errors.Is(err, sql.ErrNoRows) {
		fail(c, http.StatusUnauthorized, "wallet not found")
		return
	}
	if err != nil {
		log.Printf("topup: %v", err)
		fail(c, http.StatusInternalServerError, "database error")
		return
	}

	txID := uuid.NewString()
	if _, err := tx.ExecContext(ctx,
		`INSERT INTO transactions (id, type, to_user, amount, status, completed_at)
		 VALUES ($1, 'topup', $2, $3, 'completed', now())`, txID, userID, req.Amount); err != nil {
		log.Printf("topup insert: %v", err)
		fail(c, http.StatusInternalServerError, "database error")
		return
	}
	if err := tx.Commit(); err != nil {
		fail(c, http.StatusInternalServerError, "database error")
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"transactionID": txID,
		"message":       "Top-up successful",
		"newBalance":    newBalance,
	})
}

// -------------------------------------------------------------- transfer

type transferRequest struct {
	ToUserID string  `json:"toUserID" binding:"required"`
	Amount   float64 `json:"amount" binding:"required"`
}

// Transfer validates, records a PENDING transaction, answers 202 and then
// settles the money movement asynchronously (see settleTransfer).
func (h *Handler) Transfer(c *gin.Context) {
	var req transferRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, http.StatusBadRequest, "toUserID and amount are required")
		return
	}
	if !validAmount(req.Amount) {
		fail(c, http.StatusBadRequest, errAmount)
		return
	}
	to, err := uuid.Parse(req.ToUserID)
	if err != nil {
		fail(c, http.StatusBadRequest, "toUserID must be a valid UUID")
		return
	}
	from := auth.UserID(c)
	if to.String() == from {
		fail(c, http.StatusBadRequest, "cannot transfer to yourself")
		return
	}

	ctx := c.Request.Context()
	var recipientExists bool
	if err := h.DB.QueryRowContext(ctx,
		`SELECT EXISTS (SELECT 1 FROM wallets WHERE user_id = $1)`, to.String()).Scan(&recipientExists); err != nil {
		fail(c, http.StatusInternalServerError, "database error")
		return
	}
	if !recipientExists {
		fail(c, http.StatusBadRequest, "recipient not found")
		return
	}

	var enough bool
	err = h.DB.QueryRowContext(ctx,
		`SELECT balance >= $2::numeric FROM wallets WHERE user_id = $1`, from, req.Amount).Scan(&enough)
	if err != nil {
		fail(c, http.StatusInternalServerError, "database error")
		return
	}
	if !enough {
		fail(c, http.StatusBadRequest, "insufficient funds")
		return
	}

	id := uuid.NewString()
	if _, err := h.DB.ExecContext(ctx,
		`INSERT INTO transactions (id, type, from_user, to_user, amount, status)
		 VALUES ($1, 'transfer', $2, $3, $4, 'pending')`, id, from, to.String(), req.Amount); err != nil {
		log.Printf("transfer insert: %v", err)
		fail(c, http.StatusInternalServerError, "database error")
		return
	}

	go h.processTransfer(id)

	c.JSON(http.StatusAccepted, gin.H{
		"transactionID": id,
		"status":        "pending",
		"message":       "Transfer initiated successfully.",
	})
}

func (h *Handler) processTransfer(id string) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	if err := h.settleTransfer(ctx, id); err != nil {
		log.Printf("transfer %s: %v", id, err)
	}
}

// settleTransfer moves the money atomically. It is idempotent: a transaction
// that is no longer 'pending' is left untouched, so retries are safe.
func (h *Handler) settleTransfer(ctx context.Context, id string) error {
	tx, err := h.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	var from, to, status string
	var amount float64
	err = tx.QueryRowContext(ctx,
		`SELECT from_user::text, to_user::text, amount, status
		 FROM transactions WHERE id = $1 AND type = 'transfer' FOR UPDATE`, id).Scan(&from, &to, &amount, &status)
	if err != nil {
		return err
	}
	if status != "pending" {
		return nil
	}

	// Lock both wallets in a fixed order so concurrent opposite transfers can't deadlock.
	rows, err := tx.QueryContext(ctx,
		`SELECT user_id::text FROM wallets WHERE user_id IN ($1, $2) ORDER BY user_id FOR UPDATE`, from, to)
	if err != nil {
		return err
	}
	for rows.Next() {
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return err
	}
	rows.Close()

	var enough bool
	if err := tx.QueryRowContext(ctx,
		`SELECT balance >= $2::numeric FROM wallets WHERE user_id = $1`, from, amount).Scan(&enough); err != nil {
		return err
	}
	if !enough {
		if _, err := tx.ExecContext(ctx,
			`UPDATE transactions SET status = 'failed', completed_at = now() WHERE id = $1`, id); err != nil {
			return err
		}
		return tx.Commit()
	}

	if _, err := tx.ExecContext(ctx,
		`UPDATE wallets SET balance = balance - $1, updated_at = now() WHERE user_id = $2`, amount, from); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx,
		`UPDATE wallets SET balance = balance + $1, updated_at = now() WHERE user_id = $2`, amount, to); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx,
		`UPDATE transactions SET status = 'completed', completed_at = now() WHERE id = $1`, id); err != nil {
		return err
	}
	return tx.Commit()
}

// --------------------------------------------------------------- history

type historyItem struct {
	TransactionID string    `json:"transactionID"`
	Type          string    `json:"type"`
	Amount        float64   `json:"amount"`
	From          string    `json:"from"`
	To            string    `json:"to"`
	Timestamp     time.Time `json:"timestamp"`
	Status        string    `json:"status"`
}

func (h *Handler) History(c *gin.Context) {
	userID := auth.UserID(c)
	page, err := strconv.Atoi(c.DefaultQuery("page", "1"))
	if err != nil || page < 1 {
		page = 1
	}
	limit, err := strconv.Atoi(c.DefaultQuery("limit", "10"))
	if err != nil || limit < 1 {
		limit = 10
	}
	if limit > 100 {
		limit = 100
	}
	ctx := c.Request.Context()

	var total int
	if err := h.DB.QueryRowContext(ctx,
		`SELECT count(*) FROM transactions WHERE from_user = $1 OR to_user = $1`, userID).Scan(&total); err != nil {
		fail(c, http.StatusInternalServerError, "database error")
		return
	}

	rows, err := h.DB.QueryContext(ctx,
		`SELECT id::text, type, amount, COALESCE(from_user::text, ''), COALESCE(to_user::text, ''),
		        COALESCE(merchant_id, ''), status, created_at
		 FROM transactions
		 WHERE from_user = $1 OR to_user = $1
		 ORDER BY created_at DESC, id
		 LIMIT $2 OFFSET $3`, userID, limit, (page-1)*limit)
	if err != nil {
		log.Printf("history: %v", err)
		fail(c, http.StatusInternalServerError, "database error")
		return
	}
	defer rows.Close()

	items := make([]historyItem, 0, limit)
	for rows.Next() {
		var it historyItem
		var typ, from, to, merchant, status string
		if err := rows.Scan(&it.TransactionID, &typ, &it.Amount, &from, &to, &merchant, &status, &it.Timestamp); err != nil {
			fail(c, http.StatusInternalServerError, "database error")
			return
		}
		switch typ {
		case "topup":
			it.Type, it.From, it.To = "topup", "topup", to
		case "payment":
			it.Type, it.From, it.To = "payment", from, merchant
		default: // transfer
			it.From, it.To = from, to
			if from == userID {
				it.Type = "transfer_out"
			} else {
				it.Type = "transfer_in"
			}
		}
		// OpenAPI only knows completed|pending|failed; an expired payment is a failed one.
		if status == "expired" {
			status = "failed"
		}
		it.Status = status
		items = append(items, it)
	}
	if err := rows.Err(); err != nil {
		fail(c, http.StatusInternalServerError, "database error")
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"transactions": items,
		"pagination": gin.H{
			"currentPage": page,
			"totalPages":  int(math.Ceil(float64(total) / float64(limit))),
			"totalItems":  total,
		},
	})
}
