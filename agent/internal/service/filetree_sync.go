package service

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"sagiri-guard/agent/internal/config"
	"sagiri-guard/agent/internal/db"
	"sagiri-guard/agent/internal/logger"
	"sagiri-guard/agent/internal/state"

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

// ConnectionSender interface for sending commands
type ConnectionSender interface {
	Send(action string, data interface{}) error
}

// StartFileTreeSyncLoop periodically flushes local MonitoredFile rows to backend /filetree/sync.
func StartFileTreeSyncLoop(connMgr ConnectionSender) {
	if connMgr == nil {
		return
	}
	go func() {
		ticker := time.NewTicker(10 * time.Second)
		defer ticker.Stop()
		for range ticker.C {
			if err := syncFileTreeOnce(connMgr); err != nil {
				logger.Errorf("file tree sync failed: %v", err)
			}
		}
	}()
}

func syncFileTreeOnce(connMgr ConnectionSender) error {
	adb := db.Get()
	if adb == nil {
		return nil
	}

	deviceUUID := state.GetDeviceID()
	if deviceUUID == "" {
		return fmt.Errorf("device UUID not set")
	}

	// Query các file cần sync:
	// 1. change_pending = true (file mới hoặc đã thay đổi)
	// 2. last_backup_at IS NULL hoặc < last_event_at (chưa backup hoặc đã thay đổi sau backup)
	// 3. last_action = 'delete' hoặc 'move_out' (luôn sync delete event để backend có thể soft delete)
	var rows []db.MonitoredFile
	if err := adb.
		Where("change_pending = ? OR last_backup_at IS NULL OR last_backup_at < last_event_at OR last_action IN ?",
			true,
			[]string{string(monitorActionDelete()), string(monitorActionMoveOut())}).
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
		// Ví dụ: monitor "C:\\Users\\izumi\\Downloads\\Demo" => root logic = "Demo",
		// file "C:\\Users\\izumi\\Downloads\\Demo\\a\\b.txt" => segments = ["Demo","a","b.txt"].
		logicalSegments := buildLogicalSegments(path)
		if len(logicalSegments) == 0 {
			// fallback: dùng full path tách segment như cũ
			logicalSegments = splitSegments(path)
		}
		logicalPath := strings.Join(logicalSegments, "/")

		// Sử dụng ItemID đã lưu trong MonitoredFile, hoặc tạo mới bằng uniqueID()
		// để đảm bảo mỗi file instance có ID unique (kể cả khi cùng path và name)
		itemID := row.ItemID
		if itemID == "" {
			// Chưa có ItemID, tạo mới bằng unique ID với timestamp để đảm bảo tính duy nhất
			// Sử dụng database ID và LastEventAt để phân biệt các file instance khác nhau
			itemID = uniqueID(deviceUUID, logicalSegments, row.ID, row.LastEventAt)
			// Lưu ItemID vào MonitoredFile để dùng cho các lần sync sau
			if err := adb.Model(&db.MonitoredFile{}).
				Where("id = ?", row.ID).
				Update("item_id", itemID).Error; err != nil {
				logger.Errorf("Failed to update ItemID for %s: %v", path, err)
			}
		}

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

	if err := connMgr.Send("filetree_sync", changes); err != nil {
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
// Ví dụ: MonitorPaths = ["C:\\\\Users\\\\izumi\\\\Downloads\\\\Demo"]
// Path = "C:\\\\Users\\\\izumi\\\\Downloads\\\\Demo\\\\folder\\\\file.txt"
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

// uniqueID tạo ID duy nhất cho mỗi file instance bằng cách kết hợp
// deviceUUID, logical path segments, database ID, LastEventAt và timestamp hiện tại.
// Điều này đảm bảo mỗi file có ID khác nhau kể cả khi:
// - Các file có cùng tên ở cùng đường dẫn (sẽ có database ID khác nhau)
// - Cùng một file nhưng bị xóa và restore lại (sẽ có LastEventAt khác nhau khi restore)
// - File được tạo cùng lúc (thêm timestamp hiện tại để đảm bảo unique)
func uniqueID(deviceUUID string, segments []string, dbID uint, lastEventAt time.Time) string {
	if len(segments) == 0 {
		return ""
	}
	key := strings.Join(segments, "/")
	// Kết hợp database ID, LastEventAt và timestamp hiện tại để đảm bảo tính duy nhất
	// Thêm time.Now() để đảm bảo các file được tạo cùng lúc vẫn có ID khác nhau
	now := time.Now()
	combined := fmt.Sprintf("%s|%s|%d|%d|%d", deviceUUID, key, dbID, lastEventAt.UnixNano(), now.UnixNano())
	return uuid.NewSHA1(uuid.NameSpaceURL, []byte(combined)).String()
}

// deterministicID mirrors backend/app/services/filetree_service.go
// Tạo ID cố định từ deviceUUID và logical path segments
// ID này sẽ được đồng bộ giữa agent và backend
func deterministicID(deviceUUID string, segments []string) string {
	if len(segments) == 0 {
		return ""
	}
	key := strings.Join(segments, "/")
	return uuid.NewSHA1(uuid.NameSpaceURL, []byte(deviceUUID+"|"+key)).String()
}
