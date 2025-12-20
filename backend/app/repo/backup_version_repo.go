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

// GetByID trả về BackupFileVersion theo ID.
func (r *BackupVersionRepository) GetByID(id uint) (*models.BackupFileVersion, error) {
	var v models.BackupFileVersion
	err := r.db.Where("id = ?", id).First(&v).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &v, nil
}

// ListByFileID trả về tất cả BackupFileVersion của một file_id.
// Query trực tiếp theo FileID (đã được lưu trong BackupFileVersion).
func (r *BackupVersionRepository) ListByFileID(deviceID, fileID string) ([]models.BackupFileVersion, error) {
	var versions []models.BackupFileVersion
	err := r.db.
		Where("device_id = ? AND file_id = ?", deviceID, fileID).
		Order("version DESC").
		Find(&versions).Error
	if err != nil {
		return nil, err
	}
	return versions, nil
}

// GetLatestByFileID trả về version mới nhất của một file_id.
func (r *BackupVersionRepository) GetLatestByFileID(deviceID, fileID string) (*models.BackupFileVersion, error) {
	var v models.BackupFileVersion
	err := r.db.
		Where("device_id = ? AND file_id = ?", deviceID, fileID).
		Order("version DESC").
		First(&v).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &v, nil
}


