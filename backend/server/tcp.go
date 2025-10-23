package server

import (
	"fmt"
	"sagiri-guard/network"
)

func StartTCPServer(host string, port int, handle func(*network.TCPClient)) error {
	server, err := network.ListenTCP(host, port)
	if err != nil {
		return fmt.Errorf("listen tcp failed: %w", err)
	}
	go func() {
		for {
			client, err := server.Accept()
			if err != nil {
				continue
			}
			go handle(client)
		}
	}()
	return nil
}
