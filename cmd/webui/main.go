package main

import (
	"fmt"
	"os"

	"sagiri-guard/cmd/webui/ui"
	"sagiri-guard/network"

	tea "github.com/charmbracelet/bubbletea"
)

func main() {
	if err := network.Init(); err != nil {
		fmt.Fprintf(os.Stderr, "failed to init network: %v\n", err)
		os.Exit(1)
	}
	defer network.Cleanup()

	m := ui.NewRootModelWithCallback()
	p := tea.NewProgram(m, tea.WithAltScreen())

	if _, err := p.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "Error running program: %v\n", err)
		os.Exit(1)
	}
}
