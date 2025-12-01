package repo

import (
	"database/sql"
	"errors"

	"sagiri-guard/backend/app/models"

	"gorm.io/gorm"
)

type BackupVersionRepository struct {
	db *gorm.DB
}

func NewBackupVersionRepository(db *gorm.DB) *BackupVersionRepository {
	return &BackupVersionRepository{db: db}
}

// NextVersion trả về version tiếp theo cho một cặp (device, logical_path).
func (r *BackupVersionRepository) NextVersion(deviceID, logicalPath string) (int, error) {
	var maxVer sql.NullInt64
	if err := r.db.
		Model(&models.BackupFileVersion{}).
		Where("device_id = ? AND logical_path = ?", deviceID, logicalPath).
		Select("MAX(version)").
		Scan(&maxVer).Error; err != nil {
		return 0, err
	}
	if !maxVer.Valid {
		return 1, nil
	}
	return int(maxVer.Int64) + 1, nil
}

func (r *BackupVersionRepository) Create(v *models.BackupFileVersion) error {
	return r.db.Create(v).Error
}

func (r *BackupVersionRepository) List(deviceID, logicalPath string) ([]models.BackupFileVersion, error) {
	var out []models.BackupFileVersion
	if err := r.db.
		Where("device_id = ? AND logical_path = ?", deviceID, logicalPath).
		Order("version ASC").
		Find(&out).Error; err != nil {
		return nil, err
	}
	return out, nil
}

// Get trả về version cụ thể; nếu version <= 0 sẽ trả về bản mới nhất.
func (r *BackupVersionRepository) Get(deviceID, logicalPath string, version int) (*models.BackupFileVersion, error) {
	var v models.BackupFileVersion
	q := r.db.
		Where("device_id = ? AND logical_path = ?", deviceID, logicalPath)
	if version > 0 {
		q = q.Where("version = ?", version)
	} else {
		q = q.Order("version DESC")
	}
	err := q.First(&v).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &v, nil
}


