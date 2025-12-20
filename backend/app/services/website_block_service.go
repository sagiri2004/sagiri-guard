package services

import (
	"errors"
	"sagiri-guard/backend/app/dto"
	"sagiri-guard/backend/app/models"
	"sagiri-guard/backend/app/repo"
)

var ErrInvalidInput = errors.New("invalid input")

type WebsiteBlockService struct {
	repo *repo.WebsiteBlockRepository
}

func NewWebsiteBlockService(repo *repo.WebsiteBlockRepository) *WebsiteBlockService {
	return &WebsiteBlockService{repo: repo}
}

// CreateRule tạo rule mới
func (s *WebsiteBlockService) CreateRule(deviceID string, req dto.WebsiteBlockRuleRequest) (*dto.WebsiteBlockRuleResponse, error) {
	if req.Type != "category" && req.Type != "domain" {
		return nil, ErrInvalidInput
	}
	if req.Type == "category" && req.Category == "" {
		return nil, ErrInvalidInput
	}
	if req.Type == "domain" && req.Domain == "" {
		return nil, ErrInvalidInput
	}

	rule := &models.WebsiteBlockRule{
		DeviceID: deviceID,
		Type:     req.Type,
		Category: req.Category,
		Domain:   req.Domain,
		Enabled:  req.Enabled,
	}

	if err := s.repo.CreateRule(rule); err != nil {
		return nil, err
	}

	return s.ruleToDTO(rule), nil
}

// ListRules lấy tất cả rules của device
func (s *WebsiteBlockService) ListRules(deviceID string) ([]dto.WebsiteBlockRuleResponse, error) {
	rules, err := s.repo.ListRules(deviceID)
	if err != nil {
		return nil, err
	}

	result := make([]dto.WebsiteBlockRuleResponse, 0, len(rules))
	for _, rule := range rules {
		result = append(result, *s.ruleToDTO(&rule))
	}
	return result, nil
}

// UpdateRule cập nhật rule
func (s *WebsiteBlockService) UpdateRule(id uint, req dto.WebsiteBlockRuleRequest) error {
	updates := make(map[string]any)
	if req.Type != "" {
		updates["type"] = req.Type
	}
	if req.Category != "" {
		updates["category"] = req.Category
	}
	if req.Domain != "" {
		updates["domain"] = req.Domain
	}
	updates["enabled"] = req.Enabled

	return s.repo.UpdateRule(id, updates)
}

// DeleteRule xóa rule
func (s *WebsiteBlockService) DeleteRule(id uint) error {
	return s.repo.DeleteRule(id)
}

// GetStatus lấy trạng thái blocking
func (s *WebsiteBlockService) GetStatus(deviceID string) (*dto.WebsiteBlockStatusResponse, error) {
	status, err := s.repo.GetStatus(deviceID)
	if err != nil {
		return nil, err
	}

	return &dto.WebsiteBlockStatusResponse{
		DeviceID:  status.DeviceID,
		Enabled:   status.Enabled,
		UpdatedAt: status.UpdatedAt.Unix(),
	}, nil
}

// UpdateStatus cập nhật trạng thái blocking
func (s *WebsiteBlockService) UpdateStatus(deviceID string, enabled bool) error {
	return s.repo.UpdateStatus(deviceID, enabled)
}

// GetSyncData lấy toàn bộ data để sync xuống agent
func (s *WebsiteBlockService) GetSyncData(deviceID string) (*dto.BlockWebsiteCommand, error) {
	status, err := s.repo.GetStatus(deviceID)
	if err != nil {
		return nil, err
	}

	rules, err := s.repo.ListRules(deviceID)
	if err != nil {
		return nil, err
	}

	ruleDTOs := make([]dto.WebsiteBlockRuleResponse, 0, len(rules))
	for _, rule := range rules {
		if rule.Enabled {
			ruleDTOs = append(ruleDTOs, *s.ruleToDTO(&rule))
		}
	}

	return &dto.BlockWebsiteCommand{
		Action: "sync",
		Rules:  ruleDTOs,
		Status: &dto.WebsiteBlockStatusResponse{
			DeviceID:  status.DeviceID,
			Enabled:   status.Enabled,
			UpdatedAt: status.UpdatedAt.Unix(),
		},
	}, nil
}

func (s *WebsiteBlockService) ruleToDTO(rule *models.WebsiteBlockRule) *dto.WebsiteBlockRuleResponse {
	return &dto.WebsiteBlockRuleResponse{
		ID:        rule.ID,
		DeviceID:  rule.DeviceID,
		Type:      rule.Type,
		Category:  rule.Category,
		Domain:    rule.Domain,
		Enabled:   rule.Enabled,
		CreatedAt: rule.CreatedAt.Unix(),
		UpdatedAt: rule.UpdatedAt.Unix(),
	}
}

