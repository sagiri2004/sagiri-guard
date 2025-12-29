package ui

import (
	"encoding/json"
	"strings"

	"sagiri-guard/network"

	"github.com/charmbracelet/bubbles/table"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type DashboardModel struct {
	Session *Session
	Table   table.Model
	Devices []DeviceEntry
	Err     error
}

type DeviceEntry struct {
	DeviceID string `json:"device_id"`
	Status   string `json:"status"`
	// Add more fields if available
}

func NewDashboardModel(s *Session, width, height int) DashboardModel {
	columns := []table.Column{
		{Title: "Device ID", Width: 40},
		{Title: "Status", Width: 20},
	}

	t := table.New(
		table.WithColumns(columns),
		table.WithFocused(true),
		table.WithHeight(height-10),
	)

	sStyle := table.DefaultStyles()
	sStyle.Header = sStyle.Header.
		BorderStyle(lipgloss.NormalBorder()).
		BorderForeground(lipgloss.Color("240")).
		BorderBottom(true).
		Bold(false)
	sStyle.Selected = sStyle.Selected.
		Foreground(lipgloss.Color("229")).
		Background(lipgloss.Color("57")).
		Bold(false)
	t.SetStyles(sStyle)

	return DashboardModel{
		Session: s,
		Table:   t,
	}
}

type DeviceSelectedMsg struct {
	DeviceID string
}

func (m DashboardModel) Init() tea.Cmd {
	return func() tea.Msg {
		m.Session.SendCommand("admin_list_online", nil)
		return nil
	}
}

func (m DashboardModel) Update(msg tea.Msg) (DashboardModel, tea.Cmd) {
	var cmd tea.Cmd
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "r":
			return m, func() tea.Msg {
				m.Session.SendCommand("admin_list_online", nil)
				return nil
			}
		case "enter":
			selected := m.Table.SelectedRow()
			if len(selected) > 0 {
				return m, func() tea.Msg {
					return DeviceSelectedMsg{DeviceID: selected[0]}
				}
			}
		case "q":
			return m, tea.Quit
		}

	case MsgFromServer:
		if msg.Err != nil {
			m.Err = msg.Err
			return m, nil
		}
		// Handle specific messages
		if msg.Msg.Type == network.MsgAck {
			// If we just sent admin_list_online, parse payload
			// But wait, the ProtocolMessage.StatusMsg is likely the JSON payload
			// Need to verify protocol behavior.
			// Currently sendAckJSON puts payload into status_msg.

			// We try to parse it as device list
			var devs []string
			if err := json.Unmarshal([]byte(msg.Msg.StatusMsg), &devs); err == nil {
				// It's a list of strings
				m.Devices = make([]DeviceEntry, len(devs))
				rows := []table.Row{}
				for i, d := range devs {
					m.Devices[i] = DeviceEntry{DeviceID: d, Status: "Online"}
					rows = append(rows, table.Row{d, "Online"})
				}
				m.Table.SetRows(rows)
			} else {
				// Maybe struct list?
				var devEntries []DeviceEntry
				if err2 := json.Unmarshal([]byte(msg.Msg.StatusMsg), &devEntries); err2 == nil {
					m.Devices = devEntries
					rows := []table.Row{}
					for _, d := range devEntries {
						rows = append(rows, table.Row{d.DeviceID, d.Status})
					}
					m.Table.SetRows(rows)
				}
			}
		}
	}

	m.Table, cmd = m.Table.Update(msg)
	return m, cmd
}

func (m DashboardModel) RefreshDevicesCmd() tea.Msg {
	if err := m.Session.SendCommand("admin_list_online", nil); err != nil {
		return errMsg(err)
	}
	return nil
}

func (m DashboardModel) View() string {
	var b strings.Builder
	b.WriteString(titleStyle.Render("Dashboard - Online Devices") + "\n\n")
	b.WriteString(m.Table.View())
	b.WriteString("\n\n")
	b.WriteString(blurredStyle.Render("Press 'r' to refresh, 'q' to quit, up/down to navigate"))

	if m.Err != nil {
		b.WriteString("\n" + errorMessageStyle(m.Err.Error()))
	}
	return b.String()
}
