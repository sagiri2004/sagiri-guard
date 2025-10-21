package network

/*
#cgo CFLAGS: -I${SRCDIR}
#cgo LDFLAGS: -L${SRCDIR} -lnetwork
#include <stdlib.h>
#include "network.h"
*/
import "C"

import (
	"bytes"
	"errors"
	"strings"
	"unsafe"
)

// TCPClient wraps a connected TCP socket managed by the C library.
type TCPClient struct {
	fd C.int
}

func DialTCP(host string, port int) (*TCPClient, error) {
	if host == "" || port <= 0 {
		return nil, errors.New("invalid host or port")
	}
	cHost := C.CString(host)
	defer C.free(unsafe.Pointer(cHost))
	fd := C.tcp_client_connect(cHost, C.int(port))
	if fd < 0 {
		return nil, errors.New("tcp connect failed")
	}
	return &TCPClient{fd: fd}, nil
}

func (c *TCPClient) Close() error {
	if c == nil || c.fd < 0 {
		return errors.New("client not open")
	}
	if C.tcp_close(c.fd) != 0 {
		return errors.New("close failed")
	}
	c.fd = -1
	return nil
}

func (c *TCPClient) Write(data []byte) (int, error) {
	if c == nil || c.fd < 0 {
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

func (c *TCPClient) Read(buf []byte) (int, error) {
	if c == nil || c.fd < 0 {
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
	fd C.int
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
	if fd < 0 {
		return nil, errors.New("tcp listen failed")
	}
	return &TCPServer{fd: fd}, nil
}

func (s *TCPServer) Accept() (*TCPClient, error) {
	if s == nil || s.fd < 0 {
		return nil, errors.New("server not open")
	}
	clientFd := C.tcp_accept(s.fd)
	if clientFd < 0 {
		return nil, errors.New("accept failed")
	}
	return &TCPClient{fd: clientFd}, nil
}

func (s *TCPServer) Close() error {
	if s == nil || s.fd < 0 {
		return errors.New("server not open")
	}
	if C.tcp_close(s.fd) != 0 {
		return errors.New("close failed")
	}
	s.fd = -1
	return nil
}

// UDPConn wraps an optional connected UDP socket.
type UDPConn struct {
	fd C.int
}

func ListenUDP(host string, port int) (*UDPConn, error) {
	if port <= 0 {
		return nil, errors.New("invalid port")
	}
	cHost := (*C.char)(nil)
	if host != "" {
		cHost = C.CString(host)
		defer C.free(unsafe.Pointer(cHost))
	}
	fd := C.udp_server_start(cHost, C.int(port))
	if fd < 0 {
		return nil, errors.New("udp listen failed")
	}
	return &UDPConn{fd: fd}, nil
}

func DialUDP(host string, port int) (*UDPConn, error) {
	if host == "" || port <= 0 {
		return nil, errors.New("invalid host or port")
	}
	cHost := C.CString(host)
	defer C.free(unsafe.Pointer(cHost))
	fd := C.udp_client_connect(cHost, C.int(port))
	if fd < 0 {
		return nil, errors.New("udp connect failed")
	}
	return &UDPConn{fd: fd}, nil
}

func (u *UDPConn) Close() error {
	if u == nil || u.fd < 0 {
		return errors.New("conn not open")
	}
	if C.udp_close(u.fd) != 0 {
		return errors.New("close failed")
	}
	u.fd = -1
	return nil
}

func (u *UDPConn) WriteTo(data []byte, host string, port int) (int, error) {
	if u == nil || u.fd < 0 {
		return 0, errors.New("conn not open")
	}
	if len(data) == 0 {
		return 0, nil
	}
	var cHost *C.char
	if host != "" {
		cHost = C.CString(host)
		defer C.free(unsafe.Pointer(cHost))
	}
	sent := C.udp_send(u.fd, (*C.char)(unsafe.Pointer(&data[0])), C.size_t(len(data)), cHost, C.int(port))
	if sent < 0 {
		return 0, errors.New("udp send failed")
	}
	return int(sent), nil
}

func (u *UDPConn) ReadFrom(buf []byte) (int, string, int, error) {
	if u == nil || u.fd < 0 {
		return 0, "", 0, errors.New("conn not open")
	}
	if len(buf) == 0 {
		return 0, "", 0, nil
	}
	ipBuf := make([]byte, 64)
	var port C.int
	n := C.udp_recv(u.fd, (*C.char)(unsafe.Pointer(&buf[0])), C.size_t(len(buf)), (*C.char)(unsafe.Pointer(&ipBuf[0])), &port)
	if n < 0 {
		return 0, "", 0, errors.New("udp recv failed")
	}
	end := 0
	for end < len(ipBuf) && ipBuf[end] != 0 {
		end++
	}
	return int(n), string(ipBuf[:end]), int(port), nil
}

// HTTP helpers
func buildExtraHeaders(headers map[string]string) (*C.char, func()) {
	if len(headers) == 0 {
		return nil, func() {}
	}
	var builder strings.Builder
	for k, v := range headers {
		builder.WriteString(k)
		builder.WriteString(": ")
		builder.WriteString(v)
		builder.WriteString("\r\n")
	}
	ptr := C.CString(builder.String())
	return ptr, func() {
		if ptr != nil {
			C.free(unsafe.Pointer(ptr))
		}
	}
}

func HTTPGet(host string, port int, path string) (string, error) {
	return HTTPGetWithHeaders(host, port, path, nil)
}

func HTTPGetWithHeaders(host string, port int, path string, headers map[string]string) (string, error) {
	buf := make([]byte, 128*1024)
	cHost := C.CString(host)
	cPath := C.CString(path)
	defer C.free(unsafe.Pointer(cHost))
	defer C.free(unsafe.Pointer(cPath))
	extra, release := buildExtraHeaders(headers)
	defer release()
	if C.http_get(cHost, C.int(port), cPath, extra, (*C.char)(unsafe.Pointer(&buf[0])), C.size_t(len(buf))) != 0 {
		return "", errors.New("http get failed")
	}
	return goStringFromBuffer(buf), nil
}

func HTTPDelete(host string, port int, path string) (string, error) {
	return HTTPDeleteWithHeaders(host, port, path, nil)
}

func HTTPDeleteWithHeaders(host string, port int, path string, headers map[string]string) (string, error) {
	buf := make([]byte, 128*1024)
	cHost := C.CString(host)
	cPath := C.CString(path)
	defer C.free(unsafe.Pointer(cHost))
	defer C.free(unsafe.Pointer(cPath))
	extra, release := buildExtraHeaders(headers)
	defer release()
	if C.http_delete(cHost, C.int(port), cPath, extra, (*C.char)(unsafe.Pointer(&buf[0])), C.size_t(len(buf))) != 0 {
		return "", errors.New("http delete failed")
	}
	return goStringFromBuffer(buf), nil
}

func HTTPPost(host string, port int, path, contentType string, body []byte) (string, error) {
	return HTTPPostWithHeaders(host, port, path, contentType, body, nil)
}

func HTTPPostWithHeaders(host string, port int, path, contentType string, body []byte, headers map[string]string) (string, error) {
	buf := make([]byte, 128*1024)
	cHost := C.CString(host)
	cPath := C.CString(path)
	defer C.free(unsafe.Pointer(cHost))
	defer C.free(unsafe.Pointer(cPath))
	var cType *C.char
	if contentType != "" {
		cType = C.CString(contentType)
		defer C.free(unsafe.Pointer(cType))
	}
	var bodyPtr unsafe.Pointer
	if len(body) > 0 {
		bodyPtr = C.CBytes(body)
		defer C.free(bodyPtr)
	}
	extra, release := buildExtraHeaders(headers)
	defer release()
	rc := C.http_post(cHost, C.int(port), cPath, cType, (*C.char)(bodyPtr), C.size_t(len(body)), extra, (*C.char)(unsafe.Pointer(&buf[0])), C.size_t(len(buf)))
	if rc != 0 {
		return "", errors.New("http post failed")
	}
	return goStringFromBuffer(buf), nil
}

func HTTPPut(host string, port int, path, contentType string, body []byte) (string, error) {
	return HTTPPutWithHeaders(host, port, path, contentType, body, nil)
}

func HTTPPutWithHeaders(host string, port int, path, contentType string, body []byte, headers map[string]string) (string, error) {
	buf := make([]byte, 128*1024)
	cHost := C.CString(host)
	cPath := C.CString(path)
	defer C.free(unsafe.Pointer(cHost))
	defer C.free(unsafe.Pointer(cPath))
	var cType *C.char
	if contentType != "" {
		cType = C.CString(contentType)
		defer C.free(unsafe.Pointer(cType))
	}
	var bodyPtr unsafe.Pointer
	if len(body) > 0 {
		bodyPtr = C.CBytes(body)
		defer C.free(bodyPtr)
	}
	extra, release := buildExtraHeaders(headers)
	defer release()
	rc := C.http_put(cHost, C.int(port), cPath, cType, (*C.char)(bodyPtr), C.size_t(len(body)), extra, (*C.char)(unsafe.Pointer(&buf[0])), C.size_t(len(buf)))
	if rc != 0 {
		return "", errors.New("http put failed")
	}
	return goStringFromBuffer(buf), nil
}

func HTTPUploadFile(host string, port int, path, filepath string) (string, error) {
	return HTTPUploadFileWithHeaders(host, port, path, filepath, nil)
}

func HTTPUploadFileWithHeaders(host string, port int, path, filepath string, headers map[string]string) (string, error) {
	buf := make([]byte, 128*1024)
	cHost := C.CString(host)
	cPath := C.CString(path)
	cFile := C.CString(filepath)
	defer C.free(unsafe.Pointer(cHost))
	defer C.free(unsafe.Pointer(cPath))
	defer C.free(unsafe.Pointer(cFile))
	extra, release := buildExtraHeaders(headers)
	defer release()
	if C.http_post_file(cHost, C.int(port), cPath, cFile, (*C.char)(nil), (*C.char)(nil), extra, (*C.char)(unsafe.Pointer(&buf[0])), C.size_t(len(buf))) != 0 {
		return "", errors.New("http upload failed")
	}
	return goStringFromBuffer(buf), nil
}

func SendTokenHeaders(c *TCPClient, headers map[string]string) error {
	extra, release := buildExtraHeaders(headers)
	defer release()
	builder := strings.Builder{}
	builder.WriteString("TOKEN-HEADERS\r\n")
	if extra != nil {
		builder.WriteString(C.GoString(extra))
	}
	builder.WriteString("\r\n")
	if _, err := c.Write([]byte(builder.String())); err != nil {
		return err
	}
	return nil
}

func ReadTokenHeaders(c *TCPClient) (map[string]string, []byte, error) {
	headers := make(map[string]string)
	buf := make([]byte, 4096)
	total := 0
	for total < len(buf) {
		n, err := c.Read(buf[total:])
		if n > 0 {
			total += n
			if idx := bytes.Index(buf[:total], []byte("\r\n\r\n")); idx != -1 {
				lines := bytes.Split(buf[:idx], []byte("\r\n"))
				if len(lines) > 0 && strings.EqualFold(string(lines[0]), "TOKEN-HEADERS") {
					lines = lines[1:]
				}
				for _, line := range lines {
					kv := bytes.SplitN(line, []byte(":"), 2)
					if len(kv) == 2 {
						headers[strings.ToLower(strings.TrimSpace(string(kv[0])))] = strings.TrimSpace(string(kv[1]))
					}
				}
				remaining := append([]byte(nil), buf[idx+4:total]...)
				return headers, remaining, nil
			}
		}
		if err != nil {
			return nil, nil, err
		}
		if n == 0 {
			break
		}
	}
	return headers, nil, nil
}

func goStringFromBuffer(b []byte) string {
	n := 0
	for n < len(b) && b[n] != 0 {
		n++
	}
	return string(b[:n])
}
