package server

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sagiri-guard/backend/global"
	"sagiri-guard/network"
	"strconv"
	"strings"
	"time"
)

// StartHTTPServerC starts an HTTP server using the cgo-based TCP stack from the network package.
// It adapts the provided http.Handler so existing routers can be reused.
func StartHTTPServerC(host string, port int, handler http.Handler) error {
	srv, err := network.ListenTCP(host, port)
	if err != nil {
		return err
	}
	go func() {
		for {
			c, err := srv.Accept()
			if err != nil {
				continue
			}
			go serveConn(handler, c)
		}
	}()
	return nil
}

type tcpResponseWriter struct {
	hdr        http.Header
	statusCode int
	body       bytes.Buffer
}

func newTCPResponseWriter() *tcpResponseWriter {
	return &tcpResponseWriter{hdr: make(http.Header), statusCode: http.StatusOK}
}

func (w *tcpResponseWriter) Header() http.Header         { return w.hdr }
func (w *tcpResponseWriter) Write(b []byte) (int, error) { return w.body.Write(b) }
func (w *tcpResponseWriter) WriteHeader(statusCode int)  { w.statusCode = statusCode }

func serveConn(handler http.Handler, c *network.TCPClient) {
	defer c.Close()
	// Read headers
	headerBuf := make([]byte, 0, 8192)
	tmp := make([]byte, 2048)
	for {
		n, err := c.Read(tmp)
		if n > 0 {
			headerBuf = append(headerBuf, tmp[:n]...)
		}
		if err != nil {
			return
		}
		if bytes.Contains(headerBuf, []byte("\r\n\r\n")) {
			break
		}
		if len(headerBuf) > 128*1024 {
			return
		}
	}
	parts := bytes.SplitN(headerBuf, []byte("\r\n\r\n"), 2)
	head := string(parts[0])
	bodyRemainder := []byte{}
	if len(parts) == 2 {
		bodyRemainder = parts[1]
	}


	reader := bufio.NewReader(strings.NewReader(head))
	// Request line
	reqLine, _ := reader.ReadString('\n')
	reqLine = strings.TrimSpace(reqLine)
	rl := strings.SplitN(reqLine, " ", 3)
	if len(rl) < 2 {
		return
	}
	method, target := rl[0], rl[1]
	now := time.Now()
	global.Logger.Info().
		Str("method", method).
		Str("path", target).
		Msg("http_cgo: incoming")
	fmt.Printf("%s | --- |          | RECV  %-6s %s\n",
		now.Format("2006/01/02 15:04:05"),
		method,
		target)

	// Headers - parse by splitting on \r\n instead of reading line by line
	hdr := make(http.Header)
	headerLines := strings.Split(head, "\r\n")
	// Skip first line (request line) and last empty line
	for i := 1; i < len(headerLines); i++ {
		line := strings.TrimSpace(headerLines[i])
		if line == "" {
			break
		}
		kv := strings.SplitN(line, ":", 2)
		if len(kv) == 2 {
			key := strings.TrimSpace(kv[0])
			value := strings.TrimSpace(kv[1])
			hdr.Add(key, value)
		}
	}
	// Body
	var body []byte
	want := 0
	if cl := hdr.Get("Content-Length"); cl != "" {
		if v, err := strconv.Atoi(cl); err == nil {
			want = v
		}
	}
	if want > 0 {
		body = make([]byte, want)
		copyN := copy(body, bodyRemainder)
		read := copyN
		for read < want {
			n, err := c.Read(body[read:])
			if err != nil {
				return
			}
			read += n
		}
	} else {
		body = bodyRemainder
	}

	// Build http.Request
	u, _ := url.ParseRequestURI(target)
	req := &http.Request{
		Method:        method,
		URL:           u,
		Header:        hdr,
		Body:          io.NopCloser(bytes.NewReader(body)),
		ContentLength: int64(len(body)),
	}

	rw := newTCPResponseWriter()
	start := time.Now()
	handler.ServeHTTP(rw, req)
	duration := time.Since(start)
	global.Logger.Info().
		Str("method", method).
		Str("path", target).
		Int("status", rw.statusCode).
		Dur("duration", duration).
		Msg("http_cgo")
	fmt.Printf("%s | %3d | %10s | %-6s %s\n",
		time.Now().Format("2006/01/02 15:04:05"),
		rw.statusCode,
		duration,
		method,
		target)

	// Write response
	if rw.hdr.Get("Content-Type") == "" {
		rw.hdr.Set("Content-Type", "text/plain; charset=utf-8")
	}
	rw.hdr.Set("Content-Length", strconv.Itoa(rw.body.Len()))
	statusText := http.StatusText(rw.statusCode)
	if statusText == "" {
		statusText = "OK"
	}
	var out bytes.Buffer
	fmt.Fprintf(&out, "HTTP/1.1 %d %s\r\n", rw.statusCode, statusText)
	for k, vals := range rw.hdr {
		for _, v := range vals {
			fmt.Fprintf(&out, "%s: %s\r\n", k, v)
		}
	}
	out.WriteString("\r\n")
	out.Write(rw.body.Bytes())
	_, _ = c.Write(out.Bytes())
}
