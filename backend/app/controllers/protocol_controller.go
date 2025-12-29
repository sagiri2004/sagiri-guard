package controllers

import (
	"encoding/json"
	"sync"

	"sagiri-guard/backend/app/dto"
	jwtutil "sagiri-guard/backend/app/jwt"
	"sagiri-guard/backend/app/repo"
	"sagiri-guard/backend/app/services"
	"sagiri-guard/backend/app/socket"
	"sagiri-guard/backend/global"
	"sagiri-guard/network"
)

// ProtocolController handles protocol-level messages (login, command, etc.)
type ProtocolController struct {
	Hub     *socket.Hub
	CmdRepo *repo.AgentCommandRepository
	Devices *services.DeviceService
	Tree    *services.FileTreeService
	Logs    *services.AgentLogService
	Backup  *services.BackupService
	Users   *services.UserService
	Signer  *jwtutil.Signer

	mu             sync.Mutex
	activeUpload   map[string]*backupSessionCtx // sessionID -> ctx
	deviceTokens   map[string]string            // deviceID -> token
	activeDownload map[string]*backupSessionCtx
}

type backupSessionCtx struct {
	id    string
	token string
}

func NewProtocolController(h *socket.Hub, r *repo.AgentCommandRepository, devices *services.DeviceService, tree *services.FileTreeService, logs *services.AgentLogService, backup *services.BackupService, users *services.UserService, signer *jwtutil.Signer) *ProtocolController {
	return &ProtocolController{
		Hub:            h,
		CmdRepo:        r,
		Devices:        devices,
		Tree:           tree,
		Logs:           logs,
		Backup:         backup,
		Users:          users,
		Signer:         signer,
		activeUpload:   make(map[string]*backupSessionCtx),
		activeDownload: make(map[string]*backupSessionCtx),
		deviceTokens:   make(map[string]string),
	}
}

func (c *ProtocolController) sendAckJSON(client *network.TCPClient, code int, payload any) {
	if payload == nil {
		if err := client.SendAck(uint16(code), ""); err != nil {
			global.Logger.Warn().Err(err).Msg("send ack failed (empty payload)")
		}
		return
	}
	b, err := json.Marshal(payload)
	if err != nil {
		if er := client.SendAck(uint16(code), err.Error()); er != nil {
			global.Logger.Warn().Err(er).Msg("send ack failed (marshal error)")
		}
		return
	}
	// status_msg size limited by protocol (PROTOCOL_MAX_MESSAGE)
	// keep some headroom under PROTOCOL_MAX_MESSAGE (4KB)
	const maxLen = 3800
	if len(b) > maxLen {
		global.Logger.Warn().Int("len", len(b)).Msg("ack payload too large, sending error")
		if err := client.SendAck(uint16(413), "response too large"); err != nil {
			global.Logger.Warn().Err(err).Msg("send ack failed (response too large)")
		}
		return
	}
	global.Logger.Debug().Int("len", len(b)).Int("code", code).Msg("send ack json")
	if err := client.SendAck(uint16(code), string(b)); err != nil {
		global.Logger.Warn().Err(err).Int("payload_len", len(b)).Msg("send ack failed (likely client disconnected)")
	}
}

func (c *ProtocolController) isAuthorized(deviceID string) bool {
	if deviceID == "" {
		return false
	}
	c.mu.Lock()
	tok := c.deviceTokens[deviceID]
	c.mu.Unlock()
	return tok != ""
}

// retryPendingCommands sends queued commands when a device logs in.
func (c *ProtocolController) retryPendingCommands(deviceID string) {
	if c.CmdRepo == nil {
		return
	}
	cmds, err := c.CmdRepo.ListByDevice(deviceID, false)
	if err != nil {
		global.Logger.Error().Err(err).Str("device", deviceID).Msg("list pending commands failed")
		return
	}
	for _, cmd := range cmds {
		req := dto.CommandRequest{
			DeviceID: deviceID,
			Command:  cmd.Command,
			Kind:     cmd.Kind,
		}
		if cmd.Payload != "" {
			req.Argument = json.RawMessage(cmd.Payload)
		}
		payload, err := json.Marshal(req)
		if err != nil {
			_ = c.CmdRepo.UpdateStatus(cmd.ID, "failed", err.Error())
			continue
		}
		if err := c.Hub.Send(deviceID, payload); err != nil {
			_ = c.CmdRepo.UpdateStatus(cmd.ID, "failed", err.Error())
			break
		}
		_ = c.CmdRepo.MarkSent(cmd.ID)
	}
}

// HandleDisconnect is called when a client disconnects
func (c *ProtocolController) HandleDisconnect(client *network.TCPClient, deviceID string) {
	if deviceID == "" {
		return
	}
	
	global.Logger.Info().
		Str("device", deviceID).
		Msg("handling client disconnect, cleaning up Hub")
	
	// Cleanup Hub registration
	c.Hub.Unregister(deviceID, client)
	
	// Cleanup device tokens
	c.mu.Lock()
	delete(c.deviceTokens, deviceID)
	
	// Cleanup active upload/download sessions for this device
	// Note: We need to iterate to find sessions belonging to this device
	// Since sessions are keyed by sessionID, we'd need deviceID in context
	// For now, we just clean up tokens
	c.mu.Unlock()
	
	global.Logger.Debug().
		Str("device", deviceID).
		Msg("client disconnect cleanup completed")
}
