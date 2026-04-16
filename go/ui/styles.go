package ui

import "github.com/charmbracelet/lipgloss"

// Monochrome CRT phosphor — one amber hue at varying brightness, with a single
// alert-red accent reserved for states that actually demand user attention.
// No magenta, no cyan, no green. The shape of glyphs + header stripes does
// the work that color used to do.
var (
	colBG      = lipgloss.Color("#0A0A0F")
	colSurface = lipgloss.Color("#111118")

	// Amber scale — brightness is the hierarchy.
	colAmberDim = lipgloss.Color("#4A3000") // hints, scrollbar track, empty state
	colAmberMid = lipgloss.Color("#805400") // default frame, labels
	colAmber    = lipgloss.Color("#FFB000") // primary foreground, active glyphs
	colAmberHi  = lipgloss.Color("#FFE1A0") // focus, emphasis

	// Single accent: red = "needs your attention, now".
	colAlert = lipgloss.Color("#FF2D55")

	// Focus accent — cyan breaks from amber/red so the focused card pops
	// unambiguously.
	colFocus = lipgloss.Color("#00E1FF")

	colBlack = lipgloss.Color("#000000")
)

// Heavy box-drawing border for idle / working cards.
var borderHeavy = lipgloss.Border{
	Top:         "━",
	Bottom:      "━",
	Left:        "┃",
	Right:       "┃",
	TopLeft:     "┏",
	TopRight:    "┓",
	BottomLeft:  "┗",
	BottomRight: "┛",
}

// Double line for waiting / working — more visual weight than idle heavy.
var borderDouble = lipgloss.Border{
	Top:         "═",
	Bottom:      "═",
	Left:        "║",
	Right:       "║",
	TopLeft:     "╔",
	TopRight:    "╗",
	BottomLeft:  "╚",
	BottomRight: "╝",
}

// Focused border — standard double-border box. Corners and sides are
// designed to fit together without font-rendering gaps. Focus is carried by
// the cyan colour, which is unique to this state.
var borderFocused = lipgloss.Border{
	Top:         "═",
	Bottom:      "═",
	Left:        "║",
	Right:       "║",
	TopLeft:     "╔",
	TopRight:    "╗",
	BottomLeft:  "╚",
	BottomRight: "╝",
}

// ── Global frames ─────────────────────────────────────────────────────────

var (
	styleApp = lipgloss.NewStyle().Background(colBG).Foreground(colAmber)

	// Top banner — deep-amber stripe with bright amber text. Deliberately
	// duller than the working-card header (which owns bright amber) so the
	// eye ranks live card activity above static chrome.
	styleBanner = lipgloss.NewStyle().
			Background(colAmberDim).
			Foreground(colAmberHi).
			Bold(true).
			Padding(0, 1)

	// Bottom status line — dim amber, no emphasis.
	styleFooter = lipgloss.NewStyle().
			Foreground(colAmberDim).
			Padding(0, 1)

	// Elapsed / telemetry text — one step dimmer than body.
	styleElapsed = lipgloss.NewStyle().Foreground(colAmberDim)

	// ── Card frames ───────────────────────────────────────────────────────

	styleCard = lipgloss.NewStyle().
			Border(borderHeavy).
			BorderForeground(colAmberMid).
			Background(colBG).
			Padding(0, 1)

	// Working: double border at base-amber brightness — pairs with the
	// rotating spinner to signal live activity.
	styleCardWorking = styleCard.
				Border(borderDouble).
				BorderForeground(colAmber)

	// Waiting: red double border. The ONE color that says "look at me".
	styleCardWaiting = styleCard.
				Border(borderDouble).
				BorderForeground(colAlert)

	// Focused: cyan double border with diamond corner markers. Cyan is
	// reserved for focus — amber is for state, red for alert.
	styleCardFocused = styleCard.
				Border(borderFocused).
				BorderForeground(colFocus)

	// ── Card header stripes (inverse) ─────────────────────────────────────

	styleCardHeader = lipgloss.NewStyle().
			Bold(true).
			Foreground(colBlack).
			Background(colAmberMid)

	// Working: brighter inverse stripe. Pairs with the rotating spinner.
	styleCardHeaderWorking = lipgloss.NewStyle().
				Bold(true).
				Foreground(colBlack).
				Background(colAmber)

	// Waiting: red. The one reserved color for actual urgency.
	styleCardHeaderWaiting = lipgloss.NewStyle().
				Bold(true).
				Foreground(colBlack).
				Background(colAlert)

	// ── Event glyphs and labels ──────────────────────────────────────────
	//
	// All kinds share the same amber hue. The SHAPE of the glyph carries the
	// kind distinction — ✦ ▸ ◆ › — which is enough once you learn them.
	styleGlyph = map[string]lipgloss.Style{
		"thinking":    lipgloss.NewStyle().Foreground(colAmber).Bold(true),
		"tool_use":    lipgloss.NewStyle().Foreground(colAmber).Bold(true),
		"tool_result": lipgloss.NewStyle().Foreground(colAmber).Bold(true),
		"text":        lipgloss.NewStyle().Foreground(colAmber).Bold(true),
		"user_prompt": lipgloss.NewStyle().Foreground(colAmberHi).Bold(true),
		"unknown":     lipgloss.NewStyle().Foreground(colAmberDim),
	}

	styleLabel = lipgloss.NewStyle().Foreground(colAmberMid)

	styleToolName = map[string]lipgloss.Style{
		"tool_use":    lipgloss.NewStyle().Foreground(colAmberHi).Bold(true),
		"tool_result": lipgloss.NewStyle().Foreground(colAmberHi).Bold(true),
	}

	styleBody = lipgloss.NewStyle().Foreground(colAmber)

	styleEmpty = lipgloss.NewStyle().Foreground(colAmberDim).Italic(true)

	// ── Prompt panel (docked at bottom) ──────────────────────────────────

	stylePrompt = lipgloss.NewStyle().
			Border(borderHeavy).
			BorderForeground(colAmberHi).
			Background(colSurface).
			Padding(1, 2)

	stylePromptTitle = lipgloss.NewStyle().
				Foreground(colAmberHi).
				Bold(true)

	stylePromptHint = lipgloss.NewStyle().
				Foreground(colAmberDim).
				Italic(true)
)

// Glyph per kind — chunky, unambiguous at a glance.
var kindGlyph = map[string]string{
	"thinking":    "✦",
	"tool_use":    "▸",
	"tool_result": "◂",
	"text":        "◆",
	"user_prompt": "›",
	"unknown":     "·",
}

// Label — 5 chars, left of the summary. Blank for tool_result (hidden anyway).
var kindLabel = map[string]string{
	"thinking":    "THINK",
	"tool_use":    "TOOL ",
	"tool_result": "     ",
	"text":        "TEXT ",
	"user_prompt": "YOU  ",
	"unknown":     "???  ",
}
