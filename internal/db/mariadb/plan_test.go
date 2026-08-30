package mariadb

import "testing"

// MariaDB answers a plan as JSON, and the pane draws it as a tree.
func TestReadPlanReadsTheJsonTheServerAnswers(t *testing.T) {
	const text = `{
	  "query_block": {
	    "select_id": 1,
	    "r_loops": 1,
	    "r_total_time_ms": 12.5,
	    "table": {
	      "table_name": "orders",
	      "access_type": "ALL",
	      "r_loops": 1,
	      "r_rows": 3,
	      "r_table_time_ms": 4.5,
	      "r_other_time_ms": 1.5,
	      "attached_condition": "orders.total > 0"
	    }
	  }
	}`

	plan, is := ReadPlan(text, true)
	if !is {
		t.Fatal("a plan of the server was not read")
	}
	if plan.Root.Label == "" {
		t.Error("the top of the plan carries no label")
	}
	if len(plan.Root.Children) != 1 {
		t.Fatalf("the top holds %d steps under it, wanted the table", len(plan.Root.Children))
	}
	if !plan.Analyzed || !plan.Measurable {
		t.Error("a plan that ran does not read as one that was measured")
	}
	if plan.Raw != text {
		t.Error("the plan does not keep the text the server answered")
	}
	if !plan.HasExecutionMs || plan.ExecutionMs != 12.5 {
		t.Errorf("the run reads %v ms, wanted 12.5", plan.ExecutionMs)
	}

	// The table step is named by the table and by how it was reached.
	table := plan.Root.Children[0]
	if table.Label == "" {
		t.Error("the table step carries no label")
	}
	// The two parts a table step is measured in are added together.
	if !table.HasTotalMs || table.TotalMs != 6 {
		t.Errorf("the table step reads %v ms, wanted the 4.5 and 1.5 added", table.TotalMs)
	}
}

// The optimizer is measured apart from the run, so a reader can tell planning from running.
func TestReadPlanReadsTheTimeOfTheOptimizer(t *testing.T) {
	const text = `{
	  "query_optimization": {"r_total_time_ms": 0.8},
	  "query_block": {"select_id": 1, "r_total_time_ms": 5.0}
	}`

	plan, is := ReadPlan(text, true)
	if !is {
		t.Fatal("the plan was not read")
	}
	if !plan.HasPlanningMs || plan.PlanningMs != 0.8 {
		t.Errorf("the optimizer reads %v ms, wanted 0.8", plan.PlanningMs)
	}
}

func TestReadPlanRefusesWhatIsNotAPlan(t *testing.T) {
	for _, held := range []struct {
		name string
		text string
	}{
		{"nothing", ""},
		{"not JSON", "{not json"},
		{"JSON of another shape", `{"something_else": {}}`},
		{"a list", `[]`},
		{"a block that is not an object", `{"query_block": 7}`},
	} {
		t.Run(held.name, func(t *testing.T) {
			if _, is := ReadPlan(held.text, false); is {
				t.Errorf("%q was read as a plan", held.text)
			}
		})
	}
}

// MariaDB writes a row count for one loop, so the loops are multiplied back in to give the
// rows the step returned in all.
func TestResolveRowCountMultipliesTheLoopsBackIn(t *testing.T) {
	for _, held := range []struct {
		name   string
		source map[string]any
		want   float64
		is     bool
	}{
		{"rows over several loops",
			map[string]any{"r_rows": 3.0, "r_loops": 4.0}, 12, true},
		// A step with no count of loops ran once.
		{"rows with no loops", map[string]any{"r_rows": 3.0}, 3, true},
		{"no rows at all", map[string]any{"r_loops": 4.0}, 0, false},
		{"nothing", map[string]any{}, 0, false},
	} {
		t.Run(held.name, func(t *testing.T) {
			answered, is := resolveRowCount(held.source, "r_rows", "r_loops")
			if is != held.is {
				t.Fatalf("read = %v, wanted %v", is, held.is)
			}
			if is && answered != held.want {
				t.Errorf("the rows read %v, wanted %v", answered, held.want)
			}
		})
	}
}

// A figure JSON cannot hold as a number must not reach the pane as one, because a plan drawn
// from an infinity has no share to divide by.
func TestReadJsonNumberRefusesWhatIsNoFigure(t *testing.T) {
	for _, held := range []struct {
		name   string
		source map[string]any
		is     bool
	}{
		{"a number", map[string]any{"held": 12.5}, true},
		{"a whole number", map[string]any{"held": 3.0}, true},
		{"zero", map[string]any{"held": 0.0}, true},

		{"a key that is not there", map[string]any{}, false},
		{"text", map[string]any{"held": "12.5"}, false},
		{"nothing", map[string]any{"held": nil}, false},
		{"a list", map[string]any{"held": []any{1.0}}, false},
	} {
		t.Run(held.name, func(t *testing.T) {
			if _, is := readJSONNumber(held.source, "held"); is != held.is {
				t.Errorf("read = %v, wanted %v", is, held.is)
			}
		})
	}
}
