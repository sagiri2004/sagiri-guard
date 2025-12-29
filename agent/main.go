package main

import (
	"flag"
	"fmt"
	"os"
	"os/signal"
	"sagiri-guard/agent/internal/auth"
	"sagiri-guard/agent/internal/config"
	"sagiri-guard/agent/internal/connection"
	"sagiri-guard/agent/internal/db"
	"sagiri-guard/agent/internal/firewall"
	"sagiri-guard/agent/internal/logger"
	"sagiri-guard/agent/internal/privilege"
	"sagiri-guard/agent/internal/service"
	"sagiri-guard/agent/internal/state"
	"sagiri-guard/network"
	"strings"
	"syscall"
	"time"
)

func main() {
	var (
		cfgPath    = flag.String("config", "config/config.yaml", "Path to configuration file")
		maxRetries = flag.Int("max-retries", 10, "Maximum retry attempts for backend connection")
		retryDelay = flag.Duration("retry-delay", 1*time.Second, "Base delay between retry attempts")
		elevate    = flag.Bool("elevate", false, "Attempt to elevate to admin (disable by default for go run)")
	)
	flag.Parse()

	// osquery thường cần quyền root để đọc system_info ổn định
	if os.Geteuid() != 0 {
		fmt.Println("Agent requires root to collect stable machine UUID. Please run with sudo.")
		return
	}

	if err := network.Init(); err != nil {
		logger.Error("Cannot initialize network library:", err)
		return
	}
	defer network.Cleanup()

	// init config values for token path, etc.
	_ = config.Init()
	// init logger (zerolog to file if provided)
	cfgVals := config.Get()
	_ = logger.Init(cfgVals.LogPath)

	// init local sqlite db
	adb, dberr := db.Init(cfgVals.DBPath)
	if dberr != nil {
		logger.Error("Cannot open SQLite:", dberr)
		return
	}
	if err := adb.AutoMigrate(&db.Token{}, &db.MonitoredFile{}); err != nil {
		logger.Error("Cannot migrate SQLite:", err)
		return
	}

	// Elevate on Windows (optional)
	if *elevate && !privilege.IsElevated() {
		if relaunched, err := privilege.AttemptElevate(); err != nil {
			logger.Error("Cannot request admin privileges:", err)
		} else if relaunched {
			return
		}
	}

	token, deviceID, err := loadOrPromptToken()
	if err != nil {
		logger.Error("Missing login information:", err)
		return
	}
	auth.SetCurrentToken(token)
	state.SetToken(token)
	if deviceID != "" {
		state.SetDeviceID(deviceID)
	}

	var uuid string
	for {
		uuid, err = service.BootstrapDevice(token, deviceID)
		if err == service.ErrUnauthorized {
			logger.Warn("Current token is invalid, requesting login again")
			if clearErr := auth.ClearToken(); clearErr != nil {
				logger.Warn("Cannot clear old token: %v", clearErr)
			}
			token, deviceID, err = promptLogin()
			if err != nil {
				logger.Error("Login failed:", err)
				return
			}
			auth.SetCurrentToken(token)
			state.SetToken(token)
			if deviceID != "" {
				state.SetDeviceID(deviceID)
			}
			continue
		}
		if err != nil {
			logger.Error("Device initialization failed:", err)
			return
		}
		break
	}
	state.SetDeviceID(uuid)
	deviceID = uuid

	_ = cfgPath // viper reads from default config path set in Init()

	logger.Infof("Agent will retry up to %d times with a base delay of %v...", *maxRetries, *retryDelay)

	// Create ConnectionManager (single persistent connection)
	addr := cfgVals
	connMgr := connection.New(addr.BackendHost, addr.BackendPort, uuid, token)

	// Connect with retry logic
	if err := connMgr.Connect(*maxRetries, *retryDelay); err != nil {
		logger.Error("Failed to establish connection:", err)
		return
	}
	defer connMgr.Close()

	// Start background receive loop
	connMgr.StartReceiveLoop()

	// Start directory tree sync loop (using ConnectionManager)
	service.StartFileTreeSyncLoop(connMgr)

	// socket running; setup cleanup on shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	// Cleanup function: remove website blocking khi agent shutdown
	defer func() {
		logger.Info("Agent shutting down, cleaning up website blocking...")
		hostsMgr := firewall.GetHostsManager()
		if hostsMgr != nil && hostsMgr.IsEnabled() {
			if err := hostsMgr.SetEnabled(false); err != nil {
				logger.Errorf("Failed to cleanup website blocking: %v", err)
			} else {
				logger.Info("Website blocking cleaned up successfully")
			}
		}
	}()

	// Wait for signal
	<-sigChan
	logger.Info("Shutdown signal received, exiting...")
}

func loadOrPromptToken() (string, string, error) {
	if existing, err := auth.LoadToken(); err == nil && strings.TrimSpace(existing) != "" {
		return strings.TrimSpace(existing), state.GetDeviceID(), nil
	}
	return promptLogin()
}

func promptLogin() (string, string, error) {
	u, p, err := auth.PromptCredentials()
	if err != nil {
		return "", "", err
	}
	return service.Login(u, p)
}
