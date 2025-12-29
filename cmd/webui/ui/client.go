package ui

import (
	"encoding/json"
	"fmt"
	"sync"

	"sagiri-guard/network"

	tea "github.com/charmbracelet/bubbletea"
)

// Session manages the persistent connection to the backend
type Session struct {
	Client      *network.TCPClient
	DeviceID    string
	Token       string
	Host        string
	Port        int
	MsgChan     chan tea.Msg
	StopChan    chan struct{}
	mu          sync.Mutex
	loopRunning bool
}

// NewSession creates a new session manager
func NewSession() *Session {
	return &Session{
		MsgChan:  make(chan tea.Msg),
		StopChan: make(chan struct{}),
	}
}

// Connect dial the server
func (s *Session) Connect(host string, port int) error {
	s.Host = host
	s.Port = port
	client, err := network.DialTCP(host, port)
	if err != nil {
		return err
	}
	s.Client = client
	return nil
}

// Login sends login frame and starts the receive loop
func (s *Session) Login(deviceID, token string) error {
	if s.Client == nil {
		return fmt.Errorf("not connected")
	}
	if err := s.Client.SendLogin(deviceID, token); err != nil {
		return err
	}
	s.DeviceID = deviceID
	s.Token = token

	// Only start loop if not already running
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.loopRunning {
		s.loopRunning = true
		go s.receiveLoop()
	}
	return nil
}

// Close closes the connection
func (s *Session) Close() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.Client != nil {
		s.Client.Close()
	}
	if s.loopRunning {
		close(s.StopChan)
		s.loopRunning = false
	}
}

// SendCommand sends generic command
func (s *Session) SendCommand(action string, data any) error {
	if s.Client == nil {
		return fmt.Errorf("not connected")
	}
	payload := map[string]any{
		"action": action,
		"data":   data,
	}
	b, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	return s.Client.SendCommand(b)
}

// MsgFromServer is a Bubble Tea message wrapping a network message
type MsgFromServer struct {
	Msg *network.ProtocolMessage
	Err error
}

func (s *Session) receiveLoop() {
	for {
		select {
		case <-s.StopChan:
			return
		default:
			// Blocking read
			msg, err := s.Client.RecvProtocolMessage()
			if err != nil {
				// Handle disconnect or error
				// If recv failed, it usually means connection closed by server
				s.MsgChan <- MsgFromServer{Err: fmt.Errorf("connection lost: %v", err)}
				return
			}
			s.MsgChan <- MsgFromServer{Msg: msg}
		}
	}
}

// WaitForMsg is a tea.Cmd that waits for the next message from the channel
func (s *Session) WaitForMsg() tea.Msg {
	return <-s.MsgChan
}
