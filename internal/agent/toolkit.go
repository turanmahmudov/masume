// Package agent holds the operations a model can run on one connection. Two callers use it:
// the Model Context Protocol server, and the chat inside the client.
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
	// Undo holds the statements that reverse the write, read inside its transaction.
	Undo []string
	// UndoReason says why no undo was kept, where the caller measured the write.
	UndoReason string
}

// MeasuredWrite is what a write would do, for a caller that shows it to the user itself.
type MeasuredWrite struct {
	// Lines is the plan as text, one line each.
	Lines []string
	// Table, Rows and Total are what the write lands on.
	Table    string
	Rows     int64
	HasRows  bool
	Total    int64
	HasTotal bool
	Columns  []string
	Cascades []string
	Blocked  []string
	// UndoRows is the rows the undo would hold, and UndoReason why it holds none.
	UndoRows   int64
	UndoReason string
	// Token lets the caller run this one statement without being asked again. It is empty
	// where the client of the caller asks the user itself.
	Token string
}

// StatementRunner is the interface a caller uses to allow a statement. The caller decides:
// the chat asks the user in a card, and the server checks the access level of the connection
// and then asks the user through their own client.
type StatementRunner struct {
	// RowLimit is the number of rows one run returns, which is also the maximum a caller
	// can request.
	RowLimit int
	// AskToRun returns whether the statement can run, and the reason if it cannot.
	AskToRun func(
		ctx context.Context, risk statement.WriteRisk, statements []string,
	) RunPermission
	// MeasureWrite measures one write without running it. A caller that measures no write
	// on this connection leaves it unset, or answers false.
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

// ToolDefinition is one operation a model can run on a connection. It has a name and a
// description before a connection exists. The server lists them, and the chat binds them.
type ToolDefinition struct {
	Name        string
	Description string
	// InputSchema is the JSON Schema of the call. Every caller sends it unchanged.
	InputSchema map[string]any
	// Call validates the input against the schema, because a caller can send any value.
	Call func(ctx context.Context, deps ToolDeps, input map[string]any) any
}
