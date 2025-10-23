package service

import (
	"sagiri-guard/agent/internal/auth"
	"sagiri-guard/agent/internal/config"
	"sagiri-guard/agent/internal/device"
	"sagiri-guard/agent/internal/logger"
	osq "sagiri-guard/agent/internal/osquery"
)

func Login(username, password string) (string, error) {
	cfg := config.Get()
	host, port := cfg.BackendHost, cfg.BackendHTTP
	logger.Infof("Đăng nhập tới backend %s:%d", host, port)
	return auth.Login(host, port, username, password)
}

func BootstrapDevice(token string) (string, error) {
	cfg := config.Get()
	si, osv, err := osq.Collect()
	if err != nil {
		return "", err
	}
	dev := device.Info{UUID: si.UUID, Name: si.Hardware, OSName: osv.Name, OSVersion: osv.Version, Hostname: si.Hostname, Arch: si.CPUBrand}
	if _, code, _ := device.Get(cfg.BackendHost, cfg.BackendHTTP, token, dev.UUID); code == 404 {
		if _, _, err := device.Register(cfg.BackendHost, cfg.BackendHTTP, token, dev); err != nil {
			return "", err
		}
	}
	return dev.UUID, nil
}
