package models

import "time"

// BlockCategory định nghĩa các category chặn website
type BlockCategory string

const (
	CategorySocialMedia BlockCategory = "social_media"
	CategoryAI          BlockCategory = "ai"
	CategoryGaming      BlockCategory = "gaming"
	CategoryShopping    BlockCategory = "shopping"
	CategoryNews        BlockCategory = "news"
	CategoryEntertainment BlockCategory = "entertainment"
	CategoryAdult       BlockCategory = "adult"
	CategoryCustom      BlockCategory = "custom"
)

// WebsiteBlockRule là rule chặn website cho một device
type WebsiteBlockRule struct {
	ID          uint         `gorm:"primaryKey"`
	DeviceID    string       `gorm:"size:191;index;not null"` // UUID của device
	Type        string       `gorm:"size:32;not null"`        // "category" hoặc "domain"
	Category    string       `gorm:"size:64"`                 // category name nếu type="category"
	Domain      string       `gorm:"size:255;index"`          // domain cụ thể nếu type="domain"
	Enabled     bool         `gorm:"default:true"`             // bật/tắt rule này
	CreatedAt   time.Time    `gorm:"autoCreateTime"`
	UpdatedAt   time.Time    `gorm:"autoUpdateTime"`
	DeletedAt   *time.Time   `gorm:"index"`                   // soft delete
}

// WebsiteBlockStatus lưu trạng thái blocking của device (enabled/disabled)
type WebsiteBlockStatus struct {
	ID        uint      `gorm:"primaryKey"`
	DeviceID  string    `gorm:"size:191;uniqueIndex;not null"`
	Enabled   bool      `gorm:"default:false"` // tổng thể bật/tắt blocking
	UpdatedAt time.Time `gorm:"autoUpdateTime"`
}



