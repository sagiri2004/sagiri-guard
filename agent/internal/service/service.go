package service

import (
	"sagiri-guard/agent/internal/auth"
	"sagiri-guard/agent/internal/config"
	"sagiri-guard/agent/internal/device"
	"sagiri-guard/agent/internal/logger"
	"sagiri-guard/agent/internal/monitor"
	osq "sagiri-guard/agent/internal/osquery"

	"github.com/fsnotify/fsnotify"
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

	// monitor files
	monitor, err := monitor.NewFileMonitor(cfg.MonitorPaths)
	if err != nil {
		logger.Errorf("failed to create file monitor: %v", err)
	}
	monitorFilesChan := monitor.MonitorFiles()
	go func() {
		for event := range monitorFilesChan {
			switch event.Op {
			case fsnotify.Write:
				logger.Infof("File modified: %s", event.Name)
			case fsnotify.Create:
				logger.Infof("File created: %s", event.Name)
			case fsnotify.Remove:
				logger.Infof("File deleted: %s", event.Name)
			}
		}
	}()

	return dev.UUID, nil
}
