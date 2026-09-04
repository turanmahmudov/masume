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

// The only tool that changes data. The runner of the caller decides whether it can run.
// This file asks, runs and reports.

// describeCellForModel returns one cell as a JSON value: a number stays a number, and every
// other type becomes text.
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

	// Every field is written. The number of changed rows is null if the server reported
	// none, because a missing field looks like a field the model did not request.
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
		"anything. Where the connection measures a write before it runs, the answer carries " +
		"an `undo` list: the statements that reverse it, read inside its transaction.",
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
	Description: "Measure what a write would do before running it: the rows it lands on, " +
		"counted on the server; the columns it assigns; the relations it reaches through a " +
		"trigger or a foreign key; the relations that block it; and whether it can be undone. " +
		"Nothing is written. Call this before run_query for an UPDATE, DELETE or TRUNCATE, " +
		"show the answer to the user, and run the statement only if they agree. Where the " +
		"answer carries a `token`, pass it to run_query as `plan_token` to run that one " +
		"statement without being asked again.",
	InputSchema: buildSchema(planWriteFields),
	Call: func(ctx context.Context, deps ToolDeps, input map[string]any) any {
		read, problem := readInput(planWriteFields, input)
		if problem != "" {
			return refuseInput(problem)
		}
		sql, _ := readText(read, "sql")

		if deps.Runner.MeasureWrite == nil {
			return map[string]any{
				"measured": false, "reason": "this connection measures no write",
			}
		}
		measured, held := deps.Runner.MeasureWrite(ctx, sql)
		if !held {
			return map[string]any{"measured": false, "reason": describeUnmeasured(sql, deps)}
		}
		return describeMeasuredWrite(measured)
	},
}

// describeUnmeasured says why a statement was not measured, so a model does not call the
// tool again with the same statement.
func describeUnmeasured(sql string, deps ToolDeps) string {
	if language.ResolveBatchRisk(
		deps.Session.Language().SplitStatements(sql), deps.Session.Language()) == statement.RiskNone {
		return "this statement writes nothing, so there is nothing to measure"
	}
	return "this write was not read as one relation and one predicate, so its rows cannot " +
		"be counted. A write that joins a second relation, that names its target through an " +
		"alias, or that runs beside other statements is not measured"
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

// describeCount writes a number the server did not answer for as null, because a missing
// field reads as a field the model did not request.
func describeCount(count int64, held bool) any {
	if !held {
		return nil
	}
	return count
}
