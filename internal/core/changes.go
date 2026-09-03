package core

import (
	"errors"
	"fmt"
	"slices"
	"sort"
	"strconv"
)

// CellValueKind separates the text "NULL" from a real null.
type CellValueKind string

// The kinds of cell value the user can select.
const (
	CellText    CellValueKind = "text"
	CellNull    CellValueKind = "null"
	CellEmpty   CellValueKind = "empty"
	CellDefault CellValueKind = "default"
)

// CellValue is a selected cell value, before an engine converts it into a statement.
type CellValue struct {
	Kind CellValueKind
	Text string
}

// DescribeCellValue returns the value as the grid and the review overlay display it.
func DescribeCellValue(value CellValue) string {
	switch value.Kind {
	case CellNull:
		return NullText
	case CellEmpty:
		return ""
	case CellDefault:
		return "DEFAULT"
	default:
		return value.Text
	}
}

// CellEdit is one staged edit to one cell.
type CellEdit struct {
	RowIndex    int
	ColumnIndex int
	Value       CellValue
}

// PendingChanges holds all staged edits, deletes and inserts for one result.
type PendingChanges struct {
	Edits       map[string]CellEdit
	DeletedRows map[int]bool
	Inserts     []map[string]any
}

// NewPendingChanges returns an empty set of staged changes.
func NewPendingChanges() PendingChanges {
	return PendingChanges{Edits: map[string]CellEdit{}, DeletedRows: map[int]bool{}}
}

// BuildEditKey returns the map key of one cell of the result.
func BuildEditKey(rowIndex, columnIndex int) string {
	return strconv.Itoa(rowIndex) + ":" + strconv.Itoa(columnIndex)
}

// CountChanges returns the number of staged changes. An edit in a row marked for
// delete is not counted a second time.
func CountChanges(pending PendingChanges) int {
	count := len(pending.DeletedRows) + len(pending.Inserts)
	for _, edit := range pending.Edits {
		if !pending.DeletedRows[edit.RowIndex] {
			count++
		}
	}
	return count
}

// SortedEdits returns the staged cell edits, sorted by row and then by column.
func SortedEdits(pending PendingChanges) []CellEdit {
	edits := make([]CellEdit, 0, len(pending.Edits))
	for _, edit := range pending.Edits {
		edits = append(edits, edit)
	}
	sort.Slice(edits, func(left, right int) bool {
		if edits[left].RowIndex != edits[right].RowIndex {
			return edits[left].RowIndex < edits[right].RowIndex
		}
		return edits[left].ColumnIndex < edits[right].ColumnIndex
	})
	return edits
}

// SortedDeletedRows returns the rows marked for delete, lowest index first.
func SortedDeletedRows(pending PendingChanges) []int {
	rows := make([]int, 0, len(pending.DeletedRows))
	for rowIndex := range pending.DeletedRows {
		rows = append(rows, rowIndex)
	}
	slices.Sort(rows)
	return rows
}

// ErrEdit is the error class for an edit the client refuses to write.
var ErrEdit = errors.New("edit")

// NewEditError returns an ErrEdit error with the given reason.
func NewEditError(reason string) error {
	return fmt.Errorf("%w: %s", ErrEdit, reason)
}
