package socket

import (
	"bytes"
	"encoding/json"
	"sagiri-guard/agent/internal/command"
	"sagiri-guard/agent/internal/logger"
)

type Command struct {
	DeviceID string          `json:"deviceid"`
	Command  string          `json:"command"`
	Kind     command.Kind    `json:"kind,omitempty"`
	Argument json.RawMessage `json:"argument,omitempty"`
}

var cmdMgr = command.NewManager()

func HandleMessage(data []byte) {
	// accept single-frame or newline-terminated
	line := bytes.TrimSpace(data)
	if len(line) == 0 {
		return
	}
	var cmd Command
	if err := json.Unmarshal(line, &cmd); err != nil {
		logger.Errorf("Invalid command: %v | raw=%s", err, string(line))
		return
	}
	env := command.Envelope{DeviceID: cmd.DeviceID, Name: cmd.Command, Kind: cmd.Kind, Argument: cmd.Argument}
	logger.Infof("In: %s", command.Format(env))
	cmdMgr.Dispatch(env)
}
