package controllers

import (
	"fmt"
	"sagiri-guard/network"
	"time"
)

type SocketController struct{}

func NewSocketController() *SocketController {
	return &SocketController{}
}

// HandleClient is invoked per accepted TCP client connection.
func (c *SocketController) HandleClient(client *network.TCPClient) {
	defer client.Close()
	buf := make([]byte, 2048)
	if headers, remaining, err := network.ReadTokenHeaders(client); err == nil {
		fmt.Println("Backend nhận header từ client:", headers)
		if len(remaining) > 0 {
			fmt.Printf("Backend nhận remaining: %s\n", string(remaining))
		}
	}

	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for range ticker.C {
		n, err := client.Read(buf)
		if err != nil {
			fmt.Println("Backend đọc dữ liệu từ client thất bại:", err)
			return
		}
		if n > 0 {
			fmt.Printf("Backend nhận từ client: %s\n", string(buf[:n]))
		}
		response := fmt.Sprintf("Backend ACK %s\n", time.Now().Format(time.RFC3339))
		if _, err := client.Write([]byte(response)); err != nil {
			fmt.Println("Backend gửi phản hồi thất bại:", err)
			return
		}
	}
}
