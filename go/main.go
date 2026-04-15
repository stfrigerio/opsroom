package main

import (
	"fmt"
	"os"

	tea "github.com/charmbracelet/bubbletea"

	"opsroom/ui"
)

func main() {
	p := tea.NewProgram(
		ui.New(),
		tea.WithAltScreen(),
		tea.WithMouseAllMotion(),
	)
	if _, err := p.Run(); err != nil {
		fmt.Fprintln(os.Stderr, "opsroom:", err)
		os.Exit(1)
	}
}
