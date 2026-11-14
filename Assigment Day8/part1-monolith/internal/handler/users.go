package handler

import (
	"database/sql"
	"errors"
	"log"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
	"golang.org/x/crypto/bcrypt"

	"ewallet-monolith/internal/auth"
)

// Used to keep login timing similar whether or not the email exists.
var dummyHash, _ = bcrypt.GenerateFromPassword([]byte("not-a-real-password"), bcrypt.DefaultCost)

type registerRequest struct {
	Name     string `json:"name" binding:"required"`
	Email    string `json:"email" binding:"required,email"`
	Password string `json:"password" binding:"required,min=8,max=72"`
}

func (h *Handler) RegisterUser(c *gin.Context) {
	var req registerRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, http.StatusBadRequest, "name, valid email, and password (8-72 chars) are required")
		return
	}
	req.Name = strings.TrimSpace(req.Name)
	req.Email = strings.ToLower(strings.TrimSpace(req.Email))
	if req.Name == "" {
		fail(c, http.StatusBadRequest, "name must not be empty")
		return
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		fail(c, http.StatusInternalServerError, "could not process password")
		return
	}

	ctx := c.Request.Context()
	tx, err := h.DB.BeginTx(ctx, nil)
	if err != nil {
		fail(c, http.StatusInternalServerError, "database error")
		return
	}
	defer tx.Rollback()

	id := uuid.NewString()
	if _, err := tx.ExecContext(ctx,
		`INSERT INTO users (id, name, email, password_hash) VALUES ($1,$2,$3,$4)`,
		id, req.Name, req.Email, string(hash)); err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			fail(c, http.StatusConflict, "user with this email already exists")
			return
		}
		log.Printf("register: %v", err)
		fail(c, http.StatusInternalServerError, "database error")
		return
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO wallets (user_id) VALUES ($1)`, id); err != nil {
		log.Printf("register wallet: %v", err)
		fail(c, http.StatusInternalServerError, "database error")
		return
	}
	if err := tx.Commit(); err != nil {
		fail(c, http.StatusInternalServerError, "database error")
		return
	}

	c.JSON(http.StatusCreated, gin.H{
		"userID":  id,
		"name":    req.Name,
		"email":   req.Email,
		"message": "User registered successfully",
	})
}

type loginRequest struct {
	Email    string `json:"email" binding:"required,email"`
	Password string `json:"password" binding:"required"`
}

func (h *Handler) Login(c *gin.Context) {
	var req loginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, http.StatusBadRequest, "email and password are required")
		return
	}
	email := strings.ToLower(strings.TrimSpace(req.Email))

	var id, hash string
	err := h.DB.QueryRowContext(c.Request.Context(),
		`SELECT id::text, password_hash FROM users WHERE email = $1`, email).Scan(&id, &hash)
	if errors.Is(err, sql.ErrNoRows) {
		_ = bcrypt.CompareHashAndPassword(dummyHash, []byte(req.Password))
		fail(c, http.StatusUnauthorized, "invalid email or password")
		return
	}
	if err != nil {
		log.Printf("login: %v", err)
		fail(c, http.StatusInternalServerError, "database error")
		return
	}
	if bcrypt.CompareHashAndPassword([]byte(hash), []byte(req.Password)) != nil {
		fail(c, http.StatusUnauthorized, "invalid email or password")
		return
	}

	token, err := auth.GenerateToken(h.Cfg.JWTSecret, id, h.Cfg.JWTTTL)
	if err != nil {
		fail(c, http.StatusInternalServerError, "could not issue token")
		return
	}
	c.JSON(http.StatusOK, gin.H{"token": token})
}
