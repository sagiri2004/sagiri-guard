package controllers

import (
	"encoding/json"
	"fmt"
	"sagiri-guard/backend/app/dto"
	"sagiri-guard/backend/app/repo"
	"sagiri-guard/backend/app/socket"
	"sagiri-guard/backend/global"
	"sagiri-guard/network"
)

type SocketController struct {
	Hub      *socket.Hub
	CmdRepo  *repo.AgentCommandRepository
}

func NewSocketController(h *socket.Hub, r *repo.AgentCommandRepository) *SocketController {
	return &SocketController{Hub: h, CmdRepo: r}
}

// HandleClient is invoked per accepted TCP client connection.
func (c *SocketController) HandleClient(client *network.TCPClient) {
	defer client.Close()
	buf := make([]byte, 4096)
	deviceID := ""
	if headers, remaining, err := network.ReadTokenHeaders(client); err == nil {
		deviceID = headers["x-device-id"]
		global.Logger.Info().Str("device", deviceID).Msg("socket connected")
		if deviceID != "" {
			c.Hub.Register(deviceID, client)
			// Sau khi agent connect, retry các command pending/failed trong queue.
			go c.retryPendingCommands(deviceID)
		}
		if len(remaining) > 0 {
			global.Logger.Info().Msgf("initial data: %s", string(remaining))
		}
	}
	for {
		n, err := client.Read(buf)
		if err != nil {
			break
		}
		if n > 0 {
			fmt.Printf("Client %s: %s\n", deviceID, string(buf[:n]))
		}
	}
	if deviceID != "" {
		c.Hub.Unregister(deviceID, client)
	}
	global.Logger.Info().Str("device", deviceID).Msg("socket disconnected")
}

// retryPendingCommands gửi lại các command đang pending/failed cho device vừa online.
func (c *SocketController) retryPendingCommands(deviceID string) {
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
		payload = append(payload, '\n')
		if err := c.Hub.Send(deviceID, payload); err != nil {
			// device lại offline hoặc lỗi gửi; dừng để retry lần sau.
			_ = c.CmdRepo.UpdateStatus(cmd.ID, "failed", err.Error())
			break
		}
		_ = c.CmdRepo.MarkSent(cmd.ID)
	}
}
