package models

import "time"

type AgentLog struct {
	ID        uint   `gorm:"primaryKey"`
	DeviceID  string `gorm:"index;size:191"`
	Content   string `gorm:"type:longtext"`
	CreatedAt time.Time
}
