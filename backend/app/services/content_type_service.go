package services

import (
	"sagiri-guard/backend/app/models"
	"sagiri-guard/backend/app/repo"
)

type ContentTypeService struct {
	repo *repo.ContentTypeRepository
}

func NewContentTypeService(repo *repo.ContentTypeRepository) *ContentTypeService {
	return &ContentTypeService{repo: repo}
}

func (s *ContentTypeService) List() ([]models.ContentType, error) {
	return s.repo.List()
}

func (s *ContentTypeService) Get(id uint) (*models.ContentType, error) {
	return s.repo.Get(id)
}

func (s *ContentTypeService) Create(ct *models.ContentType) (*models.ContentType, error) {
	return s.repo.Create(ct)
}

func (s *ContentTypeService) Update(ct *models.ContentType) (*models.ContentType, error) {
	return s.repo.Update(ct)
}

func (s *ContentTypeService) Delete(id uint) error {
	return s.repo.Delete(id)
}
