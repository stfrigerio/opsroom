package ui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// renderScrollbar — 1 col × `height` rows. Thumb sized proportional to
// viewport/content ratio, positioned by scroll offset. All-blank when
// there's no overflow so it doesn't clutter short cards.
func renderScrollbar(scroll, maxScroll, height int, thumbCol lipgloss.Color) string {
	rows := make([]string, height)
	if maxScroll <= 0 || height <= 0 {
		for i := range rows {
			rows[i] = " "
		}
		return strings.Join(rows, "\n")
	}
	total := maxScroll + height
	thumbH := height * height / total
	if thumbH < 1 {
		thumbH = 1
	}
	if thumbH > height {
		thumbH = height
	}
	top := (scroll * (height - thumbH)) / maxScroll
	if top < 0 {
		top = 0
	}
	if top > height-thumbH {
		top = height - thumbH
	}
	trackStyle := lipgloss.NewStyle().Foreground(colAmberDim)
	thumbStyle := lipgloss.NewStyle().Foreground(thumbCol).Bold(true)
	// Dotted track: a small square every other row — CRT-monitor vibe
	// instead of a continuous line. Thumb is a solid block on every row
	// of its range so it pops against the dashed track.
	for i := 0; i < height; i++ {
		switch {
		case i >= top && i < top+thumbH:
			rows[i] = thumbStyle.Render("▮")
		case i%2 == 0:
			rows[i] = trackStyle.Render("▪")
		default:
			rows[i] = " "
		}
	}
	return strings.Join(rows, "\n")
}
