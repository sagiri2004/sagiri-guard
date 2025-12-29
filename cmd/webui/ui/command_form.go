package ui

import (
	"fmt"

	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// Define states
type FormState int

const (
	StateSelecting FormState = iota
	StateFilling
)

// Command items for list
type cmdItem struct {
	title, desc string
	index       int
}

func (i cmdItem) Title() string       { return i.title }
func (i cmdItem) Description() string { return i.desc }
func (i cmdItem) FilterValue() string { return i.title }

// CommandSentMsg indicates a command was sent
type CommandSentMsg struct {
	Log string
}

// CommandFormModel handles command form UI
type CommandFormModel struct {
	DeviceID    string
	Session     *Session
	State       FormState
	List        list.Model
	Inputs      []textinput.Model
	Focused     int
	SelectedCmd int
}

// Command definitions
type CommandDef struct {
	Name        string
	Description string
	Fields      []FieldDef
	Kind        string // "agent" or "shell"
}

type FieldDef struct {
	Name        string
	Placeholder string
	Required    bool
	Default     string
}

var availableCommands = []CommandDef{
	{
		Name:        "get_logs",
		Description: "Fetch logs from the agent",
		Kind:        "agent",
		Fields: []FieldDef{
			{Name: "lines", Placeholder: "Number of lines (default: 100)", Required: false, Default: "100"},
		},
	},
	{
		Name:        "backup_auto",
		Description: "Enable/Disable automatic backup",
		Kind:        "agent",
		Fields: []FieldDef{
			{Name: "enabled", Placeholder: "true or false", Required: true, Default: "true"},
		},
	},
	{
		Name:        "restore",
		Description: "Restore a file from backup",
		Kind:        "agent",
		Fields: []FieldDef{
			{Name: "file_id", Placeholder: "File ID", Required: true},
			{Name: "version_id", Placeholder: "Version ID", Required: true},
			{Name: "file_name", Placeholder: "File name", Required: true},
			{Name: "dest_path", Placeholder: "Destination path", Required: true},
		},
	},
	{
		Name:        "block_website",
		Description: "Block/Unblock websites",
		Kind:        "agent",
		Fields: []FieldDef{
			{Name: "action", Placeholder: "apply, remove, or sync", Required: true, Default: "apply"},
			{Name: "enabled", Placeholder: "true or false", Required: false, Default: "true"},
			{Name: "domains", Placeholder: "Comma-separated domains for action", Required: false},
		},
	},
	{
		Name:        "Custom Shell",
		Description: "Execute a raw shell command",
		Kind:        "shell",
		Fields: []FieldDef{
			{Name: "command", Placeholder: "e.g. ls -la", Required: true},
		},
	},
}

func NewCommandFormModel(deviceID string, session *Session, width, height int) CommandFormModel {
	items := []list.Item{}
	for i, cmd := range availableCommands {
		items = append(items, cmdItem{title: cmd.Name, desc: cmd.Description, index: i})
	}

	delegate := list.NewDefaultDelegate()
	l := list.New(items, delegate, width, height)
	l.Title = "Select Command"
	l.SetShowHelp(false)

	m := CommandFormModel{
		DeviceID: deviceID,
		Session:  session,
		State:    StateSelecting,
		List:     l,
	}
	return m
}

func (m *CommandFormModel) initInputs() {
	if m.SelectedCmd < 0 || m.SelectedCmd >= len(availableCommands) {
		m.SelectedCmd = 0
	}
	cmd := availableCommands[m.SelectedCmd]
	m.Inputs = make([]textinput.Model, len(cmd.Fields))
	for i, field := range cmd.Fields {
		ti := textinput.New()
		ti.Placeholder = field.Placeholder
		ti.CharLimit = 256
		if field.Default != "" {
			ti.SetValue(field.Default)
		}
		if i == 0 {
			ti.Focus()
		}
		m.Inputs[i] = ti
	}
	m.Focused = 0
}

func (m CommandFormModel) Init() tea.Cmd {
	return nil
}

func (m CommandFormModel) Update(msg tea.Msg) (CommandFormModel, tea.Cmd) {
	var cmd tea.Cmd
	var cmds []tea.Cmd

	if m.State == StateSelecting {
		switch msg := msg.(type) {
		case tea.KeyMsg:
			switch msg.String() {
			case "enter":
				// Select command
				if i, ok := m.List.SelectedItem().(cmdItem); ok {
					m.SelectedCmd = i.index
					m.State = StateFilling
					m.initInputs()
					return m, textinput.Blink
				}
			case "up", "k":
				m.List.CursorUp()
				return m, nil
			case "down", "j":
				m.List.CursorDown()
				return m, nil
			}
		case tea.WindowSizeMsg:
			m.List.SetWidth(msg.Width)
			m.List.SetHeight(msg.Height)
		}
		m.List, cmd = m.List.Update(msg)
		cmds = append(cmds, cmd)
	} else {
		// Filling form
		switch msg := msg.(type) {
		case tea.KeyMsg:
			switch msg.String() {
			case "esc":
				m.State = StateSelecting // Back to selection
				return m, nil
			case "enter":
				if m.Focused == len(m.Inputs) {
					// Submit button focused
					return m, m.submitCommand()
				} else if m.Focused == len(m.Inputs)+1 {
					// Back button focused
					m.State = StateSelecting
					return m, nil
				}
				// If in input, move to next field
				m.Focused++
				if m.Focused > len(m.Inputs)+1 {
					m.Focused = 0
				}
				m.updateFocus()
				return m, nil

			case "tab", "down", "j":
				m.Focused++
				if m.Focused > len(m.Inputs)+1 {
					m.Focused = 0
				}
				m.updateFocus()
				return m, nil

			case "shift+tab", "up", "k":
				m.Focused--
				if m.Focused < 0 {
					m.Focused = len(m.Inputs) + 1
				}
				m.updateFocus()
				return m, nil
			}
		}
		// Update inputs only if focused
		if m.Focused >= 0 && m.Focused < len(m.Inputs) {
			m.Inputs[m.Focused], cmd = m.Inputs[m.Focused].Update(msg)
			cmds = append(cmds, cmd)
		}
	}

	return m, tea.Batch(cmds...)
}

func (m *CommandFormModel) updateFocus() {
	for i := range m.Inputs {
		if i == m.Focused {
			m.Inputs[i].Focus()
		} else {
			m.Inputs[i].Blur()
		}
	}
}

func (m CommandFormModel) renderButton(text string, focused bool) string {
	if focused {
		return lipgloss.NewStyle().Foreground(lipgloss.Color("0")).Background(lipgloss.Color("205")).Padding(0, 3).Bold(true).Render(text)
	}
	return lipgloss.NewStyle().Foreground(lipgloss.Color("254")).Background(lipgloss.Color("240")).Padding(0, 3).Render(text)
}

func (m CommandFormModel) View() string {
	if m.State == StateSelecting {
		return m.List.View()
	}

	// Form View
	cmd := availableCommands[m.SelectedCmd]
	var s string

	title := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("99")).Render(fmt.Sprintf("Parameters: %s", cmd.Name))
	s += title + "\n\n"

	for i, field := range cmd.Fields {
		label := field.Name
		if field.Required {
			label += " *"
		}

		// Highlight active label
		labelStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("86"))
		if i == m.Focused {
			labelStyle = labelStyle.Foreground(lipgloss.Color("205")).Bold(true)
		}
		s += labelStyle.Render(label) + "\n"
		s += m.Inputs[i].View() + "\n\n"
	}

	// Buttons
	submitBtn := m.renderButton("Submit", m.Focused == len(m.Inputs))
	backBtn := m.renderButton("Back", m.Focused == len(m.Inputs)+1)

	buttons := lipgloss.JoinHorizontal(lipgloss.Top, submitBtn, lipgloss.NewStyle().MarginLeft(2).Render(backBtn))
	s += "\n" + buttons

	return lipgloss.NewStyle().Padding(1, 2).Render(s)
}

func (m CommandFormModel) submitCommand() tea.Cmd {
	return func() tea.Msg {
		cmd := availableCommands[m.SelectedCmd]
		var req map[string]interface{}

		if cmd.Kind == "shell" {
			// Custom shell command
			req = map[string]interface{}{
				"device_id": m.DeviceID,
				"command":   m.Inputs[0].Value(),
				"kind":      "shell",
				"payload":   map[string]string{},
			}
		} else {
			// Agent command
			payload := buildPayload(cmd.Name, m.Inputs)
			req = map[string]interface{}{
				"device_id": m.DeviceID,
				"command":   cmd.Name,
				"kind":      "agent",
				"payload":   payload,
			}
		}

		// DO NOT MARSHAL, send map directly
		m.Session.SendCommand("admin_send_command", req)

		// Reset state
		// m.State = StateSelecting // Optional: stay or go back
		return nil
	}
}

func buildPayload(name string, inputs []textinput.Model) map[string]interface{} {
	switch name {
	case "get_logs":
		lines := 100
		if inputs[0].Value() != "" {
			fmt.Sscanf(inputs[0].Value(), "%d", &lines)
		}
		return map[string]interface{}{"lines": lines}
	case "backup_auto":
		enabled := inputs[0].Value() == "true"
		return map[string]interface{}{"enabled": enabled}
	case "restore":
		versionID := 0
		fmt.Sscanf(inputs[1].Value(), "%d", &versionID)
		return map[string]interface{}{
			"file_id":    inputs[0].Value(),
			"version_id": versionID,
			"file_name":  inputs[2].Value(),
			"dest_path":  inputs[3].Value(),
		}
	case "block_website":
		enabled := inputs[1].Value() == "true"
		p := map[string]interface{}{
			"action": inputs[0].Value(),
			"status": map[string]bool{"enabled": enabled},
		}
		if inputs[2].Value() != "" {
			rules := []map[string]interface{}{}
			for _, d := range splitComma(inputs[2].Value()) {
				rules = append(rules, map[string]interface{}{
					"type": "domain", "domain": d, "enabled": true,
				})
			}
			p["rules"] = rules
		}
		return p
	}
	return nil
}

func splitComma(s string) []string {
	var parts []string
	current := ""
	for _, ch := range s {
		if ch == ',' {
			if current != "" {
				parts = append(parts, current)
				current = ""
			}
		} else {
			current += string(ch)
		}
	}
	if current != "" {
		parts = append(parts, current)
	}
	return parts
}
