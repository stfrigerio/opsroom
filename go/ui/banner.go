package ui

import (
	"fmt"
	"strings"
	"time"
)

// renderBanner — top bar. One row, styled inverse amber, full width.
func renderBanner(width, sessionCount int, now time.Time) string {
	clock := now.Format("15:04:05")
	content := fmt.Sprintf(
		" ▓█▓ OPSROOM ▓█▓ CLAUDE·GRID ▓█▓ SESSIONS·%02d ▓█▓ %s ▓█▓ ",
		sessionCount, clock,
	)
	return styleBanner.Width(width).Render(strings.TrimRight(content, " "))
}
