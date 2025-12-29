package network

/*
#include <stdlib.h>
#include "network.h"

// Bridge callback implemented in Go (exported below)
// Note: Go export functions use non-const for char* parameters
extern void goProtocolMessageBridge(SOCKET client_fd, protocol_message_t* msg, void* user_data);
extern void goProtocolDisconnectBridge(SOCKET client_fd, char* device_id, void* user_data);
*/
import "C"

import (
	"errors"
	"unsafe"
)

// TCPServer wraps a listening TCP socket.
type TCPServer struct {
	fd C.SOCKET
}

// ListenTCP creates a TCP server
func ListenTCP(host string, port int) (*TCPServer, error) {
	if port <= 0 {
		return nil, errors.New("invalid port")
	}
	var cHost *C.char
	if host != "" {
		cHost = C.CString(host)
		defer C.free(unsafe.Pointer(cHost))
	}
	fd := C.tcp_server_start(cHost, C.int(port))
	if fd == C.INVALID_SOCKET {
		return nil, errors.New("tcp listen failed")
	}
	return &TCPServer{fd: fd}, nil
}

// Accept waits for and returns the next connection
func (s *TCPServer) Accept() (*TCPClient, error) {
	if s == nil || s.fd == C.INVALID_SOCKET {
		return nil, errors.New("server not open")
	}
	clientFd := C.tcp_accept(s.fd)
	if clientFd == C.INVALID_SOCKET {
		return nil, errors.New("accept failed")
	}
	return &TCPClient{fd: clientFd}, nil
}

// Close closes the TCP server
func (s *TCPServer) Close() error {
	if s == nil || s.fd == C.INVALID_SOCKET {
		return errors.New("server not open")
	}
	if C.tcp_close(s.fd) != 0 {
		return errors.New("close failed")
	}
	s.fd = C.INVALID_SOCKET
	return nil
}

// ProtocolServer wraps the C multi-client protocol server (no goroutines)
type ProtocolServer struct {
	server *C.protocol_server_t
}

// ProtocolMessageHandler is invoked from C worker threads
type ProtocolMessageHandler func(client *TCPClient, msg *ProtocolMessage)

// ProtocolDisconnectHandler is invoked when a client disconnects
type ProtocolDisconnectHandler func(client *TCPClient, deviceID string)

var protocolHandler ProtocolMessageHandler
var protocolDisconnectHandler ProtocolDisconnectHandler

//export goProtocolMessageBridge
func goProtocolMessageBridge(clientFd C.SOCKET, cMsg *C.protocol_message_t, userData unsafe.Pointer) {
	handler := protocolHandler
	if handler == nil {
		return
	}
	msg := convertProtocolMessage(cMsg)
	handler(&TCPClient{fd: clientFd}, msg)
}

//export goProtocolDisconnectBridge
func goProtocolDisconnectBridge(clientFd C.SOCKET, deviceID *C.char, userData unsafe.Pointer) {
	handler := protocolDisconnectHandler
	if handler == nil {
		return
	}
	// deviceID can be const char* from C, convert safely
	var deviceIDStr string
	if deviceID != nil {
		deviceIDStr = C.GoString(deviceID)
	}
	handler(&TCPClient{fd: clientFd}, deviceIDStr)
}

// ListenProtocol creates a protocol server handled entirely in C threads
func ListenProtocol(host string, port int, handler ProtocolMessageHandler) (*ProtocolServer, error) {
	return ListenProtocolWithDisconnect(host, port, handler, nil)
}

// ListenProtocolWithDisconnect creates a protocol server with both message and disconnect handlers
func ListenProtocolWithDisconnect(host string, port int, handler ProtocolMessageHandler, disconnectHandler ProtocolDisconnectHandler) (*ProtocolServer, error) {
	if port <= 0 {
		return nil, errors.New("invalid port")
	}
	if handler == nil {
		return nil, errors.New("handler is required")
	}

	var cHost *C.char
	if host != "" {
		cHost = C.CString(host)
		defer C.free(unsafe.Pointer(cHost))
	}

	protocolHandler = handler
	protocolDisconnectHandler = disconnectHandler

	// Get bridge function pointers from C helpers
	messageCb := C.protocol_get_message_bridge()
	var disconnectCb C.protocol_disconnect_cb
	if disconnectHandler != nil {
		disconnectCb = C.protocol_get_disconnect_bridge()
	}

	var srv *C.protocol_server_t
	rc := C.protocol_server_create(
		cHost,
		C.int(port),
		messageCb,
		disconnectCb,
		nil,
		&srv,
	)
	if rc != 0 {
		return nil, errors.New("protocol server create failed")
	}

	return &ProtocolServer{server: srv}, nil
}

// Stop stops the protocol server
func (s *ProtocolServer) Stop() error {
	if s == nil || s.server == nil {
		return errors.New("server not open")
	}
	if C.protocol_server_stop(s.server) != 0 {
		return errors.New("protocol server stop failed")
	}
	return nil
}

// Close closes and destroys the protocol server
func (s *ProtocolServer) Close() error {
	if s == nil || s.server == nil {
		return errors.New("server not open")
	}
	C.protocol_server_destroy(s.server)
	s.server = nil
	protocolHandler = nil
	protocolDisconnectHandler = nil
	return nil
}

// DeviceIsOnline queries C-side registry to check if a device has an active protocol connection.
func DeviceIsOnline(deviceID string) bool {
	if deviceID == "" {
		return false
	}
	cDev := C.CString(deviceID)
	defer C.free(unsafe.Pointer(cDev))
	return C.protocol_device_is_online(cDev) != 0
}

// SendToDevice uses server-side registry (managed in C) to push a command JSON to a device.
// This works only for connections handled by the protocol server (not for ad-hoc DialTCP).
func SendToDevice(deviceID string, jsonPayload []byte) error {
	if deviceID == "" {
		return errors.New("device id required")
	}
	if len(jsonPayload) == 0 {
		return errors.New("empty payload")
	}
	cDev := C.CString(deviceID)
	defer C.free(unsafe.Pointer(cDev))
	if C.protocol_send_to_device(cDev, (*C.char)(unsafe.Pointer(&jsonPayload[0])), C.size_t(len(jsonPayload))) != 0 {
		return errors.New("send to device failed")
	}
	return nil
}
