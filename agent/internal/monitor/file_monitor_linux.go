//go:build linux

package monitor

import (
	"errors"
	"os"
	"path/filepath"
	dbpkg "sagiri-guard/agent/internal/db"
	"sagiri-guard/agent/internal/logger"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"
	"gorm.io/gorm/clause"
)

// ActionType mô tả các sự kiện nghiệp vụ cấp cao mà backend quan tâm.
type ActionType string

const (
	ActionCreate  ActionType = "create"
	ActionModify  ActionType = "modify"
	ActionDelete  ActionType = "delete"
	ActionRename  ActionType = "rename"
	ActionMove    ActionType = "move"
	ActionMoveOut ActionType = "move_out"
)

// FileEvent biểu diễn sự kiện nghiệp vụ đã được gom/chuẩn hóa.
type FileEvent struct {
	Action    ActionType
	Path      string
	OldPath   string
	Timestamp time.Time
}

const (
	eventQueueSize = 128
)

// FileMonitor sử dụng fsnotify (inotify) để theo dõi thay đổi hệ thống file trên Linux.
type FileMonitor struct {
	watcher    *fsnotify.Watcher
	watchedDir map[string]struct{}
	mu         sync.Mutex

	stop chan struct{}
	wg   sync.WaitGroup
	once sync.Once
}

// NewFileMonitor tạo watcher cho danh sách đường dẫn (file hoặc thư mục).
// Mỗi đường dẫn được chuẩn hóa thành thư mục chứa và được theo dõi đệ quy.
func NewFileMonitor(paths []string) (*FileMonitor, error) {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, err
	}

	fm := &FileMonitor{
		watcher:    watcher,
		watchedDir: make(map[string]struct{}),
		stop:       make(chan struct{}),
	}

	seen := make(map[string]struct{})
	for _, raw := range paths {
		abs, err := filepath.Abs(raw)
		if err != nil {
			logger.Errorf("Failed to resolve %s: %v", raw, err)
			continue
		}

		info, err := os.Stat(abs)
		if err != nil {
			logger.Errorf("Invalid path %s: %v", abs, err)
			continue
		}

		dir := abs
		if !info.IsDir() {
			dir = filepath.Dir(abs)
		}
		dir = filepath.Clean(dir)

		if _, ok := seen[dir]; ok {
			continue
		}

		if err := fm.watchRecursive(dir); err != nil {
			logger.Errorf("Failed to watch %s: %v", dir, err)
			continue
		}

		logger.Infof("Watching path: %s", dir)
		seen[dir] = struct{}{}
	}

	if len(fm.watchedDir) == 0 {
		_ = fm.watcher.Close()
		return nil, errors.New("file monitor: no valid directories to watch")
	}

	return fm, nil
}

// MonitorFiles khởi động goroutine theo dõi và trả về channel nhận FileEvent.
func (f *FileMonitor) MonitorFiles() <-chan FileEvent {
	out := make(chan FileEvent, eventQueueSize)

	f.wg.Add(1)
	go f.processEvents(out)

	go func() {
		f.wg.Wait()
		close(out)
	}()

	return out
}

func (f *FileMonitor) processEvents(out chan<- FileEvent) {
	defer f.wg.Done()

	for {
		select {
		case <-f.stop:
			return
		case evt, ok := <-f.watcher.Events:
			if !ok {
				return
			}
			f.handleEvent(evt, out)
		case err, ok := <-f.watcher.Errors:
			if !ok {
				return
			}
			logger.Errorf("File watcher error: %v", err)
		}
	}
}

func (f *FileMonitor) handleEvent(evt fsnotify.Event, out chan<- FileEvent) {
	path := filepath.Clean(evt.Name)
	now := time.Now()

	if evt.Op&fsnotify.Create != 0 {
		emitEvent(out, FileEvent{
			Action:    ActionCreate,
			Path:      path,
			Timestamp: now,
		})
		if f.isDir(path) {
			if err := f.watchRecursive(path); err != nil {
				logger.Warnf("Failed to watch new directory %s: %v", path, err)
			}
		}
	}

	if evt.Op&fsnotify.Write != 0 {
		emitEvent(out, FileEvent{
			Action:    ActionModify,
			Path:      path,
			Timestamp: now,
		})
	}

	if evt.Op&fsnotify.Remove != 0 {
		emitEvent(out, FileEvent{
			Action:    ActionDelete,
			Path:      path,
			Timestamp: now,
		})
		f.removeWatch(path)
	}

	if evt.Op&fsnotify.Rename != 0 {
		emitEvent(out, FileEvent{
			Action:    ActionMoveOut,
			Path:      path,
			OldPath:   path,
			Timestamp: now,
		})
		f.removeWatch(path)
	}
}

func (f *FileMonitor) watchRecursive(root string) error {
	return filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			logger.Warnf("Failed to access %s: %v", path, err)
			return nil
		}
		if !d.IsDir() {
			return nil
		}
		return f.addWatch(path)
	})
}

func (f *FileMonitor) addWatch(dir string) error {
	f.mu.Lock()
	if _, exists := f.watchedDir[dir]; exists {
		f.mu.Unlock()
		return nil
	}
	f.mu.Unlock()

	if err := f.watcher.Add(dir); err != nil {
		return err
	}

	f.mu.Lock()
	f.watchedDir[dir] = struct{}{}
	f.mu.Unlock()
	return nil
}

func (f *FileMonitor) removeWatch(path string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if _, ok := f.watchedDir[path]; ok {
		if err := f.watcher.Remove(path); err != nil {
			logger.Warnf("Failed to remove watcher for %s: %v", path, err)
		}
		delete(f.watchedDir, path)
	}
}

func (f *FileMonitor) isDir(path string) bool {
	info, err := os.Stat(path)
	if err != nil {
		return false
	}
	return info.IsDir()
}

// Close giải phóng watcher và dừng toàn bộ goroutine.
func (f *FileMonitor) Close() error {
	var closeErr error
	f.once.Do(func() {
		close(f.stop)
		if err := f.watcher.Close(); err != nil {
			closeErr = err
		}
	})
	f.wg.Wait()
	return closeErr
}

func emitEvent(out chan<- FileEvent, evt FileEvent) {
	persistMonitoredFile(evt)
	select {
	case out <- evt:
	default:
		logger.Errorf("File monitor backpressure, dropping event %+v", evt)
	}
}

func persistMonitoredFile(evt FileEvent) {
	if evt.Path == "" {
		return
	}

	db := dbpkg.Get()
	if db == nil {
		return
	}

	ts := evt.Timestamp
	if ts.IsZero() {
		ts = time.Now()
	}

	changePending := shouldMarkPending(evt.Action)
	record := dbpkg.MonitoredFile{
		Path:          evt.Path,
		LastAction:    string(evt.Action),
		LastEventAt:   ts,
		ChangePending: changePending,
	}

	if err := db.Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "path"}},
		DoUpdates: clause.Assignments(map[string]any{
			"last_action":    record.LastAction,
			"last_event_at":  record.LastEventAt,
			"change_pending": record.ChangePending,
		}),
	}).Create(&record).Error; err != nil {
		logger.Errorf("Persist file event failed for %s: %v", evt.Path, err)
	}

	if evt.OldPath != "" && evt.OldPath != evt.Path {
		persistOldPath(evt.OldPath, ts)
	}
}

func persistOldPath(oldPath string, ts time.Time) {
	db := dbpkg.Get()
	if db == nil {
		return
	}
	record := dbpkg.MonitoredFile{
		Path:          oldPath,
		LastAction:    string(ActionMoveOut),
		LastEventAt:   ts,
		ChangePending: false,
	}
	if err := db.Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "path"}},
		DoUpdates: clause.Assignments(map[string]any{
			"last_action":    record.LastAction,
			"last_event_at":  record.LastEventAt,
			"change_pending": record.ChangePending,
		}),
	}).Create(&record).Error; err != nil {
		logger.Errorf("Persist move-out file event failed for %s: %v", oldPath, err)
	}
}

func shouldMarkPending(action ActionType) bool {
	switch action {
	case ActionDelete, ActionMoveOut:
		return false
	default:
		return true
	}
}
