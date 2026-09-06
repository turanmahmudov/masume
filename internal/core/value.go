// Package core provides shared values, sorting, filters, staged changes, engine metadata, and file paths.
package core

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math"
	"reflect"
	"strconv"
	"strings"
	"time"
	"unicode"
)

// NullText is the text form of a null, used by the viewer, the clipboard and the cell
// editor.
const NullText = "NULL"

// DocumentValue is a document or array with serialized text and an entry count. The text preserves server value types.
type DocumentValue struct {
	Text string
	// Count is the number of fields in a document, or the number of elements in an array.
	Count   int
	IsArray bool
}

// DescribeShape returns the document field count or array element count for a grid cell.
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

// FormatCell converts server values to display text for statements, exports, and the grid.
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
		// Timestamps use three decimal places.
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
		// JSON fields retain server order.
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

// documentTypes is the set of JSON, document, and array column types.
var documentTypes = map[string]bool{
	"json": true, "jsonb": true, "object": true, "array": true,
}

// IsDocumentType is true for a column type that holds a document.
func IsDocumentType(dataType string) bool {
	return documentTypes[dataType]
}

// IsListValue is true for arrays and slices, excluding byte slices.
func IsListValue(value any) bool {
	switch value.(type) {
	case nil, string, []byte:
		return false
	}
	kind := reflect.TypeOf(value).Kind()
	return kind == reflect.Slice || kind == reflect.Array
}

// IsStructuredValue is true for values that the viewer displays as structured data.
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

// needsCollapse detects repeated, leading, trailing, or non-space whitespace.
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

// isBlankByte is true for ASCII whitespace used by CollapseWhitespace.
func isBlankByte(held byte) bool {
	switch held {
	case ' ', '\t', '\n', '\r', '\v', '\f':
		return true
	}
	return false
}

// FormatClockTime formats a timestamp in its existing time zone.
func FormatClockTime(at time.Time) string {
	return at.Format("2006-01-02 15:04:05.000")
}

// FormatClock returns mm:ss or hh:mm:ss. Negative durations display as zero.
func FormatClock(elapsed time.Duration) string {
	seconds := max(int64(elapsed/time.Second), 0)
	minutes, seconds := seconds/60, seconds%60
	hours, minutes := minutes/60, minutes%60
	if hours > 0 {
		return fmt.Sprintf("%02d:%02d:%02d", hours, minutes, seconds)
	}
	return fmt.Sprintf("%02d:%02d", minutes, seconds)
}

// FormatLockMode removes the PostgreSQL Lock suffix, separates capitalized words, and returns uppercase text.
func FormatLockMode(mode string) string {
	trimmed := strings.TrimSuffix(strings.TrimSpace(mode), "Lock")
	if trimmed == "" {
		return ""
	}
	var said strings.Builder
	for at, letter := range trimmed {
		if at > 0 && unicode.IsUpper(letter) {
			said.WriteByte(' ')
		}
		said.WriteRune(unicode.ToUpper(letter))
	}
	return said.String()
}

// FormatLargestUnit returns whole days, hours, minutes, or seconds. Negative durations display as zero seconds.
func FormatLargestUnit(elapsed time.Duration) string {
	seconds := max(int64(elapsed/time.Second), 0)
	for _, unit := range []struct {
		mark    string
		seconds int64
	}{
		{"d", 24 * 60 * 60},
		{"h", 60 * 60},
		{"m", 60},
	} {
		if seconds >= unit.seconds {
			return fmt.Sprintf("%d%s", seconds/unit.seconds, unit.mark)
		}
	}
	return fmt.Sprintf("%ds", seconds)
}

// byteUnits are the marks of the sizes a rate is written in, from the largest down.
var byteUnits = []struct {
	mark string
	of   float64
}{
	{"TB", 1 << 40}, {"GB", 1 << 30}, {"MB", 1 << 20}, {"kB", 1 << 10},
}

// FormatByteRate returns bytes per second with the largest applicable unit and at most one decimal place.
func FormatByteRate(perSecond float64) string {
	if perSecond < 0 {
		perSecond = 0
	}
	for _, unit := range byteUnits {
		if perSecond >= unit.of {
			return trimTrailingZero(fmt.Sprintf("%.1f", perSecond/unit.of)) + unit.mark + "/s"
		}
	}
	return fmt.Sprintf("%.0fB/s", perSecond)
}

// FormatRate formats a count per second, with a k suffix for thousands.
func FormatRate(perSecond float64) string {
	if perSecond < 0 {
		perSecond = 0
	}
	if perSecond >= 1000 {
		return trimTrailingZero(fmt.Sprintf("%.1f", perSecond/1000)) + "k"
	}
	if perSecond >= 10 {
		return fmt.Sprintf("%.0f", perSecond)
	}
	return trimTrailingZero(fmt.Sprintf("%.1f", perSecond))
}

// FormatShare returns a share from zero to one as a percentage with one decimal.
func FormatShare(share float64) string {
	return trimTrailingZero(fmt.Sprintf("%.1f", min(max(share, 0), 1)*100)) + "%"
}

// trimTrailingZero removes a trailing .0.
func trimTrailingZero(written string) string {
	return strings.TrimSuffix(written, ".0")
}

// FormatDuration returns a run time in milliseconds or in seconds, whichever is easier
// to read.
func FormatDuration(elapsed time.Duration) string {
	milliseconds := float64(elapsed) / float64(time.Millisecond)
	if milliseconds < 1 {
		return fmt.Sprintf("%.2f ms", milliseconds)
	}
	if milliseconds < 1000 {
		return fmt.Sprintf("%d ms", int64(math.Round(milliseconds)))
	}
	return fmt.Sprintf("%.2f s", milliseconds/1000)
}
