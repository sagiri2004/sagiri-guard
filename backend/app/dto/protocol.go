package dto

import "encoding/json"

// ProtocolSubCommandEnvelope wraps sub-command requests sent over the TCP protocol.
type ProtocolSubCommandEnvelope struct {
	Action string          `json:"action"`
	Data   json.RawMessage `json:"data"`
}

// ProtocolLoginRequest represents login payload coming from the agent.
type ProtocolLoginRequest struct {
	Username  string `json:"username"`
	Password  string `json:"password"`
	DeviceID  string `json:"device_id,omitempty"`
	Name      string `json:"name,omitempty"`
	OSName    string `json:"os_name,omitempty"`
	OSVersion string `json:"os_version,omitempty"`
	Hostname  string `json:"hostname,omitempty"`
	Arch      string `json:"arch,omitempty"`
}

// BackupDownloadStartRequest is sent by the agent to begin a download session.
type BackupDownloadStartRequest struct {
	SessionID string `json:"session_id"`
	Token     string `json:"token"`
	Offset    uint32 `json:"offset,omitempty"`
}
