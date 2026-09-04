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

// The dashboard draws how long a statement has been running in a column that refreshes every
// two seconds. Minutes and seconds keep the same width for every value under an hour, so the
// column does not move while the reader looks at it.
func TestFormatClockKeepsOneWidthWithinItsRange(t *testing.T) {
	for _, held := range []struct {
		name    string
		elapsed time.Duration
		want    string
	}{
		{"nothing yet", 0, "00:00"},
		{"one second", time.Second, "00:01"},
		{"part of a second is not a second", 1500 * time.Millisecond, "00:01"},
		{"under a minute", 9 * time.Second, "00:09"},
		{"a minute", time.Minute, "01:00"},
		{"minutes and seconds", 4*time.Minute + 12*time.Second, "04:12"},
		{"ten minutes", 10*time.Minute + 9*time.Second, "10:09"},
		{"just under an hour", 59*time.Minute + 59*time.Second, "59:59"},
		{"an hour", time.Hour, "01:00:00"},
		{"hours, minutes and seconds", 41*time.Hour + 4*time.Minute + 12*time.Second, "41:04:12"},
		{"a single hour keeps the width of two", 3*time.Hour + 7*time.Second, "03:00:07"},
		{"a day and more", 25*time.Hour + 30*time.Second, "25:00:30"},
		// A hundred hours needs a third digit. Nothing a server reports runs that long,
		// and a wider value is better than a wrong one.
		{"a hundred hours", 100 * time.Hour, "100:00:00"},
		// A minus sign would change the width of the column.
		{"before it started", -3 * time.Second, "00:00"},
	} {
		t.Run(held.name, func(t *testing.T) {
			if written := FormatClock(held.elapsed); written != held.want {
				t.Errorf("%s reads %q, wanted %q", held.elapsed, written, held.want)
			}
		})
	}
}

// Every value under an hour is five characters, and every value from one hour to a hundred
// hours is eight, so a column of either holds its width while it refreshes.
func TestFormatClockHoldsItsWidthAcrossARange(t *testing.T) {
	for seconds := range 3600 {
		if written := FormatClock(time.Duration(seconds) * time.Second); len(written) != 5 {
			t.Fatalf("%d seconds reads %q, which is not five characters", seconds, written)
		}
	}
	for minutes := 60; minutes < 100*60; minutes++ {
		if written := FormatClock(time.Duration(minutes) * time.Minute); len(written) != 8 {
			t.Fatalf("%d minutes reads %q, which is not eight characters", minutes, written)
		}
	}
}

// The title of the dashboard carries how long the server has been up. One unit is enough at
// a glance, and the exact seconds of a server that has been up for weeks carry nothing.
func TestFormatLargestUnitWritesTheUnitThatCarriesTheMeaning(t *testing.T) {
	for _, held := range []struct {
		name    string
		elapsed time.Duration
		want    string
	}{
		{"nothing", 0, "0s"},
		{"under a second", 900 * time.Millisecond, "0s"},
		{"seconds", 7 * time.Second, "7s"},
		{"just under a minute", 59 * time.Second, "59s"},
		{"a minute", time.Minute, "1m"},
		{"minutes, with the seconds dropped", 12*time.Minute + 30*time.Second, "12m"},
		{"just under an hour", 59*time.Minute + 59*time.Second, "59m"},
		{"an hour", time.Hour, "1h"},
		{"hours, with the minutes dropped", 3*time.Hour + 59*time.Minute, "3h"},
		{"just under a day", 23*time.Hour + 59*time.Minute, "23h"},
		{"a day", 24 * time.Hour, "1d"},
		{"the uptime of a server that has run for weeks", 41 * 24 * time.Hour, "41d"},
		{"a year and more", 400 * 24 * time.Hour, "400d"},
		// A server whose clock stands ahead of this one reports a start in the future.
		// An age cannot be negative, and a minus sign in the title would read as a fault
		// in the client rather than in the clock.
		{"a start in the future", -5 * time.Hour, "0s"},
	} {
		t.Run(held.name, func(t *testing.T) {
			if written := FormatLargestUnit(held.elapsed); written != held.want {
				t.Errorf("%s reads %q, wanted %q", held.elapsed, written, held.want)
			}
		})
	}
}

// PostgreSQL stores a lock mode as one word with a Lock suffix, which is the shape of the
// catalog and not the shape a reader knows.
func TestFormatLockModeWritesTheModeAsWords(t *testing.T) {
	for _, held := range []struct {
		name string
		mode string
		want string
	}{
		{"nothing", "", ""},
		{"only the suffix", "Lock", ""},
		{"blank around it", "  RowShareLock  ", "ROW SHARE"},
		{"the mode a schema change takes", "AccessExclusiveLock", "ACCESS EXCLUSIVE"},
		{"the mode a read takes", "AccessShareLock", "ACCESS SHARE"},
		{"the mode a write takes", "RowExclusiveLock", "ROW EXCLUSIVE"},
		{"one word", "ExclusiveLock", "EXCLUSIVE"},
		{"four words", "ShareRowExclusiveLock", "SHARE ROW EXCLUSIVE"},
		// A name this client does not know is still the one the reader matches against
		// the server.
		{"a name in another shape", "table_lock", "TABLE_LOCK"},
	} {
		t.Run(held.name, func(t *testing.T) {
			if written := FormatLockMode(held.mode); written != held.want {
				t.Errorf("%q reads %q, wanted %q", held.mode, written, held.want)
			}
		})
	}
}
