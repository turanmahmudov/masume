package result

import (
	"fmt"
	"math"
	"regexp"
	"strconv"
	"strings"

	"github.com/turanmahmudov/masume/internal/query"
)

// conditionKeys are the conditions the pane shows, in order.
var conditionKeys = []string{
	"Index Cond", "Recheck Cond", "Hash Cond", "Merge Cond", "Join Filter",
	"Filter", "Sort Key", "Group Key",
}

// misestimateFactor is how far an estimate must be wrong before the node is marked.
const misestimateFactor = 10

// nodeMarker is the arrow a server writes before every node below the first.
const nodeMarker = "->"

var (
	costGroup = regexp.MustCompile(`\(cost=[^)]*?\brows=([\d.eE+-]+)[^)]*\)`)
	// MySQL writes a loop count over a million in scientific notation, like a row
	// count, so both use the same pattern.
	actualGroup = regexp.MustCompile(
		`\(actual (?:time=[\d.eE+-]+\.\.([\d.eE+-]+) )?rows=([\d.eE+-]+) loops=([\d.eE+-]+)\)`)
	// summaryTime matches a line at the end of the plan, which is not a node.
	summaryTime = regexp.MustCompile(`^\s*(Planning|Execution) Time:\s*([\d.]+)\s*ms`)
	// propertyLine matches a property under the node it belongs to.
	propertyLine = regexp.MustCompile(`^\s*([A-Z][A-Za-z ]*[A-Za-z]):\s*(.+)$`)
)

const neverRun = "(never executed)"

// openNode is a node being read, with the lines under it read so far.
type openNode struct {
	// The column of the arrow, which decides where the next node nests.
	indent int
	label  string
	// The text after a colon on the node line, which is how MySQL writes it.
	inlineDetail     string
	properties       map[string]string
	estimatedRows    float64
	hasEstimatedRows bool
	actualRows       float64
	hasActualRows    bool
	totalMs          float64
	hasTotalMs       bool
	children         []query.PlanNode
}

func findMarkerIndent(line string) (int, bool) {
	marker := strings.Index(line, nodeMarker)
	if marker == -1 || strings.TrimSpace(line[:marker]) != "" {
		return 0, false
	}
	return marker, true
}

func countLeadingSpaces(line string) int {
	return len(line) - len(strings.TrimLeft(line, " \t"))
}

func readNumber(written string) (float64, bool) {
	value, err := strconv.ParseFloat(written, 64)
	if err != nil {
		return 0, false
	}
	return value, true
}

// buildOpenNode reads one node line. The numbers in brackets are removed first, so
// the rest is what the node does.
func buildOpenNode(line string, marker int, hasMarker bool, indent int) *openNode {
	body := strings.TrimSpace(line)
	if hasMarker {
		body = strings.TrimSpace(line[marker+len(nodeMarker):])
	}

	cost := costGroup.FindStringSubmatch(body)
	actual := actualGroup.FindStringSubmatch(body)
	loops := 1.0
	if actual != nil {
		if counted, read := readNumber(actual[3]); read {
			loops = counted
		}
	}
	ran := actual != nil || strings.Contains(body, neverRun)

	described := strings.TrimSpace(
		strings.ReplaceAll(actualGroup.ReplaceAllString(
			costGroup.ReplaceAllString(body, ""), ""), neverRun, ""))
	before, after, ok := strings.Cut(described, ": ")

	node := &openNode{indent: indent, properties: map[string]string{}}
	if !ok {
		node.label = described
	} else {
		node.label = before
		node.inlineDetail = after
	}

	if cost != nil {
		if estimated, read := readNumber(cost[1]); read {
			node.estimatedRows = estimated * loops
			node.hasEstimatedRows = true
		}
	}
	if ran {
		counted := 0.0
		if actual != nil {
			if read, isNumber := readNumber(actual[2]); isNumber {
				counted = read
			}
		}
		node.actualRows = counted * loops
		node.hasActualRows = true
		if actual != nil && actual[1] != "" {
			if perLoop, read := readNumber(actual[1]); read {
				node.totalMs = perLoop * loops
				node.hasTotalMs = true
			}
		}
	}
	return node
}

// buildDetail returns the detail of a node. A server writes several conditions, so
// they are ranked and the first one wins.
func buildDetail(open *openNode) string {
	if open.inlineDetail != "" {
		return open.inlineDetail
	}
	for _, key := range conditionKeys {
		if condition, present := open.properties[key]; present {
			return strings.ToLower(key) + ": " + condition
		}
	}
	return ""
}

func closeOpenNode(open *openNode) query.PlanNode {
	childMs := 0.0
	for _, child := range open.children {
		childMs += child.TotalMs
	}
	node := query.PlanNode{
		Label:            open.label,
		Detail:           buildDetail(open),
		EstimatedRows:    open.estimatedRows,
		HasEstimatedRows: open.hasEstimatedRows,
		ActualRows:       open.actualRows,
		HasActualRows:    open.hasActualRows,
		TotalMs:          open.totalMs,
		HasTotalMs:       open.hasTotalMs,
		Children:         open.children,
	}
	if open.hasTotalMs {
		node.SelfMs = math.Max(0, open.totalMs-childMs)
		node.HasSelfMs = true
	}
	return node
}

// planTimes are the times a plan reports at its end, outside the tree.
type planTimes struct {
	planningMs     float64
	hasPlanningMs  bool
	executionMs    float64
	hasExecutionMs bool
}

func applySummaryTime(times *planTimes, line string) bool {
	summary := summaryTime.FindStringSubmatch(line)
	if summary == nil {
		return false
	}
	value, read := readNumber(summary[2])
	if !read {
		return true
	}
	if summary[1] == "Planning" {
		times.planningMs = value
		times.hasPlanningMs = true
	} else {
		times.executionMs = value
		times.hasExecutionMs = true
	}
	return true
}

func applyProperty(carrying *openNode, line string) {
	property := propertyLine.FindStringSubmatch(line)
	if property != nil {
		carrying.properties[property[1]] = strings.TrimSpace(property[2])
	}
}

// closeNodesTo closes every node the indent has left, and hangs it under the one above.
func closeNodesTo(open *[]*openNode, topLevel *[]query.PlanNode, indent int) {
	for len(*open) > 0 && indent <= (*open)[len(*open)-1].indent {
		node := closeOpenNode((*open)[len(*open)-1])
		*open = (*open)[:len(*open)-1]
		if len(*open) == 0 {
			*topLevel = append(*topLevel, node)
			continue
		}
		parent := (*open)[len(*open)-1]
		parent.children = append(parent.children, node)
	}
}

// joinTopLevel joins the trees a server wrote apart. MySQL writes one tree per
// subquery, and each one after the first is a child of the root.
func joinTopLevel(topLevel []query.PlanNode) (query.PlanNode, bool) {
	if len(topLevel) == 0 {
		return query.PlanNode{}, false
	}
	main := topLevel[0]
	if len(topLevel) == 1 {
		return main, true
	}
	main.Children = append(append([]query.PlanNode{}, main.Children...), topLevel[1:]...)
	return main, true
}

// ParseTextPlan reads a plan a server wrote as text. Both servers write one node per
// line and nest by indent. No indent width is assumed: an arrow is compared with the
// arrows still open, and a line without an arrow is the cost or a detail of the node
// above.
func ParseTextPlan(text string, analyzed, measurable bool) (query.QueryPlan, bool) {
	open := []*openNode{}
	topLevel := []query.PlanNode{}
	times := planTimes{}

	for line := range strings.SplitSeq(text, "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		if applySummaryTime(&times, line) {
			continue
		}

		marker, hasMarker := findMarkerIndent(line)
		if !hasMarker && len(open) > 0 {
			applyProperty(open[len(open)-1], line)
			continue
		}

		// The first line opens the tree, with or without an arrow. Only MySQL puts an
		// arrow before the first node.
		indent := marker
		if !hasMarker {
			indent = countLeadingSpaces(line)
		}
		closeNodesTo(&open, &topLevel, indent)
		open = append(open, buildOpenNode(line, marker, hasMarker, indent))
	}

	closeNodesTo(&open, &topLevel, -1)
	root, found := joinTopLevel(topLevel)
	if !found {
		return query.QueryPlan{}, false
	}

	plan := query.QueryPlan{
		Root: root, Raw: text, Analyzed: analyzed, Measurable: measurable,
		PlanningMs: times.planningMs, HasPlanningMs: times.hasPlanningMs,
		ExecutionMs: times.executionMs, HasExecutionMs: times.hasExecutionMs,
	}
	// MySQL reports no total, so the root holds the time of the whole run.
	if !plan.HasExecutionMs && analyzed && root.HasTotalMs {
		plan.ExecutionMs = root.TotalMs
		plan.HasExecutionMs = true
	}
	return plan, true
}

// PlanRow is one plan node, flattened for the pane.
type PlanRow struct {
	Depth int
	Node  query.PlanNode
	// The share of the whole run this node took alone, from 0 to 1.
	Share float64
	// True for the node with the most time of its own.
	Slowest bool
	// True if the planner expected ten times more or fewer rows.
	Misestimated bool
}

type collectedNode struct {
	node  query.PlanNode
	depth int
}

func collectNodes(node query.PlanNode, depth int, into *[]collectedNode) {
	*into = append(*into, collectedNode{node: node, depth: depth})
	for _, child := range node.Children {
		collectNodes(child, depth+1, into)
	}
}

// isMisestimated marks only an estimate that is far too low. A LIMIT makes every node
// below it read fewer rows than planned, so marking that would mark half a plan.
func isMisestimated(node query.PlanNode) bool {
	if !node.HasActualRows || !node.HasEstimatedRows || node.EstimatedRows <= 0 {
		return false
	}
	return node.ActualRows/node.EstimatedRows >= misestimateFactor
}

// FlattenPlan flattens the tree into rows, each with its share of the time and its
// estimate.
func FlattenPlan(plan query.QueryPlan) []PlanRow {
	collected := []collectedNode{}
	collectNodes(plan.Root, 0, &collected)

	total := 0.0
	switch {
	case plan.HasExecutionMs:
		total = plan.ExecutionMs
	case plan.Root.HasTotalMs:
		total = plan.Root.TotalMs
	}

	slowestAt := -1
	for at, entry := range collected {
		if !entry.node.HasSelfMs {
			continue
		}
		if slowestAt == -1 || entry.node.SelfMs > collected[slowestAt].node.SelfMs {
			slowestAt = at
		}
	}

	rows := make([]PlanRow, 0, len(collected))
	for at, entry := range collected {
		share := 0.0
		if total > 0 && entry.node.HasSelfMs {
			share = entry.node.SelfMs / total
		}
		rows = append(rows, PlanRow{
			Depth: entry.depth, Node: entry.node, Share: share,
			Slowest:      at == slowestAt && entry.node.SelfMs > 0,
			Misestimated: isMisestimated(entry.node),
		})
	}
	return rows
}

// DescribePlanCost writes the line above the tree: the time the run took, or that it
// never ran.
func DescribePlanCost(plan query.QueryPlan) string {
	if !plan.Analyzed {
		if plan.Measurable {
			return "estimated"
		}
		return "estimated · the server measures no plan"
	}
	parts := []string{}
	if plan.HasPlanningMs {
		parts = append(parts, fmt.Sprintf("planning %.1f ms", plan.PlanningMs))
	}
	if plan.HasExecutionMs {
		parts = append(parts, fmt.Sprintf("execution %.1f ms", plan.ExecutionMs))
	}
	return strings.Join(parts, " · ")
}
