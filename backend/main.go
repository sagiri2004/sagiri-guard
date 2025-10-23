package main

import (
	"flag"
	"fmt"
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
		fmt.Println("Không thể khởi tạo thư viện mạng:", err)
		return
	}
	defer network.Cleanup()

	app, err := initialize.Build(*cfgPath)
	if err != nil {
		fmt.Println("Khởi tạo ứng dụng lỗi:", err)
		return
	}

	if err := server.StartHTTPServerC(app.Cfg.HTTP.Host, app.Cfg.HTTP.Port, app.Router); err != nil {
		fmt.Println("Không thể khởi động HTTP server:", err)
		return
	}
	fmt.Printf("HTTP server đang lắng nghe tại %s:%d\n", app.Cfg.HTTP.Host, app.Cfg.HTTP.Port)

	// TCP socket server: chỉ lắng nghe và nhận kết nối từ client
	go func() {
		if err := server.StartTCPServer(app.Cfg.TCP.Host, app.Cfg.TCP.Port, app.Socket.HandleClient); err != nil {
			fmt.Println("TCP server dừng với lỗi:", err)
		}
	}()
	fmt.Printf("TCP server đang lắng nghe tại %s:%d\n", app.Cfg.TCP.Host, app.Cfg.TCP.Port)

	select {}
}
