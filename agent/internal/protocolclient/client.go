package protocolclient

import (
	"encoding/json"
	"fmt"

	"sagiri-guard/network"
)

// SendAction sends a protocol MsgCommand with sub-command action and returns the first response frame.
// If token is provided, it sends a login frame first on the same connection to authorize the action.
// This opens a short-lived TCP connection.
func SendAction(host string, port int, deviceID string, token string, action string, data any) (*network.ProtocolMessage, error) {
	if action == "" {
		return nil, fmt.Errorf("action is required")
	}
	c, err := network.DialTCP(host, port)
	if err != nil {
		return nil, err
	}
	defer c.Close()

	// authorize this connection if token is available
	if token != "" && deviceID != "" {
		if err := c.SendLogin(deviceID, token); err != nil {
			return nil, fmt.Errorf("send login failed: %w", err)
		}
	}

	payload := struct {
		Action string      `json:"action"`
		Data   interface{} `json:"data,omitempty"`
	}{
		Action: action,
		Data:   data,
	}
	b, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	// Send login frame if device id is known (optional; backend currently only requires for authorization)
	// but we include device_id inside data payload; MsgLogin not required here.
	if err := c.SendCommand(b); err != nil {
		return nil, err
	}
	msg, err := c.RecvProtocolMessage()
	if err != nil {
		return nil, err
	}
	return msg, nil
}
