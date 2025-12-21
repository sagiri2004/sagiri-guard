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
