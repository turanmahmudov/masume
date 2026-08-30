package tidb

import "testing"

// TiDB numbers the depth of an operator by the marks in front of its id, so the shape of the
// plan comes out of the text rather than out of a column.
func TestCountPlanDepthReadsTheMarksInFrontOfAnId(t *testing.T) {
	for _, held := range []struct {
		id    string
		depth int
	}{
		{"HashJoin_12", 0},
		{"└─TableReader_20", 1},
		{"  └─Selection_19", 2},
		{"    └─TableFullScan_18", 3},
		{"├─IndexReader_30", 1},
		{"", 0},
	} {
		if answered := CountPlanDepth(held.id); answered != held.depth {
			t.Errorf("%q sits at depth %d, wanted %d", held.id, answered, held.depth)
		}
	}
}

// The time an operator took is inside the text the server writes about how it ran, and the
// pane marks the slowest step from it.
func TestReadSpentMsReadsTheTimeOutOfTheExecutionInfo(t *testing.T) {
	for _, held := range []struct {
		name string
		info string
		want float64
		is   bool
	}{
		{"milliseconds", "time:12.5ms, loops:2", 12.5, true},
		{"microseconds", "time:800µs, loops:1", 0.8, true},
		{"seconds", "time:1.5s, loops:1", 1500, true},
		{"whole milliseconds", "time:3ms", 3, true},

		{"nothing about time", "loops:2, rows:5", 0, false},
		{"nothing at all", "", 0, false},
		{"a time that is not a number", "time:abc", 0, false},
	} {
		t.Run(held.name, func(t *testing.T) {
			answered, is := ReadSpentMs(held.info)
			if is != held.is {
				t.Fatalf("%q read = %v, wanted %v", held.info, is, held.is)
			}
			if is && answered != held.want {
				t.Errorf("%q reads %v ms, wanted %v", held.info, answered, held.want)
			}
		})
	}
}

// A plan the server printed reads into the tree the pane draws, with the deeper operators
// under the ones that call them.
func TestReadPlanBuildsTheTreeFromTheRows(t *testing.T) {
	rows := []map[string]any{
		{"id": "HashJoin_12", "estRows": "3.00", "task": "root",
			"operator info": "inner join", "execution info": "time:20ms, loops:1",
			"actRows": "3"},
		{"id": "├─TableReader_20", "estRows": "3.00", "task": "root",
			"operator info": "data:Selection_19", "execution info": "time:5ms, loops:1",
			"actRows": "3"},
		{"id": "└─IndexReader_30", "estRows": "4.00", "task": "root",
			"operator info": "index:IndexScan_29", "execution info": "time:15ms, loops:1",
			"actRows": "4"},
	}
	order := []string{"id", "estRows", "actRows", "task", "operator info", "execution info"}

	plan, is := ReadPlan(rows, order, true)
	if !is {
		t.Fatal("a plan of the server was not read")
	}
	if plan.Root.Label == "" {
		t.Error("the top of the plan carries no label")
	}
	if len(plan.Root.Children) != 2 {
		t.Fatalf("the top holds %d operators under it, wanted 2", len(plan.Root.Children))
	}
	if !plan.Analyzed {
		t.Error("a plan that ran does not read as one")
	}
	if !plan.Measurable {
		t.Error("the server measures each operator and the plan says it does not")
	}
	if plan.Raw == "" {
		t.Error("the plan does not keep the rows the server printed")
	}
	// The time of the whole run is the time of the top operator.
	if !plan.HasExecutionMs || plan.ExecutionMs != 20 {
		t.Errorf("the run reads %v ms, wanted the 20 of the top operator", plan.ExecutionMs)
	}
}

// A statement the server planned nothing for answers no plan, so the pane says so rather than
// drawing an empty tree.
func TestReadPlanRefusesAnEmptyAnswer(t *testing.T) {
	for _, rows := range [][]map[string]any{nil, {}} {
		if _, is := ReadPlan(rows, []string{"id"}, false); is {
			t.Error("an answer with no rows was read as a plan")
		}
	}
}

// A plan whose first operator is not the shallowest still has to answer one tree, because the
// pane draws from a single root.
func TestReadPlanJoinsSeveralRootsUnderTheFirst(t *testing.T) {
	rows := []map[string]any{
		{"id": "Insert_1", "estRows": "0.00", "task": "root"},
		{"id": "Selection_2", "estRows": "1.00", "task": "root"},
	}
	plan, is := ReadPlan(rows, []string{"id", "estRows", "task"}, false)
	if !is {
		t.Fatal("two operators at the top were not read")
	}
	if len(plan.Root.Children) != 1 {
		t.Errorf("the top holds %d under it, wanted the second operator joined to it",
			len(plan.Root.Children))
	}
}
