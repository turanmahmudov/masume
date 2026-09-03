package core

import (
	"strings"
	"time"
)

// SortDirection is the direction of one sort key.
type SortDirection string

const (
	// SortAscending sorts from the lowest value to the highest.
	SortAscending SortDirection = "asc"
	// SortDescending sorts from the highest value to the lowest.
	SortDescending SortDirection = "desc"
)

// SortState is one sort key: a column and a direction.
type SortState struct {
	Column    string        `json:"column"`
	Direction SortDirection `json:"direction"`
}

// ApplySortColumn returns the new sort after the user selected this column. A column that
// is already in the sort reverses its direction and keeps its position, because a move would
// change which column sorts first. A new column is added at the end, where it has the least
// effect. If add is false, the sort is replaced by this one column.
func ApplySortColumn(sort []SortState, column string, add bool) []SortState {
	turned := TurnSortDirection(FindSortDirection(sort, column))
	if !add {
		return []SortState{{Column: column, Direction: turned}}
	}

	built := make([]SortState, 0, len(sort)+1)
	held := false
	for _, key := range sort {
		if key.Column != column {
			built = append(built, key)
			continue
		}
		built = append(built, SortState{Column: column, Direction: turned})
		held = true
	}
	if held {
		return built
	}
	return append(built, SortState{Column: column, Direction: turned})
}

// FindSortDirection returns the direction of this column, or an empty string if the
// column is not in the sort.
func FindSortDirection(sort []SortState, column string) SortDirection {
	for _, key := range sort {
		if key.Column == column {
			return key.Direction
		}
	}
	return ""
}

// TurnSortDirection returns the next direction. A column that is not sorted yet starts
// with ascending order, which is what the user expects from the first key press.
func TurnSortDirection(held SortDirection) SortDirection {
	if held == SortAscending {
		return SortDescending
	}
	return SortAscending
}

// FilterTest is the test applied to a cell. A null needs its own test, because
// `= null` matches nothing.
type FilterTest string

// The tests a compare step can use.
const (
	FilterEquals    FilterTest = "equals"
	FilterDiffers   FilterTest = "differs"
	FilterIsNull    FilterTest = "is-null"
	FilterIsNotNull FilterTest = "is-not-null"
)

// FilterKind separates a bound comparison from text the user typed.
type FilterKind string

// The two kinds of filter step.
const (
	FilterCompare FilterKind = "compare"
	FilterRaw     FilterKind = "raw"
)

// FilterStep is one step of a filter: a bound comparison, or text the user typed
// which the client does not parse.
type FilterStep struct {
	Kind   FilterKind `json:"kind"`
	Column string     `json:"column,omitempty"`
	Test   FilterTest `json:"test,omitempty"`
	Value  any        `json:"value,omitempty"`
	Text   string     `json:"text,omitempty"`
}

// resolveBindValue keeps a type that the driver can send unchanged. Only a type the
// driver cannot send becomes text, so a date keeps the offset that the display omits.
func resolveBindValue(value any) any {
	switch held := value.(type) {
	case nil:
		return nil
	case bool, int, int8, int16, int32, int64, uint, uint8, uint16, uint32, uint64, float32, float64:
		return held
	case string:
		return held
	case time.Time:
		return held.UTC().Format(time.RFC3339Nano)
	}
	return FormatCell(value, "")
}

// BuildCellFilter returns the "equal to this cell" or "not equal to this cell" step used
// by the filter keys.
func BuildCellFilter(column string, value any, exclude bool) FilterStep {
	if value == nil {
		test := FilterIsNull
		if exclude {
			test = FilterIsNotNull
		}
		return FilterStep{Kind: FilterCompare, Column: column, Test: test}
	}
	test := FilterEquals
	if exclude {
		test = FilterDiffers
	}
	return FilterStep{Kind: FilterCompare, Column: column, Test: test, Value: resolveBindValue(value)}
}

// BuildRawFilter returns a step for the predicate the user typed. Empty text clears the
// filter.
func BuildRawFilter(text string) (FilterStep, bool) {
	trimmed := strings.TrimSpace(text)
	if trimmed == "" {
		return FilterStep{}, false
	}
	return FilterStep{Kind: FilterRaw, Text: trimmed}, true
}

// ReadRewrite is the sort and filter a tab applies to a read.
type ReadRewrite struct {
	Sort   []SortState
	Filter []FilterStep
}
