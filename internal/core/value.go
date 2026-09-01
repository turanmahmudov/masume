// Package core holds what every tier above it reads: the text form of a value,
// the sort and filter of a tab, the staged work of the grid, and the paths this
// client writes to.
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

// NullText is the text form of a null, for a viewer, the clipboard and the cell editor.
const NullText = "NULL"

// DocumentValue is a value that holds fields or elements rather than one reading: an
// embedded document, or an array. It carries the text of the whole value and how much the
// value holds, so a cell can say the shape of it without reading the text again.
//
// The text is written so that every type survives it. A number written plainly cannot say
// whether the server holds it in four bytes or eight, and a moment written plainly cannot be
// told from a string of the same shape, so the tree that opens this value would name the
// wrong type for it.
type DocumentValue struct {
	Text string
	// Count is how many fields a document holds, or how many elements an array holds.
	Count   int
	IsArray bool
}

// DescribeShape says what the value holds, for a cell that draws one line. A grid draws the
// shape rather than the text, because a document cut off at the width of a column says only
// what its first field is called.
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

// FormatCell turns a server value into text, the same way for a statement, an
// export and the grid.
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
		// Three places always, whatever the server sent, so a column of stamps lines up.
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
		// A JSON value the server sent keeps the order of its fields, so it is written
		// again from its own text and not from a map.
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

// documentTypes name a column whose values are a document rather than one value: the JSON
// of a SQL server, and the embedded document and array of MongoDB.
var documentTypes = map[string]bool{
	"json": true, "jsonb": true, "object": true, "array": true,
}

// IsDocumentType is true for a column the server stores a document in.
func IsDocumentType(dataType string) bool {
	return documentTypes[dataType]
}

// IsStructuredValue is true where the driver read a value as a structure: a JSON value, a
// list, or a record. A full-height viewer indents one of those, whatever the column says its
// type is.
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

// CollapseWhitespace turns every run of blank characters into one space.
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

// needsCollapse is true where a run of blanks, a blank at either end, or a blank that is
// no space would be written differently. A document of a collection is kilobytes of text
// with nothing to collapse, so the text of one is answered without being built again.
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

// isBlankByte is true for a byte CollapseWhitespace reads as blank. Every one of them is
// ASCII, and a byte of a longer character never is, so the bytes are read one at a time.
func isBlankByte(held byte) bool {
	switch held {
	case ' ', '\t', '\n', '\r', '\v', '\f':
		return true
	}
	return false
}

// FormatClockTime writes a clock time in the zone of the terminal, because the
// app took the reading.
func FormatClockTime(at time.Time) string {
	return at.Format("2006-01-02 15:04:05.000")
}

// FormatDuration writes a run time in the unit that reads best.
func FormatDuration(elapsed time.Duration) string {
	milliseconds := float64(elapsed) / float64(time.Millisecond)
	if milliseconds < 1000 {
		return fmt.Sprintf("%d ms", int64(math.Round(milliseconds)))
	}
	return fmt.Sprintf("%.2f s", milliseconds/1000)
}
