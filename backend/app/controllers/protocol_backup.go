package controllers

import (
	"encoding/json"
	"errors"
	"io"
	"os"

	"sagiri-guard/backend/app/dto"
	"sagiri-guard/backend/global"
	"sagiri-guard/network"
)

func (c *ProtocolController) handleBackupInitUpload(deviceID string, payload json.RawMessage) (any, error) {
	if !c.isAuthorized(deviceID) {
		return nil, errors.New("unauthorized")
	}
	if c.Backup == nil {
		return nil, nil
	}
	if deviceID == "" {
		return nil, errors.New("missing device id")
	}
	var req dto.BackupUploadInitRequest
	if err := json.Unmarshal(payload, &req); err != nil {
		return nil, err
	}
	resp, err := c.Backup.PrepareUpload(deviceID, req)
	if err != nil {
		return nil, err
	}
	// Track active upload for this device (single session)
	c.mu.Lock()
	c.activeUpload[resp.SessionID] = &backupSessionCtx{id: resp.SessionID, token: resp.Token}
	c.mu.Unlock()
	return resp, nil
}

func (c *ProtocolController) handleBackupInitDownload(deviceID string, payload json.RawMessage) (any, error) {
	if !c.isAuthorized(deviceID) {
		return nil, errors.New("unauthorized")
	}
	if c.Backup == nil {
		return nil, nil
	}
	if deviceID == "" {
		return nil, errors.New("missing device id")
	}
	var req dto.BackupDownloadInitRequest
	if err := json.Unmarshal(payload, &req); err != nil {
		return nil, err
	}
	resp, err := c.Backup.PrepareDownload(deviceID, req)
	if err != nil {
		return nil, err
	}
	// For download, we do not store state here; chunks will be sent from backend to agent (not implemented)
	return resp, nil
}

func (c *ProtocolController) handleBackupDownloadStart(client *network.TCPClient, deviceID string, payload json.RawMessage) error {
	if !c.isAuthorized(deviceID) {
		return errors.New("unauthorized")
	}
	if c.Backup == nil {
		return errors.New("backup disabled")
	}
	var req dto.BackupDownloadStartRequest
	if err := json.Unmarshal(payload, &req); err != nil {
		return err
	}
	if req.SessionID == "" || req.Token == "" {
		return errors.New("missing session id or token")
	}
	sess, err := c.Backup.ValidateSession(req.SessionID, req.Token, dto.DirectionDownload)
	if err != nil {
		return err
	}
	// Send meta
	if err := client.SendFileMeta(sess.FileName, uint64(sess.FileSize)); err != nil {
		return err
	}
	f, err := os.Open(sess.FinalPath)
	if err != nil {
		return err
	}
	defer f.Close()
	const chunkSize = 512 * 1024
	buf := make([]byte, chunkSize)
	var offset uint32 = req.Offset
	if req.Offset > 0 {
		if _, err := f.Seek(int64(req.Offset), io.SeekStart); err != nil {
			return err
		}
	}
	for {
		n, er := f.Read(buf)
		if n > 0 {
			if err := client.SendFileChunkWithSession(req.SessionID, req.Token, offset, buf[:n]); err != nil {
				return err
			}
			offset += uint32(n)
		}
		if er == io.EOF {
			break
		}
		if er != nil {
			return er
		}
	}
	if err := client.SendFileDoneWithSession(req.SessionID, req.Token); err != nil {
		return err
	}
	return nil
}

func (c *ProtocolController) handleFileChunk(msg *network.ProtocolMessage) {
	if !c.isAuthorized(msg.DeviceID) {
		return
	}
	if c.Backup == nil {
		return
	}
	if msg.SessionID == "" || msg.Token == "" {
		return
	}
	c.mu.Lock()
	ctx := c.activeUpload[msg.SessionID]
	c.mu.Unlock()
	if ctx == nil || ctx.token != msg.Token {
		return
	}
	sess, err := c.Backup.ValidateSession(ctx.id, ctx.token, dto.DirectionUpload)
	if err != nil {
		global.Logger.Warn().Err(err).Str("device", msg.DeviceID).Msg("invalid upload session")
		return
	}
	if msg.ChunkData == nil || msg.ChunkLen == 0 {
		return
	}
	f, err := os.OpenFile(sess.TempPath, os.O_WRONLY, 0o644)
	if err != nil {
		global.Logger.Error().Err(err).Msg("open temp file failed")
		return
	}
	if _, err := f.Seek(int64(msg.ChunkOffset), io.SeekStart); err != nil {
		_ = f.Close()
		global.Logger.Error().Err(err).Msg("seek temp file failed")
		return
	}
	if _, err := f.Write(msg.ChunkData); err != nil {
		_ = f.Close()
		global.Logger.Error().Err(err).Msg("write chunk failed")
		return
	}
	_ = f.Close()
	if _, err := c.Backup.Advance(ctx.id, int64(msg.ChunkLen)); err != nil {
		global.Logger.Error().Err(err).Msg("advance session failed")
	}
}

func (c *ProtocolController) handleFileDone(msg *network.ProtocolMessage) {
	if !c.isAuthorized(msg.DeviceID) {
		return
	}
	if c.Backup == nil {
		return
	}
	if msg.SessionID == "" || msg.Token == "" {
		return
	}
	c.mu.Lock()
	ctx := c.activeUpload[msg.SessionID]
	c.mu.Unlock()
	if ctx == nil || ctx.token != msg.Token {
		return
	}
	sess, err := c.Backup.ValidateSession(ctx.id, ctx.token, dto.DirectionUpload)
	if err != nil {
		global.Logger.Warn().Err(err).Str("device", msg.DeviceID).Msg("invalid upload session")
		return
	}
	if err := c.Backup.MarkCompleted(sess.ID); err != nil {
		global.Logger.Error().Err(err).Msg("mark upload complete failed")
		return
	}
	if err := c.Backup.FinalizeUpload(sess.ID); err != nil {
		global.Logger.Error().Err(err).Str("session", sess.ID).Msg("finalize upload failed (rename .part/.path)")
		return
	}
	global.Logger.Info().
		Str("device", msg.DeviceID).
		Str("session", sess.ID).
		Str("file", sess.FinalPath).
		Msg("backup upload finalized")
	c.mu.Lock()
	delete(c.activeUpload, msg.SessionID)
	c.mu.Unlock()
}
