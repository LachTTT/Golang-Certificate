package middleware

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"go-auth/auth"
	"go-auth/models"
)

func Auth(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		tokenString, err := c.Cookie("token")
		if err != nil {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
			return
		}

		claims, err := auth.ParseToken(tokenString)
		if err != nil {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "invalid token"})
			return
		}

		var s models.Session
		err = db.Where("jti = ? AND user_id = ? AND expires_at > ?",
			claims.ID, claims.UserID, time.Now()).First(&s).Error
		if err != nil {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "session revoked or expired"})
			return
		}

		c.Set("user_id", claims.UserID)
		c.Set("jti", claims.ID)
		c.Next()
	}
}
