package mariadb

import (
	"encoding/json"
	"math"
	"slices"

	"github.com/turanmahmudov/masume/internal/db"
)

// MariaDB returns a plan as JSON, not as lines. A block holds its own numbers, and the
// steps under it as more blocks. The key of a block is its name.

// notAStep names the blocks that measure the whole query, not one step.
var notAStep = map[string]bool{"query_optimization": true, "r_engine_stats": true}

// detailKeys name the keys that describe a step, in reading order.
var detailKeys = []string{
	"attached_condition", "sort_key", "message", "key", "index_condition",
}

func readJSONNumber(source map[string]any, key string) (float64, bool) {
	value, present := source[key]
	if !present {
		return 0, false
	}
	number, isNumber := value.(float64)
	if !isNumber || math.IsInf(number, 0) || math.IsNaN(number) {
		return 0, false
	}
	return number, true
}

// resolveRowCount multiplies the loops back in, because MariaDB writes a row count per
// loop.
func resolveRowCount(source map[string]any, rowsKey, loopsKey string) (float64, bool) {
	rows, present := readJSONNumber(source, rowsKey)
	if !present {
		return 0, false
	}
	loops, hasLoops := readJSONNumber(source, loopsKey)
	if !hasLoops {
		loops = 1
	}
	return rows * loops, true
}

// buildMariadbLabel names a table step by the table and by the access method.
func buildMariadbLabel(key string, source map[string]any) string {
	name, isText := source["table_name"].(string)
	if !isText {
		return key
	}
	access, hasAccess := source["access_type"].(string)
	if hasAccess {
		return name + " (" + access + ")"
	}
	return name
}

func buildMariadbDetail(source map[string]any) string {
	for _, key := range detailKeys {
		value, isText := source[key].(string)
		if !isText || value == "" {
			continue
		}
		if key == "message" {
			return value
		}
		return key + ": " + value
	}
	return ""
}

// resolveMeasuredRows returns the rows a step returned. A sorting block counts them its
// own way.
func resolveMeasuredRows(source map[string]any) (float64, bool) {
	if rows, present := resolveRowCount(source, "r_rows", "r_loops"); present {
		return rows, true
	}
	return readJSONNumber(source, "r_output_rows")
}

// resolveMariadbTotalMs adds the two parts a table step is measured in.
func resolveMariadbTotalMs(source map[string]any) (float64, bool) {
	if total, present := readJSONNumber(source, "r_total_time_ms"); present {
		return total, true
	}
	inTable, hasTable := readJSONNumber(source, "r_table_time_ms")
	beside, hasBeside := readJSONNumber(source, "r_other_time_ms")
	if !hasTable && !hasBeside {
		return 0, false
	}
	return inTable + beside, true
}

// closeMariadbNode builds one node. The server measures each block alone, so nothing is
// subtracted.
func closeMariadbNode(
	label, detail string, source map[string]any, children []db.PlanNode,
) db.PlanNode {
	node := db.PlanNode{Label: label, Detail: detail, Children: children}
	if source == nil {
		return node
	}
	if total, present := resolveMariadbTotalMs(source); present {
		node.TotalMs = total
		node.HasTotalMs = true
		node.SelfMs = total
		node.HasSelfMs = true
	}
	if rows, present := resolveRowCount(source, "rows", "loops"); present {
		node.EstimatedRows = rows
		node.HasEstimatedRows = true
	}
	if rows, present := resolveMeasuredRows(source); present {
		node.ActualRows = rows
		node.HasActualRows = true
	}
	return node
}

// readMariadbSteps reads the blocks under one block. A block under its own key is one
// step. A list, such as the tables of a join, is one step per entry.
func readMariadbSteps(source map[string]any) []db.PlanNode {
	steps := []db.PlanNode{}
	keys := make([]string, 0, len(source))
	for key := range source {
		keys = append(keys, key)
	}
	slices.Sort(keys)

	for _, key := range keys {
		if notAStep[key] {
			continue
		}
		value := source[key]
		if block, isBlock := value.(map[string]any); isBlock {
			steps = append(steps, readMariadbNode(key, block))
			continue
		}
		entries, isList := value.([]any)
		if !isList {
			continue
		}
		children := []db.PlanNode{}
		for _, entry := range entries {
			if block, isBlock := entry.(map[string]any); isBlock {
				children = append(children, readMariadbSteps(block)...)
			}
		}
		if len(children) > 0 {
			steps = append(steps, closeMariadbNode(key, "", nil, children))
		}
	}
	return steps
}

func readMariadbNode(key string, source map[string]any) db.PlanNode {
	return closeMariadbNode(buildMariadbLabel(key, source), buildMariadbDetail(source),
		source, readMariadbSteps(source))
}

func sumSpentUnder(node db.PlanNode) float64 {
	total := 0.0
	for _, child := range node.Children {
		total += child.TotalMs + sumSpentUnder(child)
	}
	return total
}

// closeTopBlock returns the own time of the top block, which is measured as a whole, so
// its own time is what the steps below did not take.
func closeTopBlock(root db.PlanNode) db.PlanNode {
	if !root.HasTotalMs {
		return root
	}
	root.SelfMs = math.Max(0, root.TotalMs-sumSpentUnder(root))
	root.HasSelfMs = true
	return root
}

// ReadPlan reads the plan MariaDB wrote into one cell.
func ReadPlan(text string, analyzed bool) (db.QueryPlan, bool) {
	parsed := map[string]any{}
	if err := json.Unmarshal([]byte(text), &parsed); err != nil {
		return db.QueryPlan{}, false
	}
	block, isBlock := parsed["query_block"].(map[string]any)
	if !isBlock {
		return db.QueryPlan{}, false
	}

	root := closeTopBlock(readMariadbNode("query_block", block))
	plan := db.QueryPlan{
		Root: root, Raw: text, Analyzed: analyzed, Measurable: true,
		ExecutionMs: root.TotalMs, HasExecutionMs: root.HasTotalMs,
	}
	// The server measures the optimizer apart from the run.
	if optimization, hasOptimization := parsed["query_optimization"].(map[string]any); hasOptimization {
		if planning, present := readJSONNumber(optimization, "r_total_time_ms"); present {
			plan.PlanningMs = planning
			plan.HasPlanningMs = true
		}
	}
	return plan, true
}
