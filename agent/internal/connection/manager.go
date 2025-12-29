package connection

import (
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"sagiri-guard/agent/internal/logger"
	"sagiri-guard/agent/internal/socket"
	"sagiri-guard/network"
)

// Manager manages a single persistent TCP connection to the backend
type Manager struct {
	host     string
	port     int
	deviceID string
	token    string

	client *network.TCPClient
	mu     sync.Mutex

	// Channels for graceful shutdown
	stopCh chan struct{}
	doneCh chan struct{}
}

// New creates a new connection manager
func New(host string, port int, deviceID, token string) *Manager {
	return &Manager{
		host:     host,
		port:     port,
		deviceID: deviceID,
		token:    token,
		stopCh:   make(chan struct{}),
		doneCh:   make(chan struct{}),
	}
}

// Connect establishes the persistent connection with retry logic
func (m *Manager) Connect(maxRetries int, baseDelay time.Duration) error {
	const (
		maxDelay      = 30 * time.Second
		backoffFactor = 1.5
	)

	var retryCount int
	var delay time.Duration = baseDelay

	for {
		logger.Infof("Agent is trying to connect to backend %s:%d (attempt #%d)...", m.host, m.port, retryCount+1)

		client, err := network.DialTCP(m.host, m.port)
		if err != nil {
			logger.Errorf("Agent cannot connect to backend (attempt #%d): %v", retryCount+1, err)

			retryCount++
			if retryCount >= maxRetries {
				return fmt.Errorf("max retries reached: %w", err)
			}

			logger.Infof("Agent will retry in %v...", delay)
			time.Sleep(delay)
			delay = time.Duration(float64(delay) * backoffFactor)
			if delay > maxDelay {
				delay = maxDelay
			}
			continue
		}

		// Send login frame to authenticate
		if err := client.SendLogin(m.deviceID, m.token); err != nil {
			client.Close()
			logger.Errorf("Agent login failed: %v", err)

			retryCount++
			if retryCount >= maxRetries {
				return fmt.Errorf("max retries reached after login failure: %w", err)
			}

			time.Sleep(delay)
			delay = time.Duration(float64(delay) * backoffFactor)
			if delay > maxDelay {
				delay = maxDelay
			}
			continue
		}

		m.mu.Lock()
		m.client = client
		m.mu.Unlock()

		logger.Info("Agent connected to backend successfully!")
		return nil
	}
}

// Send sends a command with payload over the persistent connection (thread-safe)
func (m *Manager) Send(action string, data interface{}) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.client == nil {
		return fmt.Errorf("not connected")
	}

	payload := struct {
		Action string      `json:"action"`
		Data   interface{} `json:"data,omitempty"`
	}{
		Action: action,
		Data:   data,
	}

	b, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal payload: %w", err)
	}

	if err := m.client.SendCommand(b); err != nil {
		return fmt.Errorf("send command: %w", err)
	}

	return nil
}

// StartReceiveLoop starts the background goroutine to receive messages
func (m *Manager) StartReceiveLoop() {
	go m.receiveLoop()
}

// receiveLoop continuously receives messages from the backend
func (m *Manager) receiveLoop() {
	defer close(m.doneCh)

	for {
		select {
		case <-m.stopCh:
			logger.Info("Receive loop stopped")
			return
		default:
		}

		m.mu.Lock()
		client := m.client
		m.mu.Unlock()

		if client == nil {
			logger.Warn("Connection lost in receive loop")
			time.Sleep(1 * time.Second)
			continue
		}

		msg, err := client.RecvProtocolMessage()
		if err != nil {
			logger.Errorf("Agent ping loop failed: %v. Will retry...", err)
			// Connection broken, need to reconnect
			// For now, just log and continue (reconnect logic can be added later)
			time.Sleep(1 * time.Second)
			continue
		}

		// Handle received message
		switch msg.Type {
		case network.MsgCommand:
			if len(msg.CommandJSON) > 0 {
				socket.HandleMessage(msg.CommandJSON)
			}
		case network.MsgAck:
			// ACK responses - could log or handle specific acks
			// logger.Infof("Received ACK: code=%d msg=%s", msg.StatusCode, msg.StatusMsg)
		default:
			// logger.Infof("Received message type: %d", msg.Type)
		}
	}
}

// Close gracefully closes the connection
func (m *Manager) Close() error {
	close(m.stopCh)

	m.mu.Lock()
	defer m.mu.Unlock()

	if m.client != nil {
		err := m.client.Close()
		m.client = nil
		return err
	}
	return nil
}

// IsConnected returns whether the manager has an active connection
func (m *Manager) IsConnected() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.client != nil
}
