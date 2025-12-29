package ui

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
)

type errMsg error

type LoginModel struct {
	Session   *Session
	Inputs    []textinput.Model
	FocusIdx  int
	Err       error
	OnSuccess func() tea.Msg
}

const (
	inputHost = iota
	inputPort
	// inputDeviceID (hidden/auto)
	inputUsername
	inputPassword
)

func NewLoginModel(s *Session) LoginModel {
	inputs := make([]textinput.Model, 4)

	inputs[inputHost] = textinput.New()
	inputs[inputHost].Placeholder = "127.0.0.1"
	inputs[inputHost].Focus()
	inputs[inputHost].Prompt = "Host: "
	inputs[inputHost].SetValue("127.0.0.1")

	inputs[inputPort] = textinput.New()
	inputs[inputPort].Placeholder = "9200"
	inputs[inputPort].Prompt = "Port: "
	inputs[inputPort].SetValue("9200")

	inputs[inputUsername] = textinput.New()
	inputs[inputUsername].Placeholder = "admin"
	inputs[inputUsername].Prompt = "Username: "

	inputs[inputPassword] = textinput.New()
	inputs[inputPassword].Placeholder = "password"
	inputs[inputPassword].EchoMode = textinput.EchoPassword
	inputs[inputPassword].Prompt = "Password: "

	return LoginModel{
		Session:  s,
		Inputs:   inputs,
		FocusIdx: 0,
	}
}

func (m LoginModel) Init() tea.Cmd {
	return textinput.Blink
}

func (m LoginModel) Update(msg tea.Msg) (LoginModel, tea.Cmd) {
	var cmds []tea.Cmd = make([]tea.Cmd, len(m.Inputs))

	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.Type {
		case tea.KeyEnter:
			if m.FocusIdx == len(m.Inputs)-1 {
				// Submit
				return m, m.LoginCmd
			}
			m.nextInput()
		case tea.KeyTab, tea.KeyDown:
			m.nextInput()
		case tea.KeyShiftTab, tea.KeyUp:
			m.prevInput()
		}
	}

	for i := range m.Inputs {
		m.Inputs[i], cmds[i] = m.Inputs[i].Update(msg)
	}

	return m, tea.Batch(cmds...)
}

func (m *LoginModel) nextInput() {
	m.Inputs[m.FocusIdx].Blur()
	m.FocusIdx++
	if m.FocusIdx >= len(m.Inputs) {
		m.FocusIdx = 0
	}
	m.Inputs[m.FocusIdx].Focus()
}

func (m *LoginModel) prevInput() {
	m.Inputs[m.FocusIdx].Blur()
	m.FocusIdx--
	if m.FocusIdx < 0 {
		m.FocusIdx = len(m.Inputs) - 1
	}
	m.Inputs[m.FocusIdx].Focus()
}

func (m LoginModel) LoginCmd() tea.Msg {
	host := m.Inputs[inputHost].Value()
	portStr := m.Inputs[inputPort].Value()
	username := m.Inputs[inputUsername].Value()
	password := m.Inputs[inputPassword].Value()

	port, err := strconv.Atoi(portStr)
	if err != nil {
		return errMsg(fmt.Errorf("invalid port"))
	}

	// 1. Connect
	if err := m.Session.Connect(host, port); err != nil {
		return errMsg(fmt.Errorf("connect failed: %v", err))
	}

	// 2. Send Login Action to get Token
	// Using temporary dummy device ID for initial handshake?
	// Actually, the protocol needs a login frame first to even talk?
	// backend/app/controllers/protocol_router.go says:
	// "login and admin_* do not require device token; others require issued token"
	// BUT HandleMessage checks `msg.Type == network.MsgLogin` first to register.

	// So we must SendLogin with empty token or dummy token first to establish context?
	// Protocol router: if msg.Token != "", it validates. If empty, it accepts?
	// "login missing device id" if empty.

	tempDeviceID := "admin-console"
	// Send initial opaque login to get onto the Hub
	if err := m.Session.Login(tempDeviceID, "initial-handshake"); err != nil {
		return errMsg(fmt.Errorf("initial handshake failed: %v", err))
	}

	// Wait for handshake to settle? (Async) - Login() spawns receiveLoop.

	// 3. Send "login" command with username/password
	creds := map[string]string{
		"username": username,
		"password": password,
	}
	if err := m.Session.SendCommand("login", creds); err != nil {
		return errMsg(fmt.Errorf("send auth command failed: %v", err))
	}

	// We can't return OnSuccess yet, we must wait for the Token response.
	// The RootModel should likely handle the response.
	// But `LoginCmd` returns a Msg. We can return a special "WaitAuth" msg?

	return loginSubmittedMsg{}
}

type loginSubmittedMsg struct{}

func (m LoginModel) View() string {
	var b strings.Builder

	b.WriteString(titleStyle.Render("Sagiri Guard - Admin Login") + "\n\n")

	for i := range m.Inputs {
		b.WriteString(m.Inputs[i].View())
		if i < len(m.Inputs)-1 {
			b.WriteRune('\n')
		}
	}

	b.WriteString("\n\n")
	b.WriteString(blurredStyle.Render("Press Tab to changes fields, Enter to submit"))

	if m.Err != nil {
		b.WriteString("\n\n")
		b.WriteString(errorMessageStyle(m.Err.Error()))
	}

	return b.String()
}
