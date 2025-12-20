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
	"sagiri-guard/agent/internal/db"
	"sagiri-guard/agent/internal/device"
	"sagiri-guard/agent/internal/logger"
	"sagiri-guard/agent/internal/monitor"
	osq "sagiri-guard/agent/internal/osquery"
	"sagiri-guard/agent/internal/state"
)

func Login(username, password string) (string, string, error) {
	cfg := config.Get()
	host, port := cfg.BackendHost, cfg.BackendPort
	logger.Infof("Đăng nhập tới backend %s:%d", host, port)
	return auth.Login(host, port, username, password)
}

var ErrUnauthorized = errors.New("unauthorized")

func BootstrapDevice(token string, deviceID string) (string, error) {
	cfg := config.Get()
	si, osv, err := osq.Collect()
	if err != nil {
		return "", err
	}
	uuid := deviceID
	if uuid == "" {
		uuid = si.UUID
		if uuid == "" {
			uuid = si.Hostname
		}
	}
	dev := device.Info{UUID: uuid, Name: si.Hardware, OSName: osv.Name, OSVersion: osv.Version, Hostname: si.Hostname, Arch: si.CPUBrand}
	host, port := cfg.BackendHost, cfg.BackendPort
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
		enabled  bool // Flag để bật/tắt backup tự động
	}{
		inFlight: make(map[string]struct{}),
		recent:   make(map[string]time.Time),
		enabled:  true, // Mặc định bật backup
	}
	backupCooldown       = 10 * time.Second
	fileSettleInterval   = 500 * time.Millisecond
	fileSettleMaxWait    = 15 * time.Second
	fileSettleStablePass = 2
)

// SetBackupEnabled bật/tắt backup tự động
func SetBackupEnabled(enabled bool) {
	backupState.Lock()
	defer backupState.Unlock()
	backupState.enabled = enabled
	if enabled {
		logger.Info("Automatic backup enabled")
	} else {
		logger.Info("Automatic backup disabled")
	}
}

// IsBackupEnabled kiểm tra xem backup tự động có được bật không
func IsBackupEnabled() bool {
	backupState.Lock()
	defer backupState.Unlock()
	return backupState.enabled
}

func scheduleBackup(path string) {
	backupState.Lock()
	// Kiểm tra xem backup có được bật không
	if !backupState.enabled {
		backupState.Unlock()
		return
	}
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

	// Lấy file_id từ MonitoredFile để gửi lên backend
	var fileID string
	if adb := db.Get(); adb != nil {
		var mf db.MonitoredFile
		if err := adb.Where("path = ?", path).First(&mf).Error; err == nil {
			fileID = mf.ItemID // Sử dụng ItemID đã lưu trong MonitoredFile
		}
	}

	host, port := config.BackendHostPort()
	session, err := backup.InitUpload(host, port, token, path, fileID)
	if err != nil {
		return err
	}
	if err := backup.UploadFile(session, path); err != nil {
		return err
	}

	// Mark file as backed up in local DB (for sync/restore semantics).
	if adb := db.Get(); adb != nil {
		now := time.Now()
		if err := adb.Model(&db.MonitoredFile{}).
			Where("path = ?", path).
			Updates(map[string]any{
				"last_backup_at": now,
			}).Error; err != nil {
			logger.Errorf("failed to update last_backup_at for %s: %v", path, err)
		}
	}
	return nil
}
