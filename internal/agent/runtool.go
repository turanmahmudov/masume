package agent

import (
	"context"
	"maps"
	"time"

	"github.com/turanmahmudov/masume/internal/core"
	"github.com/turanmahmudov/masume/internal/db"
	"github.com/turanmahmudov/masume/internal/query/language"
	"github.com/turanmahmudov/masume/internal/query/statement"
)

// The caller authorizes statement execution and write measurement.

// describeCellForModel preserves JSON scalar types and formats other values as text.
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

	// An unreported affected row count is null.
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
		description: "Maximum rows to return, capped by the connection row limit.",
	},
}

var runQuery = ToolDefinition{
	Name: "run_query",
	Description: "Run a statement and return its rows. Call this only when the user " +
		"requests data, a count, or a value. If the user requests query text, return a fenced " +
		"code block without running the query. The connection policy may require user confirmation " +
		"or refuse execution. Results use the connection row limit. Use LIMIT and ORDER BY " +
		"for a bounded, ordered result from a large table. When the connection captures undo data, " +
		"the response includes an `undo` list of statements built from rows read inside the write transaction.",
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
		permission := runner.AskToRun(
			ctx, language.ResolveBatchRisk(statements, deps.Session.Language()), statements)
		if permission.Refusal != "" {
			return map[string]any{"ran": false, "reason": permission.Refusal}
		}

		rowLimit := runner.RowLimit
		if hasLimit && asked < rowLimit {
			rowLimit = asked
		}
		startedAt := time.Now()
		ran, err := runner.RunStatement(ctx, sql, rowLimit)
		answered := ran.Result
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
		if len(ran.Undo) > 0 {
			answer["undo"] = ran.Undo
		}
		if ran.UndoReason != "" {
			answer["undo_reason"] = ran.UndoReason
		}
		maps.Copy(answer, describeResultForModel(answered))
		return answer
	},
}

var planWriteFields = []field{
	{
		name: "sql", kind: kindString, required: true,
		description: "The exact write to measure. It is not run.",
	},
}

var planWrite = ToolDefinition{
	Name: "plan_write",
	Description: "Measure a write before execution: matching rows counted on the server, " +
		"assigned columns, triggers, foreign key effects, blocking references, and undo availability. " +
		"The write does not run. Call this before run_query for an UPDATE, DELETE, or TRUNCATE. " +
		"Show the plan to the user and run the statement only with their approval. If the " +
		"response includes a `token`, pass the token to run_query as `plan_token` to run that " +
		"statement without another confirmation request.",
	InputSchema: buildSchema(planWriteFields),
	Call: func(ctx context.Context, deps ToolDeps, input map[string]any) any {
		read, problem := readInput(planWriteFields, input)
		if problem != "" {
			return refuseInput(problem)
		}
		sql, _ := readText(read, "sql")

		if deps.Runner.MeasureWrite == nil {
			return map[string]any{
				"measured": false, "reason": "write measurement is unavailable on this connection",
			}
		}
		measured, held := deps.Runner.MeasureWrite(ctx, sql)
		if !held {
			return map[string]any{"measured": false, "reason": describeUnmeasured(sql, deps)}
		}
		return describeMeasuredWrite(measured)
	},
}

// describeUnmeasured returns the reason write measurement is unavailable.
func describeUnmeasured(sql string, deps ToolDeps) string {
	if language.ResolveBatchRisk(
		deps.Session.Language().SplitStatements(sql), deps.Session.Language()) == statement.RiskNone {
		return "this statement is classified as read-only; write measurement does not apply"
	}
	return "cannot measure this write as one known table and one predicate. " +
		"Write measurement does not support joins, target aliases, or statement batches"
}

// describeMeasuredWrite returns the plan in the form the model reads.
func describeMeasuredWrite(measured MeasuredWrite) map[string]any {
	undo := map[string]any{"rows": measured.UndoRows}
	if measured.UndoReason != "" {
		undo = map[string]any{"rows": 0, "reason": measured.UndoReason}
	}
	described := map[string]any{
		"measured": true,
		"table":    measured.Table,
		"rows":     describeCount(measured.Rows, measured.HasRows),
		"total":    describeCount(measured.Total, measured.HasTotal),
		"columns":  measured.Columns,
		"cascades": measured.Cascades,
		"blocked":  measured.Blocked,
		"undo":     undo,
		"plan":     measured.Lines,
	}
	if measured.Token != "" {
		described["token"] = measured.Token
		described["note"] = "Show the plan to the user and ask whether to run the " +
			"statement. If they agree, call run_query with this token as `plan_token`."
	}
	return described
}

// describeCount returns null for an unavailable count.
func describeCount(count int64, held bool) any {
	if !held {
		return nil
	}
	return count
}
