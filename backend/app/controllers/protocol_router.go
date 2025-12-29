package controllers

import (
	"encoding/json"
	"strings"

	"sagiri-guard/backend/app/dto"
	"sagiri-guard/backend/global"
	"sagiri-guard/network"
)

// HandleMessage is invoked per protocol frame from C server threads.
func (c *ProtocolController) HandleMessage(client *network.TCPClient, msg *network.ProtocolMessage) {
	log := global.Logger.With().Str("device", msg.DeviceID).Uint8("type", uint8(msg.Type)).Logger()
	log.Debug().Int("payload_len", len(msg.Raw)).Msg("protocol frame received")

	switch msg.Type {
	case network.MsgLogin:
		deviceID := msg.DeviceID
		if deviceID == "" {
			log.Warn().Msg("login missing device id")
			return
		}
		// If the client sent a token, validate if JWT; otherwise accept as opaque token for auth cache.
		if msg.Token != "" {
			authorized := false
			if c.Signer != nil && looksLikeJWT(msg.Token) {
				if claims, err := c.Signer.Parse(msg.Token); err != nil {
					log.Warn().Err(err).Str("token", msg.Token).Msg("protocol login token parse failed")
				} else if claims.DeviceID != "" && claims.DeviceID != deviceID {
					log.Warn().Str("token_device", claims.DeviceID).Str("frame_device", deviceID).Msg("protocol login device mismatch")
				} else {
					authorized = true
				}
			} else {
				// No signer or non-JWT token: accept as opaque token to authorize this device.
				log.Debug().Str("token", msg.Token).Msg("protocol login token treated as opaque")
				authorized = true
			}
			if authorized {
				c.mu.Lock()
				c.deviceTokens[deviceID] = msg.Token
				c.mu.Unlock()
			}
		}
		log.Info().Msg("protocol connected")
		c.Hub.Register(deviceID, client)
		go c.retryPendingCommands(deviceID)
	case network.MsgCommand:
		c.handleSubCommand(client, msg)
	case network.MsgFileChunk:
		log.Debug().Uint32("offset", msg.ChunkOffset).Uint32("len", msg.ChunkLen).Msg("file chunk received")
		c.handleFileChunk(msg)
	case network.MsgFileDone:
		log.Info().Str("session", msg.SessionID).Msg("file done received")
		c.handleFileDone(msg)
	default:
		// ignore other frames for now
	}
}

func looksLikeJWT(tok string) bool {
	if tok == "" {
		return false
	}
	return strings.Count(tok, ".") >= 2
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
		RawJSON("payload", payload).
		Msg("protocol sub-command received")
	// login and admin_* do not require device token; others require issued token
	if env.Action != "login" &&
		env.Action != "admin_send_command" &&
		env.Action != "admin_list_devices" &&
		env.Action != "admin_list_online" &&
		env.Action != "admin_list_tree" {
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
			if b, er := json.Marshal(data); er == nil {
				global.Logger.Info().RawJSON("payload", b).Msg("admin_list_devices response")
			}
			c.sendAckJSON(client, 200, data)
		}
	case "admin_list_online":
		data := c.handleAdminListOnline(msg.DeviceID)
		if b, er := json.Marshal(data); er == nil {
			global.Logger.Info().RawJSON("payload", b).Msg("admin_list_online response")
		}
		c.sendAckJSON(client, 200, data)
	case "admin_list_tree":
		if data, err := c.handleAdminListTree(payload); err != nil {
			_ = client.SendAck(500, err.Error())
		} else {
			c.sendAckJSON(client, 200, data)
		}
	default:
		// Unknown action: log payload for debug
		global.Logger.Warn().
			Str("device", msg.DeviceID).
			Str("action", env.Action).
			Int("payload_len", len(payload)).
			RawJSON("payload", payload).
			Msg("unknown protocol action")
		_ = client.SendAck(400, "unknown action")
	}
}
