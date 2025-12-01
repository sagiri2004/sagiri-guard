package models

import "time"

// BackupFileVersion lưu thông tin từng phiên bản backup của một file logic.
type BackupFileVersion struct {
	ID          uint      `gorm:"primaryKey"`
	DeviceID    string    `gorm:"size:191;index"`
	LogicalPath string    `gorm:"size:512;index"` // full path hoặc key logic
	FileName    string    `gorm:"size:255"`       // tên hiển thị gốc
	StoredName  string    `gorm:"size:255"`       // tên file thực trong thư mục backups
	Version     int       `gorm:"index"`
	Size        int64
	CreatedAt   time.Time `gorm:"autoCreateTime"`
}


