package build_test

import (
	"strings"
	"testing"

	"github.com/turanmahmudov/masume/internal/core"
	"github.com/turanmahmudov/masume/internal/query/build"
)

func TestNeedsRowCountGuardOnlyWhereTheTableHasNoKey(t *testing.T) {
	if build.NeedsRowCountGuard(buildOrdersTarget("id")) {
		t.Error("a table with a key is guarded, and its key already reaches one row")
	}
	if !build.NeedsRowCountGuard(buildOrdersTarget()) {
		t.Error("a table with no key is not guarded, and it can hold the same row twice")
	}
}

func TestBuildChangeStatementsGuardsAnUpdateOnAKeylessTable(t *testing.T) {
	target := buildOrdersTarget()
	rows := [][]any{{int64(1), "ada", "1"}}

	pending := core.NewPendingChanges()
	pending.Edits[core.BuildEditKey(0, 1)] = core.CellEdit{
		RowIndex: 0, ColumnIndex: 1,
		Value: core.CellValue{Kind: core.CellText, Text: "grace"},
	}

	answered, err := build.BuildChangeStatements(target, rows, pending)
	if err != nil {
		t.Fatalf("the changes answered %v", err)
	}
	if len(answered) != 1 {
		t.Fatalf("the staged work became %d statements, wanted 1", len(answered))
	}
	written := answered[0]
	if written.Guard == nil {
		t.Fatal("the update carries no count, and the table has no key")
	}
	if written.Expect != 1 {
		t.Errorf("the count expects %d rows, wanted 1", written.Expect)
	}
	if !strings.Contains(strings.ToLower(written.Guard.SQL), "count(") {
		t.Errorf("the count is %q, wanted it to count", written.Guard.SQL)
	}
}

func TestBuildChangeStatementsLeavesAnUpdateOnAKeyedTableUnguarded(t *testing.T) {
	target := buildOrdersTarget("id")
	rows := [][]any{{int64(1), "ada", "1"}}

	pending := core.NewPendingChanges()
	pending.Edits[core.BuildEditKey(0, 1)] = core.CellEdit{
		RowIndex: 0, ColumnIndex: 1,
		Value: core.CellValue{Kind: core.CellText, Text: "grace"},
	}

	answered, err := build.BuildChangeStatements(target, rows, pending)
	if err != nil {
		t.Fatalf("the changes answered %v", err)
	}
	if answered[0].Guard != nil {
		t.Error("the update carries a count, and its key already reaches one row")
	}
}

func TestBuildChangeStatementsNeverGuardsAnInsert(t *testing.T) {
	target := buildOrdersTarget()

	pending := core.NewPendingChanges()
	pending.Inserts = append(pending.Inserts, map[string]any{"customer": "ada"})

	answered, err := build.BuildChangeStatements(target, nil, pending)
	if err != nil {
		t.Fatalf("the changes answered %v", err)
	}
	if answered[0].Guard != nil {
		t.Error("an insert carries a count, and it names its own values")
	}
}

func TestBuildChangeStatementsGuardsADeleteWithTheRowsOfItsChunk(t *testing.T) {
	target := buildOrdersTarget()
	rows := [][]any{{int64(1), "ada", "1"}, {int64(2), "grace", "2"}}

	pending := core.NewPendingChanges()
	pending.DeletedRows[0] = true
	pending.DeletedRows[1] = true

	answered, err := build.BuildChangeStatements(target, rows, pending)
	if err != nil {
		t.Fatalf("the changes answered %v", err)
	}
	if len(answered) != 1 {
		t.Fatalf("the two deletes became %d statements, wanted one chunk", len(answered))
	}
	if answered[0].Guard == nil {
		t.Fatal("the delete carries no count, and the table has no key")
	}
	if answered[0].Expect != 2 {
		t.Errorf("the count expects %d rows, wanted the 2 of the chunk", answered[0].Expect)
	}
}

// The count and the delete have to ask about the same rows, or the count guards nothing.
func TestTheCountOfADeleteBindsTheSameValuesAsTheDelete(t *testing.T) {
	target := buildOrdersTarget()
	rows := [][]any{{int64(1), "ada", "1"}, {int64(2), "grace", "2"}}

	pending := core.NewPendingChanges()
	pending.DeletedRows[0] = true
	pending.DeletedRows[1] = true

	answered, err := build.BuildChangeStatements(target, rows, pending)
	if err != nil {
		t.Fatalf("the changes answered %v", err)
	}
	written := answered[0]
	if len(written.Guard.Params) != len(written.Statement.Params) {
		t.Fatalf("the count binds %d values and the delete binds %d",
			len(written.Guard.Params), len(written.Statement.Params))
	}
	for at := range written.Statement.Params {
		if written.Guard.Params[at] != written.Statement.Params[at] {
			t.Errorf("value %d is %v in the count and %v in the delete",
				at, written.Guard.Params[at], written.Statement.Params[at])
		}
	}

	// Both read the same rows, so both carry the same where clause.
	countWhere := written.Guard.SQL[strings.Index(written.Guard.SQL, " where "):]
	deleteWhere := written.Statement.SQL[strings.Index(written.Statement.SQL, " where "):]
	if countWhere != deleteWhere {
		t.Errorf("the count asks %q and the delete asks %q", countWhere, deleteWhere)
	}
}
