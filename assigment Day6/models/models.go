package models

import "time"

type User struct {
	ID        uint   `gorm:"primaryKey"`
	Email     string `gorm:"uniqueIndex;not null"`
	Password  string `gorm:"not null" json:"-"`
	CreatedAt time.Time
}

type Session struct {
	ID        uint   `gorm:"primaryKey"`
	JTI       string `gorm:"uniqueIndex;not null"`
	UserID    uint   `gorm:"index;not null"`
	ExpiresAt time.Time
	CreatedAt time.Time
}