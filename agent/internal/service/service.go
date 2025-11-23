package service

import (
	"os"
	"strings"
	"time"

	"sagiri-guard/agent/internal/auth"
	"sagiri-guard/agent/internal/backup"
	"sagiri-guard/agent/internal/config"
	"sagiri-guard/agent/internal/device"
	"sagiri-guard/agent/internal/logger"
	"sagiri-guard/agent/internal/monitor"
	osq "sagiri-guard/agent/internal/osquery"
	"sagiri-guard/agent/internal/state"

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
	if len(cfg.MonitorPaths) > 0 {
		if fm, err := monitor.NewFileMonitor(cfg.MonitorPaths); err != nil {
			logger.Errorf("failed to create file monitor: %v", err)
		} else {
			ch := fm.MonitorFiles()
			go func() {
				for event := range ch {
					if event.Name == "" {
						continue
					}
					switch {
					case event.Op&fsnotify.Create == fsnotify.Create:
						logger.Infof("File created: %s", event.Name)
						go scheduleBackup(event.Name)
					case event.Op&fsnotify.Write == fsnotify.Write:
						logger.Infof("File modified: %s", event.Name)
						go scheduleBackup(event.Name)
					case event.Op&fsnotify.Remove == fsnotify.Remove:
						logger.Infof("File deleted: %s", event.Name)
					case event.Op&fsnotify.Rename == fsnotify.Rename:
						logger.Infof("File renamed: %s", event.Name)
					}
				}
			}()
		}
	}

	return dev.UUID, nil
}

func scheduleBackup(path string) {
	time.Sleep(1 * time.Second)
	if err := backupFile(path); err != nil {
		logger.Errorf("auto backup failed for %s: %v", path, err)
	}
}

func backupFile(path string) error {
	info, err := os.Stat(path)
	if err != nil {
		return err
	}
	if info.IsDir() {
		return nil
	}
	token := strings.TrimSpace(state.GetToken())
	if token == "" {
		return nil
	}
	host, port := config.BackendHTTP()
	session, err := backup.InitUpload(host, port, token, path)
	if err != nil {
		return err
	}
	return backup.UploadFile(session, path)
}
