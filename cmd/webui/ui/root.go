package ui

import (
	"encoding/json"
	"fmt"
	"sagiri-guard/network"

	tea "github.com/charmbracelet/bubbletea"
)

type state int

const (
	stateLogin state = iota
	stateDashboard
	stateDeviceDetail
)

// BackToDashboardMsg signals transition back to dashboard
type BackToDashboardMsg struct{}

type RootModel struct {
	State     state
	Session   *Session
	Login     LoginModel
	Dashboard DashboardModel
	Detail    DeviceDetailModel
	Quitting  bool
	width     int
	height    int
}

func NewRootModel() RootModel {
	s := NewSession()
	return RootModel{
		State:   stateLogin,
		Session: s,
		Login:   NewLoginModel(s),
	}
}

func (m RootModel) Init() tea.Cmd {
	return tea.Batch(m.Login.Init())
}

func (m RootModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		// Propagate resize? ideally yes
		m.Dashboard.Table.SetHeight(msg.Height - 10)
		m.Detail.Files.SetHeight(msg.Height - 10)
		m.Detail.CommandLog.Height = msg.Height - 15

	case tea.KeyMsg:
		if msg.Type == tea.KeyCtrlC {
			m.Quitting = true
			m.Session.Close()
			return m, tea.Quit
		}

	case MsgFromServer:
		// Global error handling or specific routing
		if msg.Err != nil {
			// Pass error to current view
			if m.State == stateLogin {
				m.Login.Err = msg.Err
			} else if m.State == stateDashboard {
				m.Dashboard.Err = msg.Err
			}
			// Don't stop here, might need to update model
		}
	}

	// State-specific Logic
	switch m.State {
	case stateLogin:
		// Handle Login Logic
		// If we are in login state and receive a message (ACK with token), handle it
		if sMsg, ok := msg.(MsgFromServer); ok && sMsg.Msg != nil && sMsg.Msg.Type == network.MsgAck {
			if sMsg.Msg.StatusCode == 200 {
				var authResp struct {
					Token string `json:"token"`
				}
				if err := json.Unmarshal([]byte(sMsg.Msg.StatusMsg), &authResp); err == nil {
					// Perform protocol login
					deviceID := "admin-console"
					if err := m.Session.Login(deviceID, authResp.Token); err != nil {
						m.Login.Err = fmt.Errorf("protocol login failed: %v", err)
					} else {
						// Success! Transition to dashboard
						m.State = stateDashboard
						m.Dashboard = NewDashboardModel(m.Session, m.width, m.height)
						cmds = append(cmds, m.Dashboard.Init(), m.Session.WaitForMsg)
						return m, tea.Batch(cmds...)
					}
				} else {
					m.Login.Err = fmt.Errorf("invalid auth response")
				}
			} else {
				m.Login.Err = fmt.Errorf("login error: %s", sMsg.Msg.StatusMsg)
			}
		}

		newLogin, newCmd := m.Login.Update(msg)
		m.Login = newLogin
		cmds = append(cmds, newCmd)

		if _, ok := msg.(loginSubmittedMsg); ok {
			cmds = append(cmds, m.Session.WaitForMsg)
		}

	case stateDashboard:
		switch msg := msg.(type) {
		case DeviceSelectedMsg:
			m.State = stateDeviceDetail
			m.Detail = NewDeviceDetailModel(m.Session, msg.DeviceID, m.width, m.height)
			cmds = append(cmds, m.Detail.Init())
			return m, tea.Batch(cmds...)
		}

		newDash, newCmd := m.Dashboard.Update(msg)
		m.Dashboard = newDash
		cmds = append(cmds, newCmd)

		if _, ok := msg.(MsgFromServer); ok {
			cmds = append(cmds, m.Session.WaitForMsg)
		}

	case stateDeviceDetail:
		switch msg.(type) {
		case BackToDashboardMsg:
			m.State = stateDashboard
			cmds = append(cmds, m.Dashboard.Init()) // Refresh list
			return m, tea.Batch(cmds...)
		}

		newDetail, newCmd := m.Detail.Update(msg)
		m.Detail = newDetail
		cmds = append(cmds, newCmd)

		if _, ok := msg.(MsgFromServer); ok {
			cmds = append(cmds, m.Session.WaitForMsg)
		}
	}

	return m, tea.Batch(cmds...)
}

func (m RootModel) View() string {
	if m.Quitting {
		return "Bye!\n"
	}
	switch m.State {
	case stateLogin:
		return m.Login.View()
	case stateDashboard:
		return m.Dashboard.View()
	case stateDeviceDetail:
		return m.Detail.View()
	}
	return "Unknown state"
}

// Custom defined messages
type loginSuccessMsg struct{}

func (m *RootModel) onLoginSuccess() tea.Msg {
	return loginSuccessMsg{}
}

// Hook up the callback
func NewRootModelWithCallback() RootModel {
	m := NewRootModel()
	m.Login.OnSuccess = m.onLoginSuccess
	return m
}
