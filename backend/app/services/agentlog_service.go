package services

import (
	"sagiri-guard/backend/app/models"
	"sagiri-guard/backend/app/repo"
)

type AgentLogService struct{ repo *repo.AgentLogRepository }

func NewAgentLogService(r *repo.AgentLogRepository) *AgentLogService {
	return &AgentLogService{repo: r}
}

func (s *AgentLogService) Create(deviceID, content string) error {
	l := models.AgentLog{DeviceID: deviceID, Content: content}
	return s.repo.Create(&l)
}

func (s *AgentLogService) Latest(deviceID string, limit int) ([]models.AgentLog, error) {
	return s.repo.LatestByDevice(deviceID, limit)
}
