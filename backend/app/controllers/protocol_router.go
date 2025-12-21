package controllers

import (
	"encoding/json"

	"sagiri-guard/backend/app/dto"
	"sagiri-guard/backend/global"
	"sagiri-guard/network"
)

// HandleMessage is invoked per protocol frame from C server threads.
func (c *ProtocolController) HandleMessage(client *network.TCPClient, msg *network.ProtocolMessage) {
	switch msg.Type {
	case network.MsgLogin:
		deviceID := msg.DeviceID
		if deviceID == "" {
			global.Logger.Warn().Msg("login missing device id")
			return
		}
		// If the client sent a token, validate it and hydrate authorization cache
		if msg.Token != "" && c.Signer != nil {
			if claims, err := c.Signer.Parse(msg.Token); err != nil {
				global.Logger.Warn().Err(err).Str("device", deviceID).Msg("protocol login token parse failed")
			} else if claims.DeviceID != "" && claims.DeviceID != deviceID {
				global.Logger.Warn().Str("token_device", claims.DeviceID).Str("frame_device", deviceID).Msg("protocol login device mismatch")
			} else {
				c.mu.Lock()
				c.deviceTokens[deviceID] = msg.Token
				c.mu.Unlock()
			}
		}
		global.Logger.Info().Str("device", deviceID).Msg("protocol connected")
		c.Hub.Register(deviceID, client)
		go c.retryPendingCommands(deviceID)
	case network.MsgCommand:
		// There are two uses of MsgCommand:
		// 1) backend -> agent (handled in Hub.Send, not here)
		// 2) agent -> backend as sub-command JSON (handled here)
		c.handleSubCommand(client, msg)
	case network.MsgFileChunk:
		c.handleFileChunk(msg)
	case network.MsgFileDone:
		c.handleFileDone(msg)
	default:
		// ignore other frames for now
	}
}

func (c *ProtocolController) handleSubCommand(client *network.TCPClient, msg *network.ProtocolMessage) {
	if len(msg.CommandJSON) == 0 {
		_ = client.SendAck(400, "empty command payload")
		return
	}
	var env dto.ProtocolSubCommandEnvelope
	if err := json.Unmarshal(msg.CommandJSON, &env); err != nil {
		_ = client.SendAck(400, "invalid command json")
		return
	}
	payload := env.Data
	// debug log incoming sub-command
	global.Logger.Debug().
		Str("device", msg.DeviceID).
		Str("action", env.Action).
		Int("payload_len", len(payload)).
		Msg("protocol sub-command received")
	// login and admin_* do not require device token; others require issued token
	if env.Action != "login" && env.Action != "admin_send_command" && env.Action != "admin_list_devices" && env.Action != "admin_list_online" {
		if !c.isAuthorized(msg.DeviceID) {
			_ = client.SendAck(401, "unauthorized")
			return
		}
	}
	switch env.Action {
	case "ping":
		_ = client.SendAck(200, "pong")
	case "login":
		if data, err := c.handleLogin(msg.DeviceID, payload); err != nil {
			_ = client.SendAck(401, err.Error())
		} else {
			c.sendAckJSON(client, 200, data)
		}
	case "device_register":
		if err := c.handleDeviceRegister(msg.DeviceID, payload); err != nil {
			_ = client.SendAck(500, err.Error())
		} else {
			_ = client.SendAck(200, "device registered")
		}
	case "filetree_sync":
		if err := c.handleFileTreeSync(msg.DeviceID, payload); err != nil {
			_ = client.SendAck(500, err.Error())
		} else {
			_ = client.SendAck(200, "filetree synced")
		}
	case "agent_log":
		if err := c.handleAgentLog(msg.DeviceID, payload); err != nil {
			_ = client.SendAck(500, err.Error())
		} else {
			_ = client.SendAck(200, "log stored")
		}
	case "backup_init_upload":
		if data, err := c.handleBackupInitUpload(msg.DeviceID, payload); err != nil {
			_ = client.SendAck(500, err.Error())
		} else {
			c.sendAckJSON(client, 200, data)
		}
	case "backup_init_download":
		if data, err := c.handleBackupInitDownload(msg.DeviceID, payload); err != nil {
			_ = client.SendAck(500, err.Error())
		} else {
			c.sendAckJSON(client, 200, data)
		}
	case "backup_download_start":
		if err := c.handleBackupDownloadStart(client, msg.DeviceID, payload); err != nil {
			_ = client.SendAck(500, err.Error())
		}
	case "admin_send_command":
		if data, err := c.handleAdminSendCommand(payload); err != nil {
			_ = client.SendAck(500, err.Error())
		} else {
			c.sendAckJSON(client, 200, data)
		}
	case "admin_list_devices":
		if data, err := c.handleAdminListDevices(); err != nil {
			_ = client.SendAck(500, err.Error())
		} else {
			c.sendAckJSON(client, 200, data)
		}
	case "admin_list_online":
		data := c.handleAdminListOnline()
		c.sendAckJSON(client, 200, data)
	default:
		_ = client.SendAck(400, "unknown action")
	}
}
