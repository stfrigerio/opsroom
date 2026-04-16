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
		footerKey("A-⏎", "browser"),
		footerKey("i", "inject"),
		footerKey("o", "ports"),
	)
	if pageCount > 1 {
		pill := lipgloss.NewStyle().Foreground(colAmber).Render("[ / ]") +
			styleElapsed.Render("·page")
		bits = append(bits, pill)
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

// renderPortsFooter — bottom strip for the ports view.
func renderPortsFooter(width, visible, hidden int, showAll bool) string {
	var summary string
	switch {
	case showAll:
		summary = fmt.Sprintf("▪ %d listeners (all)", visible)
	case hidden > 0:
		summary = fmt.Sprintf("▪ %d listeners · %d hidden", visible, hidden)
	default:
		summary = fmt.Sprintf("▪ %d listeners", visible)
	}
	filterLabel := "all"
	if showAll {
		filterLabel = "filter"
	}
	bits := []string{
		styleElapsed.Render(summary),
		" ",
		footerKey("↑↓", "move"),
		footerKey("⏎", "hypr"),
		footerKey("x", "kill"),
		footerKey(".", filterLabel),
		footerKey("o/esc", "back"),
		footerKey("r", "rescan"),
		footerKey("q", "quit"),
	}
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
