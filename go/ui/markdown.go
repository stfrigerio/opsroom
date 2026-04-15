package ui

import (
	"strings"
	"sync"
	"unicode"

	"github.com/charmbracelet/lipgloss"
)

// Inline markdown → ANSI. Styling is applied per whitespace-separated word so
// the result survives wrapText's word-split: each rewrapped line carries
// balanced ANSI codes and no style bleeds across a wrap boundary.

var (
	styleMDBold   = lipgloss.NewStyle().Foreground(colAmberHi).Bold(true)
	styleMDItalic = lipgloss.NewStyle().Italic(true)
	styleMDCode   = lipgloss.NewStyle().Foreground(colAmberHi).Background(colSurface)
	styleMDHead   = lipgloss.NewStyle().Foreground(colAmberHi).Bold(true).Underline(true)
	styleMDBullet = lipgloss.NewStyle().Foreground(colAmberMid).Bold(true)
)

// mdCache memoizes renderInlineMarkdown outputs. The renderer does a lot of
// per-word lipgloss.Render work, and mouse-motion re-renders the whole grid
// for every pixel of motion — without this, focus-follow-cursor lags badly.
// Capped so it can't grow unbounded on long sessions.
var (
	mdCache   = make(map[string]string, 512)
	mdCacheMu sync.Mutex
)

const mdCacheMax = 1024

func renderInlineMarkdown(s string) string {
	mdCacheMu.Lock()
	if out, ok := mdCache[s]; ok {
		mdCacheMu.Unlock()
		return out
	}
	mdCacheMu.Unlock()

	lines := strings.Split(s, "\n")
	for i, line := range lines {
		lines[i] = renderMDLine(line)
	}
	out := strings.Join(lines, "\n")

	mdCacheMu.Lock()
	if len(mdCache) >= mdCacheMax {
		mdCache = make(map[string]string, 512)
	}
	mdCache[s] = out
	mdCacheMu.Unlock()
	return out
}

func renderMDLine(line string) string {
	trim := strings.TrimLeft(line, " \t")
	indent := line[:len(line)-len(trim)]

	if h := countHashes(trim); h > 0 && h < len(trim) && trim[h] == ' ' {
		return indent + renderMDSpans(strings.TrimLeft(trim[h:], " "), &styleMDHead)
	}
	if len(trim) >= 2 && (trim[0] == '-' || trim[0] == '*') && trim[1] == ' ' {
		return indent + styleMDBullet.Render("•") + " " + renderMDSpans(trim[2:], &styleBody)
	}
	return indent + renderMDSpans(trim, &styleBody)
}

func countHashes(s string) int {
	n := 0
	for n < len(s) && n < 6 && s[n] == '#' {
		n++
	}
	return n
}

// renderMDSpans parses inline markers. base, if non-nil, styles everything
// that doesn't fall inside a more specific span. Nested spans use the
// innermost style only — simpler and the common case.
func renderMDSpans(s string, base *lipgloss.Style) string {
	runes := []rune(s)
	var out, buf strings.Builder

	var styles []*lipgloss.Style
	var kinds []string
	if base != nil {
		styles = append(styles, base)
		kinds = append(kinds, "base")
	}

	flush := func() {
		if buf.Len() == 0 {
			return
		}
		if len(styles) == 0 {
			out.WriteString(buf.String())
		} else {
			emitMDWords(&out, buf.String(), *styles[len(styles)-1])
		}
		buf.Reset()
	}
	top := func() string {
		if len(kinds) == 0 {
			return ""
		}
		return kinds[len(kinds)-1]
	}
	push := func(kind string, st *lipgloss.Style) {
		kinds = append(kinds, kind)
		styles = append(styles, st)
	}
	pop := func() {
		kinds = kinds[:len(kinds)-1]
		styles = styles[:len(styles)-1]
	}

	n := len(runes)
	i := 0
	for i < n {
		r := runes[i]

		if r == '`' {
			if j := findRune(runes, i+1, '`'); j > 0 {
				flush()
				emitMDWords(&out, string(runes[i+1:j]), styleMDCode)
				i = j + 1
				continue
			}
		}

		if r == '*' && i+1 < n && runes[i+1] == '*' {
			if top() == "bold" {
				flush()
				pop()
				i += 2
				continue
			}
			if findSeq(runes, i+2, "**") > 0 {
				flush()
				push("bold", &styleMDBold)
				i += 2
				continue
			}
		}

		if (r == '*' || r == '_') && !flankedByWord(runes, i) {
			if top() == "italic" {
				flush()
				pop()
				i++
				continue
			}
			if findRune(runes, i+1, r) > 0 {
				flush()
				push("italic", &styleMDItalic)
				i++
				continue
			}
		}

		buf.WriteRune(r)
		i++
	}
	flush()
	// Any unclosed spans just drop off — their buffered text was emitted with
	// whatever style was live at the time.
	return out.String()
}

func emitMDWords(out *strings.Builder, text string, st lipgloss.Style) {
	var word strings.Builder
	flushWord := func() {
		if word.Len() > 0 {
			out.WriteString(st.Render(word.String()))
			word.Reset()
		}
	}
	for _, r := range text {
		if unicode.IsSpace(r) {
			flushWord()
			out.WriteRune(r)
		} else {
			word.WriteRune(r)
		}
	}
	flushWord()
}

func findRune(runes []rune, start int, r rune) int {
	for i := start; i < len(runes); i++ {
		if runes[i] == r {
			return i
		}
	}
	return -1
}

func findSeq(runes []rune, start int, seq string) int {
	sr := []rune(seq)
	for i := start; i+len(sr) <= len(runes); i++ {
		match := true
		for k, x := range sr {
			if runes[i+k] != x {
				match = false
				break
			}
		}
		if match {
			return i
		}
	}
	return -1
}

// flankedByWord — `foo_bar` and `2*3` shouldn't trigger italic.
func flankedByWord(runes []rune, i int) bool {
	prev := i > 0 && isWordRune(runes[i-1])
	next := i+1 < len(runes) && isWordRune(runes[i+1])
	return prev && next
}

func isWordRune(r rune) bool {
	return unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_'
}
