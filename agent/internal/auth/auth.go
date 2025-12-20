package auth

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/google/uuid"

	"sagiri-guard/agent/internal/config"
	"sagiri-guard/agent/internal/db"
	"sagiri-guard/agent/internal/logger"
	"sagiri-guard/agent/internal/osquery"
	"sagiri-guard/agent/internal/protocolclient"
	"sagiri-guard/agent/internal/state"
	"sagiri-guard/network"
)

type Credentials struct {
	Username  string `json:"username"`
	Password  string `json:"password"`
	DeviceID  string `json:"device_id"`
	Name      string `json:"name,omitempty"`
	OSName    string `json:"os_name,omitempty"`
	OSVersion string `json:"os_version,omitempty"`
	Hostname  string `json:"hostname,omitempty"`
	Arch      string `json:"arch,omitempty"`
}

type TokenResponse struct {
	AccessToken string `json:"token"`
	DeviceID    string `json:"device_id"`
}

// Login performs protocol login to backend and stores token to file.
func Login(host string, port int, username, password string) (string, string, error) {
	creds := Credentials{Username: username, Password: password}
	// collect device id via osquery
	if si, osv, err := osquery.Collect(); err == nil {
		if si.UUID != "" {
			creds.DeviceID = si.UUID
		}
		creds.Name = si.Hardware
		creds.Hostname = si.Hostname
		creds.Arch = si.CPUBrand
		creds.OSName = osv.Name
		creds.OSVersion = osv.Version
	}
	// fallback if osquery không trả UUID
	if creds.DeviceID == "" {
		if hn, _ := os.Hostname(); hn != "" {
			creds.DeviceID = hn
		} else {
			creds.DeviceID = uuid.NewString()
		}
	}

	msg, err := protocolclient.SendAction(host, port, creds.DeviceID, "", "login", creds)
	if err != nil {
		return "", "", err
	}
	if msg.Type != network.MsgAck || msg.StatusCode != 200 {
		return "", "", fmt.Errorf("login failed: code=%d msg=%s", msg.StatusCode, msg.StatusMsg)
	}
	var tr TokenResponse
	if err := json.Unmarshal([]byte(msg.StatusMsg), &tr); err != nil || tr.AccessToken == "" {
		return "", "", errors.New("invalid login response")
	}
	// persist token to SQLite
	if adb := db.Get(); adb != nil {
		_ = adb.Create(&db.Token{Value: tr.AccessToken}).Error
	}
	if err := saveToken(tr.AccessToken); err != nil {
		return "", "", err
	}
	SetCurrentToken(tr.AccessToken)
	if tr.DeviceID != "" {
		state.SetDeviceID(tr.DeviceID)
	}
	logger.Info("Đăng nhập thành công, token đã được lưu")
	return tr.AccessToken, tr.DeviceID, nil
}

func saveToken(token string) error {
	path := config.TokenFilePath()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("mkdir token dir: %w", err)
	}
	return os.WriteFile(path, []byte(token), 0o600)
}

func LoadToken() (string, error) {
	path := config.TokenFilePath()
	b, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

func ClearToken() error {
	path := config.TokenFilePath()
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}
