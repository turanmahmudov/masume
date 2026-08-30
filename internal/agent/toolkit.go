// Package agent holds what a model may ask one connection for. Both callers use it: the
// server that speaks the Model Context Protocol, and the chat inside the client.
package agent

import (
	"context"
	"time"

	"github.com/turanmahmudov/masume/internal/db"
	"github.com/turanmahmudov/masume/internal/query/statement"
)

// StatementReport is one statement that ran or failed, as the caller stores it.
type StatementReport struct {
	SQL          string
	RanAt        time.Time
	Elapsed      time.Duration
	RowCount     int64
	HasRowCount  bool
	ErrorMessage string
}

// StatementRunner is how a caller lets a statement run. The caller decides whether it may:
// the chat asks the user in a card, and the server checks the level of the connection and
// then asks the user through their own client.
type StatementRunner struct {
	// RowLimit is how many rows one run returns, which is also the most a caller may ask
	// for.
	RowLimit int
	// AskToRun returns an empty text where the statement may run, and the reason it may
	// not otherwise.
	AskToRun     func(ctx context.Context, risk statement.WriteRisk, statements []string) string
	RunStatement func(ctx context.Context, sql string, rowLimit int) (db.QueryResult, error)
	// ReportRun is called after the run, so the caller can store it.
	ReportRun func(report StatementReport)
}

type ToolSession interface {
	db.SessionInfo
	db.CatalogReader
	db.QueryRunner
	db.TransactionKeeper
}

// ToolDeps is what a tool may reach.
type ToolDeps struct {
	Session ToolSession
	// Tables returns the relations of the connection, which the caller keeps fresh.
	Tables func() []db.TableRef
	Runner StatementRunner
	// MarkTableDescribed marks a relation the model described as read in the tree too.
	MarkTableDescribed func(table db.TableRef, detail db.TableDetail)
}

// ToolDefinition is one thing a model may ask a connection for, named and described before
// any connection exists. The server lists these, and the chat binds them.
type ToolDefinition struct {
	Name        string
	Description string
	// InputSchema is the JSON Schema of the call, which every caller sends as it is.
	InputSchema map[string]any
	// Call reads the input through the schema, because a caller can send anything.
	Call func(ctx context.Context, deps ToolDeps, input map[string]any) any
}
