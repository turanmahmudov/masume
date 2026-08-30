package tidb

import (
	"math"
	"regexp"
	"strconv"
	"strings"

	"github.com/turanmahmudov/masume/internal/db"
)

// TiDB returns a plan as one row per operator, not one cell. The tree is drawn in the
// first column, so the characters before an operator give its depth.

// tidbDrawing matches the tree characters before every operator.
var tidbDrawing = regexp.MustCompile(`^[\s│├└─]*`)

// tidbLevelWidth is two characters per level, a branch or a blank.
const tidbLevelWidth = 2

// tidbSpent matches the run time beside an operator. The hours and minutes come before
// the part with the unit.
var tidbSpent = regexp.MustCompile(`\btime:(?:(\d+)h)?(?:(\d+)m)?([\d.]+)(ns|µs|us|ms|s)\b`)

var msPerUnit = map[string]float64{
	"ns": 1e-6, "µs": 1e-3, "us": 1e-3, "ms": 1, "s": 1000,
}

const (
	msPerMinute = 60_000.0
	msPerHour   = 3_600_000.0
)

type openTidbNode struct {
	depth            int
	label            string
	detail           string
	estimatedRows    float64
	hasEstimatedRows bool
	actualRows       float64
	hasActualRows    bool
	totalMs          float64
	hasTotalMs       bool
	children         []db.PlanNode
}

func readTidbCount(row map[string]any, key string) (float64, bool) {
	value, present := row[key]
	if !present {
		return 0, false
	}
	parsed, err := strconv.ParseFloat(strings.TrimSpace(db.ReadAnyText(value)), 64)
	if err != nil {
		return 0, false
	}
	return parsed, true
}

// CountPlanDepth returns the depth of the operator, from the width of the tree before it.
func CountPlanDepth(id string) int {
	return len([]rune(tidbDrawing.FindString(id))) / tidbLevelWidth
}

// ReadSpentMs returns the time beside the operator, in milliseconds.
func ReadSpentMs(executionInfo string) (float64, bool) {
	spent := tidbSpent.FindStringSubmatch(executionInfo)
	if spent == nil {
		return 0, false
	}
	total := 0.0
	if spent[1] != "" {
		hours, _ := strconv.ParseFloat(spent[1], 64)
		total += hours * msPerHour
	}
	if spent[2] != "" {
		minutes, _ := strconv.ParseFloat(spent[2], 64)
		total += minutes * msPerMinute
	}
	value, err := strconv.ParseFloat(spent[3], 64)
	if err != nil {
		return 0, false
	}
	unit, known := msPerUnit[spent[4]]
	if !known {
		unit = 1
	}
	return total + value*unit, true
}

// buildTidbDetail writes where the operator ran and what it ran on, on one line.
func buildTidbDetail(row map[string]any) string {
	task := db.ReadAnyText(row["task"])
	if task == "root" {
		task = ""
	}
	parts := []string{task, db.ReadAnyText(row["access object"]), db.ReadAnyText(row["operator info"])}
	kept := make([]string, 0, len(parts))
	for _, part := range parts {
		if part != "" {
			kept = append(kept, part)
		}
	}
	return strings.Join(kept, " · ")
}

func openTidbPlanNode(row map[string]any) *openTidbNode {
	id := db.ReadAnyText(row["id"])
	node := &openTidbNode{
		depth:  CountPlanDepth(id),
		label:  tidbDrawing.ReplaceAllString(id, ""),
		detail: buildTidbDetail(row),
	}
	node.estimatedRows, node.hasEstimatedRows = readTidbCount(row, "estRows")
	node.actualRows, node.hasActualRows = readTidbCount(row, "actRows")
	node.totalMs, node.hasTotalMs = ReadSpentMs(db.ReadAnyText(row["execution info"]))
	return node
}

// closeTidbNode returns the own time of an operator, which is measured with everything
// under it, so its own time is the rest.
func closeTidbNode(open *openTidbNode) db.PlanNode {
	childMs := 0.0
	for _, child := range open.children {
		childMs += child.TotalMs
	}
	node := db.PlanNode{
		Label: open.label, Detail: open.detail,
		EstimatedRows: open.estimatedRows, HasEstimatedRows: open.hasEstimatedRows,
		ActualRows: open.actualRows, HasActualRows: open.hasActualRows,
		TotalMs: open.totalMs, HasTotalMs: open.hasTotalMs, Children: open.children,
	}
	if open.hasTotalMs {
		node.SelfMs = math.Max(0, open.totalMs-childMs)
		node.HasSelfMs = true
	}
	return node
}

// renderTidbRows writes the rows as text, which the plan pane keeps for reading and
// copying.
func renderTidbRows(rows []map[string]any, order []string) string {
	lines := make([]string, 0, len(rows))
	for _, row := range rows {
		cells := []string{}
		for _, name := range order {
			written := db.ReadAnyText(row[name])
			if written != "" {
				cells = append(cells, written)
			}
		}
		lines = append(lines, strings.Join(cells, "  "))
	}
	return strings.Join(lines, "\n")
}

// ReadPlan reads the plan TiDB wrote, one operator per row.
func ReadPlan(rows []map[string]any, order []string, analyzed bool) (db.QueryPlan, bool) {
	open := []*openTidbNode{}
	roots := []db.PlanNode{}

	closeTo := func(depth int) {
		for len(open) > 0 && depth <= open[len(open)-1].depth {
			node := closeTidbNode(open[len(open)-1])
			open = open[:len(open)-1]
			if len(open) == 0 {
				roots = append(roots, node)
				continue
			}
			parent := open[len(open)-1]
			parent.children = append(parent.children, node)
		}
	}

	for _, row := range rows {
		node := openTidbPlanNode(row)
		closeTo(node.depth)
		open = append(open, node)
	}
	closeTo(-1)

	if len(roots) == 0 {
		return db.QueryPlan{}, false
	}
	root := roots[0]
	if len(roots) > 1 {
		root.Children = append(append([]db.PlanNode{}, root.Children...), roots[1:]...)
	}

	return db.QueryPlan{
		Root: root, Raw: renderTidbRows(rows, order), Analyzed: analyzed, Measurable: true,
		// The server measures each operator, and reports no planning time.
		ExecutionMs: root.TotalMs, HasExecutionMs: root.HasTotalMs,
	}, true
}
