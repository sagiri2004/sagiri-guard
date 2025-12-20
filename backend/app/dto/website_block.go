package dto

// WebsiteBlockRuleRequest để tạo/sửa rule
type WebsiteBlockRuleRequest struct {
	Type     string `json:"type"`      // "category" hoặc "domain"
	Category string `json:"category"`  // category name nếu type="category"
	Domain   string `json:"domain"`    // domain cụ thể nếu type="domain"
	Enabled  bool   `json:"enabled"`   // bật/tắt rule
}

// WebsiteBlockRuleResponse response cho rule
type WebsiteBlockRuleResponse struct {
	ID        uint   `json:"id"`
	DeviceID  string `json:"device_id"`
	Type      string `json:"type"`
	Category  string `json:"category,omitempty"`
	Domain    string `json:"domain,omitempty"`
	Enabled   bool   `json:"enabled"`
	CreatedAt int64  `json:"created_at"`
	UpdatedAt int64  `json:"updated_at"`
}

// WebsiteBlockStatusRequest để bật/tắt blocking tổng thể
type WebsiteBlockStatusRequest struct {
	Enabled bool `json:"enabled"`
}

// WebsiteBlockStatusResponse trạng thái blocking của device
type WebsiteBlockStatusResponse struct {
	DeviceID  string `json:"device_id"`
	Enabled   bool   `json:"enabled"`
	UpdatedAt int64  `json:"updated_at"`
}

// BlockWebsiteCommand là command gửi từ backend xuống agent
type BlockWebsiteCommand struct {
	Action string                    `json:"action"` // "apply", "remove", "sync"
	Rules  []WebsiteBlockRuleResponse `json:"rules,omitempty"`
	Status *WebsiteBlockStatusResponse `json:"status,omitempty"`
}













