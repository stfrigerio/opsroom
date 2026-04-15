package ui

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
)

// ── spinner ───────────────────────────────────────────────────────────────

var spinnerFrames = []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}

func workingSpinner(now time.Time) string {
	i := int(now.UnixNano()/int64(refreshInterval)) % len(spinnerFrames)
	if i < 0 {
		i = -i
	}
	return spinnerFrames[i]
}

// ── elapsed formatter ────────────────────────────────────────────────────

func formatElapsed(t, now time.Time) string {
	if t.IsZero() {
		return "—"
	}
	d := now.Sub(t)
	if d < time.Second {
		d = 0
	}
	s := int(d.Seconds())
	if s < 60 {
		return fmt.Sprintf("%ds", s)
	}
	if s < 3600 {
		return fmt.Sprintf("%dm%02ds", s/60, s%60)
	}
	return fmt.Sprintf("%dh%02dm", s/3600, (s%3600)/60)
}

// ── line-width guards ─────────────────────────────────────────────────────

// clampLinesToWidth — pad short lines to `width` with trailing spaces,
// hard-truncate (ANSI-aware) anything longer. Used right before a lipgloss
// frame so it has zero reason to re-wrap our content internally.
func clampLinesToWidth(s string, width int) string {
	lines := strings.Split(s, "\n")
	for i, ln := range lines {
		w := lipgloss.Width(ln)
		if w == width {
			continue
		}
		if w < width {
			lines[i] = ln + strings.Repeat(" ", width-w)
			continue
		}
		lines[i] = truncateToWidth(ln, width)
	}
	return strings.Join(lines, "\n")
}

func truncateToWidth(s string, width int) string {
	var out strings.Builder
	visible := 0
	inEscape := false
	for _, r := range s {
		if r == 0x1b {
			inEscape = true
			out.WriteRune(r)
			continue
		}
		if inEscape {
			out.WriteRune(r)
			if r == 'm' {
				inEscape = false
			}
			continue
		}
		rw := lipgloss.Width(string(r))
		if visible+rw > width {
			break
		}
		out.WriteRune(r)
		visible += rw
	}
	return out.String()
}

// ── slice helpers ─────────────────────────────────────────────────────────

func interleave(items []string, sep string) []string {
	if len(items) <= 1 {
		return items
	}
	out := make([]string, 0, len(items)*2-1)
	for i, it := range items {
		if i > 0 {
			out = append(out, sep)
		}
		out = append(out, it)
	}
	return out
}
