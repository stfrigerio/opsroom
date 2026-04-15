package ui

import (
	"fmt"
	"sort"
	"time"

	"github.com/charmbracelet/bubbles/textarea"
	tea "github.com/charmbracelet/bubbletea"

	"opsroom/claude"
	"opsroom/hypr"
	"opsroom/kitty"
)

const (
	refreshInterval = 2 * time.Second
	gridCols        = 3
	eventLimit      = 60
)

// ── messages ──────────────────────────────────────────────────────────────

type tickMsg time.Time

type scanResult struct {
	sessions []claude.Session
	windows  []hypr.Window
	err      error
}

type noteMsg struct {
	text     string
	severity string // "info" | "warn" | "error"
	expires  time.Time
}

type clearNoteMsg struct{}

type sendResult struct {
	label string
	match string
	pid   int
	err   error
}

// ── model ─────────────────────────────────────────────────────────────────

type Model struct {
	width, height int

	sessions []claude.Session
	windows  []hypr.Window

	// Stable ordering key for each session: slot index → PID.
	order []int

	focus int // index into `order`
	page  int // 0-based page of the grid when cards paginate

	// Per-pid scroll offset (in event-lines). sticky means "snap to bottom
	// on next rebuild". maxScroll is updated from View each frame and used
	// to clamp scroll input so it can't drift past the end.
	scroll    map[int]int
	sticky    map[int]bool
	maxScroll map[int]int

	// Prompt modal state.
	promptOpen bool
	promptFor  int // pid
	prompt     textarea.Model

	// Transient notification banner.
	note     string
	noteSev  string
	noteTill time.Time

	// Banner clock.
	now time.Time

	// Last error from scan, if any.
	scanErr error
}

// New — construct a fresh model.
func New() Model {
	ta := textarea.New()
	ta.Placeholder = "type your prompt…"
	ta.CharLimit = 0
	ta.ShowLineNumbers = false
	ta.SetHeight(3)
	stylePromptTextarea(&ta)

	return Model{
		scroll:    map[int]int{},
		sticky:    map[int]bool{},
		maxScroll: map[int]int{},
		prompt:    ta,
		now:       time.Now(),
	}
}

// Init — initial command batch.
func (m Model) Init() tea.Cmd {
	return tea.Batch(scanCmd(), tickCmd())
}

func tickCmd() tea.Cmd {
	return tea.Tick(refreshInterval, func(t time.Time) tea.Msg { return tickMsg(t) })
}

func scanCmd() tea.Cmd {
	return func() tea.Msg {
		sessions, err := claude.Discover(eventLimit)
		if err != nil {
			return scanResult{err: err}
		}
		wins, _ := hypr.Clients()
		return scanResult{sessions: sessions, windows: wins}
	}
}

func sendCmd(pid int, text, label string) tea.Cmd {
	return func() tea.Msg {
		match, err := kitty.SendText(pid, text, true)
		return sendResult{label: label, match: match, pid: pid, err: err}
	}
}

func jumpCmd(address string) tea.Cmd {
	return func() tea.Msg {
		_ = hypr.Focus(address)
		return nil
	}
}

// Update — message dispatch.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {

	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.prompt.SetWidth(min(120, msg.Width-8))

	case tickMsg:
		m.now = time.Time(msg)
		// Expire transient notes.
		if !m.noteTill.IsZero() && m.now.After(m.noteTill) {
			m.note = ""
		}
		return m, tea.Batch(scanCmd(), tickCmd())

	case scanResult:
		if msg.err != nil {
			m.scanErr = msg.err
			return m, nil
		}
		m.applyScan(msg.sessions, msg.windows)

	case tea.KeyMsg:
		if m.promptOpen {
			return m.updatePrompt(msg)
		}
		return m.updateGrid(msg)

	case tea.MouseMsg:
		if m.promptOpen {
			return m, nil
		}
		return m.updateMouse(msg)

	case sendResult:
		if msg.err != nil {
			m.setNote("send failed: "+msg.err.Error(), "error", 5*time.Second)
		} else {
			m.setNote(
				fmt.Sprintf("→ sent to %s  [pid %d · kitty %s]", msg.label, msg.pid, msg.match),
				"info", 4*time.Second,
			)
		}

	case clearNoteMsg:
		m.note = ""
	}
	return m, nil
}

func (m *Model) setNote(text, sev string, dur time.Duration) {
	m.note = text
	m.noteSev = sev
	m.noteTill = time.Now().Add(dur)
}

func (m Model) updateGrid(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "q", "ctrl+c":
		return m, tea.Quit
	case "r":
		return m, scanCmd()
	case "left", "h":
		m.moveFocus(-1)
	case "right", "l":
		m.moveFocus(+1)
	case "up", "k":
		m.moveFocusRow(-1)
	case "down", "j":
		m.moveFocusRow(+1)
	case "pgup":
		m.scrollFocused(-6)
	case "pgdown":
		m.scrollFocused(+6)
	case "home":
		m.scrollFocusedTo(0)
	case "end":
		m.stickFocused()
	case "enter":
		if w := m.focusedWindow(); w != nil {
			return m, jumpCmd(w.Address)
		}
	case "i":
		return m.openPrompt()
	case "]", "n", "ctrl+tab":
		m.nextPage()
	case "[", "p", "ctrl+shift+tab":
		m.prevPage()
	case "ctrl+left":
		m.swapFocus(-1)
	case "ctrl+right":
		m.swapFocus(+1)
	case "ctrl+up":
		m.swapFocus(-gridCols)
	case "ctrl+down":
		m.swapFocus(+gridCols)
	}
	return m, nil
}

// bodyHeightApprox — pagination math only needs a ballpark; the exact value
// is computed in View. Matches the approximation used in pageCount().
func (m Model) bodyHeightApprox() int {
	h := m.height - 3
	if h < 1 {
		h = 1
	}
	return h
}

// swapFocus — swap the focused pane with the one `delta` slots away in the
// linear order. Follows focus to the new position and flips the page if the
// swap crossed a page boundary.
func (m *Model) swapFocus(delta int) {
	if m.focus < 0 || m.focus >= len(m.order) {
		return
	}
	target := m.focus + delta
	if target < 0 || target >= len(m.order) {
		return
	}
	m.order[m.focus], m.order[target] = m.order[target], m.order[m.focus]
	m.focus = target
	if m.width == 0 {
		return
	}
	layout := computeGridLayout(len(m.order), m.width, m.bodyHeightApprox())
	m.page = target / layout.cardsPerPage
	m.clampPage()
}

func (m *Model) nextPage() {
	m.page++
	m.clampPage()
	m.focusIntoPage()
}

func (m *Model) prevPage() {
	m.page--
	m.clampPage()
	m.focusIntoPage()
}

// pageCount — derived, matches grid layout.
func (m Model) pageCount() int {
	if m.width == 0 || len(m.order) == 0 {
		return 1
	}
	// We don't know bodyH here exactly (depends on banner/footer/note), so
	// approximate with m.height-3. Pagination math only needs a ballpark;
	// actual rendering uses the real computed value via computeGridLayout.
	h := m.height - 3
	if h < 1 {
		h = 1
	}
	return computeGridLayout(len(m.order), m.width, h).pageCount
}

func (m *Model) clampPage() {
	pc := m.pageCount()
	if m.page < 0 {
		m.page = 0
	}
	if m.page >= pc {
		m.page = pc - 1
	}
}

// focusIntoPage — ensure m.focus lands on a card that's actually on the
// current page. If the previously-focused card is off-page, pull focus to
// the first slot of the new page.
func (m *Model) focusIntoPage() {
	if m.width == 0 || len(m.order) == 0 {
		return
	}
	h := m.height - 3
	if h < 1 {
		h = 1
	}
	layout := computeGridLayout(len(m.order), m.width, h)
	start := m.page * layout.cardsPerPage
	end := start + layout.cardsPerPage
	if end > len(m.order) {
		end = len(m.order)
	}
	if m.focus < start || m.focus >= end {
		m.focus = start
	}
}

func (m Model) updateMouse(msg tea.MouseMsg) (tea.Model, tea.Cmd) {
	// Hover-to-focus: any mouse movement auto-focuses the card under the
	// cursor. Button is MouseButtonNone for bare motion events.
	if msg.Action == tea.MouseActionMotion {
		m.focusAt(msg.X, msg.Y)
		return m, nil
	}
	switch msg.Button {
	case tea.MouseButtonWheelUp:
		m.focusAt(msg.X, msg.Y)
		m.scrollFocused(-3)
	case tea.MouseButtonWheelDown:
		m.focusAt(msg.X, msg.Y)
		m.scrollFocused(+3)
	case tea.MouseButtonLeft:
		if msg.Action == tea.MouseActionPress {
			m.focusAt(msg.X, msg.Y)
		}
	}
	return m, nil
}

// focusAt — set focus to the card under screen coords (x, y), if any.
// Mirrors View's geometry exactly: 1-row banner, 1-row top margin, optional
// 1-row note, 1-row footer. Grid layout and per-row heights come from the
// same computeGridLayout used by renderGrid, and m.page offsets the index
// into m.order so pagination stays consistent.
func (m *Model) focusAt(x, y int) {
	if len(m.order) == 0 || m.width == 0 || m.height == 0 {
		return
	}
	const bannerH, topMargin, footerH = 1, 1, 1
	noteH := 0
	if m.note != "" {
		noteH = 1
	}
	bodyTop := bannerH + topMargin
	bodyH := m.height - bannerH - topMargin - footerH - noteH
	if bodyH < 4 {
		bodyH = 4
	}
	if y < bodyTop || y >= bodyTop+bodyH {
		return
	}

	layout := computeGridLayout(len(m.order), m.width, bodyH)

	col := x / (layout.cardW + gridGap)
	if col >= layout.cols {
		col = layout.cols - 1
	}
	if col < 0 {
		return
	}

	// Resolve row by walking the per-row heights — View gives the first
	// rows an extra line when bodyH doesn't divide evenly, so a plain
	// division miscounts near row boundaries.
	ry := y - bodyTop
	row := -1
	acc := 0
	for r, h := range layout.rowHeights {
		if ry < acc+h {
			row = r
			break
		}
		acc += h
	}
	if row < 0 {
		return
	}

	idx := m.page*layout.cardsPerPage + row*layout.cols + col
	if idx < 0 || idx >= len(m.order) {
		return
	}
	m.focus = idx
}

func (m Model) updatePrompt(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.promptOpen = false
		m.prompt.Reset()
		m.prompt.Blur()
		return m, nil

	case "enter":
		// Plain Enter = send. Shift+Enter / Ctrl+J fall through to the textarea.
		text := m.prompt.Value()
		m.promptOpen = false
		m.prompt.Reset()
		m.prompt.Blur()
		if text == "" {
			return m, nil
		}
		sess := m.sessionForPID(m.promptFor)
		win := m.windowForPID(m.promptFor)
		if sess == nil || win == nil {
			m.setNote("target session gone", "warn", 3*time.Second)
			return m, nil
		}
		label := fmt.Sprintf("WS%d %s", win.Workspace, claude.ProjectName(sess.CWD))
		return m, sendCmd(sess.PID, text, label)

	case "shift+enter", "ctrl+j":
		// Insert newline.
		var cmd tea.Cmd
		m.prompt, cmd = m.prompt.Update(msg)
		return m, cmd
	}

	var cmd tea.Cmd
	m.prompt, cmd = m.prompt.Update(msg)
	return m, cmd
}

func (m Model) openPrompt() (tea.Model, tea.Cmd) {
	sess := m.focusedSession()
	if sess == nil {
		m.setNote("focus a card first", "warn", 2*time.Second)
		return m, nil
	}
	// Real gate: does this claude's env have a kitty socket we can drive?
	// Window-class checks are unreliable because users rename their kitty
	// class (scratchpads, overrides, etc.).
	if !kitty.CanInject(sess.PID) {
		m.setNote("no kitty socket — see kitty.conf setup", "warn", 4*time.Second)
		return m, nil
	}
	m.promptOpen = true
	m.promptFor = sess.PID
	m.prompt.Focus()
	return m, textarea.Blink
}

// applyScan — merge new sessions/windows into the model, preserving
// scroll state and stable ordering.
func (m *Model) applyScan(sessions []claude.Session, windows []hypr.Window) {
	m.sessions = sessions
	m.windows = windows

	// Keep existing pids pinned to their current slot so a changing sort
	// key (e.g. a kitty window moving workspaces) doesn't reshuffle the
	// grid underneath the user. New pids are inserted in sorted order by
	// (workspace, project, pid); dead pids are dropped.
	type keyed struct {
		pid  int
		ws   int
		proj string
	}
	keyFor := func(s claude.Session) keyed {
		w := hypr.WindowForPID(s.PID, windows)
		ws := 1 << 30
		if w != nil {
			ws = w.Workspace
		}
		return keyed{pid: s.PID, ws: ws, proj: claude.ProjectName(s.CWD)}
	}
	less := func(a, b keyed) bool {
		if a.ws != b.ws {
			return a.ws < b.ws
		}
		if a.proj != b.proj {
			return a.proj < b.proj
		}
		return a.pid < b.pid
	}

	live := make(map[int]bool, len(sessions))
	for _, s := range sessions {
		live[s.PID] = true
	}

	// Drop dead pids from the existing order.
	inOrder := make(map[int]bool, len(m.order))
	order := make([]int, 0, len(sessions))
	for _, pid := range m.order {
		if live[pid] {
			order = append(order, pid)
			inOrder[pid] = true
		}
	}

	// A newcomer is any live pid that isn't already in the order. This
	// includes genuinely new sessions AND ones that briefly dropped out of
	// a scan (e.g. transcript mid-write after a send) and came back.
	var newcomers []keyed
	for _, s := range sessions {
		if inOrder[s.PID] {
			continue
		}
		if _, seen := m.sticky[s.PID]; !seen {
			m.sticky[s.PID] = true
		}
		newcomers = append(newcomers, keyFor(s))
	}

	// Insert newcomers in sorted position relative to each other and to
	// the existing order. For each newcomer, find the first existing slot
	// whose key compares greater and insert before it.
	sort.Slice(newcomers, func(i, j int) bool { return less(newcomers[i], newcomers[j]) })
	for _, nc := range newcomers {
		pos := len(order)
		for i, pid := range order {
			sess := sessionByPID(sessions, pid)
			if sess == nil {
				continue
			}
			if less(nc, keyFor(*sess)) {
				pos = i
				break
			}
		}
		order = append(order[:pos], append([]int{nc.pid}, order[pos:]...)...)
	}
	m.order = order

	// Clamp focus.
	if m.focus >= len(m.order) {
		m.focus = len(m.order) - 1
	}
	if m.focus < 0 {
		m.focus = 0
	}
}

// ── lookups ───────────────────────────────────────────────────────────────

func sessionByPID(sessions []claude.Session, pid int) *claude.Session {
	for i := range sessions {
		if sessions[i].PID == pid {
			return &sessions[i]
		}
	}
	return nil
}

func (m Model) sessionForPID(pid int) *claude.Session {
	for i := range m.sessions {
		if m.sessions[i].PID == pid {
			return &m.sessions[i]
		}
	}
	return nil
}

func (m Model) windowForPID(pid int) *hypr.Window {
	return hypr.WindowForPID(pid, m.windows)
}

func (m Model) focusedSession() *claude.Session {
	if m.focus < 0 || m.focus >= len(m.order) {
		return nil
	}
	return m.sessionForPID(m.order[m.focus])
}

func (m Model) focusedWindow() *hypr.Window {
	if s := m.focusedSession(); s != nil {
		return m.windowForPID(s.PID)
	}
	return nil
}

// ── navigation ────────────────────────────────────────────────────────────

func (m *Model) moveFocus(delta int) {
	if len(m.order) == 0 {
		return
	}
	n := len(m.order)
	m.focus = (m.focus + delta + n) % n
	m.followFocusToPage()
}

func (m *Model) moveFocusRow(delta int) {
	if len(m.order) == 0 {
		return
	}
	n := len(m.order)
	m.focus = (m.focus + delta*gridCols + n) % n
	m.followFocusToPage()
}

// followFocusToPage — flip to the page that contains m.focus so arrow
// navigation that crosses a page boundary is visible without a separate
// [ / ] press.
func (m *Model) followFocusToPage() {
	if m.width == 0 || len(m.order) == 0 {
		return
	}
	layout := computeGridLayout(len(m.order), m.width, m.bodyHeightApprox())
	if layout.cardsPerPage == 0 {
		return
	}
	m.page = m.focus / layout.cardsPerPage
	m.clampPage()
}

func (m *Model) scrollFocused(delta int) {
	if m.focus < 0 || m.focus >= len(m.order) {
		return
	}
	pid := m.order[m.focus]
	next := m.scroll[pid] + delta
	if next < 0 {
		next = 0
	}
	if max, ok := m.maxScroll[pid]; ok && next >= max {
		next = max
		m.sticky[pid] = true // hit bottom → re-pin so new events push us
	} else if delta < 0 {
		m.sticky[pid] = false // scrolling up breaks stickiness
	}
	m.scroll[pid] = next
}

func (m *Model) scrollFocusedTo(offset int) {
	if m.focus < 0 || m.focus >= len(m.order) {
		return
	}
	pid := m.order[m.focus]
	if offset < 0 {
		offset = 0
	}
	m.scroll[pid] = offset
	m.sticky[pid] = false
}

func (m *Model) stickFocused() {
	if m.focus < 0 || m.focus >= len(m.order) {
		return
	}
	pid := m.order[m.focus]
	m.sticky[pid] = true
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
