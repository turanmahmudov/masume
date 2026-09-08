package clickhouse

import (
	"strings"

	"github.com/turanmahmudov/masume/internal/core"
	"github.com/turanmahmudov/masume/internal/db"
	"github.com/turanmahmudov/masume/internal/query"
)

// planPrefix is what the client writes before a statement it asks the plan of.
const planPrefix = "explain plan "

// explainPrefix asks for the plan with the parts of it this client shows: the indexes a
// read uses, and the parts and granules it reads.
const explainPrefix = "explain indexes = 1 "

// planIndent is how many spaces the server writes for one step under another.
const planIndent = 2

// BuildExplainStatement writes the statement that asks the server for a plan.
func BuildExplainStatement(sql string) string {
	return explainPrefix + sql
}

// planLine is one line of the plan the server wrote.
type planLine struct {
	indent int
	text   string
}

// BuildPlan reads the plan lines of the server into the tree the pane draws. The server
// writes one step per line, and the spaces before a line say what it stands under.
func BuildPlan(answered db.QueryResult) (query.QueryPlan, bool) {
	lines := readPlanLines(answered)
	if len(lines) == 0 {
		return query.QueryPlan{}, false
	}

	written := make([]string, 0, len(lines))
	for _, line := range lines {
		written = append(written, strings.Repeat(" ", line.indent)+line.text)
	}
	root, _ := buildPlanBranch(lines, 0)
	return query.QueryPlan{
		Root: root, Raw: strings.Join(written, "\n"), Measurable: false,
	}, true
}

// buildPlanBranch builds the step at that line with every step under it, and returns the
// line the next step of the same depth stands at.
func buildPlanBranch(lines []planLine, at int) (query.PlanNode, int) {
	node := query.PlanNode{Label: lines[at].text}
	depth := lines[at].indent
	cursor := at + 1

	for cursor < len(lines) && lines[cursor].indent > depth {
		// A line of a property carries a name and a value, and stands under its step.
		if held, value, isProperty := readPlanProperty(lines[cursor].text); isProperty {
			if node.Detail == "" {
				node.Detail = held + ": " + value
			}
			cursor++
			continue
		}
		child, next := buildPlanBranch(lines, cursor)
		node.Children = append(node.Children, child)
		cursor = next
	}
	return node, cursor
}

// readPlanProperty reads a line that names a property of the step above it.
func readPlanProperty(text string) (string, string, bool) {
	name, value, cut := strings.Cut(text, ": ")
	if !cut || strings.Contains(name, " (") {
		return "", "", false
	}
	return name, value, true
}

// readPlanLines returns the lines of the plan with the depth of each one. The server writes
// the plan as one column of text.
func readPlanLines(answered db.QueryResult) []planLine {
	lines := make([]planLine, 0, len(answered.Rows))
	for _, row := range answered.Rows {
		if len(row) == 0 {
			continue
		}
		for _, written := range strings.Split(core.FormatCell(row[0], ""), "\n") {
			trimmed := strings.TrimRight(written, " ")
			if strings.TrimSpace(trimmed) == "" {
				continue
			}
			lines = append(lines, planLine{
				indent: countIndent(trimmed), text: strings.TrimSpace(trimmed),
			})
		}
	}
	return lines
}

// countIndent returns the depth of a line, counted in the steps the server indents by.
func countIndent(text string) int {
	spaces := len(text) - len(strings.TrimLeft(text, " "))
	return spaces / planIndent
}
