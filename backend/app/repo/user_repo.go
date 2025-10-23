package repo

import (
	"sagiri-guard/backend/app/models"

	"gorm.io/gorm"
)

type UserRepository struct{ db *gorm.DB }

func NewUserRepository(db *gorm.DB) *UserRepository { return &UserRepository{db: db} }

func (r *UserRepository) CountByUsername(username string) (int64, error) {
	var count int64
	return count, r.db.Model(&models.User{}).Where("username = ?", username).Count(&count).Error
}

func (r *UserRepository) Create(u *models.User) error { return r.db.Create(u).Error }

func (r *UserRepository) FindByUsername(username string) (*models.User, error) {
	var u models.User
	if err := r.db.Where("username = ?", username).First(&u).Error; err != nil {
		return nil, err
	}
	return &u, nil
}
