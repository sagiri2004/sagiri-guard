package network

/*
#cgo CFLAGS: -I${SRCDIR}
// SỬA LỖI: Thêm -lws2_32 để link thư viện Winsock trên Windows
#cgo LDFLAGS: -L${SRCDIR} -lnetwork -lws2_32
#include <stdlib.h>
#include <stddef.h>
extern int http_request(const char* host, int port, const char* method, const char* path,
                        const char* content_type, const char* body, size_t body_len,
                        const char* extra_headers, char* response, size_t response_len);
#include "network.h"
*/
import "C"

import (
	"bytes"
	"errors"
	"strconv"
	"strings"
	"unsafe"
)

var errHTTPSUnsupported = errors.New("https not supported in local mode")

func httpRequest(method, host string, port int, path, contentType string, body []byte, headers map[string]string) (int, string, error) {
	if port == 443 {
		return 0, "", errHTTPSUnsupported
	}
	if host == "" {
		return 0, "", errors.New("host is required")
	}
	if path == "" {
		path = "/"
	}

	buf := make([]byte, 128*1024)

	cHost := C.CString(host)
	defer C.free(unsafe.Pointer(cHost))
	cMethod := C.CString(strings.ToUpper(method))
	defer C.free(unsafe.Pointer(cMethod))
	cPath := C.CString(path)
	defer C.free(unsafe.Pointer(cPath))

	var cType *C.char
	if contentType != "" {
		cType = C.CString(contentType)
		defer C.free(unsafe.Pointer(cType))
	}

	var cBody *C.char
	var bodyLen C.size_t
	if len(body) > 0 {
		tmp := C.CBytes(body)
		cBody = (*C.char)(tmp)
		bodyLen = C.size_t(len(body))
		defer C.free(tmp)
	}

	extra, release := buildExtraHeaders(headers)
	defer release()

	rc := C.http_request(
		cHost,
		C.int(port),
		cMethod,
		cPath,
		cType,
		cBody,
		bodyLen,
		extra,
		(*C.char)(unsafe.Pointer(&buf[0])),
		C.size_t(len(buf)),
	)
	if rc != 0 {
		return 0, "", errors.New("http request failed")
	}

	raw := goStringFromBuffer(buf)
	status, resp := parseStatusAndBody(strings.TrimSpace(raw))
	return status, resp, nil
}

// Init initializes the C networking library (WSAStartup on Windows).
// This MUST be called once at the start of the application.
func Init() error {
	if C.network_init() != 0 {
		return errors.New("failed to initialize C networking library (WSAStartup)")
	}
	return nil
}

// Cleanup cleans up the C networking library (WSACleanup on Windows).
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

func (c *TCPClient) Write(data []byte) (int, error) {
	// SỬA LỖI: Kiểm tra C.INVALID_SOCKET
	if c == nil || c.fd == C.INVALID_SOCKET {
		return 0, errors.New("client not open")
	}
	if len(data) == 0 {
		return 0, nil
	}
	// C.tcp_send trả về C.ssize_t (mà Go hiểu là C.longlong hoặc C.long)
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

// HTTP helpers (Các hàm này không cần sửa vì chúng không lưu trữ fd)
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
	result := builder.String()
	if result == "" {
		return nil, func() {}
	}
	ptr := C.CString(result)
	return ptr, func() {
		if ptr != nil {
			C.free(unsafe.Pointer(ptr))
		}
	}
}

func HTTPGet(host string, port int, path string) (string, error) {
	_, body, err := HTTPRequest("GET", host, port, path, "", nil, nil)
	return body, err
}

func HTTPGetWithHeaders(host string, port int, path string, headers map[string]string) (string, error) {
	_, body, err := HTTPRequest("GET", host, port, path, "", nil, headers)
	return body, err
}

func HTTPGetWithHeadersEx(host string, port int, path string, headers map[string]string) (int, string, error) {
	return httpRequest("GET", host, port, path, "", nil, headers)
}

func HTTPDelete(host string, port int, path string) (string, error) {
	_, body, err := HTTPRequest("DELETE", host, port, path, "", nil, nil)
	return body, err
}

func HTTPDeleteWithHeaders(host string, port int, path string, headers map[string]string) (string, error) {
	_, body, err := HTTPRequest("DELETE", host, port, path, "", nil, headers)
	return body, err
}

func HTTPDeleteWithHeadersEx(host string, port int, path string, headers map[string]string) (int, string, error) {
	return httpRequest("DELETE", host, port, path, "", nil, headers)
}

func HTTPPost(host string, port int, path, contentType string, body []byte) (string, error) {
	_, resp, err := HTTPRequest("POST", host, port, path, contentType, body, nil)
	return resp, err
}

func HTTPPostWithHeaders(host string, port int, path, contentType string, body []byte, headers map[string]string) (string, error) {
	_, resp, err := HTTPRequest("POST", host, port, path, contentType, body, headers)
	return resp, err
}

func HTTPPostWithHeadersEx(host string, port int, path, contentType string, body []byte, headers map[string]string) (int, string, error) {
	return httpRequest("POST", host, port, path, contentType, body, headers)
}

func HTTPPut(host string, port int, path, contentType string, body []byte) (string, error) {
	_, resp, err := HTTPRequest("PUT", host, port, path, contentType, body, nil)
	return resp, err
}

func HTTPPutWithHeaders(host string, port int, path, contentType string, body []byte, headers map[string]string) (string, error) {
	_, resp, err := HTTPRequest("PUT", host, port, path, contentType, body, headers)
	return resp, err
}

func HTTPPutWithHeadersEx(host string, port int, path, contentType string, body []byte, headers map[string]string) (int, string, error) {
	return httpRequest("PUT", host, port, path, contentType, body, headers)
}

// HTTPRequest exposes the unified HTTP helper so callers có thể truyền method/path tuỳ ý.
func HTTPRequest(method, host string, port int, path, contentType string, body []byte, headers map[string]string) (int, string, error) {
	return httpRequest(method, host, port, path, contentType, body, headers)
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

func parseStatusAndBody(raw string) (int, string) {
	if strings.HasPrefix(raw, "HTTP/") {
		lineEnd := strings.Index(raw, "\r\n")
		if lineEnd == -1 {
			lineEnd = strings.Index(raw, "\n")
		}
		if lineEnd > 0 {
			fields := strings.Fields(raw[:lineEnd])
			if len(fields) >= 2 {
				if code, err := strconv.Atoi(fields[1]); err == nil {
					return code, extractHTTPBody(raw)
				}
			}
		}
		return 0, extractHTTPBody(raw)
	}
	return 200, raw
}

// extractHTTPBody returns body if raw is a full HTTP response, otherwise returns raw.
func extractHTTPBody(raw string) string {
	if strings.HasPrefix(raw, "HTTP/") {
		if idx := strings.Index(raw, "\r\n\r\n"); idx != -1 {
			return raw[idx+4:]
		}
		if idx := strings.Index(raw, "\n\n"); idx != -1 {
			return raw[idx+2:]
		}
	}
	return raw
}
