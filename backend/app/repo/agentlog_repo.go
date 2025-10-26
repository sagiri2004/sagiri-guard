package repo

import (
	"sagiri-guard/backend/app/models"

	"gorm.io/gorm"
)

type AgentLogRepository struct{ db *gorm.DB }

func NewAgentLogRepository(db *gorm.DB) *AgentLogRepository { return &AgentLogRepository{db: db} }

func (r *AgentLogRepository) Create(l *models.AgentLog) error { return r.db.Create(l).Error }

func (r *AgentLogRepository) LatestByDevice(deviceID string, limit int) ([]models.AgentLog, error) {
	if limit <= 0 {
		limit = 1
	}
	var logs []models.AgentLog
	err := r.db.Where("device_id = ?", deviceID).Order("id DESC").Limit(limit).Find(&logs).Error
	return logs, err
}
