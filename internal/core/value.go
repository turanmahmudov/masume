// Package core holds the types every tier above it uses: the text form of a value, the
// sort and filter of a tab, the staged changes of the grid, and the paths this client
// writes to.
package core

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"
)

// NullText is the text form of a null, used by the viewer, the clipboard and the cell
// editor.
const NullText = "NULL"

// DocumentValue is a value that contains fields or elements instead of one reading: an
// embedded document, or an array. It holds the text of the whole value and the number of
// entries, so a cell can show the size without parsing the text again.
//
// The text keeps the type of every value. A plain number does not show whether the server
// stores it in four bytes or eight, and a plain timestamp is not different from a string
// of the same form, so the document tree would show the wrong type.
type DocumentValue struct {
	Text string
	// Count is the number of fields in a document, or the number of elements in an array.
	Count   int
	IsArray bool
}

// DescribeShape returns a one-line summary of the value for a grid cell. The grid shows
// the summary and not the text, because a document truncated to the column width shows
// only the name of its first field.
func (value DocumentValue) DescribeShape() string {
	if value.IsArray {
		if value.Count == 1 {
			return "[ 1 element ]"
		}
		return "[ " + strconv.Itoa(value.Count) + " elements ]"
	}
	if value.Count == 1 {
		return "{ 1 field }"
	}
	return "{ " + strconv.Itoa(value.Count) + " fields }"
}

// FormatCell converts a server value into text. A statement, an export and the grid all
// use it, so the text is the same everywhere.
func FormatCell(value any, dataType string) string {
	switch held := value.(type) {
	case nil:
		return NullText
	case string:
		return held
	case DocumentValue:
		return held.Text
	case []byte:
		return `\x` + hex.EncodeToString(held)
	case time.Time:
		if dataType == "date" {
			return held.UTC().Format("2006-01-02")
		}
		// Always three decimal places, so a column of timestamps is aligned.
		return held.UTC().Format("2006-01-02 15:04:05.000")
	case bool:
		return strconv.FormatBool(held)
	case float32:
		return formatFloat(float64(held))
	case float64:
		return formatFloat(held)
	case int, int8, int16, int32, int64, uint, uint8, uint16, uint32, uint64:
		return fmt.Sprintf("%d", held)
	case json.RawMessage:
		// A JSON value from the server keeps the order of its fields, so it is written
		// from its own text and not from a map.
		if value, isJSON := ReadJSON(string(held)); isJSON {
			return value.Write()
		}
		return string(held)
	case fmt.Stringer:
		return held.String()
	}

	written, err := json.Marshal(value)
	if err != nil {
		return fmt.Sprintf("%v", value)
	}
	return string(written)
}

// documentTypes are the column types that hold a document and not one value: JSON on a
// SQL server, and the embedded document and array of MongoDB.
var documentTypes = map[string]bool{
	"json": true, "jsonb": true, "object": true, "array": true,
}

// IsDocumentType is true for a column type that holds a document.
func IsDocumentType(dataType string) bool {
	return documentTypes[dataType]
}

// IsStructuredValue is true if the driver returned the value as a structure: a JSON
// value, a list, or a record. The full-height viewer indents such a value, whatever the
// column type is.
func IsStructuredValue(value any) bool {
	switch value.(type) {
	case nil, string, []byte, time.Time, bool, float32, float64,
		int, int8, int16, int32, int64, uint, uint8, uint16, uint32, uint64:
		return false
	case json.RawMessage, DocumentValue:
		return true
	}
	_, writesItself := value.(fmt.Stringer)
	return !writesItself
}

func formatFloat(value float64) string {
	if value == math.Trunc(value) && math.Abs(value) < 1e15 {
		return strconv.FormatFloat(value, 'f', -1, 64)
	}
	return strconv.FormatFloat(value, 'g', -1, 64)
}

// CollapseWhitespace replaces every group of blank characters with one space.
func CollapseWhitespace(text string) string {
	if !needsCollapse(text) {
		return text
	}
	var built strings.Builder
	built.Grow(len(text))
	blank := false
	for _, character := range text {
		if character == ' ' || character == '\t' || character == '\n' || character == '\r' ||
			character == '\v' || character == '\f' {
			blank = true
			continue
		}
		if blank && built.Len() > 0 {
			built.WriteByte(' ')
		}
		blank = false
		built.WriteRune(character)
	}
	if blank && built.Len() > 0 {
		built.WriteByte(' ')
	}
	return built.String()
}

// needsCollapse is true if the text has two blanks together, a blank at the start or the
// end, or a blank that is not a space. A document can be kilobytes of text with nothing to
// collapse, and this test returns that text unchanged.
func needsCollapse(text string) bool {
	previousBlank := false
	for at := 0; at < len(text); at++ {
		blank := isBlankByte(text[at])
		if blank && (previousBlank || at == 0 || at == len(text)-1 || text[at] != ' ') {
			return true
		}
		previousBlank = blank
	}
	return false
}

// isBlankByte is true for a byte that CollapseWhitespace treats as blank. All of them are
// ASCII, and no byte of a multibyte character is ASCII, so the test reads one byte at a
// time.
func isBlankByte(held byte) bool {
	switch held {
	case ' ', '\t', '\n', '\r', '\v', '\f':
		return true
	}
	return false
}

// FormatClockTime returns a clock time in the time zone of the terminal, because the app
// read the time.
func FormatClockTime(at time.Time) string {
	return at.Format("2006-01-02 15:04:05.000")
}

// FormatDuration returns a run time in milliseconds or in seconds, whichever is easier
// to read.
func FormatDuration(elapsed time.Duration) string {
	milliseconds := float64(elapsed) / float64(time.Millisecond)
	if milliseconds < 1000 {
		return fmt.Sprintf("%d ms", int64(math.Round(milliseconds)))
	}
	return fmt.Sprintf("%.2f s", milliseconds/1000)
}
