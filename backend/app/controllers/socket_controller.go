package controllers

import (
	"fmt"
	"sagiri-guard/backend/app/socket"
	"sagiri-guard/backend/global"
	"sagiri-guard/network"
)

type SocketController struct{ Hub *socket.Hub }

func NewSocketController(h *socket.Hub) *SocketController { return &SocketController{Hub: h} }

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
