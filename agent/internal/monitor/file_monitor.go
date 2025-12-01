//go:build windows

package monitor

import (
	"encoding/binary"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	dbpkg "sagiri-guard/agent/internal/db"
	"sagiri-guard/agent/internal/logger"
	"sync"
	"time"
	"unicode/utf16"
	"unsafe"

	"golang.org/x/sys/windows"
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

type FileMonitor struct {
	targets []watchTarget
	stop    chan struct{}
	wg      sync.WaitGroup
	once    sync.Once
}

type watchTarget struct {
	path   string
	handle windows.Handle
}

type pendingRename struct {
	path string
	when time.Time
}

const (
	eventBufferSize = 64 * 1024 // 64KB
	renamePairTTL   = 2 * time.Second
	eventQueueSize  = 128
)

const changeMask = windows.FILE_NOTIFY_CHANGE_FILE_NAME |
	windows.FILE_NOTIFY_CHANGE_DIR_NAME |
	windows.FILE_NOTIFY_CHANGE_ATTRIBUTES |
	windows.FILE_NOTIFY_CHANGE_SIZE |
	windows.FILE_NOTIFY_CHANGE_LAST_WRITE |
	windows.FILE_NOTIFY_CHANGE_CREATION |
	windows.FILE_NOTIFY_CHANGE_SECURITY

const (
	fileActionAdded          = 0x00000001
	fileActionRemoved        = 0x00000002
	fileActionModified       = 0x00000003
	fileActionRenamedOld     = 0x00000004
	fileActionRenamedNew     = 0x00000005
	fileActionAddedStream    = 0x00000006
	fileActionRemovedStream  = 0x00000007
	fileActionModifiedStream = 0x00000008
)

// NewFileMonitor mở handle trực tiếp tới từng thư mục cần giám sát và chuẩn bị watcher.
func NewFileMonitor(paths []string) (*FileMonitor, error) {
	logger.Infof("Creating Windows file monitor for paths: %v", paths)

	seen := make(map[string]struct{})
	targets := make([]watchTarget, 0, len(paths))

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

		// Kiểm tra thư mục có tồn tại và truy cập được không
		if _, err := os.Stat(dir); err != nil {
			logger.Errorf("Directory %s does not exist or is not accessible: %v (skipping)", dir, err)
			continue
		}

		// Thử mở thư mục để xác minh quyền truy cập
		testFile, err := os.Open(dir)
		if err != nil {
			logger.Errorf("Cannot access directory %s: %v (skipping)", dir, err)
			continue
		}
		testFile.Close()

		handle, err := openDirectoryHandle(dir)
		if err != nil {
			logger.Errorf("Failed to open directory handle for %s: %v (skipping)", dir, err)
			continue
		}

		// Verify handle is valid
		if handle == 0 || handle == windows.InvalidHandle {
			logger.Errorf("Invalid handle returned for %s (skipping)", dir)
			continue
		}

		logger.Infof("Watching path: %s", dir)
		targets = append(targets, watchTarget{path: dir, handle: handle})
		seen[dir] = struct{}{}
	}

	if len(targets) == 0 {
		return nil, errors.New("file monitor: no valid directories to watch")
	}

	return &FileMonitor{
		targets: targets,
		stop:    make(chan struct{}),
	}, nil
}

// MonitorFiles khởi động các goroutine
func (f *FileMonitor) MonitorFiles() <-chan FileEvent {
	out := make(chan FileEvent, eventQueueSize)

	for _, target := range f.targets {
		f.wg.Add(1)
		go f.streamDirectory(target, out)
	}

	go func() {
		f.wg.Wait()
		close(out)
	}()

	return out
}

func (f *FileMonitor) streamDirectory(target watchTarget, out chan<- FileEvent) {
	defer f.wg.Done()

	// Khóa OS Thread để đảm bảo tính nhất quán của Thread ID khi gọi syscall (tốt cho debugging)
	runtime.LockOSThread()

	// FIX QUAN TRỌNG: Sử dụng VirtualAlloc thay vì make([]byte).
	// Bộ nhớ cấp phát bởi VirtualAlloc nằm ngoài Heap của Go (Off-heap),
	// do đó Garbage Collector sẽ KHÔNG bao giờ di chuyển nó.
	// Điều này ngăn chặn lỗi "Invalid access to memory location" khi Debugger can thiệp vào Runtime.
	const bufSize = eventBufferSize
	addr, err := windows.VirtualAlloc(
		0,
		uintptr(bufSize),
		windows.MEM_COMMIT|windows.MEM_RESERVE,
		windows.PAGE_READWRITE,
	)
	if err != nil {
		logger.Errorf("CRITICAL: Failed to allocate memory via VirtualAlloc for %s: %v", target.path, err)
		return
	}

	// Đảm bảo giải phóng bộ nhớ khi goroutine thoát
	defer func() {
		_ = windows.VirtualFree(addr, 0, windows.MEM_RELEASE)
	}()

	// Tạo một Go Slice trỏ vào vùng nhớ VirtualAlloc để dễ thao tác
	// Cú pháp này an toàn từ Go 1.17+
	buffer := unsafe.Slice((*byte)(unsafe.Pointer(addr)), bufSize)

	var renameQueue []pendingRename
	retryCount := 0
	maxRetries := 10 // Tăng số lần retry lên

	for {
		select {
		case <-f.stop:
			return
		default:
		}

		if target.handle == 0 || target.handle == windows.InvalidHandle {
			logger.Errorf("Invalid handle for %s, stopping watch", target.path)
			return
		}

		var bytesRead uint32

		// Gọi ReadDirectoryChangesW với địa chỉ bộ nhớ thô (addr)
		// Không cần runtime.KeepAlive vì đây là bộ nhớ do OS quản lý trực tiếp.
		err := windows.ReadDirectoryChanges(
			target.handle,
			(*byte)(unsafe.Pointer(addr)), // Dùng pointer trực tiếp từ VirtualAlloc
			uint32(bufSize),
			true,
			changeMask,
			&bytesRead,
			nil,
			0,
		)

		if err != nil {
			if isTerminalWatcherErr(err) {
				return
			}
			if err == windows.ERROR_INVALID_HANDLE || err == windows.ERROR_INVALID_PARAMETER {
				logger.Errorf("ReadDirectoryChangesW failed for %s: handle invalid, stopping watch", target.path)
				return
			}

			retryCount++
			if retryCount >= maxRetries {
				logger.Errorf("ReadDirectoryChangesW failed for %s after %d retries: %v (stopping watch)", target.path, maxRetries, err)
				return
			}

			logger.Errorf("ReadDirectoryChangesW failed for %s: %v (retry %d/%d)", target.path, err, retryCount, maxRetries)
			time.Sleep(1 * time.Second)
			continue
		}

		// Reset retry count on success
		retryCount = 0

		if bytesRead == 0 {
			continue
		}

		// Xử lý dữ liệu từ buffer (lúc này buffer đã chứa dữ liệu do Windows ghi vào)
		data := buffer[:bytesRead]
		offset := 0

		for {
			if len(data[offset:]) < 12 {
				break
			}

			next := binary.LittleEndian.Uint32(data[offset:])
			action := binary.LittleEndian.Uint32(data[offset+4:])
			nameLen := binary.LittleEndian.Uint32(data[offset+8:])
			end := offset + 12 + int(nameLen)
			if end > len(data) {
				break
			}

			name := decodeUTF16(data[offset+12 : end])
			fullPath := filepath.Clean(filepath.Join(target.path, name))
			ts := time.Now()

			renameQueue = flushExpiredRenames(renameQueue, ts, out)
			renameQueue = dispatchAction(action, fullPath, ts, renameQueue, out)

			if next == 0 {
				break
			}
			offset += int(next)
		}
	}
}

func dispatchAction(action uint32, fullPath string, ts time.Time, queue []pendingRename, out chan<- FileEvent) []pendingRename {
	switch action {
	case fileActionAdded:
		emitEvent(out, FileEvent{Action: ActionCreate, Path: fullPath, Timestamp: ts})
	case fileActionRemoved:
		emitEvent(out, FileEvent{Action: ActionDelete, Path: fullPath, Timestamp: ts})
	case fileActionModified, fileActionAddedStream, fileActionRemovedStream, fileActionModifiedStream:
		emitEvent(out, FileEvent{Action: ActionModify, Path: fullPath, Timestamp: ts})
	case fileActionRenamedOld:
		queue = append(queue, pendingRename{path: fullPath, when: ts})
	case fileActionRenamedNew:
		if len(queue) == 0 {
			emitEvent(out, FileEvent{Action: ActionCreate, Path: fullPath, Timestamp: ts})
			return queue
		}
		old := queue[0]
		queue = queue[1:]
		actionType := ActionRename
		if filepath.Dir(old.path) != filepath.Dir(fullPath) {
			actionType = ActionMove
		}
		emitEvent(out, FileEvent{
			Action:    actionType,
			Path:      fullPath,
			OldPath:   old.path,
			Timestamp: ts,
		})
	default:
		emitEvent(out, FileEvent{Action: ActionModify, Path: fullPath, Timestamp: ts})
	}

	return queue
}

func flushExpiredRenames(queue []pendingRename, now time.Time, out chan<- FileEvent) []pendingRename {
	if len(queue) == 0 {
		return queue
	}

	index := 0
	for index < len(queue) && now.Sub(queue[index].when) > renamePairTTL {
		emitEvent(out, FileEvent{
			Action:    ActionMoveOut,
			Path:      queue[index].path,
			OldPath:   queue[index].path,
			Timestamp: now,
		})
		index++
	}

	return queue[index:]
}

func emitEvent(out chan<- FileEvent, evt FileEvent) {
	persistMonitoredFile(evt)
	select {
	case out <- evt:
	default:
		// Nếu consumer bị nghẽn quá lâu, bỏ block để tránh đứng watcher;
		// log để còn truy vết.
		logger.Errorf("File monitor backpressure, dropping event %+v", evt)
	}
}

func decodeUTF16(b []byte) string {
	if len(b) == 0 {
		return ""
	}
	if len(b)%2 != 0 {
		b = b[:len(b)-1]
	}

	u16 := make([]uint16, len(b)/2)
	for i := 0; i < len(u16); i++ {
		u16[i] = binary.LittleEndian.Uint16(b[i*2:])
	}

	return string(utf16.Decode(u16))
}

func openDirectoryHandle(path string) (windows.Handle, error) {
	ptr, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return 0, err
	}

	// FILE_FLAG_BACKUP_SEMANTICS là cờ bắt buộc để mở Handle cho thư mục
	handle, err := windows.CreateFile(
		ptr,
		windows.FILE_LIST_DIRECTORY,
		windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE|windows.FILE_SHARE_DELETE,
		nil,
		windows.OPEN_EXISTING,
		windows.FILE_FLAG_BACKUP_SEMANTICS,
		0,
	)
	if err != nil {
		return 0, err
	}
	return handle, nil
}

func isTerminalWatcherErr(err error) bool {
	if err == nil {
		return false
	}

	var errno windows.Errno
	if errors.As(err, &errno) {
		switch errno {
		case windows.ERROR_OPERATION_ABORTED, windows.ERROR_INVALID_HANDLE:
			return true
		}
		// Check for memory access errors (0xC0000005)
		if errno == 0xC0000005 {
			return true
		}
	}

	return false
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

// Close giải phóng mọi handle và yêu cầu toàn bộ goroutine dừng lại.
func (f *FileMonitor) Close() error {
	var closeErr error
	f.once.Do(func() {
		close(f.stop)
		for _, target := range f.targets {
			if target.handle != 0 {
				if err := windows.CloseHandle(target.handle); err != nil && closeErr == nil {
					closeErr = err
				}
			}
		}
		f.wg.Wait()
	})
	return closeErr
}
