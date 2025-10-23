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
	// simplistic upsert: try save; create if not found
	var existing models.Device
	if err := r.db.Where("uuid = ?", d.UUID).First(&existing).Error; err == nil {
		d.ID = existing.ID
		return r.db.Save(d).Error
	}
	return r.db.Create(d).Error
}
