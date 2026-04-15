package ui

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"

	"opsroom/claude"
	"opsroom/hypr"
)

// renderCardHeader — one-row inverse stripe inside the card's top border.
// Left side: `WS3 · PROJECT`. Right side: status + elapsed. Padded with
// spaces to exactly `width`.
func renderCardHeader(
	sess *claude.Session,
	win *hypr.Window,
	style lipgloss.Style,
	width int,
	now time.Time,
	slotSuffix string,
) string {
	ws := "WS?"
	if win != nil {
		ws = fmt.Sprintf("WS%d", win.Workspace)
	}
	project := claude.ProjectName(sess.CWD)
	elapsed := formatElapsed(sess.LastEventTS, now)

	left := fmt.Sprintf(" %s · %s ", ws, strings.ToUpper(project))
	if slotSuffix != "" {
		left = fmt.Sprintf(" %s · %s %s ", ws, strings.ToUpper(project), slotSuffix)
	}

	right := elapsed
	switch {
	case sess.IsWaiting:
		right = "WAITING · " + elapsed
	case sess.IsWorking:
		right = workingSpinner(now) + " WORKING · " + elapsed
	case win != nil && win.Class != "kitty":
		right = fmt.Sprintf("[%s] · %s", win.Class, elapsed)
	}
	right = " " + right + " "

	// If the combined content is too wide, shave the left label.
	if lipgloss.Width(left)+lipgloss.Width(right) >= width {
		over := lipgloss.Width(left) + lipgloss.Width(right) - width + 1
		if len(left) > over+6 {
			left = left[:len(left)-over-3] + "… "
		}
	}
	gap := width - lipgloss.Width(left) - lipgloss.Width(right)
	if gap < 0 {
		gap = 0
	}
	return style.Render(left + strings.Repeat(" ", gap) + right)
}
