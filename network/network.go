package network

/*
#cgo CFLAGS: -I${SRCDIR}
// SỬA LỖI: Thêm -lws2_32 để link thư viện Winsock trên Windows
#cgo LDFLAGS: -L${SRCDIR} -lnetwork -lws2_32 -lwinhttp
#include <stdlib.h>
#include "network.h"
*/
import "C"

import (
	"bytes"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"unsafe"
)

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
	ptr := C.CString(builder.String())
	return ptr, func() {
		if ptr != nil {
			C.free(unsafe.Pointer(ptr))
		}
	}
}

// httpDoTLS performs HTTPS requests using Go's net/http for TLS support.
func httpDoTLS(method string, host string, port int, path string, contentType string, body []byte, headers map[string]string) (string, error) {
	url := "https://" + host
	if port != 443 {
		url += fmt.Sprintf(":%d", port)
	}
	if !strings.HasPrefix(path, "/") {
		url += "/" + path
	} else {
		url += path
	}
	var reqBody io.Reader
	if len(body) > 0 {
		reqBody = bytes.NewReader(body)
	}
	req, err := http.NewRequest(method, url, reqBody)
	if err != nil {
		return "", err
	}
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	client := &http.Client{Transport: &http.Transport{TLSClientConfig: &tls.Config{MinVersion: tls.VersionTLS12}}}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	b, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	respStr := goStringFromBuffer(b)
	respStr = extractHTTPBody(strings.TrimSpace(respStr))
	return respStr, nil
}

func HTTPGet(host string, port int, path string) (string, error) {
	return HTTPGetWithHeaders(host, port, path, nil)
}

func HTTPGetWithHeaders(host string, port int, path string, headers map[string]string) (string, error) {
	if port == 443 {
		buf := make([]byte, 128*1024)
		extra, release := buildExtraHeaders(headers)
		defer release()
		if C.https_get(C.CString(host), C.int(port), C.CString(path), extra, (*C.char)(unsafe.Pointer(&buf[0])), C.size_t(len(buf))) != 0 {
			return "", errors.New("https get failed")
		}
		resp := goStringFromBuffer(buf)
		resp = extractHTTPBody(strings.TrimSpace(resp))
		return resp, nil
	}
	buf := make([]byte, 128*1024)
	cHost := C.CString(host)
	cPath := C.CString(path)
	defer C.free(unsafe.Pointer(cHost))
	defer C.free(unsafe.Pointer(cPath))
	extra, release := buildExtraHeaders(headers)
	defer release()
	// C.http_get trả về 0 nếu thành công, -1 nếu thất bại.
	if C.http_get(cHost, C.int(port), cPath, extra, (*C.char)(unsafe.Pointer(&buf[0])), C.size_t(len(buf))) != 0 {
		return "", errors.New("http get failed")
	}
	resp := goStringFromBuffer(buf)
	resp = extractHTTPBody(strings.TrimSpace(resp))
	return resp, nil
}

func HTTPDelete(host string, port int, path string) (string, error) {
	return HTTPDeleteWithHeaders(host, port, path, nil)
}

func HTTPDeleteWithHeaders(host string, port int, path string, headers map[string]string) (string, error) {
	if port == 443 {
		buf := make([]byte, 128*1024)
		extra, release := buildExtraHeaders(headers)
		defer release()
		if C.https_delete(C.CString(host), C.int(port), C.CString(path), extra, (*C.char)(unsafe.Pointer(&buf[0])), C.size_t(len(buf))) != 0 {
			return "", errors.New("https delete failed")
		}
		resp := goStringFromBuffer(buf)
		resp = extractHTTPBody(strings.TrimSpace(resp))
		return resp, nil
	}
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
	resp := goStringFromBuffer(buf)
	resp = extractHTTPBody(strings.TrimSpace(resp))
	return resp, nil
}

func HTTPPost(host string, port int, path, contentType string, body []byte) (string, error) {
	return HTTPPostWithHeaders(host, port, path, contentType, body, nil)
}

func HTTPPostWithHeaders(host string, port int, path, contentType string, body []byte, headers map[string]string) (string, error) {
	if port == 443 {
		buf := make([]byte, 128*1024)
		extra, release := buildExtraHeaders(headers)
		defer release()
		var cType *C.char
		if contentType != "" {
			cType = C.CString(contentType)
			defer C.free(unsafe.Pointer(cType))
		}
		var bodyPtr *C.char
		if len(body) > 0 {
			bodyPtr = (*C.char)(C.CBytes(body))
			defer C.free(unsafe.Pointer(bodyPtr))
		}
		if C.https_post(C.CString(host), C.int(port), C.CString(path), cType, bodyPtr, C.size_t(len(body)), extra, (*C.char)(unsafe.Pointer(&buf[0])), C.size_t(len(buf))) != 0 {
			return "", errors.New("https post failed")
		}
		resp := goStringFromBuffer(buf)
		resp = extractHTTPBody(strings.TrimSpace(resp))
		return resp, nil
	}
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
		// Dùng C.CBytes để cấp phát bộ nhớ C và sao chép dữ liệu Go sang
		bodyPtr = C.CBytes(body)
		defer C.free(bodyPtr)
	}
	extra, release := buildExtraHeaders(headers)
	defer release()
	rc := C.http_post(cHost, C.int(port), cPath, cType, (*C.char)(bodyPtr), C.size_t(len(body)), extra, (*C.char)(unsafe.Pointer(&buf[0])), C.size_t(len(buf)))
	if rc != 0 {
		return "", errors.New("http post failed")
	}
	resp := goStringFromBuffer(buf)
	resp = extractHTTPBody(strings.TrimSpace(resp))
	return resp, nil
}

func HTTPPut(host string, port int, path, contentType string, body []byte) (string, error) {
	return HTTPPutWithHeaders(host, port, path, contentType, body, nil)
}

func HTTPPutWithHeaders(host string, port int, path, contentType string, body []byte, headers map[string]string) (string, error) {
	if port == 443 {
		buf := make([]byte, 128*1024)
		extra, release := buildExtraHeaders(headers)
		defer release()
		var cType *C.char
		if contentType != "" {
			cType = C.CString(contentType)
			defer C.free(unsafe.Pointer(cType))
		}
		var bodyPtr *C.char
		if len(body) > 0 {
			bodyPtr = (*C.char)(C.CBytes(body))
			defer C.free(unsafe.Pointer(bodyPtr))
		}
		if C.https_put(C.CString(host), C.int(port), C.CString(path), cType, bodyPtr, C.size_t(len(body)), extra, (*C.char)(unsafe.Pointer(&buf[0])), C.size_t(len(buf))) != 0 {
			return "", errors.New("https put failed")
		}
		resp := goStringFromBuffer(buf)
		resp = extractHTTPBody(strings.TrimSpace(resp))
		return resp, nil
	}
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
	resp := goStringFromBuffer(buf)
	resp = extractHTTPBody(strings.TrimSpace(resp))
	return resp, nil
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
