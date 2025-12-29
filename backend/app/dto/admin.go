package dto

import "encoding/json"

type AdminSendCommandRequest struct {
	DeviceID string          `json:"device_id"`
	Command  string          `json:"command"`
	Kind     string          `json:"kind,omitempty"`
	Payload  json.RawMessage `json:"payload,omitempty"`
}

type AdminSendCommandResponse struct {
	ID     uint   `json:"id"`
	Status string `json:"status"`
	Sent   bool   `json:"sent"`
	Error  string `json:"error,omitempty"`
}

type DeviceSummary struct {
	UUID   string `json:"uuid"`
	Name   string `json:"name"`
	Online bool   `json:"online"`
}

type AdminListTreeRequest struct {
	DeviceID string  `json:"device_id"`
	ParentID *string `json:"parent_id,omitempty"`
	Page     int     `json:"page,omitempty"`
	PageSize int     `json:"page_size,omitempty"`
}

type AdminListTreeResponse struct {
	Nodes     []TreeNodeResponse `json:"nodes"`
	Truncated bool               `json:"truncated,omitempty"`
}
