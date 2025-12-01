package repo

import (
	"errors"
	"fmt"

	"sagiri-guard/backend/app/models"

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
}

func (r *FileTreeRepository) ListItems(filter ItemFilter) ([]*models.Item, int64, error) {
	if filter.Page <= 0 {
		filter.Page = 1
	}
	if filter.PageSize <= 0 {
		filter.PageSize = 50
	}

	base := r.db.Model(&models.Item{}).
		Where("device_uuid = ?", filter.DeviceUUID)

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
	if err := base.
		Preload("File").
		Preload("Folder").
		Preload("ContentTypes").
		Order("CASE WHEN folder_id IS NULL THEN 1 ELSE 0 END, updated_at DESC").
		Offset(offset).
		Limit(filter.PageSize).
		Find(&items).Error; err != nil {
		return nil, 0, err
	}
	return items, total, nil
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
				return nil
			}
			return err
		}

		if err := tx.Where("item_id = ?", itemID).Delete(&models.ItemContentTypeLink{}).Error; err != nil {
			return err
		}

		if item.FileID != nil {
			if err := tx.Delete(&models.FileNode{}, "id = ? AND device_uuid = ?", *item.FileID, deviceUUID).Error; err != nil {
				return err
			}
		}
		if item.FolderID != nil {
			if err := tx.Delete(&models.FolderNode{}, "id = ? AND device_uuid = ?", *item.FolderID, deviceUUID).Error; err != nil {
				return err
			}
		}

		if err := tx.Delete(&models.Item{}, "device_uuid = ? AND id = ?", deviceUUID, itemID).Error; err != nil {
			return err
		}
		return nil
	})
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
