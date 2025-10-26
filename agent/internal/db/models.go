package db

import "time"

type Token struct {
	ID        uint   `gorm:"primaryKey"`
	Value     string `gorm:"size:8192"`
	CreatedAt time.Time
}
