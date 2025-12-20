package repo

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"sagiri-guard/backend/app/models"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type FileTreeRepository struct {
	db *gorm.DB
}

func NewFileTreeRepository(db *gorm.DB) *FileTreeRepository {
	return &FileTreeRepository{db: db}
}

func (r *FileTreeRepository) UpsertFile(node *models.FileNode) error {
	return r.db.Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "id"}},
		DoUpdates: clause.Assignments(map[string]any{
			"device_uuid":     node.DeviceUUID,
			"origin_path":     node.OriginPath,
			"current_path":    node.CurrentPath,
			"current_name":    node.CurrentName,
			"extension":       node.Extension,
			"size":            node.Size,
			"snapshot_number": node.SnapshotNumber,
			"change_pending":  node.ChangePending,
			"last_event_at":   node.LastEventAt,
			// created_at giữ nguyên trên update
		}),
	}).Create(node).Error
}

func (r *FileTreeRepository) UpsertFolder(node *models.FolderNode) error {
	return r.db.Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "id"}},
		DoUpdates: clause.Assignments(map[string]any{
			"device_uuid":   node.DeviceUUID,
			"total_entries": node.TotalEntries,
			"ext_children":  node.ExtChildren,
			// created_at giữ nguyên trên update
		}),
	}).Create(node).Error
}

func (r *FileTreeRepository) UpsertItem(item *models.Item) error {
	return r.db.Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "id"}},
		DoUpdates: clause.Assignments(map[string]any{
			"device_uuid": item.DeviceUUID,
			"name":        item.Name,
			"parent_id":   item.ParentID,
			"file_id":     item.FileID,
			"folder_id":   item.FolderID,
			"total_size":  item.TotalSize,
			// created_at giữ nguyên trên update
		}),
	}).Create(item).Error
}

func (r *FileTreeRepository) GetItemByID(deviceUUID, id string) (*models.Item, error) {
	var item models.Item
	err := r.db.
		Preload("File").
		Preload("Folder").
		Preload("ContentTypes").
		Where("device_uuid = ? AND id = ?", deviceUUID, id).
		First(&item).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &item, nil
}

type ItemFilter struct {
	DeviceUUID     string
	ParentID       *string
	Search         string
	Extensions     []string
	ContentTypeIDs []uint
	Page           int
	PageSize       int
	IncludeDeleted bool   // Nếu true, query cả soft deleted items
	DeletedSince   string // Format: "6h", "24h", "7d" hoặc timestamp. Chỉ áp dụng khi IncludeDeleted=true
}

func (r *FileTreeRepository) ListItems(filter ItemFilter) ([]*models.Item, int64, error) {
	if filter.Page <= 0 {
		filter.Page = 1
	}
	if filter.PageSize <= 0 {
		filter.PageSize = 50
	}

	// Sử dụng Unscoped() nếu cần query cả soft deleted items
	var base *gorm.DB
	if filter.IncludeDeleted {
		base = r.db.Unscoped().Model(&models.Item{}).
		Where("device_uuid = ?", filter.DeviceUUID)
		
		// Filter theo thời gian xóa nếu có DeletedSince
		if filter.DeletedSince != "" {
			deletedSince, err := parseDeletedSince(filter.DeletedSince)
			if err != nil {
				return nil, 0, fmt.Errorf("invalid deleted_since format: %w", err)
			}
			// Chỉ lấy các item đã bị xóa sau thời điểm deletedSince
			base = base.Where("deleted_at IS NOT NULL AND deleted_at >= ?", deletedSince)
		} else {
			// Nếu không có DeletedSince, chỉ lấy các item đã bị xóa (deleted_at IS NOT NULL)
			// Mặc định: lấy các item đã xóa trong 6 giờ trước (theo yêu cầu)
			sixHoursAgo := time.Now().Add(-6 * time.Hour)
			base = base.Where("deleted_at IS NOT NULL AND deleted_at >= ?", sixHoursAgo)
		}
	} else {
		// Mặc định: chỉ lấy các item chưa xóa
		base = r.db.Model(&models.Item{}).
			Where("device_uuid = ?", filter.DeviceUUID)
	}

	if filter.ParentID == nil {
		base = base.Where("parent_id IS NULL")
	} else if *filter.ParentID != "" {
		base = base.Where("parent_id = ?", *filter.ParentID)
	}

	if len(filter.Extensions) > 0 {
		base = base.Joins("LEFT JOIN file_nodes fn ON fn.id = items.file_id").
			Where("fn.extension IN ?", filter.Extensions)
	}

	if len(filter.ContentTypeIDs) > 0 {
		base = base.Joins("JOIN item_content_type_links ictl ON ictl.item_id = items.id").
			Where("ictl.content_type_id IN ?", filter.ContentTypeIDs)
	}

	if filter.Search != "" {
		base = base.Where("items.name LIKE ?", fmt.Sprintf("%%%s%%", filter.Search))
	}

	var total int64
	if err := base.Session(&gorm.Session{}).Count(&total).Error; err != nil {
		return nil, 0, err
	}

	offset := (filter.Page - 1) * filter.PageSize
	var items []*models.Item
	query := base.
		Preload("File").
		Preload("Folder").
		Preload("ContentTypes")
	
	// Nếu query soft deleted, cần preload cả File và Folder với Unscoped
	if filter.IncludeDeleted {
		query = query.Preload("File", func(db *gorm.DB) *gorm.DB {
			return db.Unscoped()
		}).Preload("Folder", func(db *gorm.DB) *gorm.DB {
			return db.Unscoped()
		})
	}
	
	if err := query.
		Order("CASE WHEN folder_id IS NULL THEN 1 ELSE 0 END, updated_at DESC").
		Offset(offset).
		Limit(filter.PageSize).
		Find(&items).Error; err != nil {
		return nil, 0, err
	}
	return items, total, nil
}

// parseDeletedSince parse string như "6h", "24h", "7d" thành time.Time
// Format hỗ trợ:
//   - "6h" = 6 giờ trước
//   - "24h" = 24 giờ trước
//   - "7d" = 7 ngày trước
//   - "30m" = 30 phút trước
//   - ISO8601 timestamp: "2006-01-02T15:04:05Z"
func parseDeletedSince(s string) (time.Time, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return time.Time{}, errors.New("empty string")
	}

	// Thử parse như ISO8601 timestamp
	if timestamp, err := time.Parse(time.RFC3339, s); err == nil {
		return timestamp, nil
	}
	if timestamp, err := time.Parse("2006-01-02T15:04:05Z", s); err == nil {
		return timestamp, nil
	}

	// Parse relative time như "6h", "24h", "7d"
	now := time.Now()
	var duration time.Duration
	var err error

	if strings.HasSuffix(s, "h") {
		hours := strings.TrimSuffix(s, "h")
		duration, err = time.ParseDuration(hours + "h")
	} else if strings.HasSuffix(s, "d") {
		days := strings.TrimSuffix(s, "d")
		// Parse days thành hours
		var daysInt int
		if _, err := fmt.Sscanf(days, "%d", &daysInt); err == nil {
			duration = time.Duration(daysInt) * 24 * time.Hour
		} else {
			return time.Time{}, fmt.Errorf("invalid days format: %s", s)
		}
	} else if strings.HasSuffix(s, "m") {
		minutes := strings.TrimSuffix(s, "m")
		duration, err = time.ParseDuration(minutes + "m")
	} else {
		// Thử parse như số giờ (default: giờ)
		duration, err = time.ParseDuration(s + "h")
		if err != nil {
			return time.Time{}, fmt.Errorf("invalid format: %s (supported: 6h, 24h, 7d, 30m, or ISO8601)", s)
		}
	}

	if err != nil {
		return time.Time{}, fmt.Errorf("parse duration: %w", err)
	}

	return now.Add(-duration), nil
}

func (r *FileTreeRepository) DeleteItem(deviceUUID, itemID string) error {
	if itemID == "" {
		// nothing to delete; can happen when agent reports delete without a stable item ID
		return nil
	}
	return r.db.Transaction(func(tx *gorm.DB) error {
		var item models.Item
		if err := tx.Where("device_uuid = ? AND id = ?", deviceUUID, itemID).First(&item).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				// Item không tồn tại - có thể đã bị xóa rồi hoặc chưa được tạo
				// Không trả về error để không fail toàn bộ batch
				return nil
			}
			return err
		}

		return r.softDeleteItem(tx, &item, deviceUUID)
	})
}

// DeleteItemByLogicalPath tìm và xóa item theo logical path
func (r *FileTreeRepository) DeleteItemByLogicalPath(deviceUUID, logicalPath string) error {
	if logicalPath == "" {
		return nil // Không có path thì không làm gì, không trả về error
	}
	
	// Parse logical path thành segments
	segments := strings.Split(strings.Trim(logicalPath, "/"), "/")
	if len(segments) == 0 {
		return nil // Không có segments thì không làm gì
	}
	
	// Tạo deterministic ID từ logical path (giống cách backend tạo khi upsert)
	itemID := deterministicIDFromSegments(deviceUUID, segments)
	
	return r.db.Transaction(func(tx *gorm.DB) error {
		var item models.Item
		// Tìm item theo deterministic ID (có thể có nhiều items với cùng path nhưng khác ID do uniqueID)
		err := tx.Where("device_uuid = ? AND id = ?", deviceUUID, itemID).First(&item).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			// Nếu không tìm thấy theo deterministic ID, thử tìm theo FileNode CurrentPath
			var fileNode models.FileNode
			if err2 := tx.Where("device_uuid = ? AND current_path = ?", deviceUUID, logicalPath).First(&fileNode).Error; err2 == nil {
				// Tìm Item có FileID match
				if err3 := tx.Where("device_uuid = ? AND file_id = ?", deviceUUID, fileNode.ID).First(&item).Error; err3 != nil {
					// Không tìm thấy - có thể đã bị xóa rồi, không trả về error
					return nil
				}
			} else {
				// Không tìm thấy FileNode - có thể là folder hoặc đã bị xóa rồi
				// Thử tìm folder
				var folderNode models.FolderNode
				if err3 := tx.Where("device_uuid = ? AND id = ?", deviceUUID, itemID).First(&folderNode).Error; err3 == nil {
					// Tìm Item có FolderID match
					if err4 := tx.Where("device_uuid = ? AND folder_id = ?", deviceUUID, folderNode.ID).First(&item).Error; err4 != nil {
						// Không tìm thấy - có thể đã bị xóa rồi
						return nil
					}
				} else {
					// Không tìm thấy cả file và folder - có thể đã bị xóa rồi
					return nil
				}
			}
		} else if err != nil {
			return err
		}

		return r.softDeleteItem(tx, &item, deviceUUID)
	})
}

// softDeleteItem thực hiện soft delete cho item và các related records
func (r *FileTreeRepository) softDeleteItem(tx *gorm.DB, item *models.Item, deviceUUID string) error {
	// Soft delete: sử dụng GORM soft delete thay vì hard delete
	// GORM sẽ tự động set DeletedAt khi gọi Delete() trên model có DeletedAt field

	// Soft delete ItemContentTypeLink (không có soft delete, dùng hard delete)
	if err := tx.Where("item_id = ?", item.ID).Delete(&models.ItemContentTypeLink{}).Error; err != nil {
		return err
	}

	// Soft delete FileNode nếu có
		if item.FileID != nil {
		if err := tx.Where("id = ? AND device_uuid = ?", *item.FileID, deviceUUID).
			Delete(&models.FileNode{}).Error; err != nil {
				return err
			}
		}
	// Soft delete FolderNode nếu có
		if item.FolderID != nil {
		if err := tx.Where("id = ? AND device_uuid = ?", *item.FolderID, deviceUUID).
			Delete(&models.FolderNode{}).Error; err != nil {
				return err
			}
		}

	// Soft delete Item (GORM sẽ tự động set DeletedAt)
	if err := tx.Where("device_uuid = ? AND id = ?", deviceUUID, item.ID).
		Delete(&models.Item{}).Error; err != nil {
			return err
		}
		return nil
}

func (r *FileTreeRepository) ReplaceContentTypes(itemID string, ids []uint) error {
	return r.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("item_id = ?", itemID).Delete(&models.ItemContentTypeLink{}).Error; err != nil {
			return err
		}
		if len(ids) == 0 {
			return nil
		}
		links := make([]models.ItemContentTypeLink, 0, len(ids))
		for _, id := range ids {
			links = append(links, models.ItemContentTypeLink{ItemID: itemID, ContentTypeID: id})
		}
		return tx.Create(&links).Error
	})
}

// GetFileNodeByLogicalPath tìm FileNode từ logical path (bao gồm cả soft deleted).
// Logical path được convert thành Item ID, sau đó query Item để lấy FileID, rồi query FileNode.
func (r *FileTreeRepository) GetFileNodeByLogicalPath(deviceUUID, logicalPath string) (*models.FileNode, *models.Item, error) {
	// Parse logical path thành segments
	segments := strings.Split(strings.Trim(logicalPath, "/"), "/")
	if len(segments) == 0 {
		return nil, nil, errors.New("empty logical path")
	}

	// Tạo Item ID từ logical path segments (giống cách agent tạo)
	// Sử dụng hàm deterministicID giống trong filetree_service
	itemID := deterministicIDFromSegments(deviceUUID, segments)

	// Query Item bao gồm cả soft deleted
	var item models.Item
	err := r.db.Unscoped().
		Preload("File").
		Where("device_uuid = ? AND id = ?", deviceUUID, itemID).
		First(&item).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil, nil
	}
	if err != nil {
		return nil, nil, err
	}

	// Nếu không có FileID, không phải file
	if item.FileID == nil {
		return nil, &item, nil
	}

	// Query FileNode bao gồm cả soft deleted
	var fileNode models.FileNode
	err = r.db.Unscoped().
		Where("id = ? AND device_uuid = ?", *item.FileID, deviceUUID).
		First(&fileNode).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, &item, nil
	}
	if err != nil {
		return nil, &item, err
	}

	return &fileNode, &item, nil
}

// GetFileNodeByID tìm FileNode từ file_id (bao gồm cả soft deleted).
func (r *FileTreeRepository) GetFileNodeByID(deviceUUID, fileID string) (*models.FileNode, error) {
	var fileNode models.FileNode
	err := r.db.Unscoped().
		Where("id = ? AND device_uuid = ?", fileID, deviceUUID).
		First(&fileNode).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &fileNode, nil
}

// GetItemsByFileID tìm tất cả Items có FileID = file_id (bao gồm cả soft deleted).
// Một file có thể có nhiều Items nếu file bị move/rename.
func (r *FileTreeRepository) GetItemsByFileID(deviceUUID, fileID string) ([]*models.Item, error) {
	var items []*models.Item
	err := r.db.Unscoped().
		Preload("File").
		Where("device_uuid = ? AND file_id = ?", deviceUUID, fileID).
		Find(&items).Error
	if err != nil {
		return nil, err
	}
	return items, nil
}

// BuildLogicalPathFromItem build logical path từ Item bằng cách traverse parent chain.
func (r *FileTreeRepository) BuildLogicalPathFromItem(deviceUUID string, item *models.Item) (string, error) {
	if item == nil {
		return "", errors.New("item is nil")
	}
	
	// Collect path segments từ item lên root
	var segments []string
	current := item
	
	for current != nil {
		segments = append([]string{current.Name}, segments...)
		if current.ParentID == nil || *current.ParentID == "" {
			break
		}
		var parent models.Item
		err := r.db.Unscoped().
			Where("device_uuid = ? AND id = ?", deviceUUID, *current.ParentID).
			First(&parent).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			// Không tìm thấy parent, dừng lại
			break
		}
		if err != nil {
			return "", fmt.Errorf("query parent: %w", err)
		}
		current = &parent
	}
	
	if len(segments) == 0 {
		return "", errors.New("empty path segments")
	}
	
	return strings.Join(segments, "/"), nil
}

// deterministicIDFromSegments tạo ID từ device UUID và segments (giống filetree_service)
func deterministicIDFromSegments(deviceUUID string, segments []string) string {
	key := strings.Join(segments, "/")
	// Sử dụng UUID SHA1 namespace giống filetree_service
	return uuid.NewSHA1(uuid.NameSpaceURL, []byte(deviceUUID+"|"+key)).String()
}
