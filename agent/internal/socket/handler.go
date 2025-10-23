package socket

import (
	"bytes"
	"encoding/json"
	"sagiri-guard/agent/internal/logger"
)

type Command struct {
	DeviceID string      `json:"deviceid"`
	Command  string      `json:"command"`
	Argument interface{} `json:"argument"`
}

func HandleMessage(data []byte) {
	// accept single-frame or newline-terminated
	line := bytes.TrimSpace(data)
	if len(line) == 0 {
		return
	}
	logger.Infof("Socket raw: %s", string(line))
	var cmd Command
	if err := json.Unmarshal(line, &cmd); err != nil {
		logger.Errorf("Invalid command: %v | raw=%s", err, string(line))
		return
	}
	logger.Infof("Received command: %s for device %s arg=%v", cmd.Command, cmd.DeviceID, cmd.Argument)
}
