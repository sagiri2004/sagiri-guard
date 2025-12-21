package network

/*
#cgo CFLAGS: -I${SRCDIR}
#cgo LDFLAGS: -L${SRCDIR} -lnetwork
#include <stdlib.h>
#include <stddef.h>
#include <stdint.h>
#include "network.h"

// Bridge callback implemented in Go (exported below)
extern void goProtocolMessageBridge(SOCKET client_fd, protocol_message_t* msg, void* user_data);
*/
import "C"

import (
	"errors"
	"unsafe"
)

// Init initializes the C networking library (no-op on Linux).
// This MUST be called once at the start of the application.
func Init() error {
	if C.network_init() != 0 {
		return errors.New("failed to initialize C networking library")
	}
	return nil
}

// Cleanup cleans up the C networking library (no-op on Linux).
// This MUST be called once before the application exits.
func Cleanup() {
	C.network_cleanup()
}

// TCPClient wraps a connected TCP socket managed by the C library.
type TCPClient struct {
	// SỬA LỖI: Dùng C.SOCKET thay vì C.int
	fd C.SOCKET
}

func DialTCP(host string, port int) (*TCPClient, error) {
	if host == "" || port <= 0 {
		return nil, errors.New("invalid host or port")
	}
	cHost := C.CString(host)
	defer C.free(unsafe.Pointer(cHost))
	fd := C.tcp_client_connect(cHost, C.int(port))

	// SỬA LỖI: Kiểm tra C.INVALID_SOCKET
	if fd == C.INVALID_SOCKET {
		return nil, errors.New("tcp connect failed")
	}
	return &TCPClient{fd: fd}, nil
}

func (c *TCPClient) Close() error {
	// SỬA LỖI: Kiểm tra C.INVALID_SOCKET
	if c == nil || c.fd == C.INVALID_SOCKET {
		return errors.New("client not open")
	}
	if C.tcp_close(c.fd) != 0 {
		return errors.New("close failed")
	}
	// SỬA LỖI: Gán C.INVALID_SOCKET
	c.fd = C.INVALID_SOCKET
	return nil
}

// IsOpen reports whether the underlying socket is still valid.
func (c *TCPClient) IsOpen() bool {
	return c != nil && c.fd != C.INVALID_SOCKET
}

func (c *TCPClient) Write(data []byte) (int, error) {
	// SỬA LỖI: Kiểm tra C.INVALID_SOCKET
	if c == nil || c.fd == C.INVALID_SOCKET {
		return 0, errors.New("client not open")
	}
	if len(data) == 0 {
		return 0, nil
	}
	// C.tcp_send trả về C.ssize_t (mà Go hiểu là C.longlong hoặc C.long)
	// dịnh dạng data gửi đi là ví dụ "{\"deviceid\":\"1234567890\",\"command\":\"get_status\",\"argument\":{}}"
	written := C.tcp_send(c.fd, (*C.char)(unsafe.Pointer(&data[0])), C.size_t(len(data)))
	if written < 0 {
		return 0, errors.New("send failed")
	}
	return int(written), nil
}

func (c *TCPClient) Read(buf []byte) (int, error) {
	// SỬA LỖI: Kiểm tra C.INVALID_SOCKET
	if c == nil || c.fd == C.INVALID_SOCKET {
		return 0, errors.New("client not open")
	}
	if len(buf) == 0 {
		return 0, nil
	}
	n := C.tcp_recv(c.fd, (*C.char)(unsafe.Pointer(&buf[0])), C.size_t(len(buf)))
	if n < 0 {
		return 0, errors.New("recv failed")
	}
	return int(n), nil
}

func (c *TCPClient) ReadFull(buf []byte) (int, error) {
	total := 0
	for total < len(buf) {
		n, err := c.Read(buf[total:])
		if err != nil {
			return total, err
		}
		if n == 0 {
			break
		}
		total += n
	}
	return total, nil
}

// TCPServer wraps a listening TCP socket.
type TCPServer struct {
	// SỬA LỖI: Dùng C.SOCKET
	fd C.SOCKET
}

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
	// SỬA LỖI: Kiểm tra C.INVALID_SOCKET
	if fd == C.INVALID_SOCKET {
		return nil, errors.New("tcp listen failed")
	}
	return &TCPServer{fd: fd}, nil
}

func (s *TCPServer) Accept() (*TCPClient, error) {
	// SỬA LỖI: Kiểm tra C.INVALID_SOCKET
	if s == nil || s.fd == C.INVALID_SOCKET {
		return nil, errors.New("server not open")
	}
	clientFd := C.tcp_accept(s.fd)
	// SỬA LỖI: Kiểm tra C.INVALID_SOCKET
	if clientFd == C.INVALID_SOCKET {
		return nil, errors.New("accept failed")
	}
	return &TCPClient{fd: clientFd}, nil
}

func (s *TCPServer) Close() error {
	// SỬA LỖI: Kiểm tra C.INVALID_SOCKET
	if s == nil || s.fd == C.INVALID_SOCKET {
		return errors.New("server not open")
	}
	if C.tcp_close(s.fd) != 0 {
		return errors.New("close failed")
	}
	// SỬA LỖI: Gán C.INVALID_SOCKET
	s.fd = C.INVALID_SOCKET
	return nil
}

// ========== Protocol Message Functions ==========

// ProtocolMessageType represents the type of protocol message
type ProtocolMessageType uint8

const (
	MsgLogin     ProtocolMessageType = 0x01
	MsgCommand   ProtocolMessageType = 0x02
	MsgFileMeta  ProtocolMessageType = 0x03
	MsgFileChunk ProtocolMessageType = 0x04
	MsgFileDone  ProtocolMessageType = 0x05
	MsgAck       ProtocolMessageType = 0x06
	MsgError     ProtocolMessageType = 0x7F
)

// ProtocolMessage represents a decoded protocol frame
type ProtocolMessage struct {
	Type        ProtocolMessageType
	Raw         []byte
	DeviceID    string
	Token       string
	SessionID   string
	CommandJSON []byte

	FileName    string
	FileSize    uint64
	ChunkOffset uint32
	ChunkLen    uint32
	ChunkData   []byte

	StatusCode uint16
	StatusMsg  string
}

// SendLogin sends a login frame (device + token)
func (c *TCPClient) SendLogin(deviceID, token string) error {
	if c == nil || c.fd == C.INVALID_SOCKET {
		return errors.New("client not open")
	}
	cDev := C.CString(deviceID)
	cTok := C.CString(token)
	defer C.free(unsafe.Pointer(cDev))
	defer C.free(unsafe.Pointer(cTok))
	if C.protocol_send_login(c.fd, cDev, cTok) != 0 {
		return errors.New("send login failed")
	}
	return nil
}

// SendCommand sends a JSON command payload
func (c *TCPClient) SendCommand(jsonPayload []byte) error {
	if c == nil || c.fd == C.INVALID_SOCKET {
		return errors.New("client not open")
	}
	if len(jsonPayload) == 0 {
		return errors.New("empty command payload")
	}
	if C.protocol_send_command(c.fd, (*C.char)(unsafe.Pointer(&jsonPayload[0])), C.size_t(len(jsonPayload))) != 0 {
		return errors.New("send command failed")
	}
	return nil
}

// SendFileMeta sends file metadata
func (c *TCPClient) SendFileMeta(name string, size uint64) error {
	if c == nil || c.fd == C.INVALID_SOCKET {
		return errors.New("client not open")
	}
	cName := C.CString(name)
	defer C.free(unsafe.Pointer(cName))
	if C.protocol_send_file_meta(c.fd, cName, C.uint64_t(size)) != 0 {
		return errors.New("send file meta failed")
	}
	return nil
}

// SendFileChunk sends a file chunk
func (c *TCPClient) SendFileChunk(offset uint32, data []byte) error {
	if c == nil || c.fd == C.INVALID_SOCKET {
		return errors.New("client not open")
	}
	if len(data) == 0 {
		return nil
	}
	if C.protocol_send_file_chunk(c.fd, nil, nil, C.uint32_t(offset), (*C.char)(unsafe.Pointer(&data[0])), C.uint32_t(len(data))) != 0 {
		return errors.New("send file chunk failed")
	}
	return nil
}

// SendFileChunkWithSession sends a file chunk with session_id/token
func (c *TCPClient) SendFileChunkWithSession(sessionID, token string, offset uint32, data []byte) error {
	if c == nil || c.fd == C.INVALID_SOCKET {
		return errors.New("client not open")
	}
	if len(data) == 0 {
		return nil
	}
	cSid := C.CString(sessionID)
	cTok := C.CString(token)
	defer C.free(unsafe.Pointer(cSid))
	defer C.free(unsafe.Pointer(cTok))
	if C.protocol_send_file_chunk(c.fd, cSid, cTok, C.uint32_t(offset), (*C.char)(unsafe.Pointer(&data[0])), C.uint32_t(len(data))) != 0 {
		return errors.New("send file chunk failed")
	}
	return nil
}

// SendFileDone signals end of file transfer
func (c *TCPClient) SendFileDone() error {
	if c == nil || c.fd == C.INVALID_SOCKET {
		return errors.New("client not open")
	}
	if C.protocol_send_file_done(c.fd, nil, nil) != 0 {
		return errors.New("send file done failed")
	}
	return nil
}

// SendFileDoneWithSession signals end with session info
func (c *TCPClient) SendFileDoneWithSession(sessionID, token string) error {
	if c == nil || c.fd == C.INVALID_SOCKET {
		return errors.New("client not open")
	}
	cSid := C.CString(sessionID)
	cTok := C.CString(token)
	defer C.free(unsafe.Pointer(cSid))
	defer C.free(unsafe.Pointer(cTok))
	if C.protocol_send_file_done(c.fd, cSid, cTok) != 0 {
		return errors.New("send file done failed")
	}
	return nil
}

// SendAck sends an ACK/ERROR frame
func (c *TCPClient) SendAck(code uint16, msg string) error {
	if c == nil || c.fd == C.INVALID_SOCKET {
		return errors.New("client not open")
	}
	cMsg := C.CString(msg)
	defer C.free(unsafe.Pointer(cMsg))
	if C.protocol_send_ack(c.fd, C.uint16_t(code), cMsg) != 0 {
		return errors.New("send ack failed")
	}
	return nil
}

// RecvProtocolMessage receives a protocol message from the peer
func (c *TCPClient) RecvProtocolMessage() (*ProtocolMessage, error) {
	if c == nil || c.fd == C.INVALID_SOCKET {
		return nil, errors.New("client not open")
	}

	var cMsg C.protocol_message_t
	if C.protocol_recv_message(c.fd, &cMsg) != 0 {
		return nil, errors.New("recv message failed")
	}
	defer C.protocol_message_free(&cMsg)

	return convertProtocolMessage(&cMsg), nil
}

func convertProtocolMessage(cMsg *C.protocol_message_t) *ProtocolMessage {
	msgType := C.protocol_message_get_type(cMsg)
	pm := &ProtocolMessage{
		Type:        ProtocolMessageType(msgType),
		DeviceID:    C.GoString(&cMsg.device_id[0]),
		Token:       C.GoString(&cMsg.token[0]),
		SessionID:   C.GoString(&cMsg.session_id[0]),
		FileName:    C.GoString(&cMsg.file_name[0]),
		FileSize:    uint64(cMsg.file_size),
		ChunkOffset: uint32(cMsg.chunk_offset),
		ChunkLen:    uint32(cMsg.chunk_len),
		StatusCode:  uint16(cMsg.status_code),
		StatusMsg:   C.GoString(&cMsg.status_msg[0]),
	}

	if cMsg.data != nil && cMsg.data_len > 0 {
		pm.Raw = C.GoBytes(unsafe.Pointer(cMsg.data), C.int(cMsg.data_len))
	}

	switch pm.Type {
	case MsgCommand:
		pm.CommandJSON = pm.Raw
	case MsgFileChunk:
		if len(pm.Raw) >= 2 {
			sidLen := int(pm.Raw[0])
			tokLen := int(pm.Raw[1])
			pos := 2 + sidLen + tokLen + 8
			if pos <= len(pm.Raw) {
				pm.ChunkData = pm.Raw[pos:]
			}
		}
	}

	return pm
}

// Compatibility helpers
func (c *TCPClient) SendText(data []byte) error { return c.SendCommand(data) }
func (c *TCPClient) RecvMessage() (*ProtocolMessage, error) {
	return c.RecvProtocolMessage()
}

// ProtocolServer wraps the C multi-client protocol server (no goroutines)
type ProtocolServer struct {
	server *C.protocol_server_t
}

// ProtocolMessageHandler is invoked from C worker threads
type ProtocolMessageHandler func(client *TCPClient, msg *ProtocolMessage)

var protocolHandler ProtocolMessageHandler

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

//export goProtocolMessageBridge
func goProtocolMessageBridge(clientFd C.SOCKET, cMsg *C.protocol_message_t, userData unsafe.Pointer) {
	handler := protocolHandler
	if handler == nil {
		return
	}
	msg := convertProtocolMessage(cMsg)
	handler(&TCPClient{fd: clientFd}, msg)
}

// ListenProtocol creates a protocol server handled entirely in C threads
func ListenProtocol(host string, port int, handler ProtocolMessageHandler) (*ProtocolServer, error) {
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

	var srv *C.protocol_server_t
	rc := C.protocol_server_create(
		cHost,
		C.int(port),
		(C.protocol_message_cb)(C.goProtocolMessageBridge),
		nil,
		&srv,
	)
	if rc != 0 {
		return nil, errors.New("protocol server create failed")
	}

	return &ProtocolServer{server: srv}, nil
}

func (s *ProtocolServer) Stop() error {
	if s == nil || s.server == nil {
		return errors.New("server not open")
	}
	if C.protocol_server_stop(s.server) != 0 {
		return errors.New("protocol server stop failed")
	}
	return nil
}

func (s *ProtocolServer) Close() error {
	if s == nil || s.server == nil {
		return errors.New("server not open")
	}
	C.protocol_server_destroy(s.server)
	s.server = nil
	protocolHandler = nil
	return nil
}
