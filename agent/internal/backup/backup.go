package backup

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"sagiri-guard/agent/internal/protocolclient"
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
	FileID      string `json:"file_id,omitempty"` // file ID từ MonitoredFile
}

type DownloadInitRequest struct {
	FileName string `json:"file_name"`
}

func InitUpload(host string, port int, token string, filePath string, fileID string) (*Session, error) {
	info, err := os.Stat(filePath)
	if err != nil {
		return nil, fmt.Errorf("stat file: %w", err)
	}
	req := UploadInitRequest{
		FileName:    filepath.Base(filePath),
		FileSize:    info.Size(),
		LogicalPath: filePath,
		FileID:      fileID, // Gửi file_id lên backend
	}
	msg, err := protocolclient.SendAction(host, port, state.GetDeviceID(), token, "backup_init_upload", req)
	if err != nil {
		return nil, err
	}
	if msg.Type != network.MsgAck || msg.StatusCode != 200 {
		return nil, fmt.Errorf("init upload failed: code=%d msg=%s", msg.StatusCode, msg.StatusMsg)
	}
	var session Session
	if err := json.Unmarshal([]byte(msg.StatusMsg), &session); err != nil {
		return nil, fmt.Errorf("parse upload session response: %w | raw=%s", err, msg.StatusMsg)
	}
	return &session, nil
}

func InitDownload(host string, port int, token string, fileName string) (*Session, error) {
	req := DownloadInitRequest{FileName: fileName}
	msg, err := protocolclient.SendAction(host, port, state.GetDeviceID(), token, "backup_init_download", req)
	if err != nil {
		return nil, err
	}
	if msg.Type != network.MsgAck || msg.StatusCode != 200 {
		return nil, fmt.Errorf("init download failed: code=%d msg=%s", msg.StatusCode, msg.StatusMsg)
	}
	var session Session
	if err := json.Unmarshal([]byte(msg.StatusMsg), &session); err != nil {
		return nil, fmt.Errorf("parse download session response: %w | raw=%s", err, msg.StatusMsg)
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

	// send file meta
	if err := client.SendFileMeta(session.FileName, uint64(session.FileSize)); err != nil {
		return fmt.Errorf("send file meta: %w", err)
	}
	bufSize := int(session.ChunkSize)
	if bufSize <= 0 {
		bufSize = 512 * 1024
	}
	dataBuf := make([]byte, bufSize)

	var offset uint32 = 0
	for {
		n, err := file.Read(dataBuf)
		if n > 0 {
			if err := client.SendFileChunkWithSession(session.SessionID, session.Token, offset, dataBuf[:n]); err != nil {
				return fmt.Errorf("send chunk: %w", err)
			}
			offset += uint32(n)
		}
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("read file: %w", err)
		}
	}
	if err := client.SendFileDoneWithSession(session.SessionID, session.Token); err != nil {
		return fmt.Errorf("send file done: %w", err)
	}
	global.Logger.Info().Msgf("Uploaded %s (%d bytes)", session.FileName, offset)
	return nil
}

func DownloadFile(session *Session, destPath string) error {
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

	// Start download by sending action backup_download_start with session_id/token
	payload := map[string]interface{}{
		"action":     "backup_download_start",
		"session_id": session.SessionID,
		"token":      session.Token,
		"offset":     session.Offset,
	}
	body, _ := json.Marshal(payload)
	if err := client.SendCommand(body); err != nil {
		return fmt.Errorf("send download start: %w", err)
	}

	// Expect meta, chunks, done
	for {
		msg, err := client.RecvProtocolMessage()
		if err != nil {
			return err
		}
		switch msg.Type {
		case network.MsgFileMeta:
			// seek to start
			if _, err := file.Seek(0, io.SeekStart); err != nil {
				return err
			}
			session.FileName = msg.FileName
			session.FileSize = int64(msg.FileSize)
		case network.MsgFileChunk:
			if _, err := file.Write(msg.ChunkData); err != nil {
				return err
			}
			session.Offset += int64(len(msg.ChunkData))
		case network.MsgFileDone:
			global.Logger.Info().Msgf("Downloaded %s (%d bytes)", session.FileName, session.Offset)
			return nil
		case network.MsgAck, network.MsgError:
			return fmt.Errorf("download failed: code=%d msg=%s", msg.StatusCode, msg.StatusMsg)
		default:
			continue
		}
	}
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
