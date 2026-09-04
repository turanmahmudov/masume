package load

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/turanmahmudov/masume/internal/core"
)

// Reads what one value of a file holds, and what a column of them holds. A file carries
// text, so the kind of a column is read from the values in it.

// timestampLayouts are the forms a timestamp in a data file is written in, the most exact
// first.
var timestampLayouts = []string{
	time.RFC3339Nano,
	time.RFC3339,
	"2006-01-02T15:04:05",
	"2006-01-02T15:04",
	"2006-01-02 15:04:05.999999999 -0700 MST",
	"2006-01-02 15:04:05.999999999",
	"2006-01-02 15:04:05",
	"2006-01-02 15:04",
	"2006-01-02",
	"02/01/2006",
}

// booleanWords give the value of every word a data file writes a boolean with.
var booleanWords = map[string]bool{
	"true": true, "false": false, "t": true, "f": false,
	"yes": true, "no": false, "y": true, "n": false, "1": true, "0": false,
}

// ReadValueKind returns the kind of one value of a file. A value that is not there belongs
// to no kind.
func ReadValueKind(value any) (core.ColumnKind, bool) {
	switch held := value.(type) {
	case nil:
		return "", false
	case bool:
		return core.KindBoolean, true
	case json.Number:
		if _, err := strconv.ParseInt(held.String(), 10, 64); err == nil {
			return core.KindInteger, true
		}
		if holdsWholeDigits(held.String()) {
			return core.KindText, true
		}
		return core.KindNumber, true
	case string:
		return readTextKind(held), true
	}
	return core.KindText, true
}

// readTextKind returns the kind the text of a field holds. A number written with a leading
// zero is read as text, because a code such as `007` is not the number seven.
func readTextKind(written string) core.ColumnKind {
	trimmed := strings.TrimSpace(written)
	if trimmed == "" {
		return core.KindText
	}
	if _, known := booleanWords[strings.ToLower(trimmed)]; known && !holdsDigitOnly(trimmed) {
		return core.KindBoolean
	}
	if !holdsLeadingZero(trimmed) {
		if _, err := strconv.ParseInt(trimmed, 10, 64); err == nil {
			return core.KindInteger
		}
		if holdsWholeDigits(trimmed) {
			return core.KindText
		}
		if _, err := strconv.ParseFloat(trimmed, 64); err == nil {
			return core.KindNumber
		}
	}
	if _, read := ReadTimestamp(trimmed); read {
		return core.KindTimestamp
	}
	return core.KindText
}

// holdsDigitOnly is true for text of digits alone. `1` and `0` are the two words a boolean
// and a number share, and a column of them is read as a number.
func holdsDigitOnly(written string) bool {
	for _, held := range written {
		if held < '0' || held > '9' {
			return false
		}
	}
	return written != ""
}

// holdsLeadingZero is true for a number written with a zero in front of it, such as a
// postal code or a product code. A column of them is text, because the zero is part of the
// value and a number would drop it.
func holdsLeadingZero(written string) bool {
	held := strings.TrimPrefix(strings.TrimPrefix(written, "-"), "+")
	return len(held) > 1 && held[0] == '0' && held[1] != '.'
}

// ReadTimestamp parses a timestamp written in any of the forms a data file uses.
func ReadTimestamp(written string) (time.Time, bool) {
	for _, layout := range timestampLayouts {
		if held, err := time.Parse(layout, written); err == nil {
			return held, true
		}
	}
	return time.Time{}, false
}

// ResolveColumnKind returns the kind that holds every value of a column. A column of nothing
// but values that are not there is text.
func ResolveColumnKind(values []any) core.ColumnKind {
	resolved := core.ColumnKind("")
	for _, value := range values {
		kind, holds := ReadValueKind(value)
		if !holds {
			continue
		}
		resolved = core.ResolveWiderKind(resolved, kind)
		if resolved == core.KindText {
			return core.KindText
		}
	}
	if resolved == "" {
		return core.KindText
	}
	return resolved
}

// ValueError is a value of a file that the column it is mapped to cannot hold.
type ValueError struct{ Reason string }

func (err ValueError) Error() string { return err.Reason }

// CastValue returns the value as the kind of its column holds it. A value the kind cannot
// hold is reported with the reason.
func CastValue(value any, kind core.ColumnKind) (any, error) {
	if value == nil {
		return nil, nil
	}

	written := ""
	switch held := value.(type) {
	case string:
		written = strings.TrimSpace(held)
	case json.Number:
		written = held.String()
	case bool:
		if kind == core.KindBoolean || kind == core.KindText {
			return castBoolean(held, kind), nil
		}
		return nil, failValue("%v is no %s", held, kind)
	default:
		return value, nil
	}

	if written == "" {
		return nil, nil
	}
	return castText(written, kind)
}

// castBoolean returns a boolean as its own kind, or as text.
func castBoolean(held bool, kind core.ColumnKind) any {
	if kind == core.KindText {
		return strconv.FormatBool(held)
	}
	return held
}

func castText(written string, kind core.ColumnKind) (any, error) {
	switch kind {
	case core.KindText:
		return written, nil
	case core.KindInteger:
		held, err := strconv.ParseInt(written, 10, 64)
		if err != nil {
			return nil, failValue("%q is no whole number", written)
		}
		return held, nil
	case core.KindNumber:
		held, err := strconv.ParseFloat(written, 64)
		if err != nil {
			return nil, failValue("%q is no number", written)
		}
		return held, nil
	case core.KindBoolean:
		held, known := booleanWords[strings.ToLower(written)]
		if !known {
			return nil, failValue("%q is no yes or no", written)
		}
		return held, nil
	case core.KindTimestamp:
		held, read := ReadTimestamp(written)
		if !read {
			return nil, failValue("%q is no date or time", written)
		}
		return held, nil
	}
	return written, nil
}

func failValue(format string, parts ...any) error {
	return ValueError{Reason: fmt.Sprintf(format, parts...)}
}

// holdsWholeDigits is true for a whole number written in digits alone. One too large for 64
// bits is kept as text, which keeps its last digits.
func holdsWholeDigits(written string) bool {
	digits := strings.TrimPrefix(strings.TrimPrefix(written, "-"), "+")
	if digits == "" {
		return false
	}
	for _, letter := range digits {
		if letter < '0' || letter > '9' {
			return false
		}
	}
	return true
}
