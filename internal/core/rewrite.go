package core

import (
	"strings"
	"time"
)

// SortDirection is the direction of one sort key.
type SortDirection string

const (
	// SortAscending orders from the lowest value up.
	SortAscending SortDirection = "asc"
	// SortDescending orders from the highest value down.
	SortDescending SortDirection = "desc"
)

// SortState is a sort key: a column and a direction.
type SortState struct {
	Column    string        `json:"column"`
	Direction SortDirection `json:"direction"`
}

// ApplySortColumn returns the sort after the user asked to order by this column. A column
// already in the sort turns its direction over and keeps its place, because moving it would
// change which column orders the rows first. A column that is not in the sort joins the end of
// it, where it orders the rows the least. Asking without adding leaves that column alone.
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

// FindSortDirection returns the direction this column is ordered by, and nothing where the
// sort does not name it.
func FindSortDirection(sort []SortState, column string) SortDirection {
	for _, key := range sort {
		if key.Column == column {
			return key.Direction
		}
	}
	return ""
}

// TurnSortDirection returns the direction after this one. A column that is not ordered yet
// starts at the lowest value up, which is what a reader expects of a first press.
func TurnSortDirection(held SortDirection) SortDirection {
	if held == SortAscending {
		return SortDescending
	}
	return SortAscending
}

// FilterTest is how a cell is tested. A null needs its own test, because
// `= null` matches nothing.
type FilterTest string

// The tests a compare step can carry.
const (
	FilterEquals    FilterTest = "equals"
	FilterDiffers   FilterTest = "differs"
	FilterIsNull    FilterTest = "is-null"
	FilterIsNotNull FilterTest = "is-not-null"
)

// FilterKind tells a bound comparison from text the user typed.
type FilterKind string

// The two kinds of filter step.
const (
	FilterCompare FilterKind = "compare"
	FilterRaw     FilterKind = "raw"
)

// FilterStep is one step of a filter: a bound comparison, or text the user typed
// which the client does not read.
type FilterStep struct {
	Kind   FilterKind `json:"kind"`
	Column string     `json:"column,omitempty"`
	Test   FilterTest `json:"test,omitempty"`
	Value  any        `json:"value,omitempty"`
	Text   string     `json:"text,omitempty"`
}

// resolveBindValue keeps a type with a wire form as it is. Only a type with none
// becomes text, so a date keeps the offset the display drops.
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

// BuildCellFilter writes "this cell" and "not this cell", which the filter keys apply.
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

// BuildRawFilter reads the predicate the user typed. An empty one clears the filter.
func BuildRawFilter(text string) (FilterStep, bool) {
	trimmed := strings.TrimSpace(text)
	if trimmed == "" {
		return FilterStep{}, false
	}
	return FilterStep{Kind: FilterRaw, Text: trimmed}, true
}

// ReadRewrite is the sort and filter a tab lays over a read.
type ReadRewrite struct {
	Sort   []SortState
	Filter []FilterStep
}
