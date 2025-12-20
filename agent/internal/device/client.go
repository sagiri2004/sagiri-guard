package device

import (
	"encoding/json"
	"fmt"

	"sagiri-guard/agent/internal/protocolclient"
	"sagiri-guard/network"
)

type Info struct {
	UUID      string `json:"uuid"`
	Name      string `json:"name"`
	OSName    string `json:"os_name"`
	OSVersion string `json:"os_version"`
	Hostname  string `json:"hostname"`
	Arch      string `json:"arch"`
}

// Register via protocol sub-command device_register. Requires that device has logged in before.
func Register(host string, port int, token string, d Info) (*Info, int, error) {
	msg, err := protocolclient.SendAction(host, port, d.UUID, token, "device_register", d)
	if err != nil {
		return nil, 0, err
	}
	if msg.Type != network.MsgAck || msg.StatusCode != 200 {
		return nil, int(msg.StatusCode), fmt.Errorf("device register failed: code=%d msg=%s", msg.StatusCode, msg.StatusMsg)
	}
	var out Info
	if msg.StatusMsg != "" {
		_ = json.Unmarshal([]byte(msg.StatusMsg), &out)
	}
	return &out, int(msg.StatusCode), nil
}
