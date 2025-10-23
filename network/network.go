package network

/*
#cgo CFLAGS: -I${SRCDIR}
// SỬA LỖI: Thêm -lws2_32 để link thư viện Winsock trên Windows
#cgo LDFLAGS: -L${SRCDIR} -lnetwork -lws2_32
#include <stdlib.h>
#include "network.h"
*/
import "C"

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
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
	stopDemoServers()
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

type demoHTTPServer struct {
	addr      string
	listener  net.Listener
	server    *http.Server
	shutdownW sync.WaitGroup
}

type demoTCPServer struct {
	addr     string
	listener net.Listener
	stopOnce sync.Once
	stopCh   chan struct{}
	clients  sync.WaitGroup
}

var (
	serversMu sync.Mutex
	demoHTTP  *demoHTTPServer
	demoTCP   *demoTCPServer
)

// EnsureDemoServers spins up light-weight demo HTTP and TCP servers if they are not
// already running. This allows the example client helpers in this package to talk
// to predictable endpoints without requiring the caller to run another process.
func EnsureDemoServers(host string, httpPort, tcpPort int) error {
	if host == "" {
		host = "127.0.0.1"
	}
	if httpPort <= 0 {
		return errors.New("invalid http port")
	}
	if tcpPort <= 0 {
		return errors.New("invalid tcp port")
	}

	serversMu.Lock()
	defer serversMu.Unlock()

	if demoHTTP == nil {
		if err := startDemoHTTPServerLocked(host, httpPort); err != nil {
			return err
		}
	}
	if demoTCP == nil {
		if err := startDemoTCPServerLocked(host, tcpPort); err != nil {
			return err
		}
	}
	return nil
}

func startDemoHTTPServerLocked(host string, port int) error {
	mux := http.NewServeMux()
	mux.HandleFunc("/ping", func(w http.ResponseWriter, r *http.Request) {
		setDemoHeaders(w)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("pong"))
	})
	mux.HandleFunc("/echo", func(w http.ResponseWriter, r *http.Request) {
		setDemoHeaders(w)
		body, err := io.ReadAll(r.Body)
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte("failed to read body"))
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(body)
	})
	mux.HandleFunc("/update", func(w http.ResponseWriter, r *http.Request) {
		setDemoHeaders(w)
		if err := r.ParseForm(); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte("invalid form"))
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("updated"))
	})
	mux.HandleFunc("/resource", func(w http.ResponseWriter, r *http.Request) {
		setDemoHeaders(w)
		if r.Method == http.MethodDelete {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		w.WriteHeader(http.StatusMethodNotAllowed)
	})
	mux.HandleFunc("/upload", func(w http.ResponseWriter, r *http.Request) {
		setDemoHeaders(w)
		if err := r.ParseMultipartForm(10 << 20); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte("invalid multipart form"))
			return
		}
		file, header, err := r.FormFile("file")
		if err != nil {
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte("missing file"))
			return
		}
		defer file.Close()
		size, _ := io.Copy(io.Discard, file)
		w.WriteHeader(http.StatusCreated)
		_, _ = fmt.Fprintf(w, "received %s (%d bytes)", header.Filename, size)
	})

	addr := net.JoinHostPort(host, strconv.Itoa(port))
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return err
	}
	server := &http.Server{Handler: mux}
	state := &demoHTTPServer{
		addr:     addr,
		listener: ln,
		server:   server,
	}
	state.shutdownW.Add(1)
	go func() {
		defer state.shutdownW.Done()
		_ = server.Serve(ln)
	}()
	demoHTTP = state
	return nil
}

func startDemoTCPServerLocked(host string, port int) error {
	addr := net.JoinHostPort(host, strconv.Itoa(port))
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return err
	}
	state := &demoTCPServer{
		addr:     addr,
		listener: ln,
		stopCh:   make(chan struct{}),
	}
	go state.run()
	demoTCP = state
	return nil
}

func (s *demoTCPServer) run() {
	for {
		conn, err := s.listener.Accept()
		if err != nil {
			select {
			case <-s.stopCh:
				return
			default:
			}
			continue
		}
		s.clients.Add(1)
		go s.handle(conn)
	}
}

func (s *demoTCPServer) handle(conn net.Conn) {
	defer s.clients.Done()
	defer conn.Close()
	reader := bufio.NewReader(conn)
	headers := make(map[string]string)
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			return
		}
		line = strings.TrimRight(line, "\r\n")
		if line == "" {
			break
		}
		parts := strings.SplitN(line, ":", 2)
		if len(parts) == 2 {
			headers[strings.ToLower(strings.TrimSpace(parts[0]))] = strings.TrimSpace(parts[1])
		}
	}

	for {
		conn.SetReadDeadline(time.Now().Add(30 * time.Second))
		line, err := reader.ReadString('\n')
		if err != nil {
			return
		}
		response := fmt.Sprintf("ACK: %s", line)
		if _, err := conn.Write([]byte(response)); err != nil {
			return
		}
	}
}

func stopDemoServers() {
	serversMu.Lock()
	defer serversMu.Unlock()
	if demoHTTP != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		_ = demoHTTP.server.Shutdown(ctx)
		cancel()
		demoHTTP.shutdownW.Wait()
		demoHTTP = nil
	}
	if demoTCP != nil {
		demoTCP.stopOnce.Do(func() {
			close(demoTCP.stopCh)
			_ = demoTCP.listener.Close()
		})
		demoTCP.clients.Wait()
		demoTCP = nil
	}
}

func setDemoHeaders(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("X-Demo-Server", "sagiri-guard")
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

// UDPConn wraps an optional connected UDP socket.
type UDPConn struct {
	// SỬA LỖI: Dùng C.SOCKET
	fd C.SOCKET
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
	// SỬA LỖI: Kiểm tra C.INVALID_SOCKET
	if fd == C.INVALID_SOCKET {
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
	// SỬA LỖI: Kiểm tra C.INVALID_SOCKET
	if fd == C.INVALID_SOCKET {
		return nil, errors.New("udp connect failed")
	}
	return &UDPConn{fd: fd}, nil
}

func (u *UDPConn) Close() error {
	// SỬA LỖI: Kiểm tra C.INVALID_SOCKET
	if u == nil || u.fd == C.INVALID_SOCKET {
		return errors.New("conn not open")
	}
	if C.udp_close(u.fd) != 0 {
		return errors.New("close failed")
	}
	// SỬA LỖI: Gán C.INVALID_SOCKET
	u.fd = C.INVALID_SOCKET
	return nil
}

func (u *UDPConn) WriteTo(data []byte, host string, port int) (int, error) {
	// SỬA LỖI: Kiểm tra C.INVALID_SOCKET
	if u == nil || u.fd == C.INVALID_SOCKET {
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
	// SỬA LỖI: Kiểm tra C.INVALID_SOCKET
	if u == nil || u.fd == C.INVALID_SOCKET {
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
	// C.http_get trả về 0 nếu thành công, -1 nếu thất bại.
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
