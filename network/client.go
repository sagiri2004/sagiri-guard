package network

/*
#include <stdlib.h>
#include "network.h"
*/
import "C"

import (
	"errors"
	"unsafe"
)

// TCPClient wraps a connected TCP socket managed by the C library.
type TCPClient struct {
	fd C.SOCKET
}

// DialTCP connects to a TCP server
func DialTCP(host string, port int) (*TCPClient, error) {
	if host == "" || port <= 0 {
		return nil, errors.New("invalid host or port")
	}
	cHost := C.CString(host)
	defer C.free(unsafe.Pointer(cHost))
	fd := C.tcp_client_connect(cHost, C.int(port))

	if fd == C.INVALID_SOCKET {
		return nil, errors.New("tcp connect failed")
	}
	return &TCPClient{fd: fd}, nil
}

// Close closes the TCP client connection
func (c *TCPClient) Close() error {
	if c == nil || c.fd == C.INVALID_SOCKET {
		return errors.New("client not open")
	}
	if C.tcp_close(c.fd) != 0 {
		return errors.New("close failed")
	}
	c.fd = C.INVALID_SOCKET
	return nil
}

// IsOpen reports whether the underlying socket is still valid.
func (c *TCPClient) IsOpen() bool {
	return c != nil && c.fd != C.INVALID_SOCKET
}

// Equal checks if two clients share the same underlying file descriptor
func (c *TCPClient) Equal(other *TCPClient) bool {
	if c == nil || other == nil {
		return false
	}
	return c.fd == other.fd
}

// Write sends data over the TCP connection
func (c *TCPClient) Write(data []byte) (int, error) {
	if c == nil || c.fd == C.INVALID_SOCKET {
		return 0, errors.New("client not open")
	}
	if len(data) == 0 {
		return 0, nil
	}
	written := C.tcp_send(c.fd, (*C.char)(unsafe.Pointer(&data[0])), C.size_t(len(data)))
	if written < 0 {
		return 0, errors.New("send failed")
	}
	return int(written), nil
}

// Read receives data from the TCP connection
func (c *TCPClient) Read(buf []byte) (int, error) {
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

// ReadFull reads exactly len(buf) bytes
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
