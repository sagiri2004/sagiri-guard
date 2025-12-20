package repo

import (
	"errors"
	"sagiri-guard/backend/app/models"
	"time"

	"gorm.io/gorm"
)

type WebsiteBlockRepository struct {
	db *gorm.DB
}

func NewWebsiteBlockRepository(db *gorm.DB) *WebsiteBlockRepository {
	return &WebsiteBlockRepository{db: db}
}

// CreateRule tạo rule mới
func (r *WebsiteBlockRepository) CreateRule(rule *models.WebsiteBlockRule) error {
	return r.db.Create(rule).Error
}

// GetRule lấy rule theo ID
func (r *WebsiteBlockRepository) GetRule(id uint) (*models.WebsiteBlockRule, error) {
	var rule models.WebsiteBlockRule
	err := r.db.Where("id = ?", id).First(&rule).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	return &rule, err
}

// ListRules lấy tất cả rules của device
func (r *WebsiteBlockRepository) ListRules(deviceID string) ([]models.WebsiteBlockRule, error) {
	var rules []models.WebsiteBlockRule
	err := r.db.Where("device_id = ? AND deleted_at IS NULL", deviceID).
		Order("created_at DESC").
		Find(&rules).Error
	return rules, err
}

// UpdateRule cập nhật rule
func (r *WebsiteBlockRepository) UpdateRule(id uint, updates map[string]any) error {
	return r.db.Model(&models.WebsiteBlockRule{}).
		Where("id = ?", id).
		Updates(updates).Error
}

// DeleteRule soft delete rule
func (r *WebsiteBlockRepository) DeleteRule(id uint) error {
	now := time.Now()
	return r.db.Model(&models.WebsiteBlockRule{}).
		Where("id = ?", id).
		Update("deleted_at", &now).Error
}

// GetStatus lấy trạng thái blocking của device
func (r *WebsiteBlockRepository) GetStatus(deviceID string) (*models.WebsiteBlockStatus, error) {
	var status models.WebsiteBlockStatus
	err := r.db.Where("device_id = ?", deviceID).First(&status).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		// Tạo mới nếu chưa có
		status = models.WebsiteBlockStatus{
			DeviceID: deviceID,
			Enabled:  false,
		}
		if err := r.db.Create(&status).Error; err != nil {
			return nil, err
		}
		return &status, nil
	}
	return &status, err
}

// UpdateStatus cập nhật trạng thái blocking
func (r *WebsiteBlockRepository) UpdateStatus(deviceID string, enabled bool) error {
	return r.db.Where("device_id = ?", deviceID).
		Assign(models.WebsiteBlockStatus{Enabled: enabled}).
		FirstOrCreate(&models.WebsiteBlockStatus{
			DeviceID: deviceID,
			Enabled:  enabled,
		}).Error
}



