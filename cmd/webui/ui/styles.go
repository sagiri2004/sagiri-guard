package ui

import "github.com/charmbracelet/lipgloss"

var (
	focusedStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("205"))
	blurredStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
	cursorStyle  = focusedStyle.Copy()
	noStyle      = lipgloss.NewStyle()

	titleStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#FFFDF5")).
			Background(lipgloss.Color("#25A065")).
			Padding(0, 1)

	statusMessageStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("#FFFDF5")).
				Render

	errorMessageStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("#FF0000")).
				Render

	docStyle = lipgloss.NewStyle().Padding(1, 2)
)
