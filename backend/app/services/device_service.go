package services

import (
	"sagiri-guard/backend/app/models"
	"sagiri-guard/backend/app/repo"
)

type DeviceService struct{ devices *repo.DeviceRepository }

func NewDeviceService(devices *repo.DeviceRepository) *DeviceService {
	return &DeviceService{devices: devices}
}

func (s *DeviceService) UpsertDevice(d *models.Device) error { return s.devices.Upsert(d) }

func (s *DeviceService) FindByUUID(uuid string) (*models.Device, error) {
	return s.devices.FindByUUID(uuid)
}

func (s *DeviceService) ListAll() ([]models.Device, error) {
	return s.devices.ListAll()
}
