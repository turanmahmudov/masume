package present_test

import (
	"strings"
	"testing"

	"github.com/turanmahmudov/masume/internal/present"
)

// A server returns the content of a row unchanged, and the terminal acts on a part of it. A
// byte that moves the cursor or starts a colour would break the frame, so every value is
// converted to safe text before it is drawn.
func TestSafeTextTakesOutWhatATerminalWouldActOn(t *testing.T) {
	for _, held := range []struct {
		name  string
		text  string
		holds string
	}{
		{"an escape", "red\x1b[31mtext", "\x1b"},
		{"a control sequence introducer", "\x9bcsi", "\x9b"},
		{"a bell", "bell\ahere", "\a"},
		{"a carriage return", "back\rhere", "\r"},
		{"a null byte", "null\x00byte", "\x00"},
		{"a delete", "del\x7fhere", "\x7f"},
	} {
		t.Run(held.name, func(t *testing.T) {
			written := present.SafeText(held.text)
			if strings.Contains(written, held.holds) {
				t.Errorf("%q became %q, which still holds the character", held.text, written)
			}
		})
	}
}

// Invalid bytes must still draw a character and not leave a gap in the row.
func TestSafeTextAnswersTextForBytesThatAreNotText(t *testing.T) {
	written := present.SafeText(string([]byte{0xff, 0xfe, 0x41}))
	if written == "" {
		t.Error("bytes that are not text became nothing")
	}
	if present.MeasureText(written) <= 0 {
		t.Errorf("%q measures %d cells", written, present.MeasureText(written))
	}
}

// The width of a row is the number of terminal cells, so a wide character counts as two and
// a combining mark counts as zero.
func TestMeasureTextCountsTheCellsATerminalDraws(t *testing.T) {
	for _, held := range []struct {
		name  string
		text  string
		cells int
	}{
		{"nothing", "", 0},
		{"plain letters", "ada", 3},
		{"a wide character", "漢", 2},
		{"wide and narrow together", "漢a", 3},
		{"a combining mark", "é", 1},
	} {
		t.Run(held.name, func(t *testing.T) {
			if answered := present.MeasureText(held.text); answered != held.cells {
				t.Errorf("%q measures %d cells, wanted %d", held.text, answered, held.cells)
			}
		})
	}
}

// A cut must never leave more cells than the column has, or the next row moves.
func TestTruncateTextNeverPassesTheWidth(t *testing.T) {
	for _, text := range []string{
		"", "a", "ada", "a much longer value than the column",
		"漢字漢字漢字", "ééé", strings.Repeat("wide 漢 ", 8),
	} {
		for _, width := range []int{0, 1, 2, 3, 5, 10, 40} {
			written := present.TruncateText(text, width)
			if measured := present.MeasureText(written); measured > width {
				t.Errorf("%q cut to %d measures %d: %q", text, width, measured, written)
			}
		}
	}
}

// A cut cannot leave half of a wide character, so the cut is before it and the row is one
// cell short and not one cell too long.
func TestTruncateTextDoesNotSplitAWideCharacter(t *testing.T) {
	// Three wide characters are six cells. Five cells hold two of them and the ellipsis.
	written := present.TruncateText("漢字漢", 5)
	if measured := present.MeasureText(written); measured > 5 {
		t.Errorf("%q measures %d cells, wanted 5 at most", written, measured)
	}
}

// A pad fills the column, so every row of a grid has the same width.
func TestPadTextFillsToTheWidth(t *testing.T) {
	for _, held := range []struct {
		text  string
		width int
	}{
		{"", 5}, {"ada", 5}, {"ada", 3}, {"漢", 5}, {"é", 4},
	} {
		written := present.PadText(held.text, held.width)
		if measured := present.MeasureText(written); measured != held.width {
			t.Errorf("%q padded to %d measures %d", held.text, held.width, measured)
		}
	}
}

// A value longer than the column is cut and a shorter value is padded, so the result is
// always exactly the width.
func TestFitTextIsAlwaysTheWidthAsked(t *testing.T) {
	for _, text := range []string{"", "a", "ada", "a much longer value", "漢字漢字"} {
		for _, width := range []int{1, 3, 8, 20} {
			written := present.FitText(text, width)
			if measured := present.MeasureText(written); measured != width {
				t.Errorf("%q fitted to %d measures %d: %q", text, width, measured, written)
			}
		}
	}
}

// A line break inside a value would move the rest of the frame down one row, so a value in a
// grid cell contains no line break.
func TestSafeTextKeepsAValueOnOneRow(t *testing.T) {
	for _, text := range []string{"line\nbreak", "line\r\nbreak", "a\n\nb"} {
		written := present.SafeText(text)
		if strings.ContainsAny(written, "\n\r") {
			t.Errorf("%q became %q, which still breaks the row", text, written)
		}
	}
}

// A block of text keeps its line breaks, because the cell viewer draws it on several rows.
// Every other control character becomes a space.
func TestSafeLinesKeepsTheBreaksAndNothingElse(t *testing.T) {
	written := present.SafeLines("first\rline\nsecond\aline")
	if strings.Count(written, "\n") != 1 {
		t.Errorf("%q does not hold the one break it was given", written)
	}
	if strings.ContainsAny(written, "\r\a") {
		t.Errorf("%q still holds a control character", written)
	}
}

func TestWrapWordsKeepsEveryLineInsideTheWidth(t *testing.T) {
	const text = "the server refused the statement because the relation does not exist"
	for _, width := range []int{10, 20, 40} {
		for _, line := range present.WrapWords(text, width) {
			if measured := present.MeasureText(line); measured > width {
				t.Errorf("a line of %d cells passed the width of %d: %q", measured, width, line)
			}
		}
	}
}

// A word longer than the width must be broken, or it would exceed the pane.
func TestWrapWordsBreaksAWordTooLongForTheWidth(t *testing.T) {
	lines := present.WrapWords("supercalifragilistic", 6)
	if len(lines) < 2 {
		t.Fatalf("a long word wrapped into %d lines", len(lines))
	}
	for _, line := range lines {
		if measured := present.MeasureText(line); measured > 6 {
			t.Errorf("a line measures %d cells: %q", measured, line)
		}
	}
}

func TestCountWrappedRowsAgreesWithTheWrap(t *testing.T) {
	// The card size comes from the count and the card is drawn with the wrap. A count
	// that is one row too low moves the last row of the text off the card.
	for _, text := range []string{
		"", "   ", "short", "the server refused the statement because the relation does not exist",
		// These words fill the row to the last cell and more words follow. The wrap
		// starts a new row, because the space after the word also needs a cell.
		"one two three four five six", "aa bb cc dd", "a b c d e f g h i j k l",
		"  an indented line that has to wrap somewhere",
		"averylongwordthatcannotbebroken on a space",
		"wide 日本語 text mixed with narrow",
		"one\ntwo",
	} {
		for _, width := range []int{0, -3, 1, 5, 8, 10, 12, 30} {
			counted := present.CountWrappedRows(text, width)
			wrapped := len(present.WrapWords(text, width))
			if counted != wrapped {
				t.Errorf("%q at %d counts %d rows and wraps into %d",
					text, width, counted, wrapped)
			}
		}
	}
}

func TestPrettifyAndCompactJsonRoundTrip(t *testing.T) {
	const compact = `{"customer":"ada","total":12.5}`

	pretty, is := present.PrettifyJSON(compact)
	if !is {
		t.Fatal("valid JSON was not prettified")
	}
	if !strings.Contains(pretty, "\n") {
		t.Errorf("the pretty form holds no line break:\n%s", pretty)
	}

	again, is := present.CompactJSON(pretty)
	if !is {
		t.Fatal("the pretty form was not compacted")
	}
	if again != compact {
		t.Errorf("the round trip gave %q, wanted %q", again, compact)
	}
}

func TestPrettifyJsonRefusesWhatIsNotJson(t *testing.T) {
	for _, text := range []string{"", "ada", "{not json", "{}{}"} {
		if _, is := present.PrettifyJSON(text); is {
			t.Errorf("%q was read as JSON", text)
		}
	}
}

func TestFormatCountGroupsTheFigures(t *testing.T) {
	for _, held := range []struct {
		count int64
		want  string
	}{
		{0, "0"}, {1, "1"}, {999, "999"},
		{1000, "1,000"}, {1234567, "1,234,567"}, {-1234, "-1,234"},
	} {
		if answered := present.FormatCount(held.count); answered != held.want {
			t.Errorf("%d reads as %q, wanted %q", held.count, answered, held.want)
		}
	}
}

// The status bar shows the number of rows read and whether more rows exist, so a truncated
// read is never reported as the whole table.
func TestFormatResultSizeSaysWhenThereAreMoreRows(t *testing.T) {
	whole := present.FormatResultSize(3, false, 3, true)
	if strings.Contains(whole, "+") {
		t.Errorf("a whole result reads %q, which suggests there are more", whole)
	}
	truncated := present.FormatResultSize(100, true, 0, false)
	if truncated == whole {
		t.Error("a truncated read reads the same as a whole one")
	}
	if truncated == "" {
		t.Error("a truncated read is described as an empty text")
	}
}

func TestIsJsonTypeReadsTheTypesAServerNames(t *testing.T) {
	for _, dataType := range []string{"json", "jsonb"} {
		if !present.IsJSONType(dataType) {
			t.Errorf("%q does not read as a JSON type", dataType)
		}
	}
	// The name comes from the catalog of the server in lower case, so the comparison is
	// exact.
	for _, dataType := range []string{"", "text", "integer", "jsonish", "JSON"} {
		if present.IsJSONType(dataType) {
			t.Errorf("%q reads as a JSON type", dataType)
		}
	}
}

// A column that holds a document is indented in the full-height viewer, for every server.
// MongoDB uses the type names `object` and `array`, and a SQL server uses `json`.
func TestIsJSONTypeNamesEveryColumnThatHoldsADocument(t *testing.T) {
	for _, held := range []struct {
		dataType string
		want     bool
	}{
		{"json", true}, {"jsonb", true},
		{"object", true}, {"array", true},
		{"text", false}, {"varchar", false}, {"objectId", false},
		// A column the sample found with several types is not a document column.
		{"mixed", false},
	} {
		if answered := present.IsJSONType(held.dataType); answered != held.want {
			t.Errorf("%q reads as a document column=%v, wanted %v",
				held.dataType, answered, held.want)
		}
	}
}

// The viewer indents the document of a MongoDB cell. The grid draws it on one line.
func TestFormatForViewerIndentsAMongodbDocument(t *testing.T) {
	written := present.FormatForViewer(`{"sku":"MN-003","qty":2}`, "object")
	if !strings.Contains(written, "\n") {
		t.Errorf("the document was not indented:\n%s", written)
	}
	if !strings.Contains(written, `"sku": "MN-003"`) {
		t.Errorf("the document reads:\n%s", written)
	}
}

func TestWrapWordsAlwaysEndsOnACharacterWiderThanTheRow(t *testing.T) {
	// A character that does not fit would cut nothing, and the wrap would repeat with the
	// same text. It takes its own row, so the wrap always advances.
	for _, held := range []struct {
		name  string
		text  string
		width int
	}{
		{"a wide character in a row of one cell", "日本語", 1},
		{"a wide character among narrow ones", "wide 日本語 text", 1},
		{"a row of one cell and plain text", "one two", 1},
		{"a wide character in a row of two cells", "日本語", 2},
	} {
		t.Run(held.name, func(t *testing.T) {
			rows := present.WrapWords(held.text, held.width)
			if len(rows) == 0 {
				t.Fatal("the wrap answered no rows")
			}
			if len(rows) > len([]rune(held.text))+1 {
				t.Errorf("the wrap answered %d rows for %d characters",
					len(rows), len([]rune(held.text)))
			}
			for _, row := range rows {
				if row == "" && len(rows) > 1 {
					t.Errorf("the wrap answered an empty row: %q", rows)
				}
			}
		})
	}
}
