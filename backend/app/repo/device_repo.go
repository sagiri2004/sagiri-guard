package repo

import (
	"sagiri-guard/backend/app/models"

	"gorm.io/gorm"
)

type DeviceRepository struct{ db *gorm.DB }

func NewDeviceRepository(db *gorm.DB) *DeviceRepository { return &DeviceRepository{db: db} }

func (r *DeviceRepository) FindByUUID(uuid string) (*models.Device, error) {
	var d models.Device
	if err := r.db.Where("uuid = ?", uuid).First(&d).Error; err != nil {
		return nil, err
	}
	return &d, nil
}

func (r *DeviceRepository) Upsert(d *models.Device) error {
	// When exists, update only mutable fields to avoid zeroing CreatedAt
	var existing models.Device
	if err := r.db.Where("uuid = ?", d.UUID).First(&existing).Error; err == nil {
		updates := map[string]any{
			"name":       d.Name,
			"user_id":    d.UserID,
			"os_name":    d.OSName,
			"os_version": d.OSVersion,
			"hostname":   d.Hostname,
			"arch":       d.Arch,
		}
		return r.db.Model(&existing).Updates(updates).Error
	}
	return r.db.Create(d).Error
}
