package main

import (
	"flag"
	"os"
	"os/signal"
	"syscall"
	"sagiri-guard/agent/internal/auth"
	"sagiri-guard/agent/internal/config"
	"sagiri-guard/agent/internal/db"
	"sagiri-guard/agent/internal/firewall"
	"sagiri-guard/agent/internal/logger"
	"sagiri-guard/agent/internal/privilege"
	"sagiri-guard/agent/internal/service"
	"sagiri-guard/agent/internal/socket"
	"sagiri-guard/agent/internal/state"
	"sagiri-guard/backend/global"
	"sagiri-guard/network"
	"strings"
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

	token, err := loadOrPromptToken()
	if err != nil {
		logger.Error("Missing login information:", err)
		return
	}
	auth.SetCurrentToken(token)
	state.SetToken(token)

	var uuid string
	for {
		uuid, err = service.BootstrapDevice(token)
		if err == service.ErrUnauthorized {
			logger.Warn("Current token is invalid, requesting login again")
			if clearErr := auth.ClearToken(); clearErr != nil {
				logger.Warn("Cannot clear old token: %v", clearErr)
			}
			token, err = promptLogin()
			if err != nil {
				logger.Error("Login failed:", err)
				return
			}
			auth.SetCurrentToken(token)
			state.SetToken(token)
			continue
		}
		if err != nil {
			logger.Error("Device initialization failed:", err)
			return
		}
		break
	}
	state.SetDeviceID(uuid)

	_ = cfgPath // viper reads from default config path set in Init()

	logger.Infof("Agent will retry up to %d times with a base delay of %v...", *maxRetries, *retryDelay)

	// start directory tree sync loop (agent.db -> backend)
	service.StartFileTreeSyncLoop(token, uuid)

	// use TCP from config
	addr := cfgVals
	// Start TCP client with headers
	headers := map[string]string{
		"Authorization": logger.Sprintf("Bearer %s", token),
		"X-Device-ID":   uuid,
	}
	go func() {
		runAgentClientWithConfig(addr.BackendHost, addr.BackendTCP, headers, *maxRetries, *retryDelay)
	}()

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

func runReadLoop(client *network.TCPClient) error {
	buf := make([]byte, 4096)
	for {
		n, err := client.Read(buf)
		global.Logger.Info().Int("n", n).Err(err).Msg("Read from client")
		if err != nil {
			return err
		}
		if n > 0 {
			socket.HandleMessage(buf[:n])
		}
	}
}

func runAgentClientWithConfig(host string, port int, headers map[string]string, maxRetries int, baseDelay time.Duration) {
	const (
		maxDelay      = 30 * time.Second
		backoffFactor = 1.5
	)

	var retryCount int
	var delay time.Duration = baseDelay

	for {
		logger.Infof("Agent is trying to connect to backend %s:%d (attempt #%d)...", host, port, retryCount+1)

		client, err := network.DialTCP(host, port)
		if err != nil {
			retryCount++
			logger.Errorf("Agent cannot connect to backend (attempt #%d): %v", retryCount, err)

			if retryCount >= maxRetries {
				logger.Errorf("Agent has tried to connect %d times but failed. Stopping retries.", maxRetries)
				return
			}

			logger.Infof("Agent will retry in %v...", delay)
			time.Sleep(delay)

			// Exponential backoff với jitter
			delay = time.Duration(float64(delay) * backoffFactor)
			if delay > maxDelay {
				delay = maxDelay
			}
			continue
		}

		// Kết nối thành công, reset retry counter
		retryCount = 0
		delay = baseDelay
		logger.Infof("Agent connected to backend %s:%d successfully!", host, port)

		// Gửi headers và bắt đầu ping loop
		if err := network.SendTokenHeaders(client, headers); err != nil {
			logger.Errorf("Agent cannot send headers to backend: %v", err)
			client.Close()
			continue
		}

		// Read loop until disconnect
		if err := runReadLoop(client); err != nil {
			logger.Errorf("Agent ping loop failed: %v. Will retry...", err)
			client.Close()
			time.Sleep(2 * time.Second) // Short delay before retry
		}
	}
}

func loadOrPromptToken() (string, error) {
	if existing, err := auth.LoadToken(); err == nil && strings.TrimSpace(existing) != "" {
		return strings.TrimSpace(existing), nil
	}
	return promptLogin()
}

func promptLogin() (string, error) {
	u, p, err := auth.PromptCredentials()
	if err != nil {
		return "", err
	}
	return service.Login(u, p)
}
