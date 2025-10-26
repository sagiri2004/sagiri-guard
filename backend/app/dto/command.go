package dto

import "encoding/json"

type CommandRequest struct {
	DeviceID string          `json:"deviceid"`
	Command  string          `json:"command"`
	Kind     string          `json:"kind,omitempty"`
	Argument json.RawMessage `json:"argument,omitempty"`
}
