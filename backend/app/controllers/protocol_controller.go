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
			global.Logger.Error().Err(err).Msg("send ack failed")
		}
		return
	}
	b, err := json.Marshal(payload)
	if err != nil {
		if er := client.SendAck(uint16(code), err.Error()); er != nil {
			global.Logger.Error().Err(er).Msg("send ack failed")
		}
		return
	}
	// status_msg size limited by protocol (PROTOCOL_MAX_MESSAGE)
	const maxLen = 1024
	if len(b) > maxLen {
		global.Logger.Warn().Int("len", len(b)).Msg("ack payload too large, sending error")
		if err := client.SendAck(uint16(413), "response too large"); err != nil {
			global.Logger.Error().Err(err).Msg("send ack failed")
		}
		return
	}
	if err := client.SendAck(uint16(code), string(b)); err != nil {
		global.Logger.Error().Err(err).Msg("send ack failed")
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
