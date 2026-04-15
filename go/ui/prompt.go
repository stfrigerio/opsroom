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
	var ws, proj string
	if sess != nil && win != nil {
		proj = strings.ToUpper(claude.ProjectName(sess.CWD))
		if slotSuffix != "" {
			proj = proj + " " + slotSuffix
		}
		ws = fmt.Sprintf("WS%d", win.Workspace)
	}

	// Frame takes 2 cols of border + 2 cols of padding on each side.
	const pad = 2
	border := 2
	innerW := width - border - pad*2
	if innerW < 20 {
		innerW = 20
	}

	// Title row — inverse INJECT stripe followed by a chained target
	// (WS › PROJECT) so the user reads it as a single sentence. ░▒▓
	// scanline caps on both sides give it a channel-band vibe.
	deco := lipgloss.NewStyle().Foreground(colAmberHi).Render("░▒▓")
	label := lipgloss.NewStyle().
		Background(colAmberHi).Foreground(colBlack).Bold(true).
		Render(" ⏵ INJECT ")
	sepT := lipgloss.NewStyle().Foreground(colAmberDim).Render(" › ")
	bright := lipgloss.NewStyle().Foreground(colAmberHi).Bold(true)
	chain := label
	if ws != "" {
		chain += sepT + bright.Render(ws)
	}
	if proj != "" {
		chain += sepT + bright.Render(proj)
	}
	if ws == "" && proj == "" {
		chain += sepT + lipgloss.NewStyle().Foreground(colAmberDim).
			Italic(true).Render("(no target)")
	}
	titleL := deco + " " + chain
	titleR := deco
	gap := innerW - lipgloss.Width(titleL) - lipgloss.Width(titleR)
	if gap < 1 {
		gap = 1
	}
	title := titleL + strings.Repeat(" ", gap) + titleR

	// Body: prompt-arrow prefix + textarea. The arrow lives in its own col
	// so the cursor lines up with subsequent rows visually.
	arrow := lipgloss.NewStyle().Foreground(colAmberHi).Bold(true).Render("› ")
	// Reserve 2 cols for "› ".
	ta.SetWidth(innerW - 2)
	body := lipgloss.JoinHorizontal(lipgloss.Top, arrow, ta.View())

	// Hint — diamond-separated pills. Dim chrome so the eye stays on the
	// input line.
	sep := lipgloss.NewStyle().Foreground(colAmberDim).Render("  ◇  ")
	hint := keyPill("⏎", "send") + sep +
		keyPill("\\⏎", "newline") + sep +
		keyPill("␛", "abort")

	inner := lipgloss.JoinVertical(lipgloss.Left,
		title,
		"",
		body,
		"",
		hint,
	)
	return stylePrompt.Width(innerW + pad*2).Render(inner)
}

// keyPill — small keybinding marker for the inject hint line.
func keyPill(k, label string) string {
	return lipgloss.NewStyle().Foreground(colAmber).Bold(true).Render("["+k+"]") +
		lipgloss.NewStyle().Foreground(colAmberDim).Render(" "+label)
}
