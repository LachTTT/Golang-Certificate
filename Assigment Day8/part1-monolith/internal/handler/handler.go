package handler

import (
	"database/sql"
	"math"
	"net/http"

	"github.com/gin-gonic/gin"

	"ewallet-monolith/internal/auth"
	"ewallet-monolith/internal/config"
)

type Handler struct {
	DB  *sql.DB
	Cfg config.Config
}

func New(db *sql.DB, cfg config.Config) *Handler { return &Handler{DB: db, Cfg: cfg} }

func (h *Handler) Routes() *gin.Engine {
	r := gin.New()
	r.Use(gin.Logger(), gin.Recovery())

	r.GET("/health", func(c *gin.Context) {
		if err := h.DB.PingContext(c.Request.Context()); err != nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"status": "db unavailable"})
			return
		}
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})

	v1 := r.Group("/api/v1")
	v1.POST("/users/register", h.RegisterUser)
	v1.POST("/users/login", h.Login)

	authed := v1.Group("", auth.Middleware(h.Cfg.JWTSecret))
	authed.GET("/wallet/balance", h.GetBalance)
	authed.POST("/transactions/topup", h.TopUp)
	authed.POST("/transactions/transfer", h.Transfer)
	authed.GET("/transactions/history", h.History)

	// Payment sessions (expire after PAYMENT_TTL_MINUTES, default 10)
	authed.POST("/transactions/pay", h.CreatePayment)
	authed.GET("/transactions/pay/:id", h.GetPayment)
	authed.POST("/transactions/pay/:id/confirm", h.ConfirmPayment)
	return r
}

func fail(c *gin.Context, code int, msg string) {
	c.JSON(code, gin.H{"error": msg})
}

// validAmount: >= 1 (per OpenAPI), finite, at most 2 decimal places.
func validAmount(a float64) bool {
	if math.IsNaN(a) || math.IsInf(a, 0) || a < 1 || a > 1e12 {
		return false
	}
	return math.Abs(a*100-math.Round(a*100)) < 1e-6
}

const errAmount = "amount must be a number >= 1 with at most 2 decimal places"
