package mysql

import (
	"testing"

	"github.com/turanmahmudov/masume/internal/db"
)

func TestReadFirstCellReadsThePlanCell(t *testing.T) {
	if ReadFirstCell(db.QueryResult{}) != "" {
		t.Error("an empty result answered a cell")
	}
	held := ReadFirstCell(db.QueryResult{Rows: [][]any{{"nested loop"}}})
	if held != "nested loop" {
		t.Errorf("the cell reads %q", held)
	}
}

func TestReadNamedPlanRowsKeysEachCellByColumn(t *testing.T) {
	rows, order := ReadNamedPlanRows(db.QueryResult{
		Columns: []db.ResultColumn{{Name: "id"}, {Name: "operator"}},
		Rows:    [][]any{{int64(1), "HashJoin"}, {int64(2)}},
	})
	if len(order) != 2 || order[0] != "id" {
		t.Errorf("the columns read %v", order)
	}
	if len(rows) != 2 {
		t.Fatalf("the plan holds %d rows", len(rows))
	}
	if rows[0]["operator"] != "HashJoin" {
		t.Errorf("the first operator reads %v", rows[0]["operator"])
	}
	if _, held := rows[1]["operator"]; held {
		t.Error("a missing cell was invented")
	}
}

func TestBuildKillStatementNamesTheSession(t *testing.T) {
	if held := BuildKillStatement(42, false); held != "kill query 42" {
		t.Errorf("a query kill reads %q", held)
	}
	if held := BuildKillStatement(42, true); held != "kill connection 42" {
		t.Errorf("a connection kill reads %q", held)
	}
}
