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

// StatementRunner is the interface a caller uses to allow a statement. The caller decides:
// the chat asks the user in a card, and the server checks the access level of the connection
// and then asks the user through their own client.
type StatementRunner struct {
	// RowLimit is the number of rows one run returns, which is also the maximum a caller
	// can request.
	RowLimit int
	// AskToRun returns an empty text if the statement can run, and the reason if it
	// cannot.
	AskToRun     func(ctx context.Context, risk statement.WriteRisk, statements []string) string
	RunStatement func(ctx context.Context, sql string, rowLimit int) (db.QueryResult, error)
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
