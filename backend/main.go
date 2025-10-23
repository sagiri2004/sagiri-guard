package main

import (
	"archive/zip"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sagiri-guard/network"
	"time"
)

func main() {
	var (
		host       = flag.String("host", "127.0.0.1", "Server host")
		port       = flag.Int("port", 9400, "HTTP server port")
		tcpPort    = flag.Int("tcp-port", 9200, "Backend TCP server port")
		agentHost  = flag.String("agent-host", "127.0.0.1", "Agent host to connect")
		agentPort  = flag.Int("agent-port", 9100, "Agent TCP port to connect")
		maxRetries = flag.Int("max-retries", 10, "Maximum retry attempts for agent connection")
		retryDelay = flag.Duration("retry-delay", 1*time.Second, "Base delay between retry attempts")
	)
	flag.Parse()

	if err := network.Init(); err != nil {
		fmt.Println("Không thể khởi tạo thư viện mạng:", err)
		return
	}
	defer network.Cleanup()

	if err := network.EnsureDemoServers(*host, *port, *tcpPort+100); err != nil {
		fmt.Println("Không thể khởi động server demo:", err)
		return
	}

	fmt.Println("Using HTTP server", fmt.Sprintf("%s:%d", *host, *port))
	fmt.Printf("Backend sẽ retry tối đa %d lần với delay cơ bản %v\n", *maxRetries, *retryDelay)

	go runBackendSocketServer(*host, *tcpPort)
	go runBackendClientWithConfig(*agentHost, *agentPort, *maxRetries, *retryDelay)

	runHTTPDemo(*host, *port)

	select {}
}

func runHTTPDemo(host string, port int) {
	headers := map[string]string{
		"Authorization": "Bearer demo-token",
	}
	if resp, err := network.HTTPGetWithHeaders(host, port, "/ping", headers); err != nil {
		fmt.Println("HTTP GET thất bại:", err)
	} else {
		fmt.Println("\n--- GET /ping ---")
		fmt.Println(resp)
	}

	headers = map[string]string{
		"Authorization": "Bearer demo-token",
	}
	if resp, err := network.HTTPPostWithHeaders(host, port, "/echo", "application/json", []byte(`{"message":"xin chào từ Go qua thư viện C"}`), headers); err != nil {
		fmt.Println("HTTP POST thất bại:", err)
	} else {
		fmt.Println("\n--- POST /echo ---")
		fmt.Println(resp)
	}

	headers = map[string]string{
		"Authorization": "Bearer demo-token",
	}
	if resp, err := network.HTTPPutWithHeaders(host, port, "/update", "application/x-www-form-urlencoded", []byte("status=updated"), headers); err != nil {
		fmt.Println("HTTP PUT thất bại:", err)
	} else {
		fmt.Println("\n--- PUT /update ---")
		fmt.Println(resp)
	}

	headers = map[string]string{
		"Authorization": "Bearer demo-token",
	}
	if resp, err := network.HTTPDeleteWithHeaders(host, port, "/resource", headers); err != nil {
		fmt.Println("HTTP DELETE thất bại:", err)
	} else {
		fmt.Println("\n--- DELETE /resource ---")
		fmt.Println(resp)
	}

	zipPath := filepath.Join(os.TempDir(), "cgo_network_upload.zip")
	if err := createSampleZip(zipPath); err != nil {
		fmt.Println("Không thể tạo file zip demo:", err)
		return
	}
	defer os.Remove(zipPath)

	headers = map[string]string{
		"Authorization": "Bearer demo-token",
	}
	if resp, err := network.HTTPUploadFileWithHeaders(host, port, "/upload", zipPath, headers); err != nil {
		fmt.Println("HTTP upload thất bại:", err)
	} else {
		fmt.Println("\n--- POST /upload (multipart) ---")
		fmt.Println(resp)
	}
}

func runBackendSocketServer(host string, port int) {
	server, err := network.ListenTCP(host, port)
	if err != nil {
		fmt.Println("Backend không thể lắng nghe TCP:", err)
		return
	}
	fmt.Printf("\n--- Backend listening tại %s:%d ---\n", host, port)
	for {
		client, err := server.Accept()
		if err != nil {
			fmt.Println("Backend accept thất bại:", err)
			continue
		}
		go handleAgent(client)
	}
}

func handleAgent(client *network.TCPClient) {
	defer client.Close()
	buf := make([]byte, 1024)
	if headers, remaining, err := network.ReadTokenHeaders(client); err != nil {
		fmt.Println("Backend đọc header từ agent thất bại:", err)
		return
	} else {
		fmt.Println("Backend nhận header từ agent:", headers)
		fmt.Println("Backend nhận dữ liệu từ agent:", string(remaining))
	}
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for range ticker.C {
		n, err := client.Read(buf)
		if err != nil {
			fmt.Println("Backend đọc dữ liệu từ agent thất bại:", err)
			return
		}
		fmt.Printf("Backend nhận từ agent: %s\n", string(buf[:n]))
		response := fmt.Sprintf("Backend ACK %s\n", time.Now().Format(time.RFC3339))
		if _, err := client.Write([]byte(response)); err != nil {
			fmt.Println("Backend gửi phản hồi thất bại:", err)
			return
		}
	}
}

func runBackendPingLoop(client *network.TCPClient, host string, port int) error {
	buf := make([]byte, 1024)
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	count := 0

	for range ticker.C {
		count++
		msg := fmt.Sprintf("Backend ping #%d\n", count)

		// Gửi ping với timeout
		if _, err := client.Write([]byte(msg)); err != nil {
			return fmt.Errorf("gửi ping thất bại: %w", err)
		}

		// Đọc phản hồi với timeout
		n, err := client.Read(buf)
		if err != nil {
			return fmt.Errorf("đọc phản hồi từ agent thất bại: %w", err)
		}

		fmt.Printf("Backend nhận phản hồi từ agent: %s", string(buf[:n]))
	}

	return nil
}

func runBackendClientWithConfig(host string, port int, maxRetries int, baseDelay time.Duration) {
	const (
		maxDelay      = 30 * time.Second
		backoffFactor = 1.5
	)

	var retryCount int
	var delay time.Duration = baseDelay

	for {
		fmt.Printf("Backend đang thử kết nối đến agent %s:%d (lần thử #%d)...\n", host, port, retryCount+1)

		client, err := network.DialTCP(host, port)
		if err != nil {
			retryCount++
			fmt.Printf("Backend không kết nối được tới agent (lần thử #%d): %v\n", retryCount, err)

			if retryCount >= maxRetries {
				fmt.Printf("Backend đã thử kết nối %d lần nhưng thất bại. Dừng retry.\n", maxRetries)
				return
			}

			fmt.Printf("Backend sẽ thử lại sau %v...\n", delay)
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
		fmt.Printf("Backend kết nối tới agent %s:%d thành công!\n", host, port)

		// Gửi headers và bắt đầu ping loop
		headers := map[string]string{"Authorization": "Bearer backend"}
		if err := network.SendTokenHeaders(client, headers); err != nil {
			fmt.Println("Backend không gửi được header tới agent:", err)
			client.Close()
			continue
		}

		// Ping loop với reconnection logic
		if err := runBackendPingLoop(client, host, port); err != nil {
			fmt.Printf("Backend ping loop thất bại: %v. Sẽ thử kết nối lại...\n", err)
			client.Close()
			time.Sleep(2 * time.Second) // Ngắn delay trước khi retry
		}
	}
}

func createSampleZip(path string) error {
	file, err := os.Create(path)
	if err != nil {
		return err
	}
	defer file.Close()

	zw := zip.NewWriter(file)
	defer zw.Close()

	writer, err := zw.Create("message.txt")
	if err != nil {
		return err
	}
	if _, err := writer.Write([]byte("Xin chào từ file nén demo.")); err != nil {
		return err
	}
	return zw.Close()
}
