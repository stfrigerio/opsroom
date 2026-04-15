package ui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// renderFooter — bottom status strip. Telemetry on the left, keybind pills
// on the right. Truncates from the right if the terminal is too narrow.
// When `pageCount > 1`, inserts a `PAGE n/m` indicator between telemetry
// and the keybind pills and adds the `[]` pill for pagination.
func renderFooter(width int, page, pageCount int) string {
	bits := []string{
		styleElapsed.Render("● LINK HYPR"),
		styleElapsed.Render("● KITTY"),
		styleElapsed.Render(fmt.Sprintf("Δ %s", refreshInterval)),
	}
	if pageCount > 1 {
		bits = append(bits, lipgloss.NewStyle().Foreground(colAmberHi).Bold(true).
			Render(fmt.Sprintf("PAGE %d/%d", page+1, pageCount)))
	}
	bits = append(bits,
		" ",
		footerKey("←↑↓→", "focus"),
		footerKey("C-←↑↓→", "swap"),
		footerKey("⏎", "hypr"),
		footerKey("i", "inject"),
	)
	if pageCount > 1 {
		bits = append(bits, footerKey("C-⇥", "page"))
	}
	bits = append(bits,
		footerKey("r", "rescan"),
		footerKey("q", "quit"),
	)
	joined := " " + strings.Join(bits, "  ")
	if lipgloss.Width(joined) > width {
		joined = joined[:width]
	}
	return styleFooter.Width(width).Render(joined)
}

func footerKey(key, label string) string {
	return lipgloss.NewStyle().Foreground(colAmber).Render("["+key+"]") +
		styleElapsed.Render("·"+label)
}
