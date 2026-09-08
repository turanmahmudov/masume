package sqlserver

import (
	"testing"

	"github.com/turanmahmudov/masume/internal/db"
)

// buildPlanResult returns the plan the server writes, in the columns it names.
func buildPlanResult(names []string, rows [][]any) db.QueryResult {
	columns := make([]db.ResultColumn, 0, len(names))
	for _, name := range names {
		columns = append(columns, db.ResultColumn{Name: name})
	}
	return db.QueryResult{Columns: columns, Rows: rows}
}

// The server numbers every step and names the step above it, so the tree is built from the
// numbers and not from the indentation of the text.
func TestBuildPlanBuildsTheTreeFromTheNumbers(t *testing.T) {
	answered := buildPlanResult(
		[]string{"StmtText", "NodeId", "Parent", "PhysicalOp", "Argument",
			"EstimateRows", "Type", "EstimateExecutions"},
		[][]any{
			{"select customer from dbo.orders", int64(1), int64(0), nil, nil,
				2.0, "SELECT", nil},
			{"  |--Sort", int64(2), int64(1), "Sort", "ORDER BY:([customer])",
				2.0, "PLAN_ROW", 1.0},
			{"       |--Table Scan", int64(3), int64(2), "Table Scan", "OBJECT:([orders])",
				3.0, "PLAN_ROW", 2.0},
		})

	plan, built := BuildPlan(answered, false)
	if !built {
		t.Fatal("the plan of the server did not read")
	}
	if plan.Root.Label != "SELECT" {
		t.Errorf("the root reads %q, wanted the kind of statement", plan.Root.Label)
	}
	if len(plan.Root.Children) != 1 {
		t.Fatalf("the root holds %d steps, wanted 1", len(plan.Root.Children))
	}
	sort := plan.Root.Children[0]
	if sort.Label != "Sort" || sort.Detail != "ORDER BY:([customer])" {
		t.Errorf("the step under the root reads %q with %q", sort.Label, sort.Detail)
	}
	if len(sort.Children) != 1 {
		t.Fatalf("the sort holds %d steps, wanted 1", len(sort.Children))
	}
	// A step that runs more than once estimates that many rows every time it runs.
	if scan := sort.Children[0]; scan.EstimatedRows != 6 {
		t.Errorf("the scan estimates %v rows, wanted 6", scan.EstimatedRows)
	}
	if plan.Analyzed {
		t.Error("an estimated plan reads as measured")
	}
	if plan.Raw == "" {
		t.Error("the plan carries none of the text the server wrote")
	}
}

// A measured plan counts the rows of every step, which the server writes in a column of its
// own before the text.
func TestBuildPlanReadsTheCountedRowsOfAMeasuredPlan(t *testing.T) {
	answered := buildPlanResult(
		[]string{"Rows", "Executes", "StmtText", "NodeId", "Parent", "PhysicalOp",
			"Argument", "EstimateRows", "Type", "EstimateExecutions"},
		[][]any{
			{int64(3), int64(1), "select customer from dbo.orders", int64(1), int64(0),
				nil, nil, 2.0, "SELECT", nil},
			{int64(3), int64(1), "  |--Table Scan", int64(2), int64(1), "Table Scan",
				"OBJECT:([orders])", 2.0, "PLAN_ROW", 1.0},
		})

	plan, built := BuildPlan(answered, true)
	if !built {
		t.Fatal("the measured plan did not read")
	}
	if !plan.Analyzed {
		t.Error("a measured plan does not read as measured")
	}
	if !plan.Root.HasActualRows || plan.Root.ActualRows != 3 {
		t.Errorf("the root counts %v rows, has %v", plan.Root.ActualRows, plan.Root.HasActualRows)
	}
}

// A result that names no step is the result of the statement, not a plan.
func TestBuildPlanRefusesAResultThatNamesNoStep(t *testing.T) {
	answered := buildPlanResult([]string{"customer"}, [][]any{{"ada"}})
	if _, built := BuildPlan(answered, false); built {
		t.Error("a result of the statement read as a plan")
	}
}
