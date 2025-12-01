package models

import (
	"time"

	"gorm.io/gorm"
)

// ContentType mô tả phân loại nghiệp vụ mà người dùng gán cho file/folder.
type ContentType struct {
	ID          uint           `gorm:"primaryKey"`
	Name        string         `gorm:"size:191;uniqueIndex"`
	Description string         `gorm:"size:512"`
	CreatedAt   time.Time      `gorm:"autoCreateTime"`
	UpdatedAt   time.Time      `gorm:"autoUpdateTime"`
	DeletedAt   gorm.DeletedAt `gorm:"index"`
}

// FileNode lưu metadata cuối cùng của một file vật lý.
type FileNode struct {
	ID         string `gorm:"primaryKey;size:191"`
	DeviceUUID string `gorm:"size:191;index"`
	OriginPath string `gorm:"size:1024"`
	// current_path được index để tìm kiếm nhanh, nhưng cần giới hạn chiều dài
	// để tránh vượt quá max key length của MySQL (3072 bytes với utf8mb4).
	// 512 * 4 = 2048 < 3072 nên an toàn.
	CurrentPath    string `gorm:"size:512;index"`
	CurrentName    string `gorm:"size:255"`
	Extension      string `gorm:"size:64"`
	Size           int64
	SnapshotNumber int
	ChangePending  bool
	LastEventAt    time.Time
	CreatedAt      time.Time      `gorm:"autoCreateTime"`
	UpdatedAt      time.Time      `gorm:"autoUpdateTime"`
	DeletedAt      gorm.DeletedAt `gorm:"index"`
}

// FolderNode chứa thông tin tổng hợp cho thư mục.
type FolderNode struct {
	ID           string `gorm:"primaryKey;size:191"`
	DeviceUUID   string `gorm:"size:191;index"`
	TotalEntries int64
	ExtChildren  string         `gorm:"size:512"`
	CreatedAt    time.Time      `gorm:"autoCreateTime"`
	UpdatedAt    time.Time      `gorm:"autoUpdateTime"`
	DeletedAt    gorm.DeletedAt `gorm:"index"`
}

// Item biểu diễn một nút trong cây (có thể là file hoặc folder).
type Item struct {
	ID           string  `gorm:"primaryKey;size:191"`
	DeviceUUID   string  `gorm:"size:191;index"`
	Name         string  `gorm:"size:255"`
	ParentID     *string `gorm:"size:191;index"`
	FileID       *string `gorm:"size:191"`
	FolderID     *string `gorm:"size:191"`
	TotalSize    int64
	CreatedAt    time.Time      `gorm:"autoCreateTime"`
	UpdatedAt    time.Time      `gorm:"autoUpdateTime"`
	DeletedAt    gorm.DeletedAt `gorm:"index"`
	File         *FileNode      `gorm:"foreignKey:FileID;references:ID"`
	Folder       *FolderNode    `gorm:"foreignKey:FolderID;references:ID"`
	ContentTypes []ContentType  `gorm:"many2many:item_content_type_links"`
}

// ItemContentTypeLink là bảng phụ giữa item và content type.
type ItemContentTypeLink struct {
	ItemID        string `gorm:"primaryKey;size:191"`
	ContentTypeID uint   `gorm:"primaryKey"`
	CreatedAt     time.Time
}
