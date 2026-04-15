package ui

import "github.com/charmbracelet/lipgloss"

// cardState captures every signal that drives a card's visual language.
// Every renderer reads from this and nowhere else, so precedence logic —
// focus vs. waiting vs. working — lives in a single place.
type cardState struct {
	focused bool
	working bool
	waiting bool
}

// frameStyle — border around the card. Focus wins so the user always knows
// where their keystrokes go.
func (s cardState) frameStyle() lipgloss.Style {
	switch {
	case s.focused:
		return styleCardFocused
	case s.waiting:
		return styleCardWaiting
	case s.working:
		return styleCardWorking
	}
	return styleCard
}

// headerStyle — inverse stripe inside the card. Activity drives this; focus
// stays out so waiting/working remain visible on the focused card.
func (s cardState) headerStyle() lipgloss.Style {
	switch {
	case s.waiting:
		return styleCardHeaderWaiting
	case s.working:
		return styleCardHeaderWorking
	}
	return styleCardHeader
}

// thumbColor — scrollbar thumb. Red (waiting) is the one reserved alert,
// cyan for focus, amber for working, dim amber for idle.
func (s cardState) thumbColor() lipgloss.Color {
	switch {
	case s.waiting:
		return colAlert
	case s.focused:
		return colFocus
	case s.working:
		return colAmber
	}
	return colAmberMid
}
