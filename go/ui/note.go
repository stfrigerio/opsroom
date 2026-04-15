package ui

import "github.com/charmbracelet/lipgloss"

// renderNote — one-row transient notification. Amber by default, red for
// errors. The only place besides WAITING that red is allowed.
func renderNote(width int, text, severity string) string {
	fg := colAmber
	if severity == "error" {
		fg = colAlert
	}
	return lipgloss.NewStyle().
		Foreground(fg).
		Bold(true).
		Padding(0, 1).
		Width(width).
		Render("▪ " + text)
}
