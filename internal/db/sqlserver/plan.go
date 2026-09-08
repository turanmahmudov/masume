package sqlserver

import (
	"strconv"
	"strings"

	"github.com/turanmahmudov/masume/internal/core"
	"github.com/turanmahmudov/masume/internal/db"
	"github.com/turanmahmudov/masume/internal/query"
)

// planSetting returns the setting that makes the server write the plan. SHOWPLAN_ALL
// estimates the plan and runs nothing. STATISTICS PROFILE runs the statement and counts the
// rows of every step.
func planSetting(analyze bool) string {
	if analyze {
		return "statistics profile"
	}
	return "showplan_all"
}

// planColumns are the columns of a plan row this client reads.
const (
	columnStatementText = "stmttext"
	columnNodeID        = "nodeid"
	columnParent        = "parent"
	columnPhysicalOp    = "physicalop"
	columnArgument      = "argument"
	columnEstimateRows  = "estimaterows"
	columnExecutions    = "estimateexecutions"
	columnType          = "type"
	columnRows          = "rows"
)

// planRoot is the parent of the row that opens a plan.
const planRoot = 0

// planStep is one row of the plan the server wrote.
type planStep struct {
	id     int64
	parent int64
	node   query.PlanNode
}

// BuildPlan reads the plan rows of the server into the tree the pane draws.
func BuildPlan(answered db.QueryResult, analyzed bool) (query.QueryPlan, bool) {
	named := readNamedPlanRows(answered)
	if len(named) == 0 {
		return query.QueryPlan{}, false
	}

	steps := make([]planStep, 0, len(named))
	children := map[int64][]int{}
	for _, row := range named {
		step := planStep{
			id:     db.ReadNonNegativeCount(row[columnNodeID]),
			parent: db.ReadNonNegativeCount(row[columnParent]),
			node:   buildPlanNode(row),
		}
		children[step.parent] = append(children[step.parent], len(steps))
		steps = append(steps, step)
	}

	plan := query.QueryPlan{
		Raw: writePlanText(named), Analyzed: analyzed, Measurable: true,
	}
	roots := children[planRoot]
	if len(roots) == 0 {
		return query.QueryPlan{}, false
	}
	plan.Root = buildPlanBranch(steps, children, roots[0], map[int]bool{})
	return plan, true
}

// buildPlanBranch builds one step with every step under it.
func buildPlanBranch(
	steps []planStep, children map[int64][]int, at int, taken map[int]bool,
) query.PlanNode {
	taken[at] = true
	node := steps[at].node
	for _, child := range children[steps[at].id] {
		if taken[child] {
			continue
		}
		node.Children = append(node.Children, buildPlanBranch(steps, children, child, taken))
	}
	return node
}

// buildPlanNode reads one step of the plan. The row that opens a plan names the kind of
// statement instead of an operator.
func buildPlanNode(row map[string]any) query.PlanNode {
	label := db.ReadAnyText(row[columnPhysicalOp])
	if label == "" {
		label = db.ReadAnyText(row[columnType])
	}
	node := query.PlanNode{
		Label: strings.TrimSpace(label), Detail: db.ReadAnyText(row[columnArgument]),
	}

	loops := 1.0
	if held, read := readPlanNumber(row[columnExecutions]); read && held > 0 {
		loops = held
	}
	if held, read := readPlanNumber(row[columnEstimateRows]); read {
		node.EstimatedRows = held * loops
		node.HasEstimatedRows = true
	}
	if held, read := readPlanNumber(row[columnRows]); read {
		node.ActualRows = held
		node.HasActualRows = true
	}
	return node
}

// readPlanNumber reads a measurement of a plan row, which the driver hands over as a number
// or as the text of one.
func readPlanNumber(value any) (float64, bool) {
	switch held := value.(type) {
	case nil:
		return 0, false
	case float64:
		return held, true
	case float32:
		return float64(held), true
	case int64:
		return float64(held), true
	case int32:
		return float64(held), true
	}
	read, err := strconv.ParseFloat(strings.TrimSpace(db.ReadAnyText(value)), 64)
	if err != nil {
		return 0, false
	}
	return read, true
}

// readNamedPlanRows returns the plan rows keyed by the lower-case name of each column. A row
// that names no step belongs to the result of the statement, not to the plan.
func readNamedPlanRows(answered db.QueryResult) []map[string]any {
	order := make([]string, 0, len(answered.Columns))
	for _, column := range answered.Columns {
		order = append(order, strings.ToLower(column.Name))
	}
	rows := make([]map[string]any, 0, len(answered.Rows))
	for _, row := range answered.Rows {
		named := map[string]any{}
		for at, name := range order {
			if at < len(row) {
				named[name] = row[at]
			}
		}
		if named[columnNodeID] == nil {
			continue
		}
		rows = append(rows, named)
	}
	return rows
}

// writePlanText writes the plan the way the server printed it, one step per line.
func writePlanText(rows []map[string]any) string {
	lines := make([]string, 0, len(rows))
	for _, row := range rows {
		lines = append(lines, core.FormatCell(row[columnStatementText], ""))
	}
	return strings.Join(lines, "\n")
}
