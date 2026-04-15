package ui

import "github.com/charmbracelet/lipgloss"

// View — top-level compositor. The only job here is stacking banner / grid /
// (optional prompt panel) / (optional notification) / footer. Each sub-piece
// lives in its own file and has a self-contained signature, so a tweak to
// any single component can't accidentally shift another.
func (m Model) View() string {
	if m.width == 0 || m.height == 0 {
		return ""
	}

	banner := renderBanner(m.width, len(m.sessions), m.now)
	slots := computeSlotSuffixes(m.order, m.sessions, m.windows)
	pc := m.pageCount()
	footer := renderFooter(m.width, m.page, pc)

	// Reserve 1 extra row between banner and grid as breathing space; this
	// also protects us against off-by-one surprises if lipgloss.Height ever
	// miscounts the banner on wrapped/styled input.
	const topMargin = 1
	bodyH := m.height - lipgloss.Height(banner) - lipgloss.Height(footer) - topMargin
	if m.note != "" {
		bodyH--
	}
	if bodyH < 4 {
		bodyH = 4
	}

	var body string
	if m.promptOpen {
		panel := renderPromptPanel(
			m.width,
			m.sessionForPID(m.promptFor),
			m.windowForPID(m.promptFor),
			slots[m.promptFor],
			m.prompt,
		)
		panelH := lipgloss.Height(panel)
		gridH := bodyH - panelH
		if gridH < 4 {
			gridH = 4
		}
		body = lipgloss.JoinVertical(
			lipgloss.Left,
			renderGrid(m.gridInput(gridH, slots)),
			panel,
		)
	} else {
		body = renderGrid(m.gridInput(bodyH, slots))
	}

	parts := []string{banner, "", body}
	if m.note != "" {
		parts = append(parts, renderNote(m.width, m.note, m.noteSev))
	}
	parts = append(parts, footer)
	return lipgloss.JoinVertical(lipgloss.Left, parts...)
}

// gridInput — bundle model fields into renderGrid's input struct. Lives on
// Model so View() doesn't need to reach into private fields itself.
func (m Model) gridInput(height int, slots map[int]string) gridInput {
	return gridInput{
		order:     m.order,
		sessions:  m.sessions,
		windows:   m.windows,
		focus:     m.focus,
		width:     m.width,
		height:    height,
		scroll:    m.scroll,
		sticky:    m.sticky,
		maxScroll: m.maxScroll,
		slots:     slots,
		page:      m.page,
		now:       m.now,
	}
}
