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
		host    = flag.String("host", "127.0.0.1", "Server host")
		port    = flag.Int("port", 9000, "HTTP server port")
		tcpPort = flag.Int("tcp-port", 9100, "TCP server port")
	)
	flag.Parse()

	fmt.Println("Using HTTP server", fmt.Sprintf("%s:%d", *host, *port))

	runHTTPDemo(*host, *port)
	runTCPDemo(*host, *tcpPort)
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

func runTCPDemo(host string, port int) {
	fmt.Println("\n--- TCP Socket Demo ---")
	client, err := network.DialTCP(host, port)
	if err != nil {
		fmt.Println("Không thể kết nối TCP:", err)
		return
	}

	defer client.Close()

	headers := map[string]string{"Authorization": "Bearer demo-token"}
	if err := network.SendTokenHeaders(client, headers); err != nil {
		fmt.Println("Không gửi được header token:", err)
		return
	}

	message := []byte("Hello from Go TCP client!\n")
	if _, err := client.Write(message); err != nil {
		fmt.Println("Gửi dữ liệu thất bại:", err)
		return
	}

	buf := make([]byte, 1024)
	n, err := client.Read(buf)
	if err != nil {
		fmt.Println("Đọc dữ liệu thất bại:", err)
		return
	}
	fmt.Printf("Nhận phản hồi TCP: %s\n", string(buf[:n]))

	go runRealtimeLoop(client)

	select {}
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

func runRealtimeLoop(client *network.TCPClient) {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	buf := make([]byte, 1024)
	count := 0
	for range ticker.C {
		count++
		msg := fmt.Sprintf("ping #%d", count)
		if _, err := client.Write([]byte(msg)); err != nil {
			fmt.Println("Gửi ping thất bại:", err)
			return
		}
		n, err := client.Read(buf)
		if err != nil {
			fmt.Println("Đọc phản hồi thất bại:", err)
			return
		}
		fmt.Printf("Loop nhận: %s\n", string(buf[:n]))
	}
}
