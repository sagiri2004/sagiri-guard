package auth

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sagiri-guard/agent/internal/config"
	"sagiri-guard/agent/internal/logger"
	"sagiri-guard/network"
)

type Credentials struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

type TokenResponse struct {
	AccessToken string `json:"access_token"`
}

// Login performs HTTP login to backend and stores token to file.
func Login(host string, port int, username, password string) (string, error) {
	creds := Credentials{Username: username, Password: password}
	body, _ := json.Marshal(creds)
	// Use cgo-backed HTTP
	resp, err := network.HTTPPost(host, port, "/login", "application/json", body)
	if err != nil {
		return "", err
	}
	var tr TokenResponse
	if err := json.Unmarshal([]byte(resp), &tr); err != nil || tr.AccessToken == "" {
		return "", errors.New("invalid login response")
	}
	if err := saveToken(tr.AccessToken); err != nil {
		return "", err
	}
	logger.Info("Đăng nhập thành công, token đã được lưu")
	return tr.AccessToken, nil
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
