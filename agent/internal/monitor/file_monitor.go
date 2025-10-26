package monitor

import (
	"os"
	"path/filepath"
	"sagiri-guard/agent/internal/logger"
	"time"

	"github.com/fsnotify/fsnotify"
)

type FileMonitor struct {
	watcher *fsnotify.Watcher
	paths   []string
}

func NewFileMonitor(paths []string) (*FileMonitor, error) {
	logger.Infof("Creating file monitor for paths: %v", paths)

	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		logger.Errorf("Failed to create file watcher: %v", err)
		return nil, err
	}

	for _, path := range paths {
		if _, err := os.Stat(path); err != nil {
			logger.Errorf("Invalid path %s: %v", path, err)
			continue
		}

		if err := watcher.Add(path); err != nil {
			logger.Errorf("Failed to add path to watcher: %v", err)
			continue
		}
		logger.Infof("Watching path: %s", filepath.Clean(path))
	}

	return &FileMonitor{
		paths:   paths,
		watcher: watcher,
	}, nil
}

// MonitorFiles khởi động goroutine giám sát và trả về một channel (chỉ-đọc)
// nơi các sự kiện "ổn định" (đã qua debouncing) sẽ được gửi đến.
func (f *FileMonitor) MonitorFiles() <-chan fsnotify.Event {

	// Tạo một channel có buffer (kích thước 10)
	// Dùng buffer để goroutine không bị block nếu consumer xử lý chậm
	stableEvents := make(chan fsnotify.Event, 10)

	go func() {
		// Đảm bảo channel được đóng khi goroutine thoát
		// để báo hiệu cho consumer biết là đã dừng.
		defer close(stableEvents)

		var (
			timer     = time.NewTimer(time.Millisecond)
			lastEvent fsnotify.Event
			hasEvent  bool
		)

		if !timer.Stop() {
			select {
			case <-timer.C:
			default:
			}
		}

		for {
			select {
			case <-timer.C:
				if !hasEvent {
					continue
				}

				// Gửi sự kiện ổn định ra channel
				// Dùng select-default để tránh bị block nếu buffer đầy
				select {
				case stableEvents <- lastEvent:
				default:
				}

				hasEvent = false

			case event, ok := <-f.watcher.Events:
				if !ok {
					return
				}
				lastEvent = event
				hasEvent = true

				// Xả timer cũ
				if !timer.Stop() {
					select {
					case <-timer.C:
					default:
					}
				}
				// Reset (nhấn "snooze") timer
				timer.Reset(100 * time.Millisecond)

			// TRƯỜNG HỢP 3: Lỗi
			case err, ok := <-f.watcher.Errors:
				if !ok {
					return
				}
				logger.Errorf("[Monitor] File watcher error: %v", err)
			}
		}
	}()

	// Trả về channel ngay lập tức
	return stableEvents
}

// Close ra lệnh cho FileMonitor dừng giám sát.
// Nó sẽ đóng watcher, làm goroutine thoát và đóng channel đã trả về.
func (f *FileMonitor) Close() error {
	logger.Infof("[Monitor] Close() called. Shutting down watcher.")
	// Đóng watcher sẽ làm cho các kênh .Events và .Errors bị đóng,
	// khiến goroutine thoát ra ở mệnh đề 'if !ok'.
	return f.watcher.Close()
}
