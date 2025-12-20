package server

import (
	"sagiri-guard/backend/app/controllers"
	jwtutil "sagiri-guard/backend/app/jwt"
	"sagiri-guard/backend/app/repo"
	"sagiri-guard/backend/app/services"
	"sagiri-guard/backend/app/socket"
	"sagiri-guard/backend/global"
	"sagiri-guard/network"
)

// StartProtocolServer starts the protocol-based TCP server (handled in C threads).
func StartProtocolServer(host string, port int, ctrl *controllers.ProtocolController) error {
	handler := func(client *network.TCPClient, msg *network.ProtocolMessage) {
		global.Logger.Debug().
			Str("device", msg.DeviceID).
			Uint8("type", uint8(msg.Type)).
			Int("payload_len", len(msg.Raw)).
			Msg("protocol frame received")
		ctrl.HandleMessage(client, msg)
	}
	_, err := network.ListenProtocol(host, port, handler)
	if err != nil {
		return err
	}
	global.Logger.Info().Msgf("Protocol server is listening on %s:%d...", host, port)
	return nil
}

// BuildProtocolController constructs controller and hub.
func BuildProtocolController(h *socket.Hub, cmdRepo *repo.AgentCommandRepository, deviceSvc *services.DeviceService, treeSvc *services.FileTreeService, logSvc *services.AgentLogService, backupSvc *services.BackupService, userSvc *services.UserService, signer *jwtutil.Signer) *controllers.ProtocolController {
	return controllers.NewProtocolController(h, cmdRepo, deviceSvc, treeSvc, logSvc, backupSvc, userSvc, signer)
}
