package socket

import (
	"errors"
	"sagiri-guard/backend/global"
	"sagiri-guard/network"
	"sync"
)

type clientConn struct {
	c  *network.TCPClient
	mu sync.Mutex
}

type Hub struct {
	mu   sync.RWMutex
	byID map[string]*clientConn
}

func NewHub() *Hub { return &Hub{byID: make(map[string]*clientConn)} }

func (h *Hub) Register(deviceID string, c *network.TCPClient) {
	h.mu.Lock()
	h.byID[deviceID] = &clientConn{c: c}
	h.mu.Unlock()
}

func (h *Hub) Unregister(deviceID string, c *network.TCPClient) {
	h.mu.Lock()
	if cur, ok := h.byID[deviceID]; ok && cur.c == c {
		delete(h.byID, deviceID)
	}
	h.mu.Unlock()
}

func (h *Hub) IsOnline(deviceID string) bool {
	h.mu.RLock()
	_, ok := h.byID[deviceID]
	h.mu.RUnlock()
	return ok
}

// OnlineDevices trả về danh sách tất cả device đang online.
func (h *Hub) OnlineDevices() []string {
	h.mu.RLock()
	defer h.mu.RUnlock()
	out := make([]string, 0, len(h.byID))
	for id := range h.byID {
		out = append(out, id)
	}
	return out
}

func (h *Hub) Send(deviceID string, data []byte) error {
	h.mu.RLock()
	cc, ok := h.byID[deviceID]
	global.Logger.Info().Str("device", deviceID).Str("data", string(data)).Msg("Sending data to device")
	h.mu.RUnlock()
	if !ok {
		return errors.New("device offline")
	}
	cc.mu.Lock()
	defer cc.mu.Unlock()
	_, err := cc.c.Write(data)
	global.Logger.Info().Str("device", deviceID).Str("data", string(data)).Msg("Sent data to device")
	return err
}
