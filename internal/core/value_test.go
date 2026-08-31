package core

import (
	"strings"
	"testing"
	"time"
)

// Every value a driver hands over draws in one cell, whatever shape it arrived in. A cell that
// drew as nothing would read as a null, which is a different thing.
func TestFormatCellWritesEveryShapeADriverGives(t *testing.T) {
	for _, held := range []struct {
		name     string
		value    any
		dataType string
		want     string
	}{
		{"text", "ada", "text", "ada"},
		{"a whole number", int64(42), "integer", "42"},
		{"a number with a fraction", 12.5, "numeric", "12.5"},
		{"true", true, "boolean", "true"},
		{"false", false, "boolean", "false"},
		{"an empty text", "", "text", ""},
	} {
		t.Run(held.name, func(t *testing.T) {
			if answered := FormatCell(held.value, held.dataType); answered != held.want {
				t.Errorf("%v writes as %q, wanted %q", held.value, answered, held.want)
			}
		})
	}
}

// Bytes are not text. A column of bytes is drawn as the hex the server prints, because reading
// them as letters would show something the column does not hold.
func TestFormatCellWritesBytesAsHex(t *testing.T) {
	written := FormatCell([]byte("ada"), "bytea")
	if written == "ada" {
		t.Error("bytes were drawn as letters")
	}
	if !strings.HasPrefix(written, `\x`) {
		t.Errorf("bytes wrote as %q, wanted the hex the server prints", written)
	}
}

// A null is drawn as the word, so an empty cell and a null are told apart in the grid.
func TestFormatCellWritesANullAsTheWord(t *testing.T) {
	written := FormatCell(nil, "text")
	if written != NullText {
		t.Errorf("a null writes as %q, wanted %q", written, NullText)
	}
	// An empty text is not a null, and must not read as one.
	if FormatCell("", "text") == NullText {
		t.Error("an empty text writes as a null")
	}
}

// A time is drawn with its zone, because a reader comparing two rows needs to know which
// moment each one is.
func TestFormatCellKeepsTheZoneOfATime(t *testing.T) {
	at := time.Date(2026, 8, 25, 12, 30, 0, 0, time.FixedZone("CEST", 2*60*60))
	written := FormatCell(at, "timestamptz")
	if written == "" {
		t.Fatal("a time writes as nothing")
	}
	if !strings.Contains(written, "2026") {
		t.Errorf("a time writes as %q and does not hold the year", written)
	}
}

// A container is drawn as JSON, so a hash or a list reads in one cell rather than as a Go
// value nobody can read.
func TestIsStructuredValueNamesWhatIsDrawnAsJson(t *testing.T) {
	for _, held := range []struct {
		name  string
		value any
		want  bool
	}{
		{"a map", map[string]any{"a": 1}, true},
		{"a list", []any{1, 2}, true},

		{"text", "ada", false},
		{"a number", int64(1), false},
		{"nothing", nil, false},
		// Bytes are text the driver did not decode, not a container.
		{"bytes", []byte("ada"), false},
	} {
		t.Run(held.name, func(t *testing.T) {
			if answered := IsStructuredValue(held.value); answered != held.want {
				t.Errorf("%v reads as structured = %v, wanted %v",
					held.value, answered, held.want)
			}
		})
	}
}

// A value drawn in one cell of the grid holds no run of space, because the cell is one row
// and a tab or a break would push the frame about.
func TestCollapseWhitespaceMakesOneCellOfAnyText(t *testing.T) {
	for _, held := range []struct {
		name string
		text string
		want string
	}{
		{"one space stays", "a b", "a b"},
		{"a run becomes one", "a    b", "a b"},
		{"a break becomes a space", "a\nb", "a b"},
		{"a tab becomes a space", "a\tb", "a b"},
		{"a mix becomes one space", "a \n\t b", "a b"},
		// Space in front is dropped, because a cell starts where its value does. Space at
		// the end becomes one, and a trailing space draws as nothing anyway.
		{"space in front is dropped", "  a b", "a b"},
		{"space at the end becomes one", "a b  ", "a b "},
		{"nothing", "", ""},
		{"only space", "   ", ""},
		{"one space alone", " ", ""},
		// A document of a collection is a long line with nothing to collapse, and comes
		// back as it stands.
		{"a document that needs no change", `{"sku":"a-1","note":"one two three"}`,
			`{"sku":"a-1","note":"one two three"}`},
		{"a wide character beside a space", "漢 字", "漢 字"},
		{"a wide character beside a run", "漢  字", "漢 字"},
	} {
		t.Run(held.name, func(t *testing.T) {
			if answered := CollapseWhitespace(held.text); answered != held.want {
				t.Errorf("%q collapses to %q, wanted %q", held.text, answered, held.want)
			}
		})
	}
}

// The time a statement took is written so a reader can compare two runs at a glance, which
// means the unit has to follow the size of it.
func TestFormatDurationFollowsTheSizeOfIt(t *testing.T) {
	written := map[string]string{}
	for _, held := range []struct {
		name    string
		elapsed time.Duration
	}{
		{"under a millisecond", 500 * time.Microsecond},
		{"a few milliseconds", 12 * time.Millisecond},
		{"about a second", 1500 * time.Millisecond},
		{"a minute", 90 * time.Second},
		{"nothing at all", 0},
	} {
		answered := FormatDuration(held.elapsed)
		if answered == "" {
			t.Errorf("%s writes as nothing", held.name)
		}
		written[held.name] = answered
	}

	// Two runs of a different size do not read the same, or a reader cannot tell them apart.
	if written["a few milliseconds"] == written["a minute"] {
		t.Errorf("twelve milliseconds and ninety seconds both read as %q",
			written["a minute"])
	}
}

func TestFormatClockTimeWritesTheTimeOfDay(t *testing.T) {
	at := time.Date(2026, 8, 25, 14, 5, 9, 0, time.UTC)
	written := FormatClockTime(at)
	if written == "" {
		t.Fatal("a moment writes as nothing")
	}
	if !strings.Contains(written, "14") && !strings.Contains(written, "2") {
		t.Errorf("the time writes as %q and holds no hour", written)
	}
}

// The index of a row is clamped and wrapped where the cursor walks a list, and neither may
// answer a place outside it or the list is sliced past its end.
func TestClampAndWrapIndexStayInsideTheList(t *testing.T) {
	for _, count := range []int{0, 1, 3, 10} {
		for _, index := range []int{-5, -1, 0, 1, 9, 100} {
			clamped := ClampIndex(index, count)
			if count == 0 {
				if clamped != 0 {
					t.Errorf("an empty list clamped %d to %d", index, clamped)
				}
			} else if clamped < 0 || clamped >= count {
				t.Errorf("a list of %d clamped %d to %d", count, index, clamped)
			}

			wrapped := WrapIndex(index, count)
			if count == 0 {
				if wrapped != 0 {
					t.Errorf("an empty list wrapped %d to %d", index, wrapped)
				}
			} else if wrapped < 0 || wrapped >= count {
				t.Errorf("a list of %d wrapped %d to %d", count, index, wrapped)
			}
		}
	}
}

// Clamping holds at the ends, and wrapping goes round, which is the difference between a
// cursor that stops and one that carries on.
func TestClampHoldsAtTheEndsAndWrapGoesRound(t *testing.T) {
	if held := ClampIndex(10, 3); held != 2 {
		t.Errorf("clamping past the end gave %d, wanted the last place", held)
	}
	if held := ClampIndex(-5, 3); held != 0 {
		t.Errorf("clamping before the start gave %d, wanted the first place", held)
	}
	if held := WrapIndex(3, 3); held != 0 {
		t.Errorf("wrapping past the end gave %d, wanted the first place", held)
	}
	if held := WrapIndex(-1, 3); held != 2 {
		t.Errorf("wrapping before the start gave %d, wanted the last place", held)
	}
}

// A caret may stand after the last character, so the top is allowed as well as zero. The top
// is never below zero: the callers pass a length, and ClampIndex guards an empty list itself.
func TestClampWithinKeepsAPositionFromZeroToTheTop(t *testing.T) {
	for _, held := range []struct{ position, highest int }{
		{-5, 10}, {0, 10}, {5, 10}, {10, 10}, {20, 10}, {5, 0}, {0, 0},
	} {
		answered := ClampWithin(held.position, held.highest)
		if answered < 0 {
			t.Errorf("%d within %d gave %d", held.position, held.highest, answered)
		}
		if answered > held.highest {
			t.Errorf("%d within %d gave %d, past the top",
				held.position, held.highest, answered)
		}
	}
}
