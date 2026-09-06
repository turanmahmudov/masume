// Package agent provides database tools for the Model Context Protocol server and the client chat.
package agent

import (
	"context"
	"time"

	"github.com/turanmahmudov/masume/internal/db"
	"github.com/turanmahmudov/masume/internal/query/statement"
)

// StatementReport is one statement that ran or failed, in the form the caller stores.
type StatementReport struct {
	SQL          string
	RanAt        time.Time
	Elapsed      time.Duration
	RowCount     int64
	HasRowCount  bool
	ErrorMessage string
}

// RunPermission is the answer to asking whether a statement may run.
type RunPermission struct {
	// Refusal is empty where the statement may run, and the reason where it may not.
	Refusal string
}

// StatementAnswer is what one statement answered, and the undo the caller kept with it.
type StatementAnswer struct {
	Result db.QueryResult
	// Undo is the statements built from original rows captured in the write transaction.
	Undo []string
	// UndoReason is the cause of unavailable undo after write measurement.
	UndoReason string
}

// MeasuredWrite is what a write would do, for a caller that shows it to the user itself.
type MeasuredWrite struct {
	// Lines is the plan as text, one line each.
	Lines []string
	// Table is the target. Rows and Total are matching and total row counts.
	Table    string
	Rows     int64
	HasRows  bool
	Total    int64
	HasTotal bool
	Columns  []string
	Cascades []string
	Blocked  []string
	// UndoRows is the capture count. UndoReason is the cause of unavailable undo.
	UndoRows   int64
	UndoReason string
	// Token is the statement authorization token for clients without confirmation dialogs.
	Token string
}

// StatementRunner is the caller interface for statement authorization, measurement, execution, and reporting.
type StatementRunner struct {
	// RowLimit is the maximum rows per run.
	RowLimit int
	// AskToRun returns whether the statement can run, and the reason if it cannot.
	AskToRun func(
		ctx context.Context, risk statement.WriteRisk, statements []string,
	) RunPermission
	// MeasureWrite measures a write without execution. Nil or false means measurement is unavailable.
	MeasureWrite func(ctx context.Context, sql string) (MeasuredWrite, bool)
	RunStatement func(ctx context.Context, sql string, rowLimit int) (StatementAnswer, error)
	// ReportRun is called after the run, so the caller can store the result.
	ReportRun func(report StatementReport)
}

type ToolSession interface {
	db.SessionInfo
	db.CatalogReader
	db.QueryRunner
	db.TransactionKeeper
}

// ToolDeps holds the resources a tool can use.
type ToolDeps struct {
	Session ToolSession
	// Tables returns the tables of the connection. The caller keeps the list up to date.
	Tables func() []db.TableRef
	Runner StatementRunner
	// MarkTableDescribed marks a table the model described as read in the tree as well.
	MarkTableDescribed func(table db.TableRef, detail db.TableDetail)
}

// ToolDefinition is a database tool with an input schema and execution function.
type ToolDefinition struct {
	Name        string
	Description string
	// InputSchema is the tool JSON Schema before caller-specific extensions.
	InputSchema map[string]any
	// Call validates and runs the tool input.
	Call func(ctx context.Context, deps ToolDeps, input map[string]any) any
}
