package ui

import (
	"strings"

	"opsroom/claude"
)

// ── event log viewport ────────────────────────────────────────────────────

type eventLogInput struct {
	events []claude.Event
	width  int
	height int
	scroll int
	sticky bool
}

type eventLogOutput struct {
	rendered string
	scroll   int // clamped to valid range
	maxOff   int // = len(lines) - height
}

// renderEventLog — pure function. Wraps events into lines, picks the visible
// window, returns both the rendered string and the resolved scroll/max so
// the caller can persist them. Nothing reaches back into model state.
func renderEventLog(in eventLogInput) eventLogOutput {
	lines := buildEventLines(in.events, in.width)

	maxOff := len(lines) - in.height
	if maxOff < 0 {
		maxOff = 0
	}

	off := in.scroll
	if in.sticky {
		off = maxOff
	}
	if off < 0 {
		off = 0
	}
	if off > maxOff {
		off = maxOff
	}

	end := off + in.height
	if end > len(lines) {
		end = len(lines)
	}
	vis := make([]string, 0, in.height)
	vis = append(vis, lines[off:end]...)
	for len(vis) < in.height {
		vis = append(vis, "")
	}
	return eventLogOutput{
		rendered: strings.Join(vis, "\n"),
		scroll:   off,
		maxOff:   maxOff,
	}
}

// ── event → lines ────────────────────────────────────────────────────────

// Layout constants for one event block:
//
//	"  ▸  TOOL   <summary>"           ← 2 + 1 + 2 + 5 + 2 = 12 cols prefix
//	"            <continuation>"      ← 12 cols hanging indent
const (
	elRowIndent   = "  "
	elLabelGap    = "  "
	elSummaryGap  = "  "
	elPrefixWidth = 2 + 1 + 2 + 5 + 2
)

// buildEventLines — produce fully styled, wrapped lines for every event.
// Wrap math is done on plain text; ANSI is applied after wrapping so it
// never throws off width measurement.
func buildEventLines(events []claude.Event, width int) []string {
	if len(events) == 0 {
		return []string{styleEmpty.Render("(fresh session — press i to send)")}
	}

	hang := strings.Repeat(" ", elPrefixWidth)
	// -2 safety margin: terminals miscount wide chars; we also want lipgloss
	// frames to have zero reason to re-wrap us.
	summaryW := width - elPrefixWidth - 2
	if summaryW < 10 {
		summaryW = 10
	}

	var lines []string
	for i, ev := range events {
		if i > 0 {
			lines = append(lines, "") // breathing room between events
		}
		kind := string(ev.Kind)
		glyph := kindGlyph[kind]
		if glyph == "" {
			glyph = "·"
		}
		label := kindLabel[kind]
		if label == "" {
			label = "???  "
		}
		styledPrefix := elRowIndent +
			styleGlyph[kind].Render(glyph) +
			elLabelGap +
			styleLabel.Render(label) +
			elSummaryGap

		switch ev.Kind {
		case claude.KindToolUse, claude.KindToolResult:
			tool := ev.ToolName
			if tool == "" {
				tool = "(result)"
			}
			lines = append(lines, styledPrefix+styleToolName[kind].Render(tool))
			if ev.Summary != "" {
				for _, ln := range wrapText(ev.Summary, summaryW) {
					lines = append(lines, hang+styleBody.Render(ln))
				}
			}

		default:
			if ev.Summary == "" {
				lines = append(lines, styledPrefix+styleEmpty.Render("(empty)"))
				continue
			}
			summary := ev.Summary
			if ev.Kind == claude.KindText || ev.Kind == claude.KindThinking {
				summary = renderInlineMarkdown(summary)
			}
			wrapped := wrapText(summary, summaryW)
			if len(wrapped) == 0 {
				wrapped = []string{ev.Summary}
			}
			// Markdown-rendered text carries its own per-word ANSI; wrapping
			// it in styleBody would reset those codes at the outer \x1b[0m.
			// Fall back to styleBody only for kinds we didn't pre-render.
			styleLine := func(ln string) string {
				if ev.Kind == claude.KindText || ev.Kind == claude.KindThinking {
					return ln
				}
				return styleBody.Render(ln)
			}
			lines = append(lines, styledPrefix+styleLine(wrapped[0]))
			for _, ln := range wrapped[1:] {
				lines = append(lines, hang+styleLine(ln))
			}
		}
	}
	return lines
}
