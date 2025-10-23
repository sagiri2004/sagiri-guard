package models

import "time"

type User struct {
	ID           uint   `gorm:"primaryKey"`
	Username     string `gorm:"uniqueIndex;size:191;not null"`
	PasswordHash string `gorm:"size:255;not null"`
	Role         string `gorm:"size:32;not null;default:user"`
	CreatedAt    time.Time
	UpdatedAt    time.Time
}
