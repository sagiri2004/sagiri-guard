package controllers

import (
	"encoding/binary"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"

	"sagiri-guard/backend/app/dto"
	"sagiri-guard/backend/app/middleware"
	"sagiri-guard/backend/app/services"
	"sagiri-guard/backend/global"
	"sagiri-guard/network"
)

type BackupController struct {
	Backup *services.BackupService
}

func NewBackupController(backup *services.BackupService) *BackupController {
	return &BackupController{Backup: backup}
}

func (c *BackupController) InitUpload(w http.ResponseWriter, r *http.Request) {
	var req dto.BackupUploadInitRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid payload")
		return
	}
	deviceID := deviceIDFromRequest(r)
	resp, err := c.Backup.PrepareUpload(deviceID, req)
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, resp)
}

func (c *BackupController) InitDownload(w http.ResponseWriter, r *http.Request) {
	var req dto.BackupDownloadInitRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid payload")
		return
	}
	deviceID := deviceIDFromRequest(r)
	resp, err := c.Backup.PrepareDownload(deviceID, req)
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, resp)
}

func (c *BackupController) SessionStatus(w http.ResponseWriter, r *http.Request) {
	id := r.URL.Query().Get("id")
	if id == "" {
		writeJSONError(w, http.StatusBadRequest, "missing session id")
		return
	}
	resp, err := c.Backup.GetSession(id)
	if err != nil {
		writeJSONError(w, http.StatusNotFound, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, resp)
}

// HandleTransfer runs on the raw TCP backup port.
func (c *BackupController) HandleTransfer(client *network.TCPClient) {
	defer client.Close()
	headers, _, err := network.ReadTokenHeaders(client)
	if err != nil {
		global.Logger.Error().Err(err).Msg("backup headers read failed")
		return
	}
	sessionID := headers["session-id"]
	token := headers["session-token"]
	action := strings.ToLower(headers["action"])
	if sessionID == "" || token == "" || action == "" {
		global.Logger.Warn().Msg("backup missing headers")
		return
	}
	var direction dto.TransferDirection
	switch action {
	case string(dto.DirectionUpload):
		direction = dto.DirectionUpload
	case string(dto.DirectionDownload):
		direction = dto.DirectionDownload
	default:
		global.Logger.Warn().Str("action", action).Msg("unknown backup action")
		return
	}
	session, err := c.Backup.ValidateSession(sessionID, token, direction)
	if err != nil {
		global.Logger.Error().Err(err).Str("session", sessionID).Msg("session validation failed")
		return
	}
	switch direction {
	case dto.DirectionUpload:
		c.streamUpload(client, session)
	case dto.DirectionDownload:
		offset := parseOffset(headers["client-offset"])
		c.streamDownload(client, session, offset)
	}
}

func (c *BackupController) streamUpload(client *network.TCPClient, session *services.BackupSession) {
	file, err := os.OpenFile(session.TempPath, os.O_WRONLY|os.O_CREATE, 0o600)
	if err != nil {
		global.Logger.Error().Err(err).Msg("open temp file failed")
		return
	}
	closed := false
	closeFile := func() {
		if !closed {
			_ = file.Close()
			closed = true
		}
	}
	defer closeFile()

	offset := session.BytesDone
	if _, err := file.Seek(offset, io.SeekStart); err != nil {
		global.Logger.Error().Err(err).Msg("seek temp file failed")
		return
	}
	ack := make([]byte, 8)
	binary.BigEndian.PutUint64(ack, uint64(offset))
	if err := writeAll(client, ack); err != nil {
		return
	}

	header := make([]byte, 4)
	maxChunk := int(c.Backup.ChunkSize())
	if maxChunk <= 0 {
		maxChunk = 512 * 1024
	}
	chunkBuf := make([]byte, maxChunk)

	for {
		if _, err := io.ReadFull(client, header); err != nil {
			if !errors.Is(err, io.EOF) {
				global.Logger.Error().Err(err).Msg("read chunk header failed")
			}
			return
		}
		chunkLen := binary.BigEndian.Uint32(header)
		if chunkLen == 0 {
			if err := file.Sync(); err != nil {
				global.Logger.Warn().Err(err).Msg("sync temp file failed")
			}
			closeFile()
			if err := c.Backup.FinalizeUpload(session.ID); err != nil {
				global.Logger.Error().Err(err).Msg("finalize upload failed")
				return
			}
			finalAck := make([]byte, 8)
			binary.BigEndian.PutUint64(finalAck, uint64(session.FileSize))
			_ = writeAll(client, finalAck)
			return
		}
		if int(chunkLen) > len(chunkBuf) {
			global.Logger.Warn().Uint32("chunkLen", chunkLen).Msg("chunk too large")
			return
		}
		buf := chunkBuf[:int(chunkLen)]
		if _, err := io.ReadFull(client, buf); err != nil {
			global.Logger.Error().Err(err).Msg("read chunk failed")
			return
		}
		if _, err := file.Write(buf); err != nil {
			global.Logger.Error().Err(err).Msg("write chunk failed")
			return
		}
		if _, err := c.Backup.Advance(session.ID, int64(chunkLen)); err != nil {
			global.Logger.Error().Err(err).Msg("advance session failed")
			return
		}
	}
}

func (c *BackupController) streamDownload(client *network.TCPClient, session *services.BackupSession, clientOffset int64) {
	file, err := os.Open(session.FinalPath)
	if err != nil {
		global.Logger.Error().Err(err).Msg("open download file failed")
		return
	}
	defer file.Close()

	total := session.FileSize
	if clientOffset < 0 {
		clientOffset = 0
	}
	if clientOffset > total {
		clientOffset = total
	}
	if _, err := file.Seek(clientOffset, io.SeekStart); err != nil {
		global.Logger.Error().Err(err).Msg("seek download file failed")
		return
	}

	meta := make([]byte, 16)
	binary.BigEndian.PutUint64(meta[:8], uint64(total))
	binary.BigEndian.PutUint64(meta[8:], uint64(clientOffset))
	if err := writeAll(client, meta); err != nil {
		return
	}

	header := make([]byte, 4)
	maxChunk := int(c.Backup.ChunkSize())
	if maxChunk <= 0 {
		maxChunk = 512 * 1024
	}
	chunkBuf := make([]byte, maxChunk)
	for {
		n, err := file.Read(chunkBuf)
		if err == io.EOF {
			break
		}
		if err != nil {
			global.Logger.Error().Err(err).Msg("read download chunk failed")
			return
		}
		binary.BigEndian.PutUint32(header, uint32(n))
		if err := writeAll(client, header); err != nil {
			return
		}
		if err := writeAll(client, chunkBuf[:n]); err != nil {
			return
		}
		clientOffset += int64(n)
		_, _ = c.Backup.Advance(session.ID, int64(n))
	}
	// terminate stream
	header = make([]byte, 4)
	_ = writeAll(client, header)
	_ = c.Backup.MarkCompleted(session.ID)
}

func writeJSON(w http.ResponseWriter, status int, payload interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func writeJSONError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

func deviceIDFromRequest(r *http.Request) string {
	if claims := middleware.GetClaims(r.Context()); claims != nil && claims.DeviceID != "" {
		return claims.DeviceID
	}
	return r.Header.Get("X-Device-ID")
}

func parseOffset(raw string) int64 {
	if raw == "" {
		return 0
	}
	if v, err := strconv.ParseInt(raw, 10, 64); err == nil {
		return v
	}
	return 0
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
