package agent

import (
	"context"
	"maps"
	"time"

	"github.com/turanmahmudov/masume/internal/core"
	"github.com/turanmahmudov/masume/internal/db"
	"github.com/turanmahmudov/masume/internal/query/language"
)

// The only tool that changes data. The runner of the caller decides whether it may run.
// This asks, runs and reports.

// describeCellForModel writes one cell as JSON: a number stays a number, and everything
// else becomes text.
func describeCellForModel(value any, dataType string) any {
	switch held := value.(type) {
	case nil:
		return nil
	case string, bool, float64, float32,
		int, int8, int16, int32, int64,
		uint, uint8, uint16, uint32, uint64:
		return held
	}
	return core.FormatCell(value, dataType)
}

func describeResultForModel(answered db.QueryResult) map[string]any {
	columns := make([]map[string]any, 0, len(answered.Columns))
	types := make([]string, 0, len(answered.Columns))
	for _, column := range answered.Columns {
		columns = append(columns,
			map[string]any{"name": column.Name, "type": column.DataType})
		types = append(types, column.DataType)
	}
	rows := make([][]any, 0, len(answered.Rows))
	for _, row := range answered.Rows {
		written := make([]any, 0, len(row))
		for at, value := range row {
			dataType := ""
			if at < len(types) {
				dataType = types[at]
			}
			written = append(written, describeCellForModel(value, dataType))
		}
		rows = append(rows, written)
	}

	// Every field is written, and the count of rows a statement changed is null where the
	// server reported none, because a field that is missing reads as one the model forgot to
	// ask for.
	var affected any
	if answered.HasAffected {
		affected = answered.Affected
	}
	return map[string]any{
		"columns":   columns,
		"rows":      rows,
		"rowCount":  len(answered.Rows),
		"truncated": answered.Truncated,
		"affected":  affected,
		"command":   answered.Command,
		"elapsedMs": float64(answered.Elapsed.Microseconds()) / 1000,
	}
}

var runQueryFields = []field{
	{
		name: "sql", kind: kindString, required: true,
		description: "The exact statement to run.",
	},
	{
		name: "limit", kind: kindInteger, positive: true,
		description: "Rows to bring back. Capped by the limit this connection carries.",
	},
}

var runQuery = ToolDefinition{
	Name: "run_query",
	Description: "Run a statement and answer with its rows. Call this only where the user " +
		"asked for data, a count or a value; a query they asked to see belongs in a fenced " +
		"block instead, unrun. The user is asked before anything runs and may say no, and " +
		"what a statement is allowed to do depends on the connection. The result is cut to " +
		"the row limit, so a read of a large table needs its own LIMIT and ORDER BY to mean " +
		"anything.",
	InputSchema: buildSchema(runQueryFields),
	Call: func(ctx context.Context, deps ToolDeps, input map[string]any) any {
		read, problem := readInput(runQueryFields, input)
		if problem != "" {
			return refuseInput(problem)
		}
		sql, _ := readText(read, "sql")
		asked, hasLimit := readCount(read, "limit")

		runner := deps.Runner
		statements := deps.Session.Language().SplitStatements(sql)
		refusal := runner.AskToRun(
			ctx, language.ResolveBatchRisk(statements, deps.Session.Language()), statements)
		if refusal != "" {
			return map[string]any{"ran": false, "reason": refusal}
		}

		rowLimit := runner.RowLimit
		if hasLimit && asked < rowLimit {
			rowLimit = asked
		}
		startedAt := time.Now()
		answered, err := runner.RunStatement(ctx, sql, rowLimit)
		if err != nil {
			message := db.DescribeError(err)
			if runner.ReportRun != nil {
				runner.ReportRun(StatementReport{
					SQL: sql, RanAt: startedAt, Elapsed: time.Since(startedAt),
					ErrorMessage: message,
				})
			}
			return map[string]any{"ran": true, "error": message}
		}
		if runner.ReportRun != nil {
			runner.ReportRun(StatementReport{
				SQL: sql, RanAt: startedAt, Elapsed: answered.Elapsed,
				RowCount: int64(len(answered.Rows)), HasRowCount: true,
			})
		}

		answer := map[string]any{"ran": true}
		maps.Copy(answer, describeResultForModel(answered))
		return answer
	},
}
