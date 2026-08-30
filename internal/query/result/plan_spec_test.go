package result_test

import (
	"strings"
	"testing"

	"github.com/turanmahmudov/masume/internal/query"
	"github.com/turanmahmudov/masume/internal/query/result"
)

// The plan a server prints is text, and the pane draws it as a tree. The indentation is what
// says which node sits under which, so a plan read flat would lose the shape of the query.
func TestParseTextPlanReadsTheTreeFromTheIndentation(t *testing.T) {
	const text = `Hash Join  (cost=1.09..2.18 rows=3 width=68)
  Hash Cond: (o.customer_id = c.id)
  ->  Seq Scan on orders o  (cost=0.00..1.03 rows=3 width=36)
  ->  Hash  (cost=1.04..1.04 rows=4 width=36)
        ->  Seq Scan on customers c  (cost=0.00..1.04 rows=4 width=36)`

	plan, is := result.ParseTextPlan(text, false, true)
	if !is {
		t.Fatal("a plan of a server was not read")
	}
	if plan.Root.Label == "" {
		t.Error("the top of the plan carries no label")
	}
	if len(plan.Root.Children) != 2 {
		t.Fatalf("the top holds %d nodes under it, wanted 2", len(plan.Root.Children))
	}
	// The deeper scan sits under the hash, not beside it.
	hash := plan.Root.Children[1]
	if len(hash.Children) != 1 {
		t.Errorf("the hash holds %d nodes under it, wanted 1", len(hash.Children))
	}
	if plan.Raw != text {
		t.Error("the plan does not keep the text the server printed")
	}
}

func TestParseTextPlanRefusesWhatIsNotAPlan(t *testing.T) {
	for _, text := range []string{"", "   ", "\n\n"} {
		if _, is := result.ParseTextPlan(text, false, true); is {
			t.Errorf("%q was read as a plan", text)
		}
	}
}

// A plan that ran carries the time each node took, and the pane marks the slowest so a reader
// sees where the query spent itself.
func TestFlattenPlanMarksTheSlowestNodeAndItsShare(t *testing.T) {
	plan := query.QueryPlan{
		Analyzed: true, Measurable: true,
		Root: query.PlanNode{
			Label: "Hash Join", TotalMs: 100, HasTotalMs: true, SelfMs: 10, HasSelfMs: true,
			Children: []query.PlanNode{
				{Label: "Seq Scan on orders", TotalMs: 20, HasTotalMs: true,
					SelfMs: 20, HasSelfMs: true},
				{Label: "Seq Scan on customers", TotalMs: 70, HasTotalMs: true,
					SelfMs: 70, HasSelfMs: true},
			},
		},
	}

	rows := result.FlattenPlan(plan)
	if len(rows) != 3 {
		t.Fatalf("the plan flattened into %d rows, wanted 3", len(rows))
	}

	// The rows come out in the order the pane draws them, deepening as they go.
	if rows[0].Depth != 0 || rows[1].Depth != 1 || rows[2].Depth != 1 {
		t.Errorf("the depths read %d, %d, %d", rows[0].Depth, rows[1].Depth, rows[2].Depth)
	}

	slowest := 0
	for at, row := range rows {
		if row.Slowest {
			slowest++
			if row.Node.Label != "Seq Scan on customers" {
				t.Errorf("row %d is marked slowest and reads %q", at, row.Node.Label)
			}
		}
	}
	if slowest != 1 {
		t.Errorf("%d rows are marked slowest, wanted one", slowest)
	}

	// The share of the node that took seventy of a hundred milliseconds is the largest.
	for _, row := range rows {
		if row.Share < 0 || row.Share > 1 {
			t.Errorf("%q holds a share of %v, wanted it between zero and one",
				row.Node.Label, row.Share)
		}
	}
}

// The planner guessing ten times wrong is what a reader looks for, because it points at a
// statistic that needs refreshing.
func TestFlattenPlanMarksANodeThePlannerGuessedWrong(t *testing.T) {
	plan := query.QueryPlan{
		Analyzed: true, Measurable: true,
		Root: query.PlanNode{
			Label:         "Seq Scan on orders",
			EstimatedRows: 10, HasEstimatedRows: true,
			ActualRows: 5000, HasActualRows: true,
			TotalMs: 10, HasTotalMs: true, SelfMs: 10, HasSelfMs: true,
		},
	}

	rows := result.FlattenPlan(plan)
	if len(rows) != 1 {
		t.Fatalf("the plan flattened into %d rows", len(rows))
	}
	if !rows[0].Misestimated {
		t.Error("a node the planner guessed five hundred times wrong is not marked")
	}
}

// A plan that only estimated marks nothing, because there is no actual count to compare and a
// mark would be a guess about a guess.
func TestFlattenPlanMarksNothingWhereNothingRan(t *testing.T) {
	plan := query.QueryPlan{
		Root: query.PlanNode{
			Label: "Seq Scan on orders", EstimatedRows: 10, HasEstimatedRows: true,
		},
	}
	for _, row := range result.FlattenPlan(plan) {
		if row.Misestimated {
			t.Error("a plan that only estimated marks a node as guessed wrong")
		}
		if row.Slowest {
			t.Error("a plan that only estimated marks a slowest node")
		}
	}
}

// One row per node, whatever the shape, because the pane draws the rows and a node without one
// would be invisible. The top of a plan is always a node, so an empty plan is one row.
func TestFlattenPlanAnswersOneRowPerNode(t *testing.T) {
	deep := query.QueryPlan{Root: query.PlanNode{
		Label: "a", Children: []query.PlanNode{
			{Label: "b", Children: []query.PlanNode{{Label: "c"}}},
			{Label: "d"},
		},
	}}
	if rows := result.FlattenPlan(deep); len(rows) != 4 {
		t.Errorf("four nodes flattened into %d rows", len(rows))
	}
	if rows := result.FlattenPlan(query.QueryPlan{}); len(rows) != 1 {
		t.Errorf("the top of a plan flattened into %d rows, wanted the one node", len(rows))
	}
}

// The cost line under the plan says what the run took, and says nothing where the server
// measured nothing.
func TestDescribePlanCostSaysWhatTheRunTook(t *testing.T) {
	measured := result.DescribePlanCost(query.QueryPlan{
		Analyzed: true, Measurable: true,
		ExecutionMs: 12.5, HasExecutionMs: true,
		PlanningMs: 1.5, HasPlanningMs: true,
		Root: query.PlanNode{Label: "Seq Scan"},
	})
	if measured == "" {
		t.Error("a plan that ran is described as nothing")
	}

	// A plan that measured nothing says so, rather than inventing a figure or reading as a
	// run that took no time at all.
	held := result.DescribePlanCost(query.QueryPlan{})
	if held == "" {
		t.Error("a plan that measured nothing is described as nothing")
	}
	if strings.ContainsAny(held, "0123456789") {
		t.Errorf("a plan that measured nothing is described as %q, which names a figure", held)
	}
}

func TestParseTextPlanReadsTheTimesTheServerReportsAtTheEnd(t *testing.T) {
	const text = `Hash Join  (cost=1.00..2.00 rows=5 width=8) (actual time=0.100..0.200 rows=5 loops=1)
  Hash Cond: (a.id = b.id)
  ->  Seq Scan on a  (cost=0.00..1.00 rows=10 width=4) (actual time=0.010..0.020 rows=10 loops=1)
Planning Time: 0.150 ms
Execution Time: 0.400 ms`

	plan, is := result.ParseTextPlan(text, true, true)
	if !is {
		t.Fatal("a plan of a server was not read")
	}
	if !plan.HasPlanningMs || plan.PlanningMs != 0.150 {
		t.Errorf("PlanningMs = %v (present %v), want 0.150", plan.PlanningMs, plan.HasPlanningMs)
	}
	if !plan.HasExecutionMs || plan.ExecutionMs != 0.400 {
		t.Errorf("ExecutionMs = %v (present %v), want 0.400", plan.ExecutionMs, plan.HasExecutionMs)
	}
}

func TestParseTextPlanLeavesATimeItCannotReadUnset(t *testing.T) {
	// The line is still a summary line, so it never becomes a node of the tree.
	const text = `Seq Scan on t  (cost=0.00..1.00 rows=10 width=4)
Planning Time: not a number ms`

	plan, is := result.ParseTextPlan(text, true, true)
	if !is {
		t.Fatal("a plan of a server was not read")
	}
	if plan.HasPlanningMs {
		t.Errorf("PlanningMs was read as %v", plan.PlanningMs)
	}
	if len(plan.Root.Children) != 0 {
		t.Errorf("the summary line became %d nodes", len(plan.Root.Children))
	}
}

func TestParseTextPlanMultipliesTheNumbersOfANodeByItsLoops(t *testing.T) {
	// A server reports the rows and the time of one loop, so a node run many times
	// reports far less than it did.
	const text = `Nested Loop  (cost=0.00..2.00 rows=10 width=4) (actual time=0.100..0.200 rows=10 loops=1)
  ->  Index Scan on t  (cost=0.00..1.00 rows=3 width=4) (actual time=0.010..0.050 rows=3 loops=4)`

	plan, is := result.ParseTextPlan(text, true, true)
	if !is {
		t.Fatal("a plan of a server was not read")
	}
	inner := plan.Root.Children[0]
	if inner.EstimatedRows != 12 {
		t.Errorf("EstimatedRows = %v, want 12", inner.EstimatedRows)
	}
	if inner.ActualRows != 12 {
		t.Errorf("ActualRows = %v, want 12", inner.ActualRows)
	}
	if inner.TotalMs != 0.200 {
		t.Errorf("TotalMs = %v, want 0.2", inner.TotalMs)
	}
}

func TestParseTextPlanReadsANodeThatNeverRan(t *testing.T) {
	const text = `-> Table scan on t  (cost=1.00 rows=10) (actual time=0.1..0.2 rows=10 loops=1)
    -> Filter: (t.a = 1)  (cost=0.5 rows=1) (never executed)`

	plan, is := result.ParseTextPlan(text, true, true)
	if !is {
		t.Fatal("a plan of a server was not read")
	}
	filter := plan.Root.Children[0]
	if !filter.HasActualRows || filter.ActualRows != 0 {
		t.Errorf("ActualRows = %v (present %v), want 0", filter.ActualRows, filter.HasActualRows)
	}
	if filter.HasTotalMs {
		t.Errorf("a node that never ran reported %v ms", filter.TotalMs)
	}
	if filter.Label != "Filter" || filter.Detail != "(t.a = 1)" {
		t.Errorf("label %q detail %q, want the text after the colon as the detail",
			filter.Label, filter.Detail)
	}
}

func TestParseTextPlanJoinsTheTreesAServerWroteApart(t *testing.T) {
	// MySQL writes one tree per subquery. Each one after the first hangs under the root,
	// so the pane draws one tree and not several.
	const text = `-> Table scan on t  (cost=1.00 rows=10)
-> Materialize  (cost=2.00 rows=5)
    -> Table scan on u  (cost=1.00 rows=5)`

	plan, is := result.ParseTextPlan(text, false, true)
	if !is {
		t.Fatal("a plan of a server was not read")
	}
	if plan.Root.Label != "Table scan on t" {
		t.Errorf("the root is %q, want the first tree", plan.Root.Label)
	}
	if len(plan.Root.Children) != 1 {
		t.Fatalf("the root holds %d nodes, want the second tree under it",
			len(plan.Root.Children))
	}
	if plan.Root.Children[0].Label != "Materialize" {
		t.Errorf("the node under the root is %q", plan.Root.Children[0].Label)
	}
}

func TestBuildDetailRanksTheConditionsAndTakesTheFirst(t *testing.T) {
	// A node can carry several conditions. The pane has one line for it, so the ranking
	// decides which one the reader sees.
	for _, held := range []struct {
		name       string
		properties string
		want       string
	}{
		{"an index condition wins over a filter",
			"  Index Cond: (id = 1)\n  Filter: (a = 2)", "index cond: (id = 1)"},
		{"a hash condition wins over a filter",
			"  Hash Cond: (a.id = b.id)\n  Filter: (a = 2)", "hash cond: (a.id = b.id)"},
		{"a filter alone", "  Filter: (a = 2)", "filter: (a = 2)"},
		{"a sort key", "  Sort Key: a, b", "sort key: a, b"},
		{"a property that is no condition", "  Buffers: shared hit=3", ""},
		{"no property at all", "", ""},
	} {
		t.Run(held.name, func(t *testing.T) {
			text := "Seq Scan on t  (cost=0.00..1.00 rows=10 width=4)"
			if held.properties != "" {
				text += "\n" + held.properties
			}
			plan, is := result.ParseTextPlan(text, false, true)
			if !is {
				t.Fatal("a plan of a server was not read")
			}
			if plan.Root.Detail != held.want {
				t.Errorf("Detail = %q, want %q", plan.Root.Detail, held.want)
			}
		})
	}
}

func TestParseTextPlanPrefersTheDetailOnTheNodeLine(t *testing.T) {
	// MySQL writes the detail after a colon on the node line itself, and that wins over
	// a condition written under it.
	const text = `-> Filter: (t.a = 1)  (cost=0.5 rows=1)
    Index Cond: (id = 1)`
	plan, is := result.ParseTextPlan(text, false, true)
	if !is {
		t.Fatal("a plan of a server was not read")
	}
	if plan.Root.Detail != "(t.a = 1)" {
		t.Errorf("Detail = %q, want the text on the node line", plan.Root.Detail)
	}
}

func TestParseTextPlanLeavesANumberItCannotReadUnset(t *testing.T) {
	const text = `Seq Scan on t  (cost=0.00..1.00 rows=lots width=4)`
	plan, is := result.ParseTextPlan(text, false, true)
	if !is {
		t.Fatal("a plan of a server was not read")
	}
	if plan.Root.HasEstimatedRows {
		t.Errorf("EstimatedRows was read as %v", plan.Root.EstimatedRows)
	}
}

func TestFlattenPlanWalksTheWholeTreeInTheOrderItDraws(t *testing.T) {
	const text = `Hash Join  (cost=1.00..2.00 rows=5 width=8)
  ->  Seq Scan on a  (cost=0.00..1.00 rows=10 width=4)
  ->  Hash  (cost=1.00..1.00 rows=5 width=4)
        ->  Seq Scan on b  (cost=0.00..1.00 rows=5 width=4)`

	plan, is := result.ParseTextPlan(text, false, true)
	if !is {
		t.Fatal("a plan of a server was not read")
	}
	rows := result.FlattenPlan(plan)
	want := []struct {
		label string
		depth int
	}{
		{"Hash Join", 0},
		{"Seq Scan on a", 1},
		{"Hash", 1},
		{"Seq Scan on b", 2},
	}
	if len(rows) != len(want) {
		t.Fatalf("got %d rows, want %d", len(rows), len(want))
	}
	for at, row := range rows {
		if row.Node.Label != want[at].label || row.Depth != want[at].depth {
			t.Errorf("row %d = %q at depth %d, want %q at depth %d",
				at, row.Node.Label, row.Depth, want[at].label, want[at].depth)
		}
	}
}

func TestFlattenPlanAnswersOneRowForATreeOfOneNode(t *testing.T) {
	plan, is := result.ParseTextPlan("Seq Scan on t  (cost=0.00..1.00 rows=10 width=4)",
		false, true)
	if !is {
		t.Fatal("a plan of a server was not read")
	}
	if rows := result.FlattenPlan(plan); len(rows) != 1 || rows[0].Depth != 0 {
		t.Errorf("got %d rows", len(rows))
	}
}
