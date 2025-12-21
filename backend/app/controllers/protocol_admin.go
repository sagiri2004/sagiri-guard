package controllers

import (
	"encoding/json"
	"errors"
	"fmt"

	"sagiri-guard/backend/app/dto"
	"sagiri-guard/backend/app/models"
	"sagiri-guard/backend/global"
)

func (c *ProtocolController) handleAdminSendCommand(payload json.RawMessage) (any, error) {
	if c.CmdRepo == nil {
		return nil, errors.New("command repo not available")
	}
	var req dto.AdminSendCommandRequest
	if err := json.Unmarshal(payload, &req); err != nil {
		return nil, err
	}
	if req.DeviceID == "" || req.Command == "" {
		return nil, errors.New("missing device_id or command")
	}
	cmd := models.AgentCommand{
		DeviceID: req.DeviceID,
		Command:  req.Command,
		Kind:     req.Kind,
		Payload:  string(req.Payload),
		Status:   "pending",
	}
	if err := c.CmdRepo.Create(&cmd); err != nil {
		return nil, fmt.Errorf("queue command: %w", err)
	}

	sent := false
	if c.Hub != nil && c.Hub.IsOnline(req.DeviceID) {
		// try to send immediately
		wireReq := dto.CommandRequest{
			DeviceID: req.DeviceID,
			Command:  req.Command,
			Kind:     req.Kind,
			Argument: req.Payload,
		}
		b, err := json.Marshal(wireReq)
		if err == nil {
			if err := c.Hub.Send(req.DeviceID, b); err == nil {
				_ = c.CmdRepo.MarkSent(cmd.ID)
				sent = true
			} else {
				global.Logger.Warn().Err(err).Str("device", req.DeviceID).Msg("admin send command failed, keeping queued")
				_ = c.CmdRepo.UpdateStatus(cmd.ID, "pending", err.Error())
			}
		}
	}

	status := "pending"
	if sent {
		status = "sent"
	}
	return dto.AdminSendCommandResponse{
		ID:     cmd.ID,
		Status: status,
		Sent:   sent,
	}, nil
}
