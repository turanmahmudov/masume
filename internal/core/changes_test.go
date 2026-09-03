package core

import "testing"

// The staged changes run in a fixed order, so the same edits always give the same
// statements and the review card lists them in run order.
func TestSortedEditsAnswerTheSameOrderEveryTime(t *testing.T) {
	pending := NewPendingChanges()
	// Added out of order on purpose.
	for _, held := range []struct{ row, column int }{{2, 1}, {0, 3}, {0, 1}, {1, 0}} {
		pending.Edits[BuildEditKey(held.row, held.column)] = CellEdit{
			RowIndex: held.row, ColumnIndex: held.column,
			Value: CellValue{Kind: CellText, Text: "held"},
		}
	}

	first := SortedEdits(pending)
	if len(first) != 4 {
		t.Fatalf("four edits sorted into %d", len(first))
	}
	// Row first, then column, so the statements follow the row order of the grid.
	for at := 1; at < len(first); at++ {
		before, held := first[at-1], first[at]
		if before.RowIndex > held.RowIndex {
			t.Errorf("edit %d is on row %d, behind the row %d before it",
				at, held.RowIndex, before.RowIndex)
		}
		if before.RowIndex == held.RowIndex && before.ColumnIndex > held.ColumnIndex {
			t.Errorf("edit %d is on column %d, behind the column %d before it",
				at, held.ColumnIndex, before.ColumnIndex)
		}
	}

	// The same set sorts the same way again, because a map has no order of its own.
	second := SortedEdits(pending)
	for at := range first {
		if first[at] != second[at] {
			t.Errorf("edit %d sorted to %+v and then to %+v", at, first[at], second[at])
		}
	}
}

func TestSortedDeletedRowsAnswerTheRowsInOrder(t *testing.T) {
	pending := NewPendingChanges()
	for _, row := range []int{5, 1, 3} {
		pending.DeletedRows[row] = true
	}

	held := SortedDeletedRows(pending)
	if len(held) != 3 {
		t.Fatalf("three rows sorted into %d", len(held))
	}
	for at := 1; at < len(held); at++ {
		if held[at] < held[at-1] {
			t.Errorf("row %d comes after %d", held[at], held[at-1])
		}
	}
}

// The status bar and the close dialog both use this count, so it must include every kind
// of staged change and count none of them twice.
func TestCountChangesCountsEveryKindOfStagedWork(t *testing.T) {
	pending := NewPendingChanges()
	if held := CountChanges(pending); held != 0 {
		t.Errorf("nothing staged counts as %d", held)
	}

	pending.Edits[BuildEditKey(0, 0)] = CellEdit{}
	pending.Edits[BuildEditKey(0, 1)] = CellEdit{}
	pending.DeletedRows[3] = true
	pending.Inserts = append(pending.Inserts, map[string]any{"a": 1})

	if held := CountChanges(pending); held != 4 {
		t.Errorf("two edits, a delete and an insert count as %d, wanted 4", held)
	}
}

// The key of a cell is unique, so two cells never share a key and an edit never goes to the
// wrong cell.
func TestBuildEditKeyNamesOneCellOnly(t *testing.T) {
	seen := map[string]bool{}
	for row := range 4 {
		for column := range 4 {
			key := BuildEditKey(row, column)
			if seen[key] {
				t.Errorf("the key %q names more than one cell", key)
			}
			seen[key] = true
		}
	}
	// A row and a column in the other order are different cells.
	if BuildEditKey(1, 2) == BuildEditKey(2, 1) {
		t.Error("a row and a column swapped name the same cell")
	}
}

// The three kinds of selected value have three different texts, so the review card shows
// what will be written and never one kind in place of another.
func TestDescribeCellValueTellsTheKindsApart(t *testing.T) {
	written := map[string]string{}
	for _, held := range []struct {
		name  string
		value CellValue
	}{
		{"a null", CellValue{Kind: CellNull}},
		{"an empty text", CellValue{Kind: CellEmpty}},
		{"a default", CellValue{Kind: CellDefault}},
		{"a value", CellValue{Kind: CellText, Text: "ada"}},
	} {
		written[held.name] = DescribeCellValue(held.value)
	}

	if written["a null"] == written["an empty text"] {
		t.Errorf("a null and an empty text both read as %q", written["a null"])
	}
	if written["a default"] == written["a null"] {
		t.Errorf("a default and a null both read as %q", written["a default"])
	}
	if written["a value"] != "ada" {
		t.Errorf("a value reads as %q", written["a value"])
	}
	if written["an empty text"] != "" {
		t.Errorf("an empty text reads as %q, wanted nothing", written["an empty text"])
	}
}

// A sort reverses in place, and a column selected without add becomes the only sort key,
// which is what a click on a heading means.
func TestApplySortColumnTurnsOverAndReplaces(t *testing.T) {
	first := ApplySortColumn(nil, "customer", false)
	if len(first) != 1 || first[0].Column != "customer" {
		t.Fatalf("the sort reads %+v", first)
	}

	// A second selection reverses the direction and keeps the single column.
	again := ApplySortColumn(first, "customer", false)
	if len(again) != 1 {
		t.Fatalf("asking again gave %d columns", len(again))
	}
	if again[0].Direction == first[0].Direction {
		t.Error("asking for the same column again did not turn the direction over")
	}

	// Another column without add becomes the only sort key.
	other := ApplySortColumn(again, "total", false)
	if len(other) != 1 || other[0].Column != "total" {
		t.Errorf("a new column without adding gave %+v", other)
	}
}

// A column added to the sort goes to the end, where it has the least effect. A column that
// is already in the sort keeps its position, so the first column still sorts first.
func TestApplySortColumnAddedKeepsThePlaceOfTheFirst(t *testing.T) {
	held := ApplySortColumn(nil, "customer", false)
	held = ApplySortColumn(held, "total", true)
	if len(held) != 2 {
		t.Fatalf("adding a column gave %d", len(held))
	}
	if held[0].Column != "customer" {
		t.Errorf("the first column is %q, wanted the one asked for first", held[0].Column)
	}

	// A reverse of the first key must not move it after the second key.
	turned := ApplySortColumn(held, "customer", true)
	if turned[0].Column != "customer" {
		t.Errorf("turning the first over moved it: the first is now %q", turned[0].Column)
	}
	if turned[0].Direction == held[0].Direction {
		t.Error("the direction of the first column did not turn over")
	}
}

func TestFindSortDirectionAnswersTheColumnAsked(t *testing.T) {
	sort := []SortState{
		{Column: "customer", Direction: SortAscending},
		{Column: "total", Direction: SortDescending},
	}
	if held := FindSortDirection(sort, "total"); held != SortDescending {
		t.Errorf("the direction of total reads %q", held)
	}
	// A column that is not in the sort gives a direction the caller can reverse.
	held := FindSortDirection(sort, "nothing")
	if TurnSortDirection(held) == held {
		t.Errorf("a column not in the sort reads %q, which does not turn over", held)
	}
}

func TestTurnSortDirectionGoesBothWays(t *testing.T) {
	if TurnSortDirection(SortAscending) != SortDescending {
		t.Error("up did not turn over to down")
	}
	if TurnSortDirection(SortDescending) != SortAscending {
		t.Error("down did not turn over to up")
	}
}

// A filter built from a cell compares that column with that value, and the exclude form
// uses the opposite test.
func TestBuildCellFilterTestsTheColumnAgainstTheValue(t *testing.T) {
	keep := BuildCellFilter("customer", "ada", false)
	if keep.Column != "customer" {
		t.Errorf("the filter names %q", keep.Column)
	}
	drop := BuildCellFilter("customer", "ada", true)
	if drop.Test == keep.Test {
		t.Errorf("keeping and dropping both test %q", keep.Test)
	}
}

// A null cannot be compared with equals, so a cell with a null uses the null test.
func TestBuildCellFilterUsesTheNullTestForANull(t *testing.T) {
	held := BuildCellFilter("total", nil, false)
	if held.Test != FilterIsNull {
		t.Errorf("a null filters on %q, wanted the null test", held.Test)
	}
	excluded := BuildCellFilter("total", nil, true)
	if excluded.Test != FilterIsNotNull {
		t.Errorf("excluding a null filters on %q", excluded.Test)
	}
}

func TestBuildRawFilterRefusesAnEmptyText(t *testing.T) {
	if _, is := BuildRawFilter("total > 0"); !is {
		t.Error("a written test was refused")
	}
	for _, text := range []string{"", "   ", "\n"} {
		if _, is := BuildRawFilter(text); is {
			t.Errorf("%q was read as a test", text)
		}
	}
}
