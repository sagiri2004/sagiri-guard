package repo

import (
	"sagiri-guard/backend/app/models"

	"gorm.io/gorm"
)

type AgentCommandRepository struct {
	db *gorm.DB
}

func NewAgentCommandRepository(db *gorm.DB) *AgentCommandRepository {
	return &AgentCommandRepository{db: db}
}

func (r *AgentCommandRepository) Create(cmd *models.AgentCommand) error {
	return r.db.Create(cmd).Error
}

// UpdateStatus cập nhật trạng thái + lỗi (nếu có).
func (r *AgentCommandRepository) UpdateStatus(id uint, status, lastError string) error {
	return r.db.Model(&models.AgentCommand{}).
		Where("id = ?", id).
		Updates(map[string]any{
			"status":     status,
			"last_error": lastError,
		}).Error
}

// MarkSent đánh dấu đã gửi xuống agent.
func (r *AgentCommandRepository) MarkSent(id uint) error {
	return r.db.Model(&models.AgentCommand{}).
		Where("id = ?", id).
		Updates(map[string]any{
			"status":  "sent",
			"sent_at": gorm.Expr("NOW()"),
		}).Error
}

// ListByDevice trả về queue command cho 1 device; nếu includeSent=false chỉ lấy pending/failed.
func (r *AgentCommandRepository) ListByDevice(deviceID string, includeSent bool) ([]models.AgentCommand, error) {
	q := r.db.Where("device_id = ?", deviceID)
	if !includeSent {
		q = q.Where("status IN ?", []string{"pending", "failed"})
	}
	var cmds []models.AgentCommand
	if err := q.Order("id ASC").Find(&cmds).Error; err != nil {
		return nil, err
	}
	return cmds, nil
}


