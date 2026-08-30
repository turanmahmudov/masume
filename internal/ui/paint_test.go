package ui

import (
	"image/color"
	"strings"
	"testing"

	"charm.land/lipgloss/v2"
)

// The cheap painter must write exactly what a style writes, or a frame would change.
func TestPaintTextWritesWhatAStyleWrites(t *testing.T) {
	colours := []color.Color{
		nil,
		lipgloss.Color("#0d1017"),
		lipgloss.Color("#bfbdb6"),
		lipgloss.Color("#000000"),
		lipgloss.ANSIColor(4),
		color.RGBA{R: 230, G: 180, B: 80, A: 255},
	}
	texts := []string{" ", "a", "select", "  ", "ArtistId", "→ order_id", "…", "⠋ running…"}

	for _, ink := range colours {
		for _, ground := range colours {
			style := lipgloss.NewStyle()
			if ink != nil {
				style = style.Foreground(ink)
			}
			if ground != nil {
				style = style.Background(ground)
			}
			for _, text := range texts {
				wanted := style.Render(text)
				held := paintText(ink, ground, text)
				if held != wanted {
					t.Errorf("painting %q in %v on %v gave %q, want %q",
						text, ink, ground, held, wanted)
				}
			}
		}
	}
}

func TestPaintTextKeepsAnEmptyTextEmpty(t *testing.T) {
	if held := paintText(lipgloss.Color("#ffffff"), lipgloss.Color("#000000"), ""); held != "" {
		t.Errorf("an empty text painted as %q", held)
	}
}

func TestPaintBlanksWritesTheRunOnTheGround(t *testing.T) {
	ground := lipgloss.Color("#0d1017")
	wanted := lipgloss.NewStyle().Background(ground).Render(strings.Repeat(" ", 4))
	if held := paintBlanks(ground, 4); held != wanted {
		t.Errorf("four blanks painted as %q, want %q", held, wanted)
	}
	if held := paintBlanks(ground, 0); held != "" {
		t.Errorf("no blanks painted as %q", held)
	}
}

func TestPackColorTellsBlackFromNothing(t *testing.T) {
	if packColor(nil) == packColor(lipgloss.Color("#000000")) {
		t.Error("black and no colour pack the same")
	}
	if packColor(lipgloss.Color("#010203")) == packColor(lipgloss.Color("#010204")) {
		t.Error("two colours pack the same")
	}
}

// The fast measure must answer what the library answers, for every line of a real frame and
// for the awkward runs a frame can hold.
func TestMeasureStyledWidthAnswersWhatTheLibraryAnswers(t *testing.T) {
	ground := lipgloss.Color("#0d1017")
	ink := lipgloss.Color("#bfbdb6")
	cases := []string{
		"",
		"plain text",
		" ",
		"╭─ explorer ───────────────────────╮",
		"│ ▾ ◇ sample                     4 │",
		"⠋ running… 2s",
		"→ order_id",
		"…",
		"★ 4",
		"á",        // a base letter and the mark that joins it
		"日本語",       // cells two wide each
		"🙂",         // one grapheme, two cells
		"👨‍👩‍👧",     // one grapheme joined by zero width joiners
		"ȩ́x",      // two marks over one letter
		"tab\there", // a control character
		paintText(ink, ground, "cell"),
		paintText(ink, ground, "│ ▾ ◇ sample"),
		paintText(ink, ground, "日本語") + paintText(nil, ground, "  "),
		paintBlanks(ground, 7),
		paintText(ink, ground, "⠋") + paintText(nil, ground, " running… 2s"),
	}

	for _, held := range cases {
		wanted := lipgloss.Width(held)
		if measured := measureStyledWidth(held); measured != wanted {
			t.Errorf("%q measured %d, want %d", held, measured, wanted)
		}
	}
}

func TestMeasureStyledWidthReadsEveryRowOfAFrame(t *testing.T) {
	model := buildFrameModel(t)
	for at, line := range strings.Split(model.View().Content, "\n") {
		wanted := lipgloss.Width(line)
		if measured := measureStyledWidth(line); measured != wanted {
			t.Fatalf("row %d measured %d, want %d: %q", at, measured, wanted, line)
		}
	}
}

// A block of rows is as wide as its widest row. Measuring it as the sum of its rows put a card
// far off its place, so the blocks are measured here too.
func TestMeasureStyledWidthTakesTheWidestRowOfABlock(t *testing.T) {
	blocks := []string{
		"aaa\nbbbbbbbbbb\ncc",
		"aaa\n",
		"\n\n",
		"aaa\r\nbb",
		"one row only",
		"",
		strings.Join([]string{
			paintText(lipgloss.Color("#bfbdb6"), lipgloss.Color("#0d1017"), "╭─ ai chat ─╮"),
			paintText(nil, lipgloss.Color("#0d1017"), "│ a longer row inside it │"),
			paintText(lipgloss.Color("#bfbdb6"), lipgloss.Color("#0d1017"), "╰───────────╯"),
		}, "\n"),
	}
	for _, held := range blocks {
		wanted := lipgloss.Width(held)
		if measured := measureStyledWidth(held); measured != wanted {
			t.Errorf("%q measured %d, want %d", held, measured, wanted)
		}
	}
}

// A style renders an empty text as nothing at all where it carries a rule, so the escapes
// are read from a text of one character. A highlight the theme underlines, such as the mark
// of a fault, is drawn in no colour at all without this.
func TestOpeningOfAStyleWithARuleCarriesItsColours(t *testing.T) {
	ink := lipgloss.Color("#db4b4b")
	ground := lipgloss.Color("#1a1b26")
	style := lipgloss.NewStyle().Foreground(ink).Background(ground).Underline(true)

	key := paintKey{ink: packColor(ink), ground: packColor(ground), underline: true}
	opening := keepOpening(key, style)
	if opening == "" {
		t.Fatal("an underlined style opens with no escapes at all")
	}
	// A style writes one character with its whole opening, which is what is read back.
	if wanted := style.Render("x"); opening+"x"+resetSequence != wanted {
		t.Errorf("the opening writes %q, wanted %q", opening+"x"+resetSequence, wanted)
	}
}
