package backup

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"sagiri-guard/agent/internal/state"
	"sagiri-guard/backend/global"
	"sagiri-guard/network"
)

type Session struct {
	SessionID string `json:"session_id"`
	Token     string `json:"token"`
	FileName  string `json:"file_name"`
	FileSize  int64  `json:"file_size"`
	Offset    int64  `json:"offset"`
	ChunkSize int64  `json:"chunk_size"`
	TCPHost   string `json:"tcp_host"`
	TCPPort   int    `json:"tcp_port"`
	Direction string `json:"direction"`
	Status    string `json:"status"`
}

type UploadInitRequest struct {
	FileName    string `json:"file_name"`
	FileSize    int64  `json:"file_size"`
	Checksum    string `json:"checksum,omitempty"`
	LogicalPath string `json:"logical_path,omitempty"`
}

type DownloadInitRequest struct {
	FileName string `json:"file_name"`
}

func InitUpload(host string, port int, token string, filePath string) (*Session, error) {
	info, err := os.Stat(filePath)
	if err != nil {
		return nil, fmt.Errorf("stat file: %w", err)
	}
	req := UploadInitRequest{
		FileName:    filepath.Base(filePath),
		FileSize:    info.Size(),
		LogicalPath: filePath,
	}
	body, _ := json.Marshal(req)
	resp, err := callBackupAPI(host, port, "/backup/upload/init", body, authHeaders(token))
	if err != nil {
		return nil, err
	}
	var session Session
	if err := json.Unmarshal(resp, &session); err != nil {
		return nil, fmt.Errorf("parse upload session response: %w | body=%s", err, bytes.TrimSpace(resp))
	}
	return &session, nil
}

func InitDownload(host string, port int, token string, fileName string) (*Session, error) {
	req := DownloadInitRequest{FileName: fileName}
	body, _ := json.Marshal(req)
	resp, err := callBackupAPI(host, port, "/backup/download/init", body, authHeaders(token))
	if err != nil {
		return nil, err
	}
	var session Session
	if err := json.Unmarshal(resp, &session); err != nil {
		return nil, fmt.Errorf("parse download session response: %w | body=%s", err, bytes.TrimSpace(resp))
	}
	return &session, nil
}

func UploadFile(session *Session, filePath string) error {
	file, err := os.Open(filePath)
	if err != nil {
		return fmt.Errorf("open file: %w", err)
	}
	defer file.Close()

	client, err := network.DialTCP(session.TCPHost, session.TCPPort)
	if err != nil {
		return fmt.Errorf("dial backup server: %w", err)
	}
	defer client.Close()

	headers := map[string]string{
		"Session-ID":    session.SessionID,
		"Session-Token": session.Token,
		"Action":        "upload",
	}
	if dev := state.GetDeviceID(); dev != "" {
		headers["X-Device-ID"] = dev
	}
	if err := network.SendTokenHeaders(client, headers); err != nil {
		return fmt.Errorf("send headers: %w", err)
	}

	offsetBuf := make([]byte, 8)
	if _, err := client.ReadFull(offsetBuf); err != nil {
		return fmt.Errorf("read offset ack: %w", err)
	}
	offset := int64(binary.BigEndian.Uint64(offsetBuf))
	if offset > 0 {
		if _, err := file.Seek(offset, io.SeekStart); err != nil {
			return fmt.Errorf("seek file: %w", err)
		}
	}
	bufSize := int(session.ChunkSize)
	if bufSize <= 0 {
		bufSize = 512 * 1024
	}
	dataBuf := make([]byte, bufSize)
	lenBuf := make([]byte, 4)

	for {
		n, err := file.Read(dataBuf)
		if n > 0 {
			binary.BigEndian.PutUint32(lenBuf, uint32(n))
			if err := writeAll(client, lenBuf); err != nil {
				return fmt.Errorf("write chunk header: %w", err)
			}
			if err := writeAll(client, dataBuf[:n]); err != nil {
				return fmt.Errorf("write chunk: %w", err)
			}
		}
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("read file: %w", err)
		}
	}
	// signal completion
	var zero [4]byte
	if err := writeAll(client, zero[:]); err != nil {
		return fmt.Errorf("send close chunk: %w", err)
	}
	// final ack
	if _, err := client.ReadFull(offsetBuf); err == nil {
		session.Offset = int64(binary.BigEndian.Uint64(offsetBuf))
	}
	global.Logger.Info().Msgf("Uploaded %s (%d bytes)", session.FileName, session.Offset)
	return nil
}

func DownloadFile(session *Session, destPath string) error {
	var resumeOffset int64
	if info, err := os.Stat(destPath); err == nil {
		resumeOffset = info.Size()
	}
	if dir := filepath.Dir(destPath); dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return fmt.Errorf("mkdir dest: %w", err)
		}
	}
	file, err := os.OpenFile(destPath, os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return fmt.Errorf("open dest: %w", err)
	}
	defer file.Close()

	client, err := network.DialTCP(session.TCPHost, session.TCPPort)
	if err != nil {
		return fmt.Errorf("dial backup server: %w", err)
	}
	defer client.Close()

	headers := map[string]string{
		"Session-ID":    session.SessionID,
		"Session-Token": session.Token,
		"Action":        "download",
		"Client-Offset": fmt.Sprintf("%d", resumeOffset),
	}
	if dev := state.GetDeviceID(); dev != "" {
		headers["X-Device-ID"] = dev
	}
	if err := network.SendTokenHeaders(client, headers); err != nil {
		return fmt.Errorf("send headers: %w", err)
	}

	meta := make([]byte, 16)
	if _, err := client.ReadFull(meta); err != nil {
		return fmt.Errorf("read meta: %w", err)
	}
	totalSize := int64(binary.BigEndian.Uint64(meta[:8]))
	startOffset := int64(binary.BigEndian.Uint64(meta[8:]))
	if _, err := file.Seek(startOffset, io.SeekStart); err != nil {
		return fmt.Errorf("seek dest: %w", err)
	}

	bufSize := int(session.ChunkSize)
	if bufSize <= 0 {
		bufSize = 512 * 1024
	}
	dataBuf := make([]byte, bufSize)
	lenBuf := make([]byte, 4)

	for {
		if _, err := client.ReadFull(lenBuf); err != nil {
			return fmt.Errorf("read chunk header: %w", err)
		}
		chunkLen := binary.BigEndian.Uint32(lenBuf)
		if chunkLen == 0 {
			break
		}
		if int(chunkLen) > len(dataBuf) {
			return fmt.Errorf("chunk too large: %d", chunkLen)
		}
		if _, err := client.ReadFull(dataBuf[:chunkLen]); err != nil {
			return fmt.Errorf("read chunk: %w", err)
		}
		if _, err := file.Write(dataBuf[:chunkLen]); err != nil {
			return fmt.Errorf("write dest: %w", err)
		}
		startOffset += int64(chunkLen)
	}
	session.Offset = startOffset
	session.FileSize = totalSize
	global.Logger.Info().Msgf("Downloaded %s (%d/%d bytes)", session.FileName, startOffset, totalSize)
	return nil
}

func authHeaders(token string) map[string]string {
	headers := map[string]string{
		"Authorization": "Bearer " + token,
	}
	if dev := state.GetDeviceID(); dev != "" {
		headers["X-Device-ID"] = dev
	}
	return headers
}

func writeAll(client *network.TCPClient, data []byte) error {
	total := 0
	for total < len(data) {
		n, err := client.Write(data[total:])
		if err != nil {
			return err
		}
		if n == 0 {
			return io.ErrUnexpectedEOF
		}
		total += n
	}
	return nil
}

func callBackupAPI(host string, port int, path string, payload []byte, headers map[string]string) ([]byte, error) {
	baseHost := strings.TrimSpace(host)
	if strings.HasPrefix(baseHost, "http://") {
		baseHost = strings.TrimPrefix(baseHost, "http://")
	} else if strings.HasPrefix(baseHost, "https://") {
		return nil, fmt.Errorf("https scheme is not supported by the agent transport")
	}
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}

	reqHeaders := make(map[string]string, len(headers)+2)
	for k, v := range headers {
		reqHeaders[k] = v
	}
	reqHeaders["Content-Type"] = "application/json"
	reqHeaders["Accept"] = "application/json"

	status, body, err := network.HTTPRequest("POST", baseHost, port, path, "application/json", payload, reqHeaders)
	if err != nil {
		return nil, err
	}
	if status >= 300 {
		snippet := strings.TrimSpace(body)
		if snippet == "" {
			snippet = fmt.Sprintf("status %d", status)
		}
		return nil, fmt.Errorf("backup endpoint %s failed: %s (status %d)", path, snippet, status)
	}
	return []byte(body), nil
}
