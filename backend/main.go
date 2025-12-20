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

	// Start protocol server (replaces HTTP + TCP)
	if err := server.StartProtocolServer(app.Cfg.TCP.Host, app.Cfg.TCP.Port, app.Protocol); err != nil {
		global.Logger.Error().Msgf("Cannot start protocol server: %v", err)
		return
	}
	// global.Logger.Info().Msgf("Protocol server is listening on %s:%d...", app.Cfg.TCP.Host, app.Cfg.TCP.Port)

	select {}
}
