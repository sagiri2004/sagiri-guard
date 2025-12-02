package models

import "time"

// AgentCommand lưu các command backend gửi xuống agent (queue bền + retry).
type AgentCommand struct {
	ID        uint      `gorm:"primaryKey"`
	DeviceID  string    `gorm:"size:191;index"`
	Command   string    `gorm:"size:64"`
	Kind      string    `gorm:"size:32"`
	Payload   string    `gorm:"type:longtext"` // JSON argument
	Status    string    `gorm:"size:32;index"` // pending,sent,failed
	LastError string    `gorm:"size:512"`
	CreatedAt time.Time `gorm:"autoCreateTime"`
	UpdatedAt time.Time `gorm:"autoUpdateTime"`
	SentAt    *time.Time
}


