package ui

import (
	"image/color"
	"maps"
	"strings"
	"sync"
	"sync/atomic"
	"unicode"

	"charm.land/lipgloss/v2"
	"github.com/mattn/go-runewidth"
)

// A style measures every grapheme of the text it renders and checks the border, the margin
// and the padding it carries. A cell of a grid needs none of that: it needs the escapes of
// its colours, its text, and the reset. The escapes of one pair of colours are the same every
// time, so they are built once and kept.

// paintKey names one pair of colours and the weight, slant and rule they are written with.
type paintKey struct {
	ink, ground uint64
	bold        bool
	italic      bool
	underline   bool
}

// packColor returns a colour as one number. The high bit says a colour is there, so black is
// not read as no colour at all.
func packColor(held color.Color) uint64 {
	if held == nil {
		return 0
	}
	red, green, blue, alpha := held.RGBA()
	return 1<<32 | uint64(red>>8)<<24 | uint64(green>>8)<<16 |
		uint64(blue>>8)<<8 | uint64(alpha>>8)
}

// openings holds the escapes of each pair of colours the frames have used. The map is
// replaced whole rather than written into, so a frame reads it without a lock. Every pair a
// theme draws is found in the first frames and none is added after that.
var (
	openings     atomic.Pointer[map[paintKey]string]
	openingsLock sync.Mutex
)

// resolveOpening returns the escapes that open these colours, without the reset that closes
// them. A pair is built once and read from then on.
func resolveOpening(ink, ground color.Color) string {
	return resolveOpeningFor(ink, ground, false)
}

// resolveBoldOpening returns the escapes that open these colours in a heavier weight.
func resolveBoldOpening(ink, ground color.Color) string {
	return resolveOpeningFor(ink, ground, true)
}

func resolveOpeningFor(ink, ground color.Color, bold bool) string {
	key := paintKey{ink: packColor(ink), ground: packColor(ground), bold: bold}
	if opening, found := findOpening(key); found {
		return opening
	}
	style := lipgloss.NewStyle()
	if ink != nil {
		style = style.Foreground(ink)
	}
	if ground != nil {
		style = style.Background(ground)
	}
	if bold {
		style = style.Bold(true)
	}
	return keepOpening(key, style)
}

// findOpening returns the escapes of one key, and reports whether a frame has drawn it.
func findOpening(key paintKey) (string, bool) {
	held := openings.Load()
	if held == nil {
		return "", false
	}
	opening, found := (*held)[key]
	return opening, found
}

// openingProbe is the text the escapes are read from. A style renders an empty string as
// nothing at all where it carries a rule such as an underline, so one character is rendered
// and then cut away. No escape a style writes holds this character.
const openingProbe = "x"

// keepOpening reads the escapes this style opens with and keeps them under that key, so the
// frames after this one find them.
func keepOpening(key paintKey, style lipgloss.Style) string {
	rendered := style.Render(openingProbe)
	opening := ""
	if at := strings.LastIndex(rendered, openingProbe); at > 0 {
		opening = rendered[:at]
	}

	openingsLock.Lock()
	defer openingsLock.Unlock()
	grown := map[paintKey]string{key: opening}
	if held := openings.Load(); held != nil {
		maps.Copy(grown, *held)
	}
	openings.Store(&grown)
	return opening
}

// paintText writes the text in these colours, as a style renders it and without measuring it.
func paintText(ink, ground color.Color, text string) string {
	if text == "" {
		return ""
	}
	opening := resolveOpening(ink, ground)
	if opening == "" {
		return text
	}
	return opening + text + resetSequence
}

// paintBoldText writes the text in these colours in a heavier weight.
func paintBoldText(ink, ground color.Color, text string) string {
	if text == "" {
		return ""
	}
	opening := resolveBoldOpening(ink, ground)
	if opening == "" {
		return text
	}
	return opening + text + resetSequence
}

// writeTextOn writes what paintText returns into the builder.
func writeTextOn(dst *strings.Builder, ink, ground color.Color, text string) {
	if text == "" {
		return
	}
	writeOpenedText(dst, resolveOpening(ink, ground), text)
}

// writeOpenedText writes the text between these escapes and the reset that closes them.
func writeOpenedText(dst *strings.Builder, opening, text string) {
	if opening == "" {
		dst.WriteString(text)
		return
	}
	dst.WriteString(opening)
	dst.WriteString(text)
	dst.WriteString(resetSequence)
}

// paintOn writes the text on this ground, and leaves the ink to whatever drew before it.
func paintOn(ground color.Color, text string) string {
	return paintText(nil, ground, text)
}

// cellEscapeBytes is the room the colours of one cell take, which a buffer is grown by so a
// row of many cells is not grown again for each of them. An ink and a ground written as
// 24-bit colours take 33 bytes, and the reset that closes them takes 3.
const cellEscapeBytes = 40

// blankRun is the run a fill of blanks is cut from, so filling a row out to its width does
// not build the blanks first.
const blankRun = "                                                                "

// paintBlanks writes a run of blank cells on this ground.
func paintBlanks(ground color.Color, count int) string {
	if count <= 0 {
		return ""
	}
	var written strings.Builder
	written.Grow(count + 2*len(resetSequence))
	writeBlanksOn(&written, ground, count)
	return written.String()
}

// writeBlanksOn writes what paintBlanks returns into the builder.
func writeBlanksOn(dst *strings.Builder, ground color.Color, count int) {
	if count <= 0 {
		return
	}
	opening := resolveOpening(nil, ground)
	if opening != "" {
		dst.WriteString(opening)
	}
	writeBlanks(dst, count)
	if opening != "" {
		dst.WriteString(resetSequence)
	}
}

// writeBlanks writes that many blank cells, cut from the run rather than built.
func writeBlanks(dst *strings.Builder, count int) {
	for count > len(blankRun) {
		dst.WriteString(blankRun)
		count -= len(blankRun)
	}
	if count > 0 {
		dst.WriteString(blankRun[:count])
	}
}

// Measuring a drawn line means walking its escapes and then every grapheme of its text. Most
// of a frame is plain ASCII, where one byte is one cell, so the walk only asks the library
// about the runs that are not.

// measureStyledWidth returns how many cells a drawn line covers. A block of several rows goes
// to the library, which decides what a row of a block is and how the block is measured; this
// only reads one row, where a plain byte is one cell and the library is asked about the rest.
// Measuring a block here as the sum of its rows put a card at the edge of the screen once, so
// the case this cannot prove is the case it does not answer.
func measureStyledWidth(line string) int {
	if strings.IndexByte(line, '\n') >= 0 {
		return lipgloss.Width(line)
	}
	return measureRowWidth(line)
}

// measureRowWidth returns how many cells one drawn row covers.
func measureRowWidth(line string) int {
	cells := 0
	at := 0
	for at < len(line) {
		held := line[at]
		if held == 0x1b {
			at += measureEscape(line[at:])
			continue
		}
		if held >= 0x20 && held < 0x7f {
			plain := at
			for at < len(line) {
				next := line[at]
				if next == 0x1b || next < 0x20 || next >= 0x7f {
					break
				}
				at++
			}
			cells += at - plain
			continue
		}
		// A run of characters that are not plain bytes.
		odd := at
		for at < len(line) {
			next := line[at]
			if next == 0x1b || (next >= 0x20 && next < 0x7f) {
				break
			}
			at++
		}
		cells += measureOddRun(line[odd:at])
	}
	return cells
}

// measureOddRun returns the cells of a run that is not plain bytes. A run of characters that
// each stand on their own is measured from the width table, one lookup each. A run that holds
// a character which joins the one before it has to be read as graphemes, and only that run
// goes to the library.
func measureOddRun(run string) int {
	cells := 0
	for _, character := range run {
		if joinsNeighbour(character) {
			return lipgloss.Width(run)
		}
		cells += runewidth.RuneWidth(character)
	}
	return cells
}

// joinsNeighbour is true for a character that reads as part of the one beside it: a mark, a
// joiner, a variation selector, a skin tone, or one half of a flag.
func joinsNeighbour(character rune) bool {
	// Nothing below the first combining mark joins anything, which is most of the text a
	// frame holds that is not a plain byte.
	if character < 0x0300 {
		return false
	}
	switch {
	case character == 0x200d, character == 0xfe0e, character == 0xfe0f:
		return true
	case character >= 0x1f1e6 && character <= 0x1f1ff:
		return true
	case character >= 0x1f3fb && character <= 0x1f3ff:
		return true
	}
	return unicode.In(character, unicode.Mn, unicode.Me, unicode.Mc)
}

// measureEscape returns how many bytes the escape at the front of the text takes.
func measureEscape(text string) int {
	if len(text) < 2 {
		return len(text)
	}
	switch text[1] {
	case '[':
		for at := 2; at < len(text); at++ {
			if text[at] >= 0x40 && text[at] <= 0x7e {
				return at + 1
			}
		}
		return len(text)
	case ']':
		// An operating system command ends with a bell or with a string terminator.
		for at := 2; at < len(text); at++ {
			if text[at] == 0x07 {
				return at + 1
			}
			if text[at] == 0x1b && at+1 < len(text) && text[at+1] == '\\' {
				return at + 2
			}
		}
		return len(text)
	}
	return 2
}
