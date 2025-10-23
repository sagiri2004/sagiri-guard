package main

import (
	"flag"
	"fmt"
	"sagiri-guard/agent/internal/auth"
	"sagiri-guard/agent/internal/config"
	"sagiri-guard/agent/internal/logger"
	"sagiri-guard/agent/internal/privilege"
	"sagiri-guard/agent/internal/service"
	"sagiri-guard/network"
	"time"
)

func main() {
	var (
		cfgPath    = flag.String("config", "config/config.yaml", "Path to configuration file")
		maxRetries = flag.Int("max-retries", 10, "Maximum retry attempts for backend connection")
		retryDelay = flag.Duration("retry-delay", 1*time.Second, "Base delay between retry attempts")
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

	// Elevate on Windows
	if !privilege.IsElevated() {
		if relaunched, err := privilege.AttemptElevate(); err != nil {
			logger.Error("Không thể yêu cầu quyền admin:", err)
		} else if relaunched {
			return
		}
	}

	// Prompt login if no token
	token, err := auth.LoadToken()
	if err != nil || token == "" {
		u, p, perr := auth.PromptCredentials()
		if perr != nil {
			logger.Error("Thiếu thông tin đăng nhập")
			return
		}
		token, err = service.Login(u, p)
		if err != nil {
			logger.Error("Đăng nhập thất bại:", err)
			return
		}
	}

	// Bootstrap device
	uuid, derr := service.BootstrapDevice(token)
	if derr != nil {
		logger.Error("Khởi tạo thiết bị thất bại:", derr)
		return
	}

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

func runPingLoop(client *network.TCPClient, host string, port int) error {
	buf := make([]byte, 1024)
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	count := 0

	for range ticker.C {
		count++
		msg := fmt.Sprintf("Agent ping #%d\n", count)

		// Gửi ping với timeout
		if _, err := client.Write([]byte(msg)); err != nil {
			return fmt.Errorf("gửi ping thất bại: %w", err)
		}

		// Đọc phản hồi với timeout
		n, err := client.Read(buf)
		if err != nil {
			return fmt.Errorf("đọc phản hồi từ backend thất bại: %w", err)
		}

		fmt.Printf("Agent nhận phản hồi backend: %s", string(buf[:n]))
	}

	return nil
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

		// Ping loop với reconnection logic
		if err := runPingLoop(client, host, port); err != nil {
			fmt.Printf("Agent ping loop thất bại: %v. Sẽ thử kết nối lại...\n", err)
			client.Close()
			time.Sleep(2 * time.Second) // Ngắn delay trước khi retry
		}
	}
}
