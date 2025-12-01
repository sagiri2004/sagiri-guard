package db

import "time"

type Token struct {
	ID        uint   `gorm:"primaryKey"`
	Value     string `gorm:"size:8192"`
	CreatedAt time.Time
}

// MonitoredFile lưu trạng thái cuối cùng của các file/dir được watcher phát hiện.
type MonitoredFile struct {
	ID            uint   `gorm:"primaryKey"`
	Path          string `gorm:"uniqueIndex"`
	LastAction    string `gorm:"size:32"`
	LastEventAt   time.Time
	LastBackupAt  *time.Time
	ChangePending bool
	CreatedAt     time.Time
	UpdatedAt     time.Time
}
