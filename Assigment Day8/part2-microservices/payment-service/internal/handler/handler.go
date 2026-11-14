package handler

import (
	"database/sql"
	"math"
	"net/http"

	"github.com/gin-gonic/gin"

	"ewallet-payment/internal/auth"
	"ewallet-payment/internal/config"
	"ewallet-payment/internal/corekit"
)

type Handler struct {
	DB   *sql.DB
	Cfg  config.Config
	Core *corekit.Client
}

func New(db *sql.DB, cfg config.Config, core *corekit.Client) *Handler {
	return &Handler{DB: db, Cfg: cfg, Core: core}
}

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

	authed := r.Group("/api/v1/transactions/pay", auth.Middleware(h.Cfg.JWTSecret))
	authed.POST("", h.CreatePayment)
	authed.GET("/:id", h.GetPayment)
	authed.POST("/:id/confirm", h.ConfirmPayment)
	return r
}

func fail(c *gin.Context, code int, msg string) {
	c.JSON(code, gin.H{"error": msg})
}

func validAmount(a float64) bool {
	if math.IsNaN(a) || math.IsInf(a, 0) || a < 1 || a > 1e12 {
		return false
	}
	return math.Abs(a*100-math.Round(a*100)) < 1e-6
}

const errAmount = "amount must be a number >= 1 with at most 2 decimal places"
