package ui

import (
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"

	"wall/claude"
	"wall/hypr"
)

// gridInput — everything renderGrid needs. `scroll` and `maxScroll` are maps
// passed by reference; renderGrid writes back the clamped scroll and the
// measured maxOff per pid so scroll handlers can clamp user input next time.
type gridInput struct {
	order     []int
	sessions  []claude.Session
	windows   []hypr.Window
	focus     int
	width     int
	height    int
	scroll    map[int]int
	sticky    map[int]bool
	maxScroll map[int]int
	slots       map[int]string // pid → "#1"/"#2"/… (empty / missing = solo)
	page        int            // 0-based current page when cards paginate
	focusedOnly bool           // render only the focused card at full body size
	hideLabels  bool           // drop TOOL/TEXT/YOU/etc. labels from event rows
	now         time.Time
}

// Layout tunables.
const (
	gridGap         = 1  // blank columns between cards horizontally
	minCardH        = 12 // below this we paginate instead of shrinking
	maxRowsPerPage  = 2
)

// gridLayout — pure computation of page/row/col/card dimensions given the
// total number of cards and the available viewport. Separated from rendering
// so pagination math has exactly one source of truth.
type gridLayout struct {
	cols           int
	rowsPerPage    int
	pageCount      int
	cardsPerPage   int
	cardW          int
	rowHeights     []int // per-row card height within the page; lengths vary ≤ rowsPerPage
}

func computeGridLayout(n, width, height int) gridLayout {
	cols := gridCols
	if n < cols {
		cols = n
	}
	if cols < 1 {
		cols = 1
	}
	totalRows := (n + cols - 1) / cols

	// How many rows fit without squashing cards below minCardH.
	maxByHeight := height / minCardH
	if maxByHeight < 1 {
		maxByHeight = 1
	}
	rowsPerPage := totalRows
	if rowsPerPage > maxRowsPerPage {
		rowsPerPage = maxRowsPerPage
	}
	if rowsPerPage > maxByHeight {
		rowsPerPage = maxByHeight
	}
	if rowsPerPage < 1 {
		rowsPerPage = 1
	}
	pageCount := (totalRows + rowsPerPage - 1) / rowsPerPage

	cardW := (width - gridGap*(cols-1)) / cols
	if cardW < 20 {
		cardW = 20
	}

	// Distribute height across the rows we're actually drawing. Remainder
	// pixels go to the first rows so nothing is clipped at the bottom.
	base := height / rowsPerPage
	extra := height % rowsPerPage
	rowHeights := make([]int, rowsPerPage)
	for i := range rowHeights {
		rowHeights[i] = base
		if i < extra {
			rowHeights[i]++
		}
	}

	return gridLayout{
		cols:         cols,
		rowsPerPage:  rowsPerPage,
		pageCount:    pageCount,
		cardsPerPage: rowsPerPage * cols,
		cardW:        cardW,
		rowHeights:   rowHeights,
	}
}

// renderGrid — lay out the current page of cards, padding empty slots so
// the grid keeps a consistent shape.
func renderGrid(in gridInput) string {
	if len(in.order) == 0 {
		return renderEmpty(in.width, in.height)
	}

	if in.focusedOnly && in.focus >= 0 && in.focus < len(in.order) {
		if s := renderFocusedOnly(in); s != "" {
			return s
		}
	}

	layout := computeGridLayout(len(in.order), in.width, in.height)

	// Clamp page to valid range.
	page := in.page
	if page < 0 {
		page = 0
	}
	if page >= layout.pageCount {
		page = layout.pageCount - 1
	}
	startIdx := page * layout.cardsPerPage

	sessByPID := make(map[int]*claude.Session, len(in.sessions))
	for i := range in.sessions {
		sessByPID[in.sessions[i].PID] = &in.sessions[i]
	}

	var rowStrs []string
	for r := 0; r < layout.rowsPerPage; r++ {
		cardH := layout.rowHeights[r]
		var cells []string
		for c := 0; c < layout.cols; c++ {
			idx := startIdx + r*layout.cols + c
			if idx >= len(in.order) {
				cells = append(cells, blankCell(layout.cardW, cardH))
				continue
			}
			pid := in.order[idx]
			sess := sessByPID[pid]
			if sess == nil {
				cells = append(cells, blankCell(layout.cardW, cardH))
				continue
			}
			stale := sess.IsWaiting && !sess.LastEventTS.IsZero() &&
				in.now.Sub(sess.LastEventTS) > staleAfter
			out := renderCard(cardInput{
				session: sess,
				window:  hypr.WindowForPID(pid, in.windows),
				state: cardState{
					focused: idx == in.focus,
					working: sess.IsWorking,
					waiting: sess.IsWaiting,
					stale:   stale,
				},
				width:      layout.cardW,
				height:     cardH,
				scroll:     in.scroll[pid],
				sticky:     in.sticky[pid],
				hideLabels: in.hideLabels,
				now:        in.now,
				slotSuffix: in.slots[pid],
			})
			in.scroll[pid] = out.scroll
			in.maxScroll[pid] = out.maxOff
			cells = append(cells, out.rendered)
		}
		row := lipgloss.JoinHorizontal(
			lipgloss.Top,
			interleave(cells, strings.Repeat(" ", gridGap))...,
		)
		rowStrs = append(rowStrs, row)
	}
	return lipgloss.JoinVertical(lipgloss.Left, rowStrs...)
}

// renderFocusedOnly — single-card zoom path: render just the focused
// session filling the whole body area. Returns "" if the focused pid has no
// live session so the caller falls back to normal grid layout.
func renderFocusedOnly(in gridInput) string {
	pid := in.order[in.focus]
	var sess *claude.Session
	for i := range in.sessions {
		if in.sessions[i].PID == pid {
			sess = &in.sessions[i]
			break
		}
	}
	if sess == nil {
		return ""
	}
	stale := sess.IsWaiting && !sess.LastEventTS.IsZero() &&
		in.now.Sub(sess.LastEventTS) > staleAfter
	out := renderCard(cardInput{
		session: sess,
		window:  hypr.WindowForPID(pid, in.windows),
		state: cardState{
			focused: true,
			working: sess.IsWorking,
			waiting: sess.IsWaiting,
			stale:   stale,
		},
		width:      in.width,
		height:     in.height,
		scroll:     in.scroll[pid],
		sticky:     in.sticky[pid],
		hideLabels: in.hideLabels,
		now:        in.now,
		slotSuffix: in.slots[pid],
	})
	in.scroll[pid] = out.scroll
	in.maxScroll[pid] = out.maxOff
	return out.rendered
}

// blankCell — a correctly-sized empty rectangle for unoccupied grid slots.
func blankCell(w, h int) string {
	row := strings.Repeat(" ", w)
	rows := make([]string, h)
	for i := range rows {
		rows[i] = row
	}
	return strings.Join(rows, "\n")
}

// renderEmpty — centred italic amber-dim message when no claude sessions
// are detected.
func renderEmpty(width, height int) string {
	msg := lipgloss.NewStyle().
		Foreground(colAmberDim).
		Italic(true).
		Render("▪ no active claude sessions detected ▪")
	return lipgloss.Place(width, height, lipgloss.Center, lipgloss.Center, msg)
}
