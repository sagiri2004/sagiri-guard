package repo

import (
	"errors"

	"sagiri-guard/backend/app/models"

	"gorm.io/gorm"
)

type ContentTypeRepository struct {
	db *gorm.DB
}

func NewContentTypeRepository(db *gorm.DB) *ContentTypeRepository {
	return &ContentTypeRepository{db: db}
}

func (r *ContentTypeRepository) List() ([]models.ContentType, error) {
	var result []models.ContentType
	if err := r.db.Find(&result).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return []models.ContentType{}, nil
		}
		return nil, err
	}
	return result, nil
}

func (r *ContentTypeRepository) Get(id uint) (*models.ContentType, error) {
	var ct models.ContentType
	err := r.db.First(&ct, id).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &ct, nil
}

func (r *ContentTypeRepository) Create(ct *models.ContentType) (*models.ContentType, error) {
	if err := r.db.Create(ct).Error; err != nil {
		return nil, err
	}
	return ct, nil
}

func (r *ContentTypeRepository) Update(ct *models.ContentType) (*models.ContentType, error) {
	if err := r.db.Save(ct).Error; err != nil {
		return nil, err
	}
	return ct, nil
}

func (r *ContentTypeRepository) Delete(id uint) error {
	return r.db.Delete(&models.ContentType{}, id).Error
}
