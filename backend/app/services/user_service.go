package services

import (
	"errors"
	"sagiri-guard/backend/app/models"
	"sagiri-guard/backend/app/repo"

	"golang.org/x/crypto/bcrypt"
)

type UserService struct{ users *repo.UserRepository }

func NewUserService(users *repo.UserRepository) *UserService { return &UserService{users: users} }

func (s *UserService) EnsureAdmin(username, password string) error {
	count, err := s.users.CountByUsername(username)
	if err != nil {
		return err
	}
	if count > 0 {
		return nil
	}
	hash, _ := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	return s.users.Create(&models.User{Username: username, PasswordHash: string(hash), Role: "admin"})
}

func (s *UserService) CreateUser(username, password, role string) error {
	if role == "" {
		role = "user"
	}
	hash, _ := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	return s.users.Create(&models.User{Username: username, PasswordHash: string(hash), Role: role})
}

func (s *UserService) ValidateCredentials(username, password string) (*models.User, error) {
	u, err := s.users.FindByUsername(username)
	if err != nil {
		return nil, err
	}
	if bcrypt.CompareHashAndPassword([]byte(u.PasswordHash), []byte(password)) != nil {
		return nil, errors.New("invalid credentials")
	}
	return u, nil
}
