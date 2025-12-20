package main

import (
	"flag"
	"sagiri-guard/backend/global"
	"sagiri-guard/backend/initialize"
	"sagiri-guard/backend/server"
	"sagiri-guard/network"
)

func main() {
	var (
		cfgPath = flag.String("config", "config/config.yaml", "Path to configuration file")
	)
	flag.Parse()

	if err := network.Init(); err != nil {
		global.Logger.Error().Msgf("Cannot initialize network library: %v", err)
		return
	}
	defer network.Cleanup()

	app, err := initialize.Build(*cfgPath)
	if err != nil {
		global.Logger.Error().Msgf("Application initialization failed: %v", err)
		return
	}

	if err := server.StartHTTPServerC(app.Cfg.HTTP.Host, app.Cfg.HTTP.Port, app.Router); err != nil {
		global.Logger.Error().Msgf("Cannot start HTTP server: %v", err)
		return
	}
	global.Logger.Info().Msgf("HTTP server is listening on %s:%d...", app.Cfg.HTTP.Host, app.Cfg.HTTP.Port)

	go func() {
		if err := server.StartTCPServer(app.Cfg.TCP.Host, app.Cfg.TCP.Port, app.Socket.HandleClient); err != nil {
			global.Logger.Error().Msgf("TCP server stopped with error: %v", err)
		}
	}()
	global.Logger.Info().Msgf("TCP server is listening on %s:%d...", app.Cfg.TCP.Host, app.Cfg.TCP.Port)

	if app.Backup != nil {
		go func() {
			if err := server.StartTCPServer(app.Cfg.Backup.TCP.Host, app.Cfg.Backup.TCP.Port, app.Backup.HandleTransfer); err != nil {
				global.Logger.Error().Msgf("Backup TCP server stopped with error: %v", err)
			}
		}()
		global.Logger.Info().Msgf("Backup TCP server is listening on %s:%d...", app.Cfg.Backup.TCP.Host, app.Cfg.Backup.TCP.Port)
	}

	select {}
}
