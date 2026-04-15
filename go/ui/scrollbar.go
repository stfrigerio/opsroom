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
	track := lipgloss.NewStyle().Foreground(colAmberDim).Render("│")
	thumb := lipgloss.NewStyle().Foreground(thumbCol).Bold(true).Render("█")
	for i := 0; i < height; i++ {
		if i >= top && i < top+thumbH {
			rows[i] = thumb
		} else {
			rows[i] = track
		}
	}
	return strings.Join(rows, "\n")
}
