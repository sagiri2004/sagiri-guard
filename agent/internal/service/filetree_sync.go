package service

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"sagiri-guard/agent/internal/config"
	"sagiri-guard/agent/internal/db"
	"sagiri-guard/agent/internal/logger"
	"sagiri-guard/network"

	"github.com/google/uuid"
)

// fileTreeChange mirrors backend dto.FileChange without importing backend code.
type fileTreeChange struct {
	ID             string `json:"id"`
	OriginPath     string `json:"origin_path"`
	CurrentPath    string `json:"cur_path"`
	CurrentName    string `json:"cur_name"`
	Extension      string `json:"cur_ext"`
	Size           int64  `json:"total_size"`
	SnapshotNumber int    `json:"snapshot_number"`
	IsDir          bool   `json:"is_dir"`
	Deleted        bool   `json:"deleted"`
	ChangePending  bool   `json:"change_pending"`
	ContentTypes   []uint `json:"content_type_ids"`
}

// StartFileTreeSyncLoop periodically flushes local MonitoredFile rows to backend /filetree/sync.
func StartFileTreeSyncLoop(token, deviceUUID string) {
	if strings.TrimSpace(token) == "" || strings.TrimSpace(deviceUUID) == "" {
		return
	}
	go func() {
		ticker := time.NewTicker(10 * time.Second)
		defer ticker.Stop()
		for range ticker.C {
			if err := syncFileTreeOnce(token, deviceUUID); err != nil {
				logger.Errorf("file tree sync failed: %v", err)
			}
		}
	}()
}

func syncFileTreeOnce(token, deviceUUID string) error {
	adb := db.Get()
	if adb == nil {
		return nil
	}

	var rows []db.MonitoredFile
	if err := adb.
		Where("change_pending = ? OR last_backup_at IS NULL OR last_backup_at < last_event_at", true).
		Order("last_event_at ASC").
		Limit(256).
		Find(&rows).Error; err != nil {
		return err
	}
	if len(rows) == 0 {
		return nil
	}

	changes := make([]fileTreeChange, 0, len(rows))
	now := time.Now()

	for _, row := range rows {
		path := strings.TrimSpace(row.Path)
		if path == "" {
			continue
		}

		var (
			size    int64
			isDir   bool
			deleted bool
		)

		info, err := os.Stat(path)
		if err != nil {
			if os.IsNotExist(err) {
				deleted = row.LastAction == string(monitorActionDelete()) || row.LastAction == string(monitorActionMoveOut())
			} else {
				logger.Errorf("stat %s failed: %v", path, err)
				continue
			}
		} else {
			isDir = info.IsDir()
			if !isDir {
				size = info.Size()
			}
		}

		// Xây dựng đường dẫn logic dựa trên folder được monitor.
		// Ví dụ: monitor "C:\Users\izumi\Downloads\Demo" => root logic = "Demo",
		// file "C:\Users\izumi\Downloads\Demo\a\b.txt" => segments = ["Demo","a","b.txt"].
		logicalSegments := buildLogicalSegments(path)
		if len(logicalSegments) == 0 {
			// fallback: dùng full path tách segment như cũ
			logicalSegments = splitSegments(path)
		}
		logicalPath := strings.Join(logicalSegments, "/")

		// build deterministic ID từ device + logical path segments
		itemID := deterministicID(deviceUUID, logicalSegments)

		name := ""
		if len(logicalSegments) > 0 {
			name = logicalSegments[len(logicalSegments)-1]
		} else {
			name = filepath.Base(path)
		}
		ext := ""
		if isDir {
			ext = "folder"
		} else {
			ext = strings.TrimPrefix(strings.ToLower(filepath.Ext(path)), ".")
		}

		needsBackup := row.LastBackupAt == nil || row.LastBackupAt.Before(row.LastEventAt)

		changes = append(changes, fileTreeChange{
			ID:             itemID,
			OriginPath:     logicalPath,
			CurrentPath:    logicalPath,
			CurrentName:    name,
			Extension:      ext,
			Size:           size,
			SnapshotNumber: 0,
			IsDir:          isDir,
			Deleted:        deleted,
			ChangePending:  needsBackup,
			ContentTypes:   nil,
		})
	}

	if len(changes) == 0 {
		return nil
	}

	if err := sendFileTreeChanges(token, deviceUUID, changes); err != nil {
		return err
	}

	// Mark rows as synced (tree state up-to-date). Backup status is tracked via LastBackupAt separately.
	if err := adb.Model(&db.MonitoredFile{}).
		Where("id IN ?", collectIDs(rows)).
		Update("change_pending", false).Error; err != nil {
		logger.Errorf("failed to clear change_pending flags: %v", err)
	}

	_ = now
	return nil
}

func sendFileTreeChanges(token, deviceUUID string, changes []fileTreeChange) error {
	cfg := config.Get()
	host, port := cfg.BackendHost, cfg.BackendHTTP

	body, err := json.Marshal(changes)
	if err != nil {
		return err
	}

	headers := map[string]string{
		"Authorization": "Bearer " + strings.TrimSpace(token),
		"X-Device-ID":   deviceUUID,
		"Content-Type":  "application/json",
		"Accept":        "application/json",
	}

	status, resp, err := network.HTTPRequest("POST", host, port, "/filetree/sync", "application/json", body, headers)
	if err != nil {
		return err
	}
	if status >= 300 {
		snippet := strings.TrimSpace(resp)
		if snippet == "" {
			snippet = fmt.Sprintf("status %d", status)
		}
		return fmt.Errorf("filetree sync failed: %s", snippet)
	}
	return nil
}

func collectIDs(rows []db.MonitoredFile) []uint {
	out := make([]uint, 0, len(rows))
	for _, r := range rows {
		out = append(out, r.ID)
	}
	return out
}

// helpers so this file does not import monitor directly (avoid windows build tags leakage here).
func monitorActionDelete() string  { return "delete" }
func monitorActionMoveOut() string { return "move_out" }

// buildLogicalSegments chuyển absolute path thành các segment logic, dựa trên MonitorPaths.
// Ví dụ: MonitorPaths = ["C:\\Users\\izumi\\Downloads\\Demo"]
// Path = "C:\\Users\\izumi\\Downloads\\Demo\\folder\\file.txt"
// => ["Demo", "folder", "file.txt"]
func buildLogicalSegments(p string) []string {
	cfg := config.Get()
	if len(cfg.MonitorPaths) == 0 || strings.TrimSpace(p) == "" {
		return splitSegments(p)
	}

	absPath, err := filepath.Abs(p)
	if err != nil {
		return splitSegments(p)
	}
	absPathLower := strings.ToLower(absPath)

	for _, root := range cfg.MonitorPaths {
		if strings.TrimSpace(root) == "" {
			continue
		}
		absRoot, err := filepath.Abs(root)
		if err != nil {
			continue
		}
		rootLower := strings.ToLower(absRoot)
		// match root prefix
		if absPathLower == rootLower || strings.HasPrefix(absPathLower, rootLower+string(os.PathSeparator)) {
			rel, err := filepath.Rel(absRoot, absPath)
			if err != nil {
				break
			}
			var segs []string
			rootName := filepath.Base(absRoot)
			if rootName != "" {
				segs = append(segs, rootName)
			}
			if rel != "." {
				for _, part := range strings.Split(filepath.ToSlash(rel), "/") {
					if trimmed := strings.TrimSpace(part); trimmed != "" {
						segs = append(segs, trimmed)
					}
				}
			}
			return segs
		}
	}

	// không match monitor root nào -> fallback: full path
	return splitSegments(p)
}

// splitSegments normalizes a Windows/Unix path into path segments, matching backend logic.
func splitSegments(p string) []string {
	if p == "" {
		return nil
	}
	normalized := filepath.ToSlash(p)
	parts := strings.Split(normalized, "/")
	var segments []string
	for _, part := range parts {
		if trimmed := strings.TrimSpace(part); trimmed != "" {
			segments = append(segments, trimmed)
		}
	}
	return segments
}

// deterministicID mirrors backend/app/services/filetree_service.go
// so that deletes/moves can address the correct tree node.
func deterministicID(deviceUUID string, segments []string) string {
	if len(segments) == 0 {
		return ""
	}
	key := strings.Join(segments, "/")
	return uuid.NewSHA1(uuid.NameSpaceURL, []byte(deviceUUID+"|"+key)).String()
}
