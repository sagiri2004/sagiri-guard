package ui

import (
	"encoding/json"
	"fmt"
	"sagiri-guard/network"
	"sort"
	"strings"

	"github.com/charmbracelet/bubbles/table"
	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type DeviceDetailModel struct {
	Session    *Session
	DeviceID   string
	DeviceName string
	Width      int
	Height     int

	// File Tree
	Files        table.Model
	CurrentNodes []FileNode         // Store data to map table rows to IDs
	CurrentPath  []string           // visual path breadcrumbs
	CurrentDirID *string            // current directory ID (UUID string)
	ParentMap    map[string]*string // map dirID -> parentID to go back

	// Command
	CommandInput textinput.Model
	CommandLog   viewport.Model
	LogContent   string
	CommandForm  *CommandFormModel // New: form UI for commands
	ShowForm     bool              // Toggle between raw input and form

	// Focus State
	Focus int // 0: Tree, 1: Command
}

// Focus constants
const (
	FocusTree = iota
	FocusCommand
)

type FileNode struct {
	ID             string  `json:"id"`
	Name           string  `json:"name"`
	Type           string  `json:"type"` // "dir" or "file"
	ParentID       *string `json:"parent_id,omitempty"`
	Size           int64   `json:"total_size"` // Fixed: use total_size to match backend
	Extension      string  `json:"extension,omitempty"`
	ContentTypeIDs []uint  `json:"content_type_ids"`
}

type AdminListTreeResponse struct {
	Nodes     []FileNode `json:"nodes"`
	Truncated bool       `json:"truncated"`
}

type fileLoadedMsg struct {
	Nodes []FileNode
	DirID *string
	Error error
}

type commandResultMsg struct {
	Result string
	Error  error
}

func NewDeviceDetailModel(s *Session, deviceID string, width, height int) DeviceDetailModel {
	// File Table
	columns := []table.Column{
		{Title: "Type", Width: 4},
		{Title: "Name", Width: 30},
		{Title: "Size", Width: 10},
	}
	t := table.New(
		table.WithColumns(columns),
		table.WithFocused(true),
		table.WithHeight(height-10),
	)
	sT := table.DefaultStyles()
	sT.Header = sT.Header.
		BorderStyle(lipgloss.NormalBorder()).
		BorderForeground(lipgloss.Color("240")).
		BorderBottom(true).
		Bold(false)
	sT.Selected = sT.Selected.
		Foreground(lipgloss.Color("229")).
		Background(lipgloss.Color("57")).
		Bold(false)
	t.SetStyles(sT)

	// Command Input
	ti := textinput.New()
	ti.Placeholder = "Enter command..."
	ti.Focus()
	ti.CharLimit = 156
	ti.Width = 40

	// Right Panel Dimensions
	rightPanelHeight := height - 5
	formHeight := int(float64(rightPanelHeight) * 0.6)
	// Log Viewport
	// Log Viewport
	vp := viewport.New(50, 10)
	vp.Style = lipgloss.NewStyle().PaddingLeft(1)
	// Empty content initially

	// Init Command Form
	formWidth := 50 // Fixed width or relative?

	form := NewCommandFormModel(deviceID, s, formWidth, formHeight)

	m := DeviceDetailModel{
		Session:      s,
		DeviceID:     deviceID,
		Width:        width,
		Height:       height,
		Files:        t,
		CommandInput: ti,
		CommandLog:   vp,
		ParentMap:    make(map[string]*string),
		CommandForm:  &form, // Ready to use
		ShowForm:     true,  // Default to form view
		Focus:        FocusTree,
	}

	// Initial fetch root
	return m
}

func (m DeviceDetailModel) Init() tea.Cmd {
	return tea.Batch(
		textinput.Blink,
		m.fetchDir(nil),
	)
}

func (m DeviceDetailModel) Update(msg tea.Msg) (DeviceDetailModel, tea.Cmd) {
	var cmd tea.Cmd
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "esc":
			return m, func() tea.Msg { return BackToDashboardMsg{} } // Signal to root
		case "tab":
			// Switch focus between Tree and Form
			if m.Files.Focused() {
				m.Focus = FocusCommand
				m.Files.Blur()
			} else {
				m.Focus = FocusTree
				m.Files.Focus()
			}
			return m, nil
		case "f1":
			m.Focus = FocusTree
			m.Files.Focus()
			return m, nil
		case "f2":
			m.Focus = FocusCommand
			m.Files.Blur()
			return m, nil

		}

	case tea.WindowSizeMsg:
		m.Width = msg.Width
		m.Height = msg.Height
		m.Files.SetHeight(msg.Height - 12)

		// Resize Right Panel Components
		rightPanelHeight := msg.Height - 5

		var formHeight, logHeight int
		if m.LogContent != "" {
			formHeight = int(float64(rightPanelHeight) * 0.75)
			logHeight = rightPanelHeight - formHeight
		} else {
			formHeight = rightPanelHeight
			logHeight = 0
		}

		m.CommandLog.Height = logHeight
		m.CommandLog.Width = msg.Width/2 - 8

		if m.CommandForm != nil {
			formMsg := tea.WindowSizeMsg{Width: msg.Width/2 - 8, Height: formHeight}
			*m.CommandForm, _ = m.CommandForm.Update(formMsg)
		}

	case MsgFromServer:
		if msg.Err != nil {
			m.LogContent += fmt.Sprintf("\nError: %v", msg.Err)
			m.CommandLog.SetContent(m.LogContent)
		} else if msg.Msg != nil {
			if msg.Msg.Type == network.MsgAck {
				// Check status code for error
				if msg.Msg.StatusCode != 200 {
					m.LogContent += fmt.Sprintf("\nError (Server): %s", msg.Msg.StatusMsg)
					m.CommandLog.SetContent(m.LogContent)
					m.CommandLog.GotoBottom()
					// return m, nil // Don't return, keep updating components
				} else {
					// Check if it is a response to list_tree or command
					var resp AdminListTreeResponse
					if err := json.Unmarshal([]byte(msg.Msg.StatusMsg), &resp); err == nil && (len(resp.Nodes) > 0 || strings.Contains(msg.Msg.StatusMsg, "\"nodes\":")) {
						// It's a tree response - successfully unmarshaled
						cmds = append(cmds, func() tea.Msg {
							return fileLoadedMsg{Nodes: resp.Nodes, DirID: m.CurrentDirID}
						})
					} else if err != nil && strings.Contains(msg.Msg.StatusMsg, "\"nodes\":") {
						// Contains nodes but failed to unmarshal - log error
						m.LogContent += fmt.Sprintf("\nError parsing tree: %v", err)
						m.CommandLog.SetContent(m.LogContent)
						m.CommandLog.GotoBottom()
					} else {
						// Assume command response
						m.LogContent += fmt.Sprintf("\nResponse: %s", msg.Msg.StatusMsg)
						m.CommandLog.SetContent(m.LogContent)
						m.CommandLog.GotoBottom()
						cmds = append(cmds, func() tea.Msg { return tea.WindowSizeMsg{Width: m.Width, Height: m.Height} })
					}
				}
			}
		}

	case fileLoadedMsg:
		// Save data for lookups
		m.CurrentNodes = msg.Nodes

		// Sort
		sort.Slice(m.CurrentNodes, func(i, j int) bool {
			// Dirs first
			if m.CurrentNodes[i].Type == "dir" && m.CurrentNodes[j].Type != "dir" {
				return true
			}
			if m.CurrentNodes[i].Type != "dir" && m.CurrentNodes[j].Type == "dir" {
				return false
			}
			return m.CurrentNodes[i].Name < m.CurrentNodes[j].Name
		})

		// Prepend ".." if not root
		if m.CurrentDirID != nil {
			m.CurrentNodes = append([]FileNode{{ID: "UP", Name: "..", Type: "dir"}}, m.CurrentNodes...)
		}

		rows := make([]table.Row, len(m.CurrentNodes))
		for i, n := range m.CurrentNodes {
			icon := "📄"
			if n.Type == "dir" {
				icon = "📁"
			}
			rows[i] = table.Row{icon, n.Name, fmt.Sprintf("%d", n.Size)}
		}
		m.Files.SetRows(rows)
		m.Files.SetCursor(0) // Reset cursor to top on dir change

	case CommandSentMsg:
		m.LogContent += fmt.Sprintf("\n%s", msg.Log)
		m.CommandLog.SetContent(m.LogContent)
		m.CommandLog.GotoBottom()
		cmds = append(cmds, func() tea.Msg { return tea.WindowSizeMsg{Width: m.Width, Height: m.Height} })
	}

	// Dispatch update ONLY to focused component
	if m.Focus == FocusTree {
		m.Files, cmd = m.Files.Update(msg)
		cmds = append(cmds, cmd)

		// Handle Tree Navigation
		if keyMsg, ok := msg.(tea.KeyMsg); ok {
			switch keyMsg.String() {
			case "enter":
				idx := m.Files.Cursor()
				if idx >= 0 && idx < len(m.CurrentNodes) {
					node := m.CurrentNodes[idx]
					if node.Type == "dir" {
						// Store parent to go back
						if m.CurrentDirID != nil {
							m.ParentMap[node.ID] = m.CurrentDirID
						} else {
							m.ParentMap[node.ID] = nil
						}

						if node.ID == "UP" {
							// Go Up logic
							if m.CurrentDirID != nil {
								parentID := m.ParentMap[*m.CurrentDirID]
								m.CurrentDirID = parentID
								cmds = append(cmds, m.fetchDir(parentID))
								if len(m.CurrentPath) > 0 {
									m.CurrentPath = m.CurrentPath[:len(m.CurrentPath)-1]
								}
							}
						} else {
							// Go Down Logic
							m.CurrentDirID = &node.ID
							cmds = append(cmds, m.fetchDir(&node.ID))
							m.CurrentPath = append(m.CurrentPath, node.Name)
						}
					}
				}
			case "backspace":
				if m.CurrentDirID != nil {
					parentID := m.ParentMap[*m.CurrentDirID]
					m.CurrentDirID = parentID
					cmds = append(cmds, m.fetchDir(parentID))
					if len(m.CurrentPath) > 0 {
						m.CurrentPath = m.CurrentPath[:len(m.CurrentPath)-1]
					}
				}
			}
		}
	} else {
		if m.ShowForm && m.CommandForm != nil {
			*m.CommandForm, cmd = m.CommandForm.Update(msg)
			cmds = append(cmds, cmd)
		} else {
			m.CommandInput, cmd = m.CommandInput.Update(msg)
			cmds = append(cmds, cmd)
			// Handle legacy enter
			if keyMsg, ok := msg.(tea.KeyMsg); ok && keyMsg.String() == "enter" {
				cmdText := m.CommandInput.Value()
				if cmdText != "" {
					m.CommandInput.SetValue("")
					m.LogContent += fmt.Sprintf("\n> %s", cmdText)
					m.CommandLog.SetContent(m.LogContent)
					m.CommandLog.GotoBottom()
					cmds = append(cmds, m.sendCommand(cmdText))
				}
			}
		}
	}

	// Global logs update always
	m.CommandLog, cmd = m.CommandLog.Update(msg)
	cmds = append(cmds, cmd)

	return m, tea.Batch(cmds...)
}

func (m DeviceDetailModel) View() string {
	// Breadcrumbs
	pathStr := "/" + strings.Join(m.CurrentPath, "/")
	header := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("205")).Render(pathStr)

	filesView := lipgloss.JoinVertical(lipgloss.Left, header, m.Files.View())

	// Dynamic Border Styles
	// Increase padding to prevent clipping
	activeBorder := lipgloss.NewStyle().BorderStyle(lipgloss.ThickBorder()).BorderForeground(lipgloss.Color("205")).Padding(1, 2).Width(m.Width/2 - 6)
	inactiveBorder := lipgloss.NewStyle().BorderStyle(lipgloss.NormalBorder()).BorderForeground(lipgloss.Color("240")).Padding(1, 2).Width(m.Width/2 - 6)

	var leftStyle, rightStyle lipgloss.Style
	if m.Focus == FocusTree {
		leftStyle = activeBorder
		rightStyle = inactiveBorder
	} else {
		leftStyle = inactiveBorder
		rightStyle = activeBorder
	}

	left := leftStyle.Render(filesView)

	var rightContent string
	var topContent string

	if m.ShowForm && m.CommandForm != nil {
		topContent = m.CommandForm.View()
	} else {
		topContent = m.CommandInput.View()
	}

	if m.LogContent != "" {
		sep := lipgloss.NewStyle().Foreground(lipgloss.Color("240")).Render(strings.Repeat("─", m.Width/2-6))

		rightContent = lipgloss.JoinVertical(lipgloss.Left,
			topContent,
			sep,
			lipgloss.NewStyle().Foreground(lipgloss.Color("205")).Bold(true).Render("Output Log:"),
			m.CommandLog.View(),
		)
	} else {
		rightContent = topContent
	}

	right := rightStyle.Render(rightContent)

	content := lipgloss.JoinHorizontal(lipgloss.Top, left, right)
	help := lipgloss.NewStyle().Foreground(lipgloss.Color("241")).MarginTop(1).Render("F1: Files • F2: Commands • Tab: Switch • Enter: Select/Submit • Esc: Dashboard")

	return lipgloss.JoinVertical(lipgloss.Left, content, help)
}

func (m DeviceDetailModel) fetchDir(parentID *string) tea.Cmd {
	return func() tea.Msg {
		req := map[string]interface{}{
			"device_id": m.DeviceID,
			"page":      1,
			"page_size": 100,
		}
		if parentID != nil {
			req["parent_id"] = *parentID
		}
		// Do not marshal here, Session.SendCommand will do it
		m.Session.SendCommand("admin_list_tree", req)
		// Store intent?
		return nil
	}
}

func (m DeviceDetailModel) sendCommand(cmd string) tea.Cmd {
	return func() tea.Msg {
		req := map[string]interface{}{
			"device_id": m.DeviceID,
			"command":   cmd,
			"kind":      "shell",
			"payload":   map[string]string{}, // empty args
		}
		// Do not marshal here
		m.Session.SendCommand("admin_send_command", req)
		return nil
	}
}
