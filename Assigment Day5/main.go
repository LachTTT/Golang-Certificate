// main.go
package main

import (
	"encoding/base64"
	"encoding/binary"
	"fmt"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/joho/godotenv"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

type URLMapping struct {
	ID        uint      `gorm:"primaryKey"`
	ShortCode string    `gorm:"size:16;not null;uniqueIndex"`
	LongURL   string    `gorm:"type:text;not null"`
	CreatedAt time.Time `gorm:"autoCreateTime"`
	ExpiresAt *time.Time
}

type URLClick struct {
	ID        uint      `gorm:"primaryKey"`
	MappingID uint      `gorm:"not null;index"`
	ClickedAt time.Time `gorm:"autoCreateTime"`
	IPAddress string    `gorm:"size:45"`
	UserAgent string    `gorm:"type:text"`
}

type shortenRequest struct {
	LongURL string `json:"long_url" binding:"required,url"`
}

type shortenResponse struct {
	ShortURL  string `json:"short_url"`
	ShortCode string `json:"short_code"`
}

var (
	db         *gorm.DB
	baseDomain string
)

func encodeID(id uint) string {
	buf := make([]byte, 8)
	binary.BigEndian.PutUint64(buf, uint64(id))

	return base64.RawURLEncoding.EncodeToString(buf)
}

func setupDatabase() {
	host := os.Getenv("DB_HOST")
	port := os.Getenv("DB_PORT")
	user := os.Getenv("DB_USER")
	password := os.Getenv("DB_PASSWORD")
	name := os.Getenv("DB_NAME")
	sslmode := os.Getenv("DB_SSLMODE")

	if host == "" || port == "" || user == "" || password == "" || name == "" || sslmode == "" {
		log.Fatal("One or more required DB environment variables are missing")
	}

	dsn := fmt.Sprintf(
		"host=%s user=%s password=%s dbname=%s port=%s sslmode=%s",
		host, user, password, name, port, sslmode,
	)

	var err error
	db, err = gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		log.Fatalf("failed to connect to database: %v", err)
	}

	if err := db.AutoMigrate(&URLMapping{}, &URLClick{}); err != nil {
		log.Fatalf("auto-migrate failed: %v", err)
	}
}

func shortenHandler(c *gin.Context) {
	var req shortenRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request payload"})
		return
	}

	var existing URLMapping
	if err := db.Where("long_url = ?", req.LongURL).First(&existing).Error; err == nil {
		shortURL := baseDomain + "/" + existing.ShortCode
		c.JSON(http.StatusOK, shortenResponse{ShortURL: shortURL, ShortCode: existing.ShortCode})
		return
	}

	mapping := URLMapping{
		LongURL: req.LongURL,
	}
	if err := db.Create(&mapping).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to store URL mapping"})
		return
	}

	shortCode := encodeID(mapping.ID)
	mapping.ShortCode = shortCode
	if err := db.Save(&mapping).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update short code"})
		return
	}

	shortURL := baseDomain + "/" + shortCode
	c.JSON(http.StatusCreated, shortenResponse{ShortURL: shortURL, ShortCode: shortCode})
}

func redirectHandler(c *gin.Context) {
	code := c.Param("code")
	log.Printf("Redirect requested for code: %q", code)

	var mapping URLMapping
	err := db.Where("short_code = ? AND (expires_at IS NULL OR expires_at > now())", code).First(&mapping).Error
	if err != nil {
		log.Printf("Redirect query error for %q: %v", code, err)

		if err == gorm.ErrRecordNotFound {
			c.String(http.StatusNotFound, "404 not found")
		} else {
			c.String(http.StatusInternalServerError, "Internal server error")
		}
		return
	}

	click := URLClick{
		MappingID: mapping.ID,
		IPAddress: c.ClientIP(),
		UserAgent: c.Request.UserAgent(),
	}
	if err := db.Create(&click).Error; err != nil {
		// Log the error but continue redirect
		log.Printf("failed to log click: %v", err)
	}

	c.Redirect(http.StatusFound, mapping.LongURL)
}

func statsHandler(c *gin.Context) {
	code := c.Param("code")

	var mapping URLMapping
	err := db.Where("short_code = ?", code).First(&mapping).Error

	if err != nil {
		if err == gorm.ErrRecordNotFound {
			c.JSON(http.StatusNotFound, gin.H{
				"error": "Short URL not found",
			})
		} else {
			c.JSON(http.StatusInternalServerError, gin.H{
				"error": "Database error",
			})
		}
		return
	}

	var totalClicks int64
	if err := db.Model(&URLClick{}).
		Where("mapping_id = ?", mapping.ID).
		Count(&totalClicks).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Failed to count clicks",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"short_code":   mapping.ShortCode,
		"long_url":     mapping.LongURL,
		"total_clicks": totalClicks,
		"created_at":   mapping.CreatedAt,
		"expires_at":   mapping.ExpiresAt,
	})
}

func main() {
	if err := godotenv.Load(); err != nil {
		log.Println("No .env file found; using system environment variables")
	}

	baseDomain = os.Getenv("BASE_DOMAIN")
	if baseDomain == "" {
		log.Fatal("BASE_DOMAIN environment variable is required (e.g. https://sho.rt)")
	}

	setupDatabase()

	router := gin.Default()

	router.POST("/shorten", shortenHandler)
	router.GET("/stats/:code", statsHandler)
	router.GET("/:code", redirectHandler)

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	log.Printf("Server running on port %s", port)
	if err := router.Run(":" + port); err != nil {
		log.Fatalf("failed to start server: %v", err)
	}
}
