package command

import (
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"sagiri-guard/agent/internal/backup"
	"sagiri-guard/agent/internal/config"
	"sagiri-guard/agent/internal/db"
	"sagiri-guard/agent/internal/firewall"
	"sagiri-guard/agent/internal/logger"
	"sagiri-guard/agent/internal/protocolclient"
	"sagiri-guard/agent/internal/service"
	"sagiri-guard/agent/internal/state"
	"sagiri-guard/network"
	"strings"
	"time"
)

type getLogsArg struct {
	Lines int `json:"lines,omitempty"`
}

type getLogsHandler struct{}

func (h getLogsHandler) Kind() Kind { return KindOnce }
func (h getLogsHandler) DecodeArg(raw json.RawMessage) (any, error) {
	if len(raw) == 0 {
		return getLogsArg{Lines: 100}, nil
	}
	var a getLogsArg
	if err := json.Unmarshal(raw, &a); err != nil {
		return nil, err
	}
	if a.Lines == 0 {
		a.Lines = 100
	}
	return a, nil
}
func (h getLogsHandler) HandleOnce(arg any) error {
	cfg := config.Get()
	// read log file if configured; fallback to default alongside token path
	logPath := cfg.LogPath
	if logPath == "" {
		logPath = filepath.Join(filepath.Dir(config.TokenFilePath()), "agent.log")
	}
	data, err := os.ReadFile(logPath)
	if err != nil {
		logger.Errorf("read log failed: %v", err)
		data = []byte("<no log available>")
	}
	// load token & device id from state
	token := strings.TrimSpace(state.GetToken())
	if token == "" {
		logger.Error("no token for posting logs")
		return nil
	}
	deviceID := state.GetDeviceID()
	logger.Infof("Sending logs to backend, bytes=%d device=%s", len(data), deviceID)
	// backend address (single protocol port)
	host, port := config.BackendHostPort()
	q := url.Values{}
	q.Set("deviceid", deviceID)
	msg, err := protocolclient.SendAction(host, port, deviceID, token, "agent_log", map[string]string{"lines": string(data)})
	if err != nil || msg.Type != network.MsgAck || msg.StatusCode >= 300 {
		logger.Errorf("post logs failed: %v code=%d msg=%s", err, msg.StatusCode, msg.StatusMsg)
		return nil
	}
	logger.Infof("Posted logs success, status=%d", msg.StatusCode)
	return nil
}
func (h getLogsHandler) Start(arg any) (func() error, error) { return nil, nil }

// remaining handlers ...

type backupAutoArg struct {
	Enabled     *bool `json:"enabled,omitempty"`      // true = bật, false = tắt, nil = không đổi
	IntervalSec int   `json:"interval_sec,omitempty"` // Deprecated: không dùng nữa
}

type backupAutoHandler struct{}

func (h backupAutoHandler) Kind() Kind { return KindStream }
func (h backupAutoHandler) DecodeArg(raw json.RawMessage) (any, error) {
	if len(raw) == 0 {
		return backupAutoArg{Enabled: nil}, nil
	}
	var a backupAutoArg
	if err := json.Unmarshal(raw, &a); err != nil {
		return nil, err
	}
	return a, nil
}
func (h backupAutoHandler) HandleOnce(arg any) error {
	a, ok := arg.(backupAutoArg)
	if !ok {
		return fmt.Errorf("invalid argument type")
	}
	// Xử lý bật/tắt backup
	if a.Enabled != nil {
		service.SetBackupEnabled(*a.Enabled)
	}
	return nil
}
func (h backupAutoHandler) Start(arg any) (func() error, error) {
	a, ok := arg.(backupAutoArg)
	if !ok {
		return nil, fmt.Errorf("invalid argument type")
	}

	// Xử lý bật/tắt backup
	if a.Enabled != nil {
		service.SetBackupEnabled(*a.Enabled)
		logger.Infof("Backup auto %s via command", map[bool]string{true: "enabled", false: "disabled"}[*a.Enabled])
	}

	// Trả về hàm stop để có thể tắt backup sau này
	stop := func() error {
		service.SetBackupEnabled(false)
		logger.Info("Backup auto stopped via command")
		return nil
	}
	return stop, nil
}

type restoreArg struct {
	FileID    string `json:"file_id"`    // file ID (required, được enrich từ backend)
	VersionID uint   `json:"version_id"` // BackupFileVersion ID (required, được enrich từ backend)
	FileName  string `json:"file_name"`  // stored_name từ backup version (được enrich từ backend)
	DestPath  string `json:"dest_path"`  // đường dẫn đích (được enrich từ backend)
}

type restoreHandler struct{}

func (h restoreHandler) Kind() Kind { return KindOnce }
func (h restoreHandler) DecodeArg(raw json.RawMessage) (any, error) {
	var a restoreArg
	if len(raw) > 0 {
		if err := json.Unmarshal(raw, &a); err != nil {
			return nil, err
		}
	}
	// Backend đã enrich file_name và dest_path, nhưng vẫn cần validate
	if a.FileName == "" {
		return nil, fmt.Errorf("missing file_name (should be enriched by backend)")
	}
	if a.DestPath == "" {
		return nil, fmt.Errorf("missing dest_path (should be enriched by backend)")
	}
	return a, nil
}
func (h restoreHandler) HandleOnce(arg any) error {
	a, ok := arg.(restoreArg)
	if !ok {
		return fmt.Errorf("invalid argument type")
	}
	token := strings.TrimSpace(state.GetToken())
	if token == "" {
		return fmt.Errorf("missing token")
	}

	// Backend đã enrich dest_path và file_name, sử dụng trực tiếp
	destPath := a.DestPath
	if destPath == "" {
		// Fallback: nếu backend không enrich được, dùng temp directory
		destPath = filepath.Join(os.TempDir(), "sagiri-restore", filepath.Base(a.FileName))
		logger.Warnf("Backend did not provide dest_path, using temp: %s", destPath)
	}

	// Đảm bảo thư mục đích tồn tại
	if dir := filepath.Dir(destPath); dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return fmt.Errorf("create dest directory: %w", err)
		}
	}

	// Download file từ server
	host, port := config.BackendHostPort()
	session, err := backup.InitDownload(host, port, token, a.FileName)
	if err != nil {
		return fmt.Errorf("init download: %w", err)
	}

	// Download và ghi đè file hiện tại (nếu có)
	if err := backup.DownloadFile(session, destPath); err != nil {
		return fmt.Errorf("download file: %w", err)
	}

	// Cập nhật MonitoredFile trong local DB để đánh dấu file đã được restore
	if adb := db.Get(); adb != nil {
		now := time.Now()
		// Tìm hoặc tạo MonitoredFile record cho file đã restore
		var mf db.MonitoredFile
		if err := adb.Where("path = ?", destPath).First(&mf).Error; err != nil {
			// Nếu không tìm thấy, tạo mới
			mf = db.MonitoredFile{
				Path:          destPath,
				ItemID:        a.FileID, // Lưu file_id để đồng bộ với backend
				LastAction:    "restore",
				LastEventAt:   now,
				ChangePending: true, // Đánh dấu cần backup lại sau khi restore
			}
			if err := adb.Create(&mf).Error; err != nil {
				logger.Errorf("Failed to create MonitoredFile for restored file %s: %v", destPath, err)
			}
		} else {
			// Cập nhật record hiện có
			if err := adb.Model(&mf).Updates(map[string]interface{}{
				"item_id":        a.FileID, // Cập nhật file_id nếu chưa có
				"last_action":    "restore",
				"last_event_at":  now,
				"change_pending": true, // Đánh dấu cần backup lại sau khi restore
			}).Error; err != nil {
				logger.Errorf("Failed to update MonitoredFile for restored file %s: %v", destPath, err)
			}
		}
	}

	logger.Infof("Restored file_id=%s version_id=%d file_name=%s to %s", a.FileID, a.VersionID, a.FileName, destPath)
	return nil
}

// convertLogicalPathToPhysical chuyển logical path thành physical path dựa trên MonitorPaths
func convertLogicalPathToPhysical(logicalPath string) string {
	cfg := config.Get()
	if len(cfg.MonitorPaths) == 0 || strings.TrimSpace(logicalPath) == "" {
		return ""
	}

	// Logical path có format: "root/folder/file.txt" hoặc "root/file.txt"
	// Cần tìm root path tương ứng
	logicalSegments := strings.Split(strings.Trim(logicalPath, "/"), "/")
	if len(logicalSegments) == 0 {
		return ""
	}

	rootName := logicalSegments[0]
	for _, monitorPath := range cfg.MonitorPaths {
		if strings.TrimSpace(monitorPath) == "" {
			continue
		}
		absRoot, err := filepath.Abs(monitorPath)
		if err != nil {
			continue
		}
		// Kiểm tra xem base name của monitor path có khớp với root name không
		if filepath.Base(absRoot) == rootName {
			// Tìm thấy root, build physical path
			if len(logicalSegments) == 1 {
				// Chỉ có root name, trả về root path
				return absRoot
			}
			// Có sub-paths, append vào root
			relPath := filepath.Join(logicalSegments[1:]...)
			return filepath.Join(absRoot, relPath)
		}
	}

	return ""
}

// findPathFromFileID tìm physical path từ file ID trong local database
func findPathFromFileID(fileID string) string {
	// TODO: Implement nếu cần query từ local DB
	// Hiện tại chưa có mapping file_id -> path trong local DB
	// Có thể cần thêm vào MonitoredFile hoặc query từ backend
	return ""
}
func (h restoreHandler) Start(arg any) (func() error, error) { return nil, nil }

type blockWebsiteArg struct {
	Action string              `json:"action"` // "apply", "remove", "sync"
	Rules  []blockWebsiteRule  `json:"rules,omitempty"`
	Status *blockWebsiteStatus `json:"status,omitempty"`
}

type blockWebsiteRule struct {
	ID       uint   `json:"id"`
	Type     string `json:"type"`     // "category" hoặc "domain"
	Category string `json:"category"` // category name
	Domain   string `json:"domain"`   // domain cụ thể
	Enabled  bool   `json:"enabled"`
}

type blockWebsiteStatus struct {
	Enabled bool `json:"enabled"`
}

type blockWebsiteHandler struct{}

func (h blockWebsiteHandler) Kind() Kind { return KindOnce }
func (h blockWebsiteHandler) DecodeArg(raw json.RawMessage) (any, error) {
	var a blockWebsiteArg
	if len(raw) > 0 {
		if err := json.Unmarshal(raw, &a); err != nil {
			return nil, err
		}
	}
	return a, nil
}

func (h blockWebsiteHandler) HandleOnce(arg any) error {
	a, ok := arg.(blockWebsiteArg)
	if !ok {
		return fmt.Errorf("invalid argument type")
	}

	// Import firewall package
	hostsMgr := firewall.GetHostsManager()
	if hostsMgr == nil {
		return fmt.Errorf("failed to get hosts manager")
	}

	// Xử lý action
	switch a.Action {
	case "sync", "apply":
		// Thu thập tất cả domains cần block
		domainsToBlock := make(map[string]bool)

		// Nếu status.enabled = false, không block gì cả
		if a.Status != nil && !a.Status.Enabled {
			// Tắt blocking
			if err := hostsMgr.SetEnabled(false); err != nil {
				logger.Errorf("Failed to disable website blocking: %v", err)
				return fmt.Errorf("disable blocking: %w", err)
			}
			logger.Info("Website blocking disabled")
			return nil
		}

		// Thu thập domains từ rules
		for _, rule := range a.Rules {
			if !rule.Enabled {
				continue
			}

			if rule.Type == "category" {
				// Lấy domains từ category
				categoryDomains := firewall.GetDomainsByCategory(rule.Category)
				for _, d := range categoryDomains {
					domainsToBlock[d] = true
				}
			} else if rule.Type == "domain" && rule.Domain != "" {
				// Domain cụ thể
				domainsToBlock[rule.Domain] = true
			}
		}

		// Convert map to slice
		domainList := make([]string, 0, len(domainsToBlock))
		for d := range domainsToBlock {
			domainList = append(domainList, d)
		}

		// Set domains và enable blocking
		if err := hostsMgr.SetDomains(domainList); err != nil {
			logger.Errorf("Failed to set blocked domains: %v", err)
			return fmt.Errorf("set domains: %w", err)
		}

		if err := hostsMgr.SetEnabled(true); err != nil {
			logger.Errorf("Failed to enable website blocking: %v", err)
			return fmt.Errorf("enable blocking: %w", err)
		}

		logger.Infof("Website blocking enabled with %d domains", len(domainList))

	case "remove":
		// Tắt blocking hoàn toàn
		if err := hostsMgr.SetEnabled(false); err != nil {
			logger.Errorf("Failed to disable website blocking: %v", err)
			return fmt.Errorf("disable blocking: %w", err)
		}
		logger.Info("Website blocking removed")

	default:
		return fmt.Errorf("unknown action: %s", a.Action)
	}

	return nil
}

func (h blockWebsiteHandler) Start(arg any) (func() error, error) { return nil, nil }

func init() {
	Register("get_logs", getLogsHandler{})
	Register("backup_auto", backupAutoHandler{})
	Register("restore", restoreHandler{})
	Register("block_website", blockWebsiteHandler{})
}
