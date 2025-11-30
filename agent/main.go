package main

import (
	"flag"
	"fmt"
	"sagiri-guard/agent/internal/auth"
	"sagiri-guard/agent/internal/config"
	"sagiri-guard/agent/internal/db"
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
		fmt.Println("Không thể khởi tạo thư viện mạng:", err)
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
		logger.Error("Không thể mở SQLite:", dberr)
		return
	}
	_ = adb.AutoMigrate(&db.Token{})

	// Elevate on Windows (optional)
	if *elevate && !privilege.IsElevated() {
		if relaunched, err := privilege.AttemptElevate(); err != nil {
			logger.Error("Không thể yêu cầu quyền admin:", err)
		} else if relaunched {
			return
		}
	}

	token, err := loadOrPromptToken()
	if err != nil {
		logger.Error("Thiếu thông tin đăng nhập:", err)
		return
	}
	auth.SetCurrentToken(token)
	state.SetToken(token)

	var uuid string
	for {
		uuid, err = service.BootstrapDevice(token)
		if err == service.ErrUnauthorized {
			logger.Warn("Token hiện tại không hợp lệ, yêu cầu đăng nhập lại")
			if clearErr := auth.ClearToken(); clearErr != nil {
				logger.Warn("Không thể xoá token cũ: %v", clearErr)
			}
			token, err = promptLogin()
			if err != nil {
				logger.Error("Đăng nhập thất bại:", err)
				return
			}
			auth.SetCurrentToken(token)
			state.SetToken(token)
			continue
		}
		if err != nil {
			logger.Error("Khởi tạo thiết bị thất bại:", err)
			return
		}
		break
	}
	state.SetDeviceID(uuid)

	_ = cfgPath // viper reads from default config path set in Init()

	fmt.Printf("Agent sẽ retry tối đa %d lần với delay cơ bản %v\n", *maxRetries, *retryDelay)

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

	// socket running; main blocks

	select {}
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
		fmt.Printf("Agent đang thử kết nối đến backend %s:%d (lần thử #%d)...\n", host, port, retryCount+1)

		client, err := network.DialTCP(host, port)
		if err != nil {
			retryCount++
			fmt.Printf("Agent không kết nối được tới backend (lần thử #%d): %v\n", retryCount, err)

			if retryCount >= maxRetries {
				fmt.Printf("Agent đã thử kết nối %d lần nhưng thất bại. Dừng retry.\n", maxRetries)
				return
			}

			fmt.Printf("Agent sẽ thử lại sau %v...\n", delay)
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
		fmt.Printf("Agent kết nối tới backend %s:%d thành công!\n", host, port)

		// Gửi headers và bắt đầu ping loop
		if err := network.SendTokenHeaders(client, headers); err != nil {
			fmt.Println("Agent không gửi được header tới backend:", err)
			client.Close()
			continue
		}

		// Read loop until disconnect
		if err := runReadLoop(client); err != nil {
			fmt.Printf("Agent ping loop thất bại: %v. Sẽ thử kết nối lại...\n", err)
			client.Close()
			time.Sleep(2 * time.Second) // Ngắn delay trước khi retry
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
