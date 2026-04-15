package ui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// wrapHang produces a list of lines from `body` so that:
//   - the first line fits in `width`,
//   - any continuation lines are prefixed with `hang` (spaces) and fit in `width`.
//
// Respects existing newlines in `body`. Never splits mid-word unless a single
// word is longer than the available width, in which case it hard-breaks.
func wrapHang(body string, firstIndent, hang string, width int) []string {
	if width <= 0 {
		return []string{body}
	}
	firstW := width - visWidth(firstIndent)
	restW := width - visWidth(hang)
	if firstW < 4 {
		firstW = 4
	}
	if restW < 4 {
		restW = 4
	}

	var out []string
	first := true
	for _, para := range strings.Split(body, "\n") {
		if para == "" {
			out = append(out, "")
			continue
		}
		w := restW
		ind := hang
		if first {
			w = firstW
			ind = firstIndent
			first = false
		}
		lines := wrapLine(para, w)
		for i, ln := range lines {
			if i == 0 {
				out = append(out, ind+ln)
			} else {
				out = append(out, hang+ln)
			}
		}
	}
	return out
}

// wrapText wraps plain body text to width, with no special first-line indent.
func wrapText(body string, width int) []string {
	if width <= 0 {
		return []string{body}
	}
	var out []string
	for _, para := range strings.Split(body, "\n") {
		if para == "" {
			out = append(out, "")
			continue
		}
		out = append(out, wrapLine(para, width)...)
	}
	return out
}

func wrapLine(line string, width int) []string {
	if visWidth(line) <= width {
		return []string{line}
	}
	var out []string
	words := strings.Fields(line)
	if len(words) == 0 {
		return []string{line}
	}
	var cur strings.Builder
	curW := 0
	for _, w := range words {
		wW := visWidth(w)
		if wW > width {
			// Single word longer than width: flush current, then hard-break.
			if cur.Len() > 0 {
				out = append(out, cur.String())
				cur.Reset()
				curW = 0
			}
			for visWidth(w) > width {
				cut := cutRunes(w, width)
				out = append(out, cut)
				w = w[len(cut):]
			}
			if w != "" {
				cur.WriteString(w)
				curW = visWidth(w)
			}
			continue
		}
		sep := 0
		if cur.Len() > 0 {
			sep = 1
		}
		if curW+sep+wW > width {
			out = append(out, cur.String())
			cur.Reset()
			cur.WriteString(w)
			curW = wW
		} else {
			if sep == 1 {
				cur.WriteByte(' ')
				curW++
			}
			cur.WriteString(w)
			curW += wW
		}
	}
	if cur.Len() > 0 {
		out = append(out, cur.String())
	}
	return out
}

// visWidth — visual column width (unicode-width aware, ANSI-stripping).
func visWidth(s string) int {
	return lipgloss.Width(s)
}

// cutRunes — return the prefix of s whose visual width is at most `n`.
// ANSI-aware: escape sequences pass through without contributing to width,
// and we never stop mid-sequence (otherwise the unterminated CSI swallows
// whatever follows, corrupting layout).
func cutRunes(s string, n int) string {
	if visWidth(s) <= n {
		return s
	}
	out := strings.Builder{}
	w := 0
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
		if w+rw > n {
			break
		}
		out.WriteRune(r)
		w += rw
	}
	return out.String()
}
