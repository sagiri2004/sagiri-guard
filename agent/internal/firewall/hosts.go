//go:build windows

package firewall

import (
	"bufio"
	"fmt"
	"os"
	"strings"
	"sync"
	"sagiri-guard/agent/internal/logger"
)

const (
	windowsHostsPath = `C:\Windows\System32\drivers\etc\hosts`
	blockMarkerStart = "# === SAGIRI-GUARD BLOCK START ==="
	blockMarkerEnd   = "# === SAGIRI-GUARD BLOCK END ==="
)

type HostsManager struct {
	hostsPath string
	mu        sync.RWMutex
	enabled   bool
	domains   map[string]bool // domain -> true nếu đang block
}

var globalHostsManager *HostsManager
var hostsManagerOnce sync.Once

func GetHostsManager() *HostsManager {
	hostsManagerOnce.Do(func() {
		globalHostsManager = &HostsManager{
			hostsPath: windowsHostsPath,
			domains:   make(map[string]bool),
		}
	})
	return globalHostsManager
}

// SetEnabled bật/tắt blocking (chỉ apply khi enabled=true)
func (h *HostsManager) SetEnabled(enabled bool) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.enabled = enabled
	if enabled {
		return h.applyBlocks()
	}
	return h.removeBlocks()
}

// AddDomain thêm domain vào danh sách block
func (h *HostsManager) AddDomain(domain string) error {
	if domain == "" {
		return fmt.Errorf("empty domain")
	}
	domain = strings.ToLower(strings.TrimSpace(domain))
	h.mu.Lock()
	defer h.mu.Unlock()
	h.domains[domain] = true
	if h.enabled {
		return h.applyBlocks()
	}
	return nil
}

// RemoveDomain xóa domain khỏi danh sách block
func (h *HostsManager) RemoveDomain(domain string) error {
	domain = strings.ToLower(strings.TrimSpace(domain))
	h.mu.Lock()
	defer h.mu.Unlock()
	delete(h.domains, domain)
	if h.enabled {
		return h.applyBlocks()
	}
	return nil
}

// SetDomains set toàn bộ danh sách domains (replace)
func (h *HostsManager) SetDomains(domains []string) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.domains = make(map[string]bool)
	for _, d := range domains {
		if d != "" {
			h.domains[strings.ToLower(strings.TrimSpace(d))] = true
		}
	}
	if h.enabled {
		return h.applyBlocks()
	}
	return nil
}

// GetDomains lấy danh sách domains đang block
func (h *HostsManager) GetDomains() []string {
	h.mu.RLock()
	defer h.mu.RUnlock()
	result := make([]string, 0, len(h.domains))
	for d := range h.domains {
		result = append(result, d)
	}
	return result
}

// IsEnabled kiểm tra blocking có đang bật không
func (h *HostsManager) IsEnabled() bool {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.enabled
}

// applyBlocks apply blocking vào hosts file
func (h *HostsManager) applyBlocks() error {
	// Đọc hosts file hiện tại
	content, err := h.readHostsFile()
	if err != nil {
		return fmt.Errorf("read hosts file: %w", err)
	}

	// Xóa block cũ (nếu có)
	content = h.removeBlockSection(content)

	// Thêm block mới
	if len(h.domains) > 0 {
		var blockLines []string
		blockLines = append(blockLines, blockMarkerStart)
		for domain := range h.domains {
			// Chặn cả domain và www.domain
			blockLines = append(blockLines, fmt.Sprintf("127.0.0.1 %s", domain))
			if !strings.HasPrefix(domain, "www.") {
				blockLines = append(blockLines, fmt.Sprintf("127.0.0.1 www.%s", domain))
			}
		}
		blockLines = append(blockLines, blockMarkerEnd)
		content = append(content, blockLines...)
	}

	// Ghi lại hosts file
	return h.writeHostsFile(content)
}

// removeBlocks xóa blocking khỏi hosts file
func (h *HostsManager) removeBlocks() error {
	content, err := h.readHostsFile()
	if err != nil {
		return fmt.Errorf("read hosts file: %w", err)
	}
	content = h.removeBlockSection(content)
	return h.writeHostsFile(content)
}

// readHostsFile đọc hosts file
func (h *HostsManager) readHostsFile() ([]string, error) {
	file, err := os.Open(h.hostsPath)
	if err != nil {
		if os.IsNotExist(err) {
			return []string{}, nil
		}
		return nil, err
	}
	defer file.Close()

	var lines []string
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		lines = append(lines, scanner.Text())
	}
	return lines, scanner.Err()
}

// writeHostsFile ghi hosts file
func (h *HostsManager) writeHostsFile(lines []string) error {
	// Tạo backup trước khi ghi
	backupPath := h.hostsPath + ".sagiri-backup"
	if err := h.backupHostsFile(backupPath); err != nil {
		logger.Warnf("Failed to backup hosts file: %v", err)
		// Không return error, chỉ warn vì backup không bắt buộc
	}

	file, err := os.OpenFile(h.hostsPath, os.O_WRONLY|os.O_TRUNC|os.O_CREATE, 0644)
	if err != nil {
		// Trên Windows, lỗi này thường do thiếu quyền admin
		logger.Errorf("Failed to open hosts file for write (may need admin privileges): %v", err)
		return fmt.Errorf("open hosts file for write (may need admin privileges): %w", err)
	}
	defer file.Close()

	writer := bufio.NewWriter(file)
	for _, line := range lines {
		if _, err := writer.WriteString(line + "\n"); err != nil {
			logger.Errorf("Failed to write line to hosts file: %v", err)
			return fmt.Errorf("write line: %w", err)
		}
	}
	if err := writer.Flush(); err != nil {
		logger.Errorf("Failed to flush hosts file: %v", err)
		return fmt.Errorf("flush hosts file: %w", err)
	}
	return nil
}

// removeBlockSection xóa section block của sagiri-guard
func (h *HostsManager) removeBlockSection(lines []string) []string {
	var result []string
	inBlock := false
	for _, line := range lines {
		if strings.Contains(line, blockMarkerStart) {
			inBlock = true
			continue
		}
		if strings.Contains(line, blockMarkerEnd) {
			inBlock = false
			continue
		}
		if !inBlock {
			result = append(result, line)
		}
	}
	return result
}

// backupHostsFile tạo backup của hosts file
func (h *HostsManager) backupHostsFile(backupPath string) error {
	src, err := os.Open(h.hostsPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil // chưa có file thì không cần backup
		}
		return err
	}
	defer src.Close()

	dst, err := os.Create(backupPath)
	if err != nil {
		return err
	}
	defer dst.Close()

	_, err = dst.ReadFrom(src)
	return err
}

