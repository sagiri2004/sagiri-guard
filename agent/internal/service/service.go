package service

import (
	"errors"
	"fmt"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"sagiri-guard/agent/internal/auth"
	"sagiri-guard/agent/internal/backup"
	"sagiri-guard/agent/internal/config"
	"sagiri-guard/agent/internal/device"
	"sagiri-guard/agent/internal/logger"
	"sagiri-guard/agent/internal/monitor"
	osq "sagiri-guard/agent/internal/osquery"
	"sagiri-guard/agent/internal/state"
)

func Login(username, password string) (string, error) {
	cfg := config.Get()
	host, port := cfg.BackendHost, cfg.BackendHTTP
	logger.Infof("Đăng nhập tới backend %s:%d", host, port)
	return auth.Login(host, port, username, password)
}

var ErrUnauthorized = errors.New("unauthorized")

func BootstrapDevice(token string) (string, error) {
	cfg := config.Get()
	si, osv, err := osq.Collect()
	if err != nil {
		return "", err
	}
	dev := device.Info{UUID: si.UUID, Name: si.Hardware, OSName: osv.Name, OSVersion: osv.Version, Hostname: si.Hostname, Arch: si.CPUBrand}
	host, port := cfg.BackendHost, cfg.BackendHTTP
	if _, code, err := device.Register(host, port, token, dev); err != nil {
		if code == http.StatusUnauthorized {
			return "", ErrUnauthorized
		}
		return "", err
	}

	// monitor files
	if len(cfg.MonitorPaths) > 0 {
		if fm, err := monitor.NewFileMonitor(cfg.MonitorPaths); err != nil {
			logger.Errorf("failed to create file monitor: %v", err)
		} else {
			ch := fm.MonitorFiles()
			go func() {
				for event := range ch {
					if event.Path == "" {
						continue
					}

					switch event.Action {
					case monitor.ActionCreate, monitor.ActionModify:
						logger.Infof("File %s: %s", event.Action, event.Path)
						scheduleBackup(event.Path)
					case monitor.ActionRename, monitor.ActionMove:
						logger.Infof("File %s: %s -> %s", event.Action, event.OldPath, event.Path)
						scheduleBackup(event.Path)
					case monitor.ActionDelete, monitor.ActionMoveOut:
						logger.Infof("File %s: %s", event.Action, event.Path)
					default:
						logger.Infof("File event %s: %s", event.Action, event.Path)
					}
				}
			}()
		}
	}

	return dev.UUID, nil
}

var (
	backupState = struct {
		sync.Mutex
		inFlight map[string]struct{}
		recent   map[string]time.Time
	}{
		inFlight: make(map[string]struct{}),
		recent:   make(map[string]time.Time),
	}
	backupCooldown       = 10 * time.Second
	fileSettleInterval   = 500 * time.Millisecond
	fileSettleMaxWait    = 15 * time.Second
	fileSettleStablePass = 2
)

func scheduleBackup(path string) {
	backupState.Lock()
	if last, ok := backupState.recent[path]; ok && time.Since(last) < backupCooldown {
		backupState.Unlock()
		return
	}
	if _, busy := backupState.inFlight[path]; busy {
		backupState.Unlock()
		return
	}
	backupState.inFlight[path] = struct{}{}
	backupState.Unlock()

	go func() {
		defer func() {
			backupState.Lock()
			delete(backupState.inFlight, path)
			backupState.recent[path] = time.Now()
			backupState.Unlock()
		}()

		if err := waitForStableFile(path); err != nil {
			logger.Errorf("skip backup for %s: %v", path, err)
			return
		}

		if err := backupFile(path); err != nil {
			logger.Errorf("auto backup failed for %s: %v", path, err)
		}
	}()
}

func waitForStableFile(path string) error {
	deadline := time.Now().Add(fileSettleMaxWait)
	var (
		lastSize     int64 = -1
		stablePasses       = 0
	)

	for time.Now().Before(deadline) {
		info, err := os.Stat(path)
		if err != nil {
			if os.IsNotExist(err) {
				time.Sleep(fileSettleInterval)
				continue
			}
			return err
		}
		if info.IsDir() {
			return fmt.Errorf("path %s is a directory", path)
		}

		size := info.Size()
		if size == lastSize {
			stablePasses++
			if stablePasses >= fileSettleStablePass {
				if f, err := os.Open(path); err == nil {
					_ = f.Close()
					return nil
				}
			}
		} else {
			lastSize = size
			stablePasses = 0
		}
		time.Sleep(fileSettleInterval)
	}

	f, err := os.Open(path)
	if err == nil {
		_ = f.Close()
		return nil
	}
	return fmt.Errorf("file %s not ready after %s: %w", path, fileSettleMaxWait, err)
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
