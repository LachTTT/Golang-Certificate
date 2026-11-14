package main

import (
	"log"
	"os"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"

	"go-auth/handlers"
	"go-auth/middleware"
	"go-auth/models"
)

func main() {
	if os.Getenv("JWT_SECRET") == "" {
		log.Fatal("JWT_SECRET is required")
	}

	db, err := gorm.Open(sqlite.Open("app.db"), &gorm.Config{})
	if err != nil {
		log.Fatal(err)
	}
	db.AutoMigrate(&models.User{}, &models.Session{})

	h := handlers.New(db)
	r := gin.Default()

	r.POST("/register", h.Register)
	r.POST("/login", h.Login)

	protected := r.Group("/", middleware.Auth(db))
	protected.GET("/me", h.Me)
	protected.POST("/logout", h.Logout)

	r.Run(":8080")
}