package sqlite

import (
	"strings"

	_ "modernc.org/sqlite"

	"github.com/turanmahmudov/masume/internal/core"
	"github.com/turanmahmudov/masume/internal/db"
)

// sqlitePlanHeader is the header the command line writes over a plan of several steps.
const sqlitePlanHeader = "QUERY PLAN"

// buildSqlitePlan builds the plan from the rows of EXPLAIN QUERY PLAN, each of which
// names its parent. SQLite measures no step, so a node has no number.
func buildSqlitePlan(rows [][]any, measurable bool) db.QueryPlan {
	labels := map[int64]string{}
	parents := map[int64]int64{}
	children := map[int64][]int64{}
	order := []int64{}

	for _, row := range rows {
		if len(row) < 4 {
			continue
		}
		id := db.ReadNonNegativeCount(row[0])
		parent := db.ReadNonNegativeCount(row[1])
		if _, held := labels[id]; !held {
			order = append(order, id)
		}
		labels[id] = core.FormatCell(row[3], "")
		parents[id] = parent
		children[parent] = append(children[parent], id)
	}

	taken := map[int64]bool{}
	var build func(id int64) db.PlanNode
	build = func(id int64) db.PlanNode {
		taken[id] = true
		node := db.PlanNode{Label: labels[id]}
		for _, child := range children[id] {
			if taken[child] {
				continue
			}
			node.Children = append(node.Children, build(child))
		}
		return node
	}

	// A step whose parent is not in this plan is a root.
	roots := []int64{}
	for _, id := range order {
		if _, held := labels[parents[id]]; !held {
			roots = append(roots, id)
		}
	}

	written := make([]string, 0, len(rows))
	for _, row := range rows {
		cells := make([]string, 0, 4)
		for at := 0; at < 4 && at < len(row); at++ {
			cells = append(cells, core.FormatCell(row[at], ""))
		}
		written = append(written, strings.Join(cells, "|"))
	}

	plan := db.QueryPlan{Raw: strings.Join(written, "\n"), Measurable: measurable}
	if len(roots) == 1 {
		plan.Root = build(roots[0])
		return plan
	}
	root := db.PlanNode{Label: sqlitePlanHeader}
	for _, id := range roots {
		root.Children = append(root.Children, build(id))
	}
	plan.Root = root
	return plan
}
