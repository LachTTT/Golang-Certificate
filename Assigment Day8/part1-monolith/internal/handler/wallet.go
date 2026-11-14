package handler

import (
	"database/sql"
	"errors"
	"log"
	"net/http"

	"github.com/gin-gonic/gin"

	"ewallet-monolith/internal/auth"
)

func (h *Handler) GetBalance(c *gin.Context) {
	userID := auth.UserID(c)
	var balance float64
	err := h.DB.QueryRowContext(c.Request.Context(),
		`SELECT balance FROM wallets WHERE user_id = $1`, userID).Scan(&balance)
	if errors.Is(err, sql.ErrNoRows) {
		fail(c, http.StatusUnauthorized, "wallet not found")
		return
	}
	if err != nil {
		log.Printf("balance: %v", err)
		fail(c, http.StatusInternalServerError, "database error")
		return
	}
	c.JSON(http.StatusOK, gin.H{"userID": userID, "balance": balance})
}
