package ui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"

	"wall/ports"
)

// renderPortsView — full-body table of every listening TCP socket. Columns:
// PORT · PROJECT · PROCESS · PID · BIND · CMD. `focus` highlights a single
// row; `off` scrolls the visible window when the table overflows.
func renderPortsView(ps []ports.Port, width, height, focus int) string {
	off := 0
	if len(ps) == 0 {
		return lipgloss.Place(width, height, lipgloss.Center, lipgloss.Center,
			lipgloss.NewStyle().Foreground(colAmberDim).Italic(true).
				Render("▪ no listening sockets ▪"))
	}

	// Column widths. CMD flexes to fill remaining space.
	const (
		wPort = 6
		wProj = 42
		wProc = 14
		wPID  = 7
		wBind = 15
		gap   = "  "
	)
	fixed := wPort + wProj + wProc + wPID + wBind + len(gap)*5
	wCmd := width - fixed - 2 // -2 breathing room
	if wCmd < 10 {
		wCmd = 10
	}

	headerStyle := lipgloss.NewStyle().Foreground(colAmberMid).Bold(true)
	header := lipgloss.JoinHorizontal(lipgloss.Left,
		headerStyle.Render(fit("PORT", wPort)), gap,
		headerStyle.Render(fit("PROJECT", wProj)), gap,
		headerStyle.Render(fit("PROCESS", wProc)), gap,
		headerStyle.Render(fit("PID", wPID)), gap,
		headerStyle.Render(fit("BIND", wBind)), gap,
		headerStyle.Render(fit("CMD", wCmd)),
	)
	rule := lipgloss.NewStyle().Foreground(colAmberDim).
		Render(strings.Repeat("─", width-2))

	bodyH := height - 2 // header + rule
	if bodyH < 1 {
		bodyH = 1
	}

	// Clamp off so focus stays on-screen.
	if focus < off {
		off = focus
	}
	if focus >= off+bodyH {
		off = focus - bodyH + 1
	}
	if off < 0 {
		off = 0
	}
	end := off + bodyH
	if end > len(ps) {
		end = len(ps)
	}

	var rows []string
	for i := off; i < end; i++ {
		p := ps[i]
		pidStr := "—"
		if p.PID > 0 {
			pidStr = fmt.Sprintf("%d", p.PID)
		}
		row := lipgloss.JoinHorizontal(lipgloss.Left,
			fit(fmt.Sprintf(":%d", p.Port), wPort), gap,
			fit(p.Project(), wProj), gap,
			fit(orDash(p.Comm), wProc), gap,
			fit(pidStr, wPID), gap,
			fit(p.Bind, wBind), gap,
			fit(orDash(p.Cmd), wCmd),
		)
		if i == focus {
			row = lipgloss.NewStyle().
				Foreground(colBlack).
				Background(colAmber).
				Render(row)
		} else if p.PID == 0 {
			row = lipgloss.NewStyle().Foreground(colAmberDim).Render(row)
		} else {
			row = lipgloss.NewStyle().Foreground(colAmber).Render(row)
		}
		rows = append(rows, row)
	}
	for len(rows) < bodyH {
		rows = append(rows, "")
	}

	return lipgloss.JoinVertical(lipgloss.Left,
		append([]string{header, rule}, rows...)...,
	)
}

// fit — left-align s in a column of width w, truncating with an ellipsis
// if it overflows and padding with spaces if it's shorter.
func fit(s string, w int) string {
	if w <= 0 {
		return ""
	}
	vw := lipgloss.Width(s)
	if vw == w {
		return s
	}
	if vw < w {
		return s + strings.Repeat(" ", w-vw)
	}
	// Truncate — reserve last col for "…".
	if w == 1 {
		return "…"
	}
	return truncateToWidth(s, w-1) + "…"
}

func orDash(s string) string {
	if s == "" {
		return "—"
	}
	return s
}
