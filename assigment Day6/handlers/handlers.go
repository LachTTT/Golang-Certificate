package handlers

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"

	"go-auth/auth"
	"go-auth/models"
)

type Handler struct{ DB *gorm.DB }

func New(db *gorm.DB) *Handler { return &Handler{DB: db} }

type credentials struct {
	Email    string `json:"email" binding:"required,email"`
	Password string `json:"password" binding:"required,min=6"`
}

func (h *Handler) Register(c *gin.Context) {
	var in credentials
	if err := c.ShouldBindJSON(&in); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	hash, _ := bcrypt.GenerateFromPassword([]byte(in.Password), bcrypt.DefaultCost)
	user := models.User{Email: in.Email, Password: string(hash)}
	if err := h.DB.Create(&user).Error; err != nil {
		c.JSON(http.StatusConflict, gin.H{"error": "email already used"})
		return
	}
	c.JSON(http.StatusCreated, gin.H{"id": user.ID, "email": user.Email})
}

func (h *Handler) Login(c *gin.Context) {
	var in credentials
	if err := c.ShouldBindJSON(&in); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	var user models.User
	if err := h.DB.Where("email = ?", in.Email).First(&user).Error; err != nil ||
		bcrypt.CompareHashAndPassword([]byte(user.Password), []byte(in.Password)) != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "wrong email or password"})
		return
	}

	token, jti, exp, err := auth.GenerateToken(user.ID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not create token"})
		return
	}
	h.DB.Create(&models.Session{JTI: jti, UserID: user.ID, ExpiresAt: exp})

	c.SetSameSite(http.SameSiteLaxMode)
	// Secure=false hanya untuk dev lokal. Production wajib true (HTTPS).
	c.SetCookie("token", token, 86400, "/", "", false, true)
	c.JSON(http.StatusOK, gin.H{"message": "logged in"})
}

func (h *Handler) Me(c *gin.Context) {
	var user models.User
	if err := h.DB.First(&user, c.GetUint("user_id")).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "user not found"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"id": user.ID, "email": user.Email})
}

func (h *Handler) Logout(c *gin.Context) {
	h.DB.Where("jti = ?", c.GetString("jti")).Delete(&models.Session{})
	c.SetCookie("token", "", -1, "/", "", false, true)
	c.JSON(http.StatusOK, gin.H{"message": "logged out"})
}
