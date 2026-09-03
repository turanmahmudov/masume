package ai

import (
	"strings"

	"github.com/turanmahmudov/masume/internal/core"
)

// A readable description of one tool call: the table it reads, or the statement it runs, and
// not the name of the tool.

// maxShownSQL is long enough to identify a statement and short enough for one line of the
// panel, next to the step and the run time.
const maxShownSQL = 48

// activityStep holds the text of a step without a subject and the text with a subject.
type activityStep struct {
	alone  string
	naming func(subject string) string
}

// tableSteps holds the calls that read one table.
var tableSteps = map[string]activityStep{
	"describe_table": {"reading the columns",
		func(table string) string { return "reading the columns of " + table }},
	"list_indexes": {"reading the indexes",
		func(table string) string { return "reading the indexes of " + table }},
	"list_constraints": {"reading the constraints",
		func(table string) string { return "reading the constraints of " + table }},
	"get_table_ddl": {"reading a definition",
		func(table string) string { return "reading the definition of " + table }},
	"list_relationships": {"reading how tables join",
		func(table string) string { return "reading what joins to " + table }},
}

// sqlSteps holds the calls that contain a statement.
var sqlSteps = map[string]activityStep{
	"validate_query": {"checking a query",
		func(sql string) string { return "checking " + sql }},
	"run_query": {"running a statement",
		func(sql string) string { return "running " + sql }},
}

// findShownSQL returns the statement of a call on one line, and whether the call has one.
func findShownSQL(input map[string]any) (string, bool) {
	asked, is := input["sql"].(string)
	if !is || strings.TrimSpace(asked) == "" {
		return "", false
	}
	written := core.CollapseWhitespace(strings.TrimSpace(asked))
	if len([]rune(written)) > maxShownSQL {
		written = string([]rune(written)[:maxShownSQL]) + "…"
	}
	return written, true
}

// writeStep returns the text of one step, with the subject if the call has one.
func writeStep(step activityStep, subject string, named bool) string {
	if !named {
		return step.alone
	}
	return step.naming(subject)
}

// DescribeToolActivity returns a description of one tool call.
func DescribeToolActivity(toolName string, input map[string]any) string {
	if step, known := tableSteps[toolName]; known {
		table, named := input["table"].(string)
		return writeStep(step, table, named)
	}
	if step, known := sqlSteps[toolName]; known {
		sql, named := findShownSQL(input)
		return writeStep(step, sql, named)
	}

	switch toolName {
	case "list_tables":
		if database, named := input["database"].(string); named {
			return "listing the tables of " + database
		}
		return "listing the tables"
	case "explain_query":
		verb := "planning"
		if measured, is := input["analyze"].(bool); is && measured {
			verb = "measuring"
		}
		sql, named := findShownSQL(input)
		return writeStep(activityStep{
			verb + " a query", func(held string) string { return verb + " " + held },
		}, sql, named)
	}
	return "running " + toolName
}
