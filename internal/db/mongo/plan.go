package mongo

import (
	"context"
	"fmt"
	"strings"

	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/turanmahmudov/masume/internal/db"
)

// The plan of a statement. The server explains a read only, and returns a tree of
// stages: the planner alone, or the planner with what each stage did when it ran.

// planDetailFields are the fields of a stage that say what it did, in the order the
// detail names them. Every other field is a counter the tree already shows.
var planDetailFields = []string{
	"indexName", "keyPattern", "direction", "sortPattern", "filter", "indexBounds",
	"transformBy", "keysExamined", "docsExamined",
}

// ExplainQuery returns the plan of a read. A write is refused, because the server plans
// no statement that changes a document.
func (session *mongoSession) ExplainQuery(
	ctx context.Context, written string, analyze bool,
) (db.QueryPlan, error) {
	parsed, err := ReadStatement(written)
	if err != nil {
		return db.QueryPlan{}, db.WrapDatabaseError(err)
	}
	command, buildErr := buildExplainCommand(parsed, analyze)
	if buildErr != nil {
		return db.QueryPlan{}, db.WrapDatabaseError(buildErr)
	}

	var reply bson.D
	if decodeErr := session.readDatabase(parsed.Database).
		RunCommand(ctx, command).Decode(&reply); decodeErr != nil {
		return db.QueryPlan{}, db.WrapDatabaseError(decodeErr)
	}
	return ReadExplainReply(reply, analyze)
}

// verbosityOf returns how much the server is asked to report.
func verbosityOf(analyze bool) string {
	if analyze {
		return "executionStats"
	}
	return "queryPlanner"
}

// buildExplainCommand writes the command that asks the server to explain this statement.
func buildExplainCommand(parsed Statement, analyze bool) (bson.D, error) {
	if parsed.Collection == "" {
		return nil, newSyntaxError("only a read of a collection is planned")
	}
	inner, err := buildExplainedCall(parsed)
	if err != nil {
		return nil, err
	}
	return bson.D{
		{Key: "explain", Value: inner},
		{Key: "verbosity", Value: verbosityOf(analyze)},
	}, nil
}

// buildExplainedCall writes the call the explain covers, as the command form of it.
func buildExplainedCall(parsed Statement) (bson.D, error) {
	call := parsed.Calls[0]
	switch call.Name {
	case "find", "findOne":
		plan, err := buildFindPlan(parsed)
		if err != nil {
			return nil, err
		}
		return buildExplainedFind(parsed.Collection, plan), nil
	case "aggregate":
		pipeline, err := ReadArray(call.ReadArgument(0))
		if err != nil {
			return nil, err
		}
		return bson.D{
			{Key: "aggregate", Value: parsed.Collection},
			{Key: "pipeline", Value: pipeline},
			{Key: "cursor", Value: bson.D{}},
		}, nil
	case "count", "countDocuments":
		filter, err := ReadDocument(call.ReadArgument(0))
		if err != nil {
			return nil, err
		}
		return bson.D{
			{Key: "count", Value: parsed.Collection}, {Key: "query", Value: filter},
		}, nil
	case "distinct":
		field, err := ReadText(call.ReadArgument(0))
		if err != nil {
			return nil, err
		}
		filter, filterErr := ReadDocument(call.ReadArgument(1))
		if filterErr != nil {
			return nil, filterErr
		}
		return bson.D{
			{Key: "distinct", Value: parsed.Collection},
			{Key: "key", Value: field}, {Key: "query", Value: filter},
		}, nil
	}
	return nil, newSyntaxError("the server does not plan " + call.Name)
}

// buildExplainedFind writes a find as the command form the explain takes.
func buildExplainedFind(collection string, plan findPlan) bson.D {
	command := bson.D{
		{Key: "find", Value: collection}, {Key: "filter", Value: plan.filter},
	}
	if plan.sort != nil {
		command = append(command, bson.E{Key: "sort", Value: plan.sort})
	}
	if plan.projection != nil {
		command = append(command, bson.E{Key: "projection", Value: plan.projection})
	}
	if plan.skip > 0 {
		command = append(command, bson.E{Key: "skip", Value: plan.skip})
	}
	if plan.hasLimit {
		command = append(command, bson.E{Key: "limit", Value: plan.limit})
	}
	return command
}

// ReadExplainReply reads the reply of an explain as the plan the pane draws.
func ReadExplainReply(reply bson.D, analyze bool) (db.QueryPlan, error) {
	plan := db.QueryPlan{
		Raw: writeIndentedJSON(reply), Analyzed: analyze, Measurable: true,
	}

	stats, hasStats := findField(reply, "executionStats")
	if analyze && hasStats {
		if held, isDocument := stats.(bson.D); isDocument {
			if millis, reported := findField(held, "executionTimeMillis"); reported {
				plan.ExecutionMs = readPlanNumber(millis)
				plan.HasExecutionMs = true
			}
			if stages, reported := findField(held, "executionStages"); reported {
				plan.Root = readPlanNode(stages)
				return plan, nil
			}
		}
	}

	if stages, reported := findField(reply, "stages"); reported {
		plan.Root = readPipelineStages(stages)
		return plan, nil
	}
	planner, reported := findField(reply, "queryPlanner")
	if !reported {
		return db.QueryPlan{}, db.FailUnreadablePlan()
	}
	held, isDocument := planner.(bson.D)
	if !isDocument {
		return db.QueryPlan{}, db.FailUnreadablePlan()
	}
	winning, hasWinning := findField(held, "winningPlan")
	if !hasWinning {
		return db.QueryPlan{}, db.FailUnreadablePlan()
	}
	plan.Root = readPlanNode(winning)
	return plan, nil
}

// readPipelineStages reads the stages of an aggregation, which the server returns as a
// list rather than as a tree.
func readPipelineStages(value any) db.PlanNode {
	stages, isList := value.(bson.A)
	if !isList {
		return db.PlanNode{Label: "pipeline"}
	}
	root := db.PlanNode{Label: "pipeline"}
	for _, stage := range stages {
		root.Children = append(root.Children, readPipelineStage(stage))
	}
	return root
}

// readPipelineStage reads one stage of an aggregation. The stage names itself with its
// first field, and a $cursor stage carries the plan of the read under it.
func readPipelineStage(value any) db.PlanNode {
	held, isDocument := value.(bson.D)
	if !isDocument || len(held) == 0 {
		return db.PlanNode{Label: "stage"}
	}
	node := db.PlanNode{Label: held[0].Key}
	readStageTiming(&node, held)

	inner, isInner := held[0].Value.(bson.D)
	if !isInner {
		node.Detail = WriteExtendedJSON(held[0].Value)
		return node
	}
	if planner, reported := findField(inner, "queryPlanner"); reported {
		if plannerDocument, isPlanner := planner.(bson.D); isPlanner {
			if winning, hasWinning := findField(plannerDocument, "winningPlan"); hasWinning {
				node.Children = append(node.Children, readPlanNode(winning))
				return takeChildTime(node)
			}
		}
	}
	if stages, reported := findField(inner, "executionStages"); reported {
		node.Children = append(node.Children, readPlanNode(stages))
		return takeChildTime(node)
	}
	node.Detail = WriteExtendedJSON(held[0].Value)
	return node
}

// takeChildTime leaves a stage with the time it took on its own, which is what is left
// of it after the stages under it.
func takeChildTime(node db.PlanNode) db.PlanNode {
	node.SelfMs = node.TotalMs - sumChildTime(node.Children)
	if node.SelfMs < 0 {
		node.SelfMs = 0
	}
	return node
}

// readStageTiming reads what a stage of an aggregation reported about its run.
func readStageTiming(node *db.PlanNode, stage bson.D) {
	if millis, reported := findField(stage, "executionTimeMillisEstimate"); reported {
		node.TotalMs = readPlanNumber(millis)
		node.HasTotalMs = true
		node.SelfMs = node.TotalMs
		node.HasSelfMs = true
	}
	if returned, reported := findField(stage, "nReturned"); reported {
		node.ActualRows = readPlanNumber(returned)
		node.HasActualRows = true
	}
}

// readPlanNode reads one stage of a plan, and the stages under it.
func readPlanNode(value any) db.PlanNode {
	held, isDocument := value.(bson.D)
	if !isDocument {
		return db.PlanNode{Label: "stage"}
	}
	// A newer server wraps the plan it chose, and the plan itself sits under it.
	if inner, wrapped := findField(held, "queryPlan"); wrapped {
		if document, isInner := inner.(bson.D); isInner {
			held = document
		}
	}

	node := db.PlanNode{Label: readStageName(held), Detail: buildStageDetail(held)}
	readStageTiming(&node, held)

	if inner, reported := findField(held, "inputStage"); reported {
		node.Children = append(node.Children, readPlanNode(inner))
	}
	if inner, reported := findField(held, "inputStages"); reported {
		if list, isList := inner.(bson.A); isList {
			for _, stage := range list {
				node.Children = append(node.Children, readPlanNode(stage))
			}
		}
	}
	if inner, reported := findField(held, "shards"); reported {
		if list, isList := inner.(bson.A); isList {
			for _, shard := range list {
				node.Children = append(node.Children, readPlanNode(shard))
			}
		}
	}

	return takeChildTime(node)
}

// sumChildTime returns how much of the time of a stage its children took.
func sumChildTime(children []db.PlanNode) float64 {
	total := 0.0
	for _, child := range children {
		total += child.TotalMs
	}
	return total
}

// readStageName returns what a stage is called.
func readStageName(stage bson.D) string {
	if name, reported := findField(stage, "stage"); reported {
		if written := db.ReadAnyText(name); written != "" {
			return written
		}
	}
	return "stage"
}

// buildStageDetail writes what a stage says about the way it works.
func buildStageDetail(stage bson.D) string {
	written := []string{}
	for _, name := range planDetailFields {
		value, reported := findField(stage, name)
		if !reported {
			continue
		}
		written = append(written, name+"="+writeDetailValue(value))
	}
	return strings.Join(written, ", ")
}

// writeDetailValue writes one value of a detail in short.
func writeDetailValue(value any) string {
	switch held := value.(type) {
	case string:
		return held
	case bson.D, bson.A:
		return WriteExtendedJSON(held)
	}
	return fmt.Sprintf("%v", value)
}

// findField returns the value of that field, and whether the document holds one.
func findField(document bson.D, name string) (any, bool) {
	for _, field := range document {
		if field.Key == name {
			return field.Value, true
		}
	}
	return nil, false
}

// readPlanNumber reads a number a plan reports, whatever width the server sent.
func readPlanNumber(value any) float64 {
	switch held := value.(type) {
	case int32:
		return float64(held)
	case int64:
		return float64(held)
	case float64:
		return held
	}
	return 0
}

// writeIndentedJSON writes the whole reply, which the plan pane shows as it stands.
func writeIndentedJSON(reply bson.D) string {
	written, err := bson.MarshalExtJSONIndent(reply, false, false, "", "  ")
	if err != nil {
		return WriteExtendedJSON(reply)
	}
	return string(written)
}
