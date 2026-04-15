package ui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/lipgloss"

	"opsroom/claude"
	"opsroom/hypr"
)

// stylePromptTextarea — apply the opsroom palette to a bubbles textarea.
// Call this once at construction. Keeps textarea aesthetics local to the
// prompt component.
//
// Minimal surface: only kill the grey cursor-line background that bubbles
// ships by default. Touching more was shown to cause visual regressions.
func stylePromptTextarea(ta *textarea.Model) {
	clear := lipgloss.NewStyle()
	ta.FocusedStyle.CursorLine = clear
	ta.FocusedStyle.CursorLineNumber = clear
	ta.BlurredStyle.CursorLine = clear
	ta.BlurredStyle.CursorLineNumber = clear
}

// renderPromptPanel — docked injection panel. Renders at `width` wide; the
// caller decides where to place it. Takes the textarea by value — the
// widget's state is owned by the caller. `slotSuffix` (e.g. "#2") is
// appended when the target shares (workspace, project) with another claude.
func renderPromptPanel(
	width int,
	sess *claude.Session,
	win *hypr.Window,
	slotSuffix string,
	ta textarea.Model,
) string {
	target := "(no target)"
	if sess != nil && win != nil {
		proj := claude.ProjectName(sess.CWD)
		if slotSuffix != "" {
			proj = proj + " " + slotSuffix
		}
		target = fmt.Sprintf("WS%d · %s", win.Workspace, proj)
	}

	// Title stripe — inverse amber-hi, full width. Action on the left,
	// target right-aligned. Same visual weight as a card header.
	titleStyle := lipgloss.NewStyle().
		Background(colAmberHi).
		Foreground(colBlack).
		Bold(true).
		Width(width)
	left := " ⟶ INJECT "
	right := target + " "
	pad := width - lipgloss.Width(left) - lipgloss.Width(right)
	if pad < 1 {
		pad = 1
	}
	title := titleStyle.Render(left + strings.Repeat(" ", pad) + right)

	// Body — no border. The textarea has its own 1-col left pad so the
	// cursor isn't flush against the edge. Explicit width so the textarea
	// fills the panel exactly.
	bodyStyle := lipgloss.NewStyle().Width(width).Padding(0, 1)
	body := bodyStyle.Render(ta.View())

	// Hint line — dim amber, full width, aligns with the title stripe.
	hint := lipgloss.NewStyle().
		Foreground(colAmberDim).
		Width(width).
		Render(
			" " + keyPill("⏎", "send") +
				"   " + keyPill("⇧⏎", "newline") +
				"   " + keyPill("␛", "abort"),
		)

	return lipgloss.JoinVertical(lipgloss.Left, title, body, hint)
}

// keyPill — small keybinding marker for the inject hint line.
func keyPill(k, label string) string {
	return lipgloss.NewStyle().Foreground(colAmber).Bold(true).Render("["+k+"]") +
		lipgloss.NewStyle().Foreground(colAmberDim).Render("·"+label)
}
