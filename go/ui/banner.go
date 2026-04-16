package ui

import (
	"fmt"
	"strings"
	"time"
)

// renderBanner — top bar. One row, styled inverse amber, full width.
func renderBanner(width, sessionCount, portCount int, now time.Time) string {
	clock := now.Format("15:04:05")
	content := fmt.Sprintf(
		" ▓█▓ OPSROOM ▓█▓ CLAUDE·GRID ▓█▓ SESSIONS·%02d ▓█▓ PORTS·%02d ▓█▓ %s ▓█▓ ",
		sessionCount, portCount, clock,
	)
	return styleBanner.Width(width).Render(strings.TrimRight(content, " "))
}
