package models

import "time"

type Device struct {
	ID        uint   `gorm:"primaryKey"`
	UUID      string `gorm:"uniqueIndex;size:191;not null"`
	Name      string `gorm:"size:255"`
	UserID    uint   `gorm:"index"`
	OSName    string `gorm:"size:128"`
	OSVersion string `gorm:"size:128"`
	Hostname  string `gorm:"size:255"`
	Arch      string `gorm:"size:64"`
	CreatedAt time.Time
	UpdatedAt time.Time
}
