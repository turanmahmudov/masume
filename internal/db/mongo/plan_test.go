package mongo

import (
	"strings"
	"testing"

	"go.mongodb.org/mongo-driver/v2/bson"
)

// The explain of a find carries the sort and the limit, or the plan would be the plan of
// a different read.
func TestBuildExplainCommandWritesTheReadItCovers(t *testing.T) {
	parsed, err := ReadStatement(`db.orders.find({status: "new"}).sort({total: -1}).limit(10)`)
	if err != nil {
		t.Fatalf("the statement answered %v", err)
	}
	command, buildErr := buildExplainCommand(parsed, true)
	if buildErr != nil {
		t.Fatalf("the explain answered %v", buildErr)
	}

	written := WriteExtendedJSON(command)
	for _, wanted := range []string{
		`"explain"`, `"find":"orders"`, `"status":"new"`, `"total":-1`, `"limit":10`,
		`"verbosity":"executionStats"`,
	} {
		if !strings.Contains(written, wanted) {
			t.Errorf("the explain holds no %s:\n%s", wanted, written)
		}
	}
}

// A write is never planned, because the server would have to run it to plan it.
func TestBuildExplainCommandRefusesWhatTheServerDoesNotPlan(t *testing.T) {
	parsed, err := ReadStatement(`db.orders.insertOne({total: 5})`)
	if err != nil {
		t.Fatalf("the statement answered %v", err)
	}
	if _, buildErr := buildExplainCommand(parsed, false); buildErr == nil {
		t.Error("an insert was sent to be planned")
	}
}

// The planner answers the plan it chose as a tree of stages, and a newer server wraps
// that tree in a record of whether it came from the cache.
func TestReadExplainReplyReadsTheTreeThePlannerChose(t *testing.T) {
	reply := bson.D{{Key: "queryPlanner", Value: bson.D{
		{Key: "winningPlan", Value: bson.D{
			{Key: "isCached", Value: false},
			{Key: "queryPlan", Value: bson.D{
				{Key: "stage", Value: "FETCH"},
				{Key: "inputStage", Value: bson.D{
					{Key: "stage", Value: "IXSCAN"},
					{Key: "indexName", Value: "total_1"},
					{Key: "keyPattern", Value: bson.D{{Key: "total", Value: int32(1)}}},
				}},
			}},
		}},
	}}}

	plan, err := ReadExplainReply(reply, false)
	if err != nil {
		t.Fatalf("the plan answered %v", err)
	}
	if plan.Root.Label != "FETCH" {
		t.Errorf("the plan opens at %q, wanted FETCH", plan.Root.Label)
	}
	if len(plan.Root.Children) != 1 {
		t.Fatalf("the plan holds %d stages under it, wanted one", len(plan.Root.Children))
	}
	scan := plan.Root.Children[0]
	if scan.Label != "IXSCAN" {
		t.Errorf("the stage under it is %q, wanted IXSCAN", scan.Label)
	}
	if !strings.Contains(scan.Detail, "total_1") {
		t.Errorf("the index it reads is not named: %q", scan.Detail)
	}
	if plan.Analyzed {
		t.Error("a plan the server only worked out reads as one it measured")
	}
}

// A measured plan reports what each stage returned and how long it took, and the time of
// a stage on its own is what is left after its children.
func TestReadExplainReplyReadsWhatAMeasuredPlanDid(t *testing.T) {
	reply := bson.D{{Key: "executionStats", Value: bson.D{
		{Key: "executionTimeMillis", Value: int32(12)},
		{Key: "executionStages", Value: bson.D{
			{Key: "stage", Value: "FETCH"},
			{Key: "nReturned", Value: int32(3)},
			{Key: "executionTimeMillisEstimate", Value: int32(10)},
			{Key: "inputStage", Value: bson.D{
				{Key: "stage", Value: "IXSCAN"},
				{Key: "nReturned", Value: int32(3)},
				{Key: "executionTimeMillisEstimate", Value: int32(4)},
			}},
		}},
	}}}

	plan, err := ReadExplainReply(reply, true)
	if err != nil {
		t.Fatalf("the plan answered %v", err)
	}
	if !plan.HasExecutionMs || plan.ExecutionMs != 12 {
		t.Errorf("the run took %v ms, wanted 12", plan.ExecutionMs)
	}
	if !plan.Root.HasActualRows || plan.Root.ActualRows != 3 {
		t.Errorf("the plan returned %v rows, wanted 3", plan.Root.ActualRows)
	}
	if plan.Root.SelfMs != 6 {
		t.Errorf("the stage took %v ms of its own, wanted 6", plan.Root.SelfMs)
	}
	if !plan.Analyzed {
		t.Error("a measured plan reads as one the server only worked out")
	}
}

// An aggregation is answered as a list of stages, not as a tree, and the read under the
// first of them carries the plan of the collection.
func TestReadExplainReplyReadsThePipelineOfAnAggregation(t *testing.T) {
	reply := bson.D{{Key: "stages", Value: bson.A{
		bson.D{{Key: "$cursor", Value: bson.D{
			{Key: "queryPlanner", Value: bson.D{
				{Key: "winningPlan", Value: bson.D{{Key: "stage", Value: "COLLSCAN"}}},
			}},
		}}},
		bson.D{{Key: "$group", Value: bson.D{{Key: "_id", Value: "$status"}}}},
	}}}

	plan, err := ReadExplainReply(reply, false)
	if err != nil {
		t.Fatalf("the plan answered %v", err)
	}
	if len(plan.Root.Children) != 2 {
		t.Fatalf("the pipeline holds %d stages, wanted two", len(plan.Root.Children))
	}
	if plan.Root.Children[0].Label != "$cursor" {
		t.Errorf("the first stage is %q", plan.Root.Children[0].Label)
	}
	if len(plan.Root.Children[0].Children) != 1 ||
		plan.Root.Children[0].Children[0].Label != "COLLSCAN" {
		t.Errorf("the read under the first stage was lost: %v", plan.Root.Children[0])
	}
}

// A reply of a shape no reader here knows is reported, rather than drawn as an empty
// plan the user would read as a plan of nothing.
func TestReadExplainReplyReportsAPlanItCannotRead(t *testing.T) {
	if _, err := ReadExplainReply(bson.D{{Key: "ok", Value: 1}}, false); err == nil {
		t.Error("a reply that holds no plan was read as one")
	}
}
