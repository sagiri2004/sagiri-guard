package server

import (
	"fmt"
	"net"
	"net/http"
)

func StartHTTPServer(host string, port int, handler http.Handler) error {
	addr := net.JoinHostPort(host, fmt.Sprintf("%d", port))
	go func() {
		_ = http.ListenAndServe(addr, handler)
	}()
	return nil
}


