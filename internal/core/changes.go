package core

import (
	"errors"
	"fmt"
	"slices"
	"sort"
	"strconv"
)

// CellValueKind tells the text "NULL" and a real null apart.
type CellValueKind string

// The kinds a chosen cell value can have.
const (
	CellText    CellValueKind = "text"
	CellNull    CellValueKind = "null"
	CellEmpty   CellValueKind = "empty"
	CellDefault CellValueKind = "default"
)

// CellValue is a chosen cell value, before an engine turns it into a statement.
type CellValue struct {
	Kind CellValueKind
	Text string
}

// DescribeCellValue writes the value as the grid and the review overlay show it.
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

// PendingChanges is everything staged against one result: edits, deletes and inserts.
type PendingChanges struct {
	Edits       map[string]CellEdit
	DeletedRows map[int]bool
	Inserts     []map[string]any
}

// NewPendingChanges builds an empty set of staged work.
func NewPendingChanges() PendingChanges {
	return PendingChanges{Edits: map[string]CellEdit{}, DeletedRows: map[int]bool{}}
}

// BuildEditKey names one cell of the result.
func BuildEditKey(rowIndex, columnIndex int) string {
	return strconv.Itoa(rowIndex) + ":" + strconv.Itoa(columnIndex)
}

// CountChanges counts the staged work. A cell of a row marked for delete is not
// counted twice.
func CountChanges(pending PendingChanges) int {
	count := len(pending.DeletedRows) + len(pending.Inserts)
	for _, edit := range pending.Edits {
		if !pending.DeletedRows[edit.RowIndex] {
			count++
		}
	}
	return count
}

// SortedEdits lists the staged cell edits by row and then by column.
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

// SortedDeletedRows lists the rows marked for delete, lowest first.
func SortedDeletedRows(pending PendingChanges) []int {
	rows := make([]int, 0, len(pending.DeletedRows))
	for rowIndex := range pending.DeletedRows {
		rows = append(rows, rowIndex)
	}
	slices.Sort(rows)
	return rows
}

// ErrEdit marks an edit the client refuses to write.
var ErrEdit = errors.New("edit")

// NewEditError builds a refusal with its reason.
func NewEditError(reason string) error {
	return fmt.Errorf("%w: %s", ErrEdit, reason)
}
