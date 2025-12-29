package socket

import (
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
	byID map[string]*clientConn // keep for listing online devices; send uses C registry
}

func NewHub() *Hub { return &Hub{byID: make(map[string]*clientConn)} }

func (h *Hub) Register(deviceID string, c *network.TCPClient) {
	h.mu.Lock()
	h.byID[deviceID] = &clientConn{c: c}
	h.mu.Unlock()
}

func (h *Hub) Unregister(deviceID string, c *network.TCPClient) {
	h.mu.Lock()
	if cur, ok := h.byID[deviceID]; ok && cur.c.Equal(c) {
		delete(h.byID, deviceID)
	}
	h.mu.Unlock()
}

func (h *Hub) IsOnline(deviceID string) bool {
	if network.DeviceIsOnline(deviceID) {
		return true
	}
	h.mu.RLock()
	_, ok := h.byID[deviceID]
	h.mu.RUnlock()
	return ok
}

// OnlineDevices trả về danh sách tất cả device đang online.
func (h *Hub) OnlineDevices() []string {
	h.mu.RLock()
	out := make([]string, 0, len(h.byID))
	for id := range h.byID {
		if network.DeviceIsOnline(id) {
			out = append(out, id)
		}
	}
	h.mu.RUnlock()

	// Debug log summary
	global.Logger.Debug().
		Int("cached", len(h.byID)).
		Int("online", len(out)).
		Strs("online_ids", out).
		Msg("hub online devices")

	return out
}

func (h *Hub) Send(deviceID string, data []byte) error {
	global.Logger.Info().Str("device", deviceID).Str("data", string(data)).Msg("Sending data to device")
	if err := network.SendToDevice(deviceID, data); err != nil {
		// fallback: if we still have a TCPClient cached, try once
		h.mu.RLock()
		cc, ok := h.byID[deviceID]
		h.mu.RUnlock()
		if ok && cc != nil && cc.c != nil && cc.c.IsOpen() {
			cc.mu.Lock()
			err2 := cc.c.SendCommand(data)
			cc.mu.Unlock()
			if err2 == nil {
				global.Logger.Info().Str("device", deviceID).Str("data", string(data)).Msg("Sent data via cached client")
				return nil
			}
		}
		return err
	}
	global.Logger.Info().Str("device", deviceID).Str("data", string(data)).Msg("Sent data to device")
	return nil
}
