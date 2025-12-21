package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"sagiri-guard/network"

	webview "github.com/webview/webview_go"
)

type commandRequest struct {
	DeviceID string `json:"device_id"`
	Token    string `json:"token,omitempty"`
	Action   string `json:"action"`
	Data     string `json:"data"` // raw JSON string
}

type commandResponse struct {
	OK         bool   `json:"ok"`
	StatusCode uint16 `json:"status_code"`
	StatusMsg  string `json:"status_msg"`
	Error      string `json:"error,omitempty"`
	Log        string `json:"log,omitempty"` // debug info
}

// sendCommand opens a protocol connection, optionally sends login frame with token,
// then sends MsgCommand with sub-command JSON payload, and waits for the first response.
func sendCommand(ctx context.Context, host string, port int, req commandRequest) commandResponse {
	if req.Action == "" {
		return commandResponse{Error: "action is required"}
	}
	var data json.RawMessage
	if req.Data != "" {
		if err := json.Unmarshal([]byte(req.Data), &data); err != nil {
			return commandResponse{Error: fmt.Sprintf("invalid data JSON: %v", err)}
		}
	}
	payload := map[string]any{
		"action": req.Action,
		"data":   data,
	}
	b, err := json.Marshal(payload)
	if err != nil {
		return commandResponse{Error: fmt.Sprintf("marshal payload: %v", err)}
	}

	client, err := network.DialTCP(host, port)
	if err != nil {
		return commandResponse{Error: fmt.Sprintf("dial: %v", err)}
	}
	defer client.Close()

	// optional login frame with token to authorize subsequent command
	if req.Token != "" && req.DeviceID != "" {
		if err := client.SendLogin(req.DeviceID, req.Token); err != nil {
			return commandResponse{Error: fmt.Sprintf("send login: %v", err)}
		}
	}

	if err := client.SendCommand(b); err != nil {
		return commandResponse{Error: fmt.Sprintf("send command: %v", err)}
	}

	type result struct {
		resp *network.ProtocolMessage
		err  error
	}
	ch := make(chan result, 1)
	go func() {
		msg, er := client.RecvProtocolMessage()
		ch <- result{resp: msg, err: er}
	}()

	select {
	case <-ctx.Done():
		return commandResponse{Error: "timeout waiting for response", Log: "context deadline exceeded"}
	case res := <-ch:
		if res.err != nil {
			return commandResponse{Error: fmt.Sprintf("recv: %v", res.err), Log: "RecvProtocolMessage failed"}
		}
		return commandResponse{
			OK:         res.resp.Type == network.MsgAck && res.resp.StatusCode < 300,
			StatusCode: res.resp.StatusCode,
			StatusMsg:  res.resp.StatusMsg,
			Log:        fmt.Sprintf("recv type=%d len=%d", res.resp.Type, len(res.resp.Raw)),
		}
	}
}

func main() {
	host := flag.String("host", "127.0.0.1", "Protocol server host")
	port := flag.Int("port", 9200, "Protocol server port")
	timeout := flag.Duration("timeout", 15*time.Second, "Command timeout")
	dist := flag.String("dist", filepath.Join("frontend", "dist", "index.html"), "Path to frontend dist index.html (built by Vite)")
	flag.Parse()

	w := webview.New(true)
	defer w.Destroy()
	w.SetTitle("Sagiri Protocol Console")
	w.SetSize(900, 720, webview.Hint(webview.HintNone))

	// Expose sendCommand to JS
	w.Bind("sendCommand", func(hostJS string, portJS int, req commandRequest) commandResponse {
		ctx, cancel := context.WithTimeout(context.Background(), *timeout)
		defer cancel()

		// fallbacks if UI leaves host/port empty
		h := hostJS
		if h == "" {
			h = *host
		}
		p := portJS
		if p == 0 {
			p = *port
		}
		return sendCommand(ctx, h, p, req)
	})

	absDist, err := filepath.Abs(*dist)
	if err != nil {
		log.Fatalf("resolve dist path: %v", err)
	}
	info, err := os.Stat(absDist)
	if err != nil {
		log.Fatalf("dist file not found: %s (build frontend first: npm run build)", absDist)
	}

	// Serve dist directory over local HTTP to avoid file:// origin issues
	distDir := absDist
	if !info.IsDir() {
		distDir = filepath.Dir(absDist)
	}
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		log.Fatalf("listen: %v", err)
	}
	srv := &http.Server{
		Handler: http.FileServer(http.Dir(distDir)),
	}
	go func() {
		_ = srv.Serve(ln)
	}()
	defer srv.Close()

	w.Navigate("http://" + ln.Addr().String() + "/")
	log.Println("UI ready at embedded webview (no HTTP server).")
	w.Run()
}
