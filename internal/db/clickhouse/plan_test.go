package clickhouse

import (
	"strings"
	"testing"

	"github.com/turanmahmudov/masume/internal/db"
)

// buildPlanResult returns the plan of the server, which is one column of text.
func buildPlanResult(written string) db.QueryResult {
	rows := [][]any{}
	for _, line := range strings.Split(written, "\n") {
		rows = append(rows, []any{line})
	}
	return db.QueryResult{
		Columns: []db.ResultColumn{{Name: "explain"}}, Rows: rows,
	}
}

// The server writes one step per line, and the spaces before a line say what it stands
// under.
func TestBuildPlanReadsTheTreeFromTheIndentation(t *testing.T) {
	plan, built := BuildPlan(buildPlanResult(
		"Expression ((Project names + Projection))\n" +
			"  Expression ((WHERE + Change column names))\n" +
			"    ReadFromMergeTree (shop.orders)\n" +
			"    Indexes:\n" +
			"      PrimaryKey\n" +
			"        Keys:\n" +
			"          id\n" +
			"        Condition: (id in [2, 2])\n" +
			"        Granules: 1/1"))
	if !built {
		t.Fatal("the plan of the server did not read")
	}
	if plan.Root.Label != "Expression ((Project names + Projection))" {
		t.Errorf("the root reads %q", plan.Root.Label)
	}
	if len(plan.Root.Children) != 1 {
		t.Fatalf("the root holds %d steps, wanted 1", len(plan.Root.Children))
	}
	held := plan.Root.Children[0]
	if len(held.Children) != 2 {
		t.Fatalf("the step holds %d steps, wanted the read and its indexes",
			len(held.Children))
	}
	if held.Children[0].Label != "ReadFromMergeTree (shop.orders)" {
		t.Errorf("the read reads %q", held.Children[0].Label)
	}
	if plan.Measurable {
		t.Error("the plan reads as measurable, and no plan of this server is measured")
	}
	if plan.Raw == "" {
		t.Error("the plan carries none of the text the server wrote")
	}
}

// A line that names a property of a step belongs to that step and not under it.
func TestBuildPlanReadsAPropertyAsTheDetailOfItsStep(t *testing.T) {
	plan, built := BuildPlan(buildPlanResult(
		"ReadFromMergeTree (shop.orders)\n" +
			"  Condition: (id in [2, 2])"))
	if !built {
		t.Fatal("the plan did not read")
	}
	if plan.Root.Detail != "Condition: (id in [2, 2])" {
		t.Errorf("the step carries the detail %q", plan.Root.Detail)
	}
	if len(plan.Root.Children) != 0 {
		t.Errorf("the step holds %d steps, wanted none", len(plan.Root.Children))
	}
}

// A result with no line is no plan.
func TestBuildPlanRefusesAnEmptyResult(t *testing.T) {
	if _, built := BuildPlan(db.QueryResult{}); built {
		t.Error("an empty result read as a plan")
	}
}

// The client asks for the plan with the parts of it the pane shows.
func TestBuildExplainStatementAsksForTheIndexes(t *testing.T) {
	written := BuildExplainStatement("select 1")
	if !strings.HasPrefix(written, "explain indexes = 1 ") {
		t.Errorf("the plan is asked for as %q", written)
	}
}
