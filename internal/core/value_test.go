package core

import (
	"strings"
	"testing"
	"time"
)

// Every value from a driver must have a cell text, whatever its type. An empty cell would
// look like a null, which is a different value.
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

// Bytes are not text. A byte column is displayed as the hex form the server prints,
// because a text form would show characters the column does not hold.
func TestFormatCellWritesBytesAsHex(t *testing.T) {
	written := FormatCell([]byte("ada"), "bytea")
	if written == "ada" {
		t.Error("bytes were drawn as letters")
	}
	if !strings.HasPrefix(written, `\x`) {
		t.Errorf("bytes wrote as %q, wanted the hex the server prints", written)
	}
}

// A null is displayed as the word NULL, so the grid separates an empty value from a null.
func TestFormatCellWritesANullAsTheWord(t *testing.T) {
	written := FormatCell(nil, "text")
	if written != NullText {
		t.Errorf("a null writes as %q, wanted %q", written, NullText)
	}
	// An empty text is not a null, and must not look like one.
	if FormatCell("", "text") == NullText {
		t.Error("an empty text writes as a null")
	}
}

// A time is displayed with its time zone, because a comparison of two rows needs the exact
// point in time of each one.
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

// A container is displayed as JSON, so a map or a list is readable in one cell and not
// printed as a Go value.
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
		// Bytes are undecoded data, not a container.
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

// A value in a grid cell has no group of blank characters, because the cell is one row and
// a tab or a line break would break the layout.
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
		// Leading space is removed, because a cell starts at its value. Trailing space
		// becomes one space, which is not visible.
		{"space in front is dropped", "  a b", "a b"},
		{"space at the end becomes one", "a b  ", "a b "},
		{"nothing", "", ""},
		{"only space", "   ", ""},
		{"one space alone", " ", ""},
		// A document is a long line with nothing to collapse, and is returned
		// unchanged.
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

// The run time of a statement is formatted so that two runs are easy to compare, so the
// unit depends on the size of the value.
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

	// Two runs of a different size must not have the same text.
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

// A row index is clamped and wrapped when the cursor moves through a list. Neither
// function can return an index outside the list, or a slice operation fails.
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

// Clamping stops at the ends, and wrapping continues at the other end.
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

// A caret can stand after the last character, so the highest position is allowed, as is
// zero. The highest position is never below zero: the callers pass a length, and ClampIndex
// handles an empty list itself.
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
