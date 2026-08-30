package ui

import (
	"image/color"
	"strconv"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	uv "github.com/charmbracelet/ultraviolet"
	"github.com/charmbracelet/x/ansi"
)

// Asks the terminal for its own colours, so the system theme is drawn in them. The ground
// and the ink each come over their own request. The sixteen palette slots come over OSC 4,
// which the terminal returns one slot at a time, and a terminal need not answer at all.

// paletteSlots is how many slots the theme reads. The hues live in the first sixteen.
const paletteSlots = 16

// standardPalette is what a terminal that returns nothing is taken to hold. These are the
// colours every terminal starts from, so a theme built on them is readable.
var standardPalette = [paletteSlots]color.Color{
	rgb(0, 0, 0), rgb(128, 0, 0), rgb(0, 128, 0), rgb(128, 128, 0),
	rgb(0, 0, 128), rgb(128, 0, 128), rgb(0, 128, 128), rgb(192, 192, 192),
	rgb(128, 128, 128), rgb(255, 0, 0), rgb(0, 255, 0), rgb(255, 255, 0),
	rgb(0, 0, 255), rgb(255, 0, 255), rgb(0, 255, 255), rgb(255, 255, 255),
}

func rgb(red, green, blue uint8) color.Color {
	return color.RGBA{R: red, G: green, B: blue, A: 0xff}
}

// RequestTerminalPalette asks the terminal for every slot the theme reads. One request per
// slot, because a terminal returns each one on its own.
func RequestTerminalPalette() tea.Cmd {
	commands := make([]tea.Cmd, 0, paletteSlots)
	for slot := range paletteSlots {
		commands = append(commands, tea.Raw("\x1b]4;"+strconv.Itoa(slot)+";?\x07"))
	}
	return tea.Batch(commands...)
}

// readPaletteAnswer reads one answer to a palette request. An answer names the slot and the
// colour, as `4;3;rgb:cdcd/cdcd/0000`. Anything else is not an answer to this request.
func readPaletteAnswer(event uv.UnknownOscEvent) (int, color.Color, bool) {
	written := string(event)
	// The opening and the closing of the sequence are cut off, so only the body is read.
	if at := strings.IndexByte(written, ']'); at != -1 {
		written = written[at+1:]
	}
	written = strings.TrimRight(written, "\a\x1b\\")

	parts := strings.SplitN(written, ";", 3)
	if len(parts) != 3 || parts[0] != "4" {
		return 0, nil, false
	}
	slot, err := strconv.Atoi(parts[1])
	if err != nil || slot < 0 || slot >= paletteSlots {
		return 0, nil, false
	}
	held := ansi.XParseColor(parts[2])
	if held == nil {
		return 0, nil, false
	}
	return slot, held, true
}

// sameColor is true where two answers name the same colour, so an answer that changed nothing
// repaints nothing.
func sameColor(held, reported color.Color) bool {
	if held == nil || reported == nil {
		return held == nil && reported == nil
	}
	heldRed, heldGreen, heldBlue, heldAlpha := held.RGBA()
	red, green, blue, alpha := reported.RGBA()
	return heldRed == red && heldGreen == green && heldBlue == blue && heldAlpha == alpha
}

type terminalColorState struct {
	ground    color.Color
	hasGround bool
	ink       color.Color
	hasInk    bool
	// The palette slots the terminal answered for, each in its own message.
	palette    [paletteSlots]color.Color
	hasPalette [paletteSlots]bool
	// True until the terminal has answered, or the wait for it has run out.
	waiting bool
	// When the colours of the terminal were last read again, so a change of them is
	// picked up without asking on every wake.
	watchedAt time.Time
	// How many seconds are left before the terminal is asked for its colours again, and how
	// long the next wait is.
	askAgainIn    int
	askAgainAfter int
}

func newTerminalColorState(waiting bool) terminalColorState {
	return terminalColorState{
		waiting:       waiting,
		askAgainIn:    firstAskAgainSeconds,
		askAgainAfter: firstAskAgainSeconds,
	}
}

func (state *terminalColorState) keepGround(reported color.Color) bool {
	if state.hasGround && sameColor(state.ground, reported) {
		return false
	}
	state.ground, state.hasGround = reported, true
	return true
}

func (state *terminalColorState) keepInk(reported color.Color) bool {
	if state.hasInk && sameColor(state.ink, reported) {
		return false
	}
	state.ink, state.hasInk = reported, true
	return true
}

func (state *terminalColorState) keepSlot(slot int, reported color.Color) bool {
	if state.hasPalette[slot] && sameColor(state.palette[slot], reported) {
		return false
	}
	state.palette[slot], state.hasPalette[slot] = reported, true
	return true
}

// hasEvery is true once the terminal has answered for its ground, its ink and every slot the
// theme reads.
func (state *terminalColorState) hasEvery() bool {
	if !state.hasGround || !state.hasInk {
		return false
	}
	for _, answered := range state.hasPalette {
		if !answered {
			return false
		}
	}
	return true
}

func (state *terminalColorState) describe() TerminalColors {
	return TerminalColors{
		Background: state.ground, HasBackground: state.hasGround,
		Foreground: state.ink, HasForeground: state.hasInk,
		Palette: state.palette, HasPalette: state.hasPalette,
	}
}

// A terminal reports nothing at all when a slot of its palette is edited.
func (state *terminalColorState) takeWatch(now time.Time) bool {
	if now.Sub(state.watchedAt) < watchTerminalWait {
		return false
	}
	state.watchedAt = now
	return true
}

// A window the compositor stopped drawing answers nothing.
func (state *terminalColorState) takeAsk() bool {
	state.askAgainIn--
	if state.askAgainIn > 0 {
		return false
	}
	state.askAgainAfter = min(state.askAgainAfter*2, longestAskAgainSeconds)
	state.askAgainIn = state.askAgainAfter
	return true
}
