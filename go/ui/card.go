package ui

import (
	"time"

	"github.com/charmbracelet/lipgloss"

	"opsroom/claude"
	"opsroom/hypr"
)

// cardInput — everything needed to render one card. Explicit dependencies
// keep this renderer decoupled from model internals.
type cardInput struct {
	session *claude.Session
	window  *hypr.Window
	state   cardState
	width   int
	height  int
	scroll  int
	sticky  bool
	now     time.Time
	// slotSuffix — "#1"/"#2"/… when multiple claudes share the same
	// (workspace, project). Empty when this card is unique.
	slotSuffix string
}

// cardOutput — rendered card plus the clamped scroll/max that the caller
// should persist back into the model.
type cardOutput struct {
	rendered string
	scroll   int
	maxOff   int
}

// renderCard — assemble frame + header + body + scrollbar into one string,
// sized exactly (width × height). Composes pure sub-renderers, so nothing
// here needs to know how any component measures itself.
func renderCard(in cardInput) cardOutput {
	frame := in.state.frameStyle()

	innerW := in.width - frame.GetHorizontalBorderSize() - frame.GetHorizontalPadding()
	innerH := in.height - frame.GetVerticalBorderSize() - frame.GetVerticalPadding()
	if innerW < 8 {
		innerW = 8
	}
	if innerH < 3 {
		innerH = 3
	}

	header := renderCardHeader(in.session, in.window, in.state.headerStyle(), innerW, in.now, in.slotSuffix)

	bodyH := innerH - lipgloss.Height(header)
	if bodyH < 1 {
		bodyH = 1
	}
	bodyW := innerW - 1 // reserve 1 col for the scrollbar
	if bodyW < 8 {
		bodyW = 8
	}

	log := renderEventLog(eventLogInput{
		events: in.session.Events,
		width:  bodyW,
		height: bodyH,
		scroll: in.scroll,
		sticky: in.sticky,
	})

	bar := renderScrollbar(log.scroll, log.maxOff, bodyH, in.state.thumbColor())
	bodyRow := lipgloss.JoinHorizontal(lipgloss.Top, log.rendered, bar)

	inner := lipgloss.JoinVertical(lipgloss.Left, header, bodyRow)
	// Guarantee every inner line is exactly innerW wide so the frame style
	// can't trigger an internal re-wrap.
	inner = clampLinesToWidth(inner, innerW)

	return cardOutput{
		rendered: frame.Width(innerW).Height(innerH).Render(inner),
		scroll:   log.scroll,
		maxOff:   log.maxOff,
	}
}
