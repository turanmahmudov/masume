// Package present holds where things sit on a screen of a given width, and how a
// value, a count and a time are written. Nothing here draws.
package present

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/mattn/go-runewidth"

	"github.com/turanmahmudov/masume/internal/core"
)

// NullDisplay marks a null in the grid, because the text "NULL" can be a stored value.
const NullDisplay = "∅"

// typeAbbreviations shorten only the names too long for a column of a list. A name the
// server writes in one word is kept, because `integer` is what SQLite and MySQL call it,
// and PostgreSQL already returns with `int4`.
var typeAbbreviations = map[string]string{
	"character varying":           "varchar",
	"character":                   "char",
	"double precision":            "float8",
	"timestamp with time zone":    "timestamptz",
	"timestamp without time zone": "timestamp",
	"time with time zone":         "timetz",
	"time without time zone":      "time",
}

var typeModifier = regexp.MustCompile(`\(.*\)`)

// AbbreviateDataType returns the short name of a type, without the length modifier.
func AbbreviateDataType(dataType string) string {
	base := strings.ToLower(strings.TrimSpace(typeModifier.ReplaceAllString(dataType, "")))
	array := strings.HasSuffix(base, "[]")
	singular := base
	if array {
		singular = strings.TrimSpace(strings.TrimSuffix(base, "[]"))
	}
	written, known := typeAbbreviations[singular]
	if !known {
		written = singular
	}
	if array {
		return written + "[]"
	}
	return written
}

// MeasureText returns how many terminal cells the text takes. A CJK glyph takes two
// cells and a combining mark takes none, so the count of runes does not fit.
func MeasureText(text string) int {
	return runewidth.StringWidth(SafeText(text))
}

// MeasureTextUpTo returns how many cells the text takes and stops counting at the limit.
// The answer is exact below the limit, and is the limit or a little over it where the text
// passes it. A cell of a column whose width is capped is measured this way, because a
// value already wider than the cap is as wide as the cap allows whatever the rest holds.
func MeasureTextUpTo(text string, limit int) int {
	if limit <= 0 {
		return 0
	}
	used := 0
	for _, character := range text {
		// A byte that is no text, or the replacement character itself. The exact measure
		// reads a run of those as one character, so the whole text goes through it.
		if character == utf8.RuneError {
			return MeasureText(text)
		}
		if isControlCharacter(character) {
			used++
		} else {
			used += runewidth.RuneWidth(character)
		}
		if used >= limit {
			return used
		}
	}
	return used
}

// SafeText returns the text a terminal can draw. A value the server sent can hold a byte
// that is no text, or a control character: an escape would colour the rest of the frame, a
// carriage return would move the cursor back over the border of a pane, and neither takes the
// cell the measure gives it. Each one becomes a space.
func SafeText(text string) string {
	if !needsSafeText(text) {
		return text
	}
	built := &strings.Builder{}
	built.Grow(len(text))
	for _, character := range strings.ToValidUTF8(text, "\uFFFD") {
		if isControlCharacter(character) {
			built.WriteByte(' ')
			continue
		}
		built.WriteRune(character)
	}
	return built.String()
}

// needsSafeText is true where the text holds anything a terminal would act on, so the text of
// an ordinary value is answered without being built again.
func needsSafeText(text string) bool {
	high := false
	for at := 0; at < len(text); at++ {
		switch {
		case text[at] < 0x20 || text[at] == 0x7f:
			return true
		// The C1 controls, which UTF-8 writes as 0xc2 and the byte itself.
		case text[at] == 0xc2 && at+1 < len(text) && text[at+1] >= 0x80 && text[at+1] <= 0x9f:
			return true
		case text[at] >= 0x80:
			high = true
		}
	}
	return high && !utf8.ValidString(text)
}

// isControlCharacter is true for a character a terminal acts on rather than draws.
func isControlCharacter(character rune) bool {
	return character < 0x20 || character == 0x7f || (character >= 0x80 && character <= 0x9f)
}

// TruncateText cuts the text to the width, with an ellipsis where it was cut.
func TruncateText(text string, width int) string {
	text = SafeText(text)
	if width <= 0 {
		return ""
	}
	if MeasureText(text) <= width {
		return text
	}
	if width == 1 {
		return "…"
	}

	var kept strings.Builder
	used := 0
	for _, character := range text {
		cost := runewidth.RuneWidth(character)
		if used+cost > width-1 {
			break
		}
		kept.WriteRune(character)
		used += cost
	}
	return kept.String() + strings.Repeat(" ", width-1-used) + "…"
}

// PadText fills the text out to the width with spaces.
func PadText(text string, width int) string {
	text = SafeText(text)
	used := MeasureText(text)
	if used >= width {
		return text
	}
	return text + strings.Repeat(" ", width-used)
}

// FitText writes the text in exactly this many cells: cut with an ellipsis, or padded
// with spaces.
func FitText(text string, width int) string {
	return PadText(TruncateText(text, width), width)
}

// FormatRow writes one row as cell text, which the grid draws, searches and measures. A
// masked column is hidden on screen only: a copy or an export keeps the value.
func FormatRow(row []any, dataTypes []string, masked map[int]bool) []string {
	written := make([]string, 0, len(row))
	for index, cell := range row {
		if masked[index] {
			written = append(written, MaskedDisplay)
			continue
		}
		if cell == nil {
			written = append(written, NullDisplay)
			continue
		}
		dataType := ""
		if index < len(dataTypes) {
			dataType = dataTypes[index]
		}
		written = append(written, SafeText(core.CollapseWhitespace(core.FormatCell(cell, dataType))))
	}
	return written
}

// FormatRows writes every row as cell text.
func FormatRows(rows [][]any, dataTypes []string, masked map[int]bool) [][]string {
	written := make([][]string, 0, len(rows))
	for _, row := range rows {
		written = append(written, FormatRow(row, dataTypes, masked))
	}
	return written
}

// FormatWhen writes a time for a column of a list. A time from another day also shows
// the date.
func FormatWhen(at, now time.Time) string {
	clock := at.Format("15:04")
	if at.Year() == now.Year() && at.YearDay() == now.YearDay() {
		return clock + at.Format(":05")
	}
	return at.Format("01-02 15:04")
}

// FormatCount writes a number with a comma every three digits.
func FormatCount(count int64) string {
	written := strconv.FormatInt(count, 10)
	negative := strings.HasPrefix(written, "-")
	if negative {
		written = written[1:]
	}
	var grouped strings.Builder
	for at, digit := range written {
		if at > 0 && (len(written)-at)%3 == 0 {
			grouped.WriteByte(',')
		}
		grouped.WriteRune(digit)
	}
	if negative {
		return "-" + grouped.String()
	}
	return grouped.String()
}

// FormatCountOf writes a count and the name of what it counts, in the right number.
func FormatCountOf(count int64, one, many string) string {
	if count == 1 {
		return FormatCount(count) + " " + one
	}
	return FormatCount(count) + " " + many
}

// FormatRowCount writes a count of rows.
func FormatRowCount(count int64) string {
	return FormatCountOf(count, "row", "rows")
}

// FormatStagedChanges writes the number of staged changes, as every question and report
// writes it.
func FormatStagedChanges(count int) string {
	return FormatCountOf(int64(count), "change", "changes") + " staged"
}

// DescribeStagedChanges writes the staged changes as a question about them names them,
// with the verb that agrees.
func DescribeStagedChanges(count int) string {
	verb := "were"
	if count == 1 {
		verb = "was"
	}
	return fmt.Sprintf("%s that %s never applied", FormatStagedChanges(count), verb)
}

// DescribeDroppedChanges writes the staged changes a run threw away, with the verb that
// agrees. A change names a row of the result it was staged against, and a run puts other rows
// in their place, so the change goes with them.
func DescribeDroppedChanges(count int) string {
	written := FormatCountOf(int64(count), "staged change", "staged changes")
	if count == 1 {
		return written + " was dropped with the rows it named"
	}
	return written + " were dropped with the rows they named"
}

// FormatResultSize writes the size of a result. It never claims more than the client
// knows, so a read that stopped early is marked until the user asks for the total.
func FormatResultSize(shown int, truncated bool, totalRows int64, hasTotal bool) string {
	if hasTotal {
		if int64(shown) >= totalRows {
			return FormatRowCount(int64(shown))
		}
		return fmt.Sprintf("%s of %s rows", FormatCount(int64(shown)), FormatCount(totalRows))
	}
	if truncated {
		return FormatCount(int64(shown)) + "+ rows"
	}
	return FormatRowCount(int64(shown))
}

// FormatEstimatedRows writes a row estimate short enough for the detail column.
func FormatEstimatedRows(count int64) string {
	if count <= 0 {
		return ""
	}
	if count >= 1_000_000 {
		return fmt.Sprintf("%.1fM", float64(count)/1_000_000)
	}
	if count >= 1_000 {
		return fmt.Sprintf("%.1fk", float64(count)/1_000)
	}
	return strconv.FormatInt(count, 10)
}

// IsJSONType is true for a column the server stores a document in. A viewer indents one,
// the cell editor writes it over several lines, and a save puts it back on one.
func IsJSONType(dataType string) bool {
	return core.IsDocumentType(dataType)
}

var jsonStart = regexp.MustCompile(`^[[{"]|^-?\d|^true$|^false$|^null$`)

// PrettifyJSON returns indented JSON, or nothing when the text is not JSON.
func PrettifyJSON(text string) (string, bool) {
	trimmed := strings.TrimSpace(text)
	if trimmed == "" || !jsonStart.MatchString(trimmed) {
		return "", false
	}
	value, isJSON := core.ReadJSON(trimmed)
	if !isJSON {
		return "", false
	}
	return value.WriteIndented("  "), true
}

// CompactJSON returns the value on one line, or nothing when the text is not JSON.
func CompactJSON(text string) (string, bool) {
	trimmed := strings.TrimSpace(text)
	if trimmed == "" {
		return "", false
	}
	value, isJSON := core.ReadJSON(trimmed)
	if !isJSON {
		return "", false
	}
	return value.Write(), true
}

// FormatForViewer writes a value for a full-height viewer: JSON is indented, and
// everything else keeps its newlines.
func FormatForViewer(value any, dataType string) string {
	if value == nil {
		return core.NullText
	}
	text := core.FormatCell(value, dataType)
	if IsJSONType(dataType) || core.IsStructuredValue(value) {
		if written, isJSON := PrettifyJSON(text); isJSON {
			return written
		}
	}
	return text
}

// SafeLines returns a block of text a terminal can draw, keeping the line breaks that make it
// a block. Every other control character becomes a space.
func SafeLines(text string) string {
	if !needsSafeText(text) {
		return text
	}
	lines := strings.Split(text, "\n")
	for at, line := range lines {
		lines[at] = SafeText(line)
	}
	return strings.Join(lines, "\n")
}

// FormatDuration writes a run time in the unit that reads best.
func FormatDuration(elapsed time.Duration) string {
	return core.FormatDuration(elapsed)
}
