package db

import (
	"context"
	"database/sql"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/turanmahmudov/masume/internal/core"
	"github.com/turanmahmudov/masume/internal/query"
	"github.com/turanmahmudov/masume/internal/query/language"
	"github.com/turanmahmudov/masume/internal/query/statement"
	"github.com/turanmahmudov/masume/internal/query/syntax"
)

// EngineSupport joins the static facts of an engine to its dialect and language.
type EngineSupport struct {
	core.EngineInfo
	Dialect  *query.Dialect
	Language language.Language
	Compose  Composer
}

// SessionFacts is the session descriptor and engine support information.
type SessionFacts struct {
	Descriptor SessionDescriptor
	Support    EngineSupport
}

func (facts SessionFacts) Describe() SessionDescriptor     { return facts.Descriptor }
func (facts SessionFacts) Dialect() *query.Dialect         { return facts.Support.Dialect }
func (facts SessionFacts) Language() language.Language     { return facts.Support.Language }
func (facts SessionFacts) Capabilities() core.Capabilities { return facts.Support.Capabilities }
func (facts SessionFacts) Composer() Composer              { return facts.Support.Compose }

// HoldsSeveralCommands is true if the buffer contains multiple statements.
func HoldsSeveralCommands(sql string, flavour syntax.SyntaxFlavour) bool {
	return len(statement.SplitStatements(sql, flavour)) > 1
}

// RefuseSeveralCommands rejects multiple statements for a single export cursor.
func RefuseSeveralCommands(sql string, flavour syntax.SyntaxFlavour) error {
	if HoldsSeveralCommands(sql, flavour) {
		return NewDatabaseError("only one statement can be written to a file at a time")
	}
	return nil
}

// RefuseSeveralPlans rejects multiple statements. An EXPLAIN prefix applies only to the first statement.
func RefuseSeveralPlans(sql string, flavour syntax.SyntaxFlavour) error {
	if HoldsSeveralCommands(sql, flavour) {
		return NewDatabaseError("only one statement can be planned at a time")
	}
	return nil
}

// ReadLastStatement returns the last statement of a buffer that holds a command.
func ReadLastStatement(sql string, flavour syntax.SyntaxFlavour) string {
	statements := statement.SplitStatements(sql, flavour)
	for at := len(statements) - 1; at >= 0; at-- {
		if syntax.ReadCommandWord(statements[at], flavour) != "" {
			return statements[at]
		}
	}
	return ""
}

// ReadLastCommandWord returns the last statement command, which is the displayed result label.
func ReadLastCommandWord(sql string, flavour syntax.SyntaxFlavour) string {
	return syntax.ReadCommandWord(ReadLastStatement(sql, flavour), flavour)
}

// HoldsReturningClause is true for a statement with a top-level RETURNING clause, the
// one form of a write that answers with rows.
func HoldsReturningClause(sql string, flavour syntax.SyntaxFlavour) bool {
	return len(syntax.FindTopLevelKeywords(sql, []string{"returning"}, flavour)) > 0
}

// ReadNonNegativeCount reads a catalog count, which is an estimate and never below
// zero.
func ReadNonNegativeCount(value any) int64 {
	switch held := value.(type) {
	case nil:
		return 0
	case int64:
		if held > 0 {
			return held
		}
	case int32:
		if held > 0 {
			return int64(held)
		}
	case int:
		if held > 0 {
			return int64(held)
		}
	case float64:
		if held > 0 {
			return int64(held)
		}
	case string:
		parsed, err := strconv.ParseFloat(strings.TrimSpace(held), 64)
		if err == nil && parsed > 0 {
			return int64(parsed)
		}
	case []byte:
		parsed, err := strconv.ParseFloat(strings.TrimSpace(string(held)), 64)
		if err == nil && parsed > 0 {
			return int64(parsed)
		}
	}
	return 0
}

// BuildMissingDefinition returns a comment for an unavailable DDL definition.
func BuildMissingDefinition(name string) []string {
	return []string{"-- no definition available for " + name}
}

// FailUnreadablePlan reports a plan no parser here can read.
func FailUnreadablePlan() error {
	return NewDatabaseError("cannot parse the query plan")
}

// writeCommands is the set of commands with an affected row count.
var writeCommands = map[string]bool{
	"insert": true, "update": true, "delete": true, "replace": true, "merge": true,
}

// IsWriteCommand is true for a command that reports a row count.
func IsWriteCommand(command string) bool {
	return command != "" && writeCommands[strings.ToLower(command)]
}

// ReadOverscanRowLimit asks for one row more than wanted, to tell a full page from
// the last page.
func ReadOverscanRowLimit(rowLimit int) int {
	return rowLimit + 1
}

// CappedRead is a query result before row limit enforcement.
type CappedRead struct {
	Rows        [][]any
	RowLimit    int
	Columns     []ResultColumn
	Elapsed     time.Duration
	Command     string
	Affected    int64
	HasAffected bool
	// HoldsResultSet is true for a result set, including one without columns.
	HoldsResultSet bool
}

// BuildCappedResult drops the extra row, and reports it as truncated.
func BuildCappedResult(read CappedRead) QueryResult {
	rows := read.Rows
	truncated := false
	if read.RowLimit >= 0 && len(rows) > read.RowLimit {
		rows = rows[:read.RowLimit]
		truncated = true
	}
	return QueryResult{
		Columns: read.Columns, Rows: rows, Elapsed: read.Elapsed, Truncated: truncated,
		Command: read.Command, Affected: read.Affected, HasAffected: read.HasAffected,
		HoldsResultSet: read.HoldsResultSet,
	}
}

// RunStatement is how a session runs one statement, which is all paging and counting
// need.
type RunStatement func(
	ctx context.Context, sql string, rowLimit int, params []any,
) (QueryResult, error)

// ReadSQLPage returns one page of a read. A read that cannot be paged returns its
// whole result.
func ReadSQLPage(
	ctx context.Context, run RunStatement, read ComposedRead, window ReadWindow,
	flavour syntax.SyntaxFlavour,
) (QueryResult, error) {
	if !read.Pageable {
		return run(ctx, read.Text, window.Limit, read.Params)
	}
	paged := statement.ApplyPaging(
		read.Text, ReadOverscanRowLimit(window.Limit), window.Offset, flavour)
	return run(ctx, paged, window.Limit, read.Params)
}

// CountSQLRead counts the whole result of a read, once.
func CountSQLRead(
	ctx context.Context, run RunStatement, read ComposedRead, dialect *query.Dialect,
) (int64, bool, error) {
	if !read.Pageable {
		return 0, false, nil
	}
	counted, err := run(ctx, statement.BuildCountSQL(read.Text, dialect), 1, read.Params)
	if err != nil {
		return 0, false, err
	}
	if len(counted.Rows) == 0 || len(counted.Rows[0]) == 0 {
		return 0, false, nil
	}
	return ReadNonNegativeCount(counted.Rows[0][0]), true, nil
}

// RowBatcher sends streamed rows to a callback in batches.
type RowBatcher struct {
	size    int
	onBatch func(rows [][]any, columns []ResultColumn) error
	batch   [][]any
	total   int64
	// True after the first callback, including an empty batch with column metadata.
	handed bool
}

// NewRowBatcher returns a batcher with a row limit and callback.
func NewRowBatcher(
	size int, onBatch func(rows [][]any, columns []ResultColumn) error,
) *RowBatcher {
	if size < 1 {
		size = 1
	}
	return &RowBatcher{size: size, onBatch: onBatch, batch: make([][]any, 0, size)}
}

// AddRow takes one row, and hands the batch over once it is full.
func (batcher *RowBatcher) AddRow(row []any, columns []ResultColumn) error {
	batcher.batch = append(batcher.batch, row)
	batcher.total++
	if len(batcher.batch) < batcher.size {
		return nil
	}
	return batcher.FlushRows(columns)
}

// FlushRows sends the final batch. An empty result sends column metadata once.
func (batcher *RowBatcher) FlushRows(columns []ResultColumn) error {
	if len(batcher.batch) == 0 {
		if batcher.handed || len(columns) == 0 {
			return nil
		}
		batcher.handed = true
		return batcher.onBatch(nil, columns)
	}
	held := batcher.batch
	batcher.batch = make([][]any, 0, batcher.size)
	batcher.handed = true
	return batcher.onBatch(held, columns)
}

// CountRows returns how many rows were taken, which is what a stream answers with.
func (batcher *RowBatcher) CountRows() int64 { return batcher.total }

// ChangeApplication is how one session applies staged work, which is all that differs
// between engines.
type ChangeApplication struct {
	// True while the user holds a transaction open, which is joined, not nested.
	JoinsUserTransaction bool
	Begin                func(ctx context.Context) error
	Commit               func(ctx context.Context) error
	Rollback             func(ctx context.Context) error
	Apply                func(ctx context.Context, change Change) error
	// CountMatches runs the guard of a change and returns the rows it matched. A session
	// that leaves it unset runs no guard.
	CountMatches func(ctx context.Context, guard BoundStatement) (int64, error)
	ReportFailed func(err error)
}

// ScanOneRow is one row of a result, which is how every driver answers a count.
type ScanOneRow interface {
	Scan(targets ...any) error
}

// BuildGuardCounter returns the CountMatches of a session: the guard runs, and the one
// number it answers is read back. The drivers differ only in how they hand the row over.
func BuildGuardCounter(
	queryRow func(ctx context.Context, sql string, params ...any) ScanOneRow,
) func(ctx context.Context, guard BoundStatement) (int64, error) {
	return func(ctx context.Context, guard BoundStatement) (int64, error) {
		var matched int64
		if err := queryRow(ctx, guard.SQL, guard.Params...).Scan(&matched); err != nil {
			return 0, err
		}
		return matched, nil
	}
}

// rollbackWait is the rollback time limit after a failed change.
const rollbackWait = 5 * time.Second

// rollBackWhatFailed attempts rollback with a separate timeout, even after the original context is cancelled.
func rollBackWhatFailed(ctx context.Context, session ChangeApplication) {
	held, stop := context.WithTimeout(context.WithoutCancel(ctx), rollbackWait)
	defer stop()
	_ = session.Rollback(held)
}

// ApplyChangesInTransaction applies staged changes in a new or existing transaction. Existing transactions remain open for user action.
func ApplyChangesInTransaction(
	ctx context.Context, changes []Change, session ChangeApplication,
) error {
	if len(changes) == 0 {
		return nil
	}
	if !session.JoinsUserTransaction {
		if err := session.Begin(ctx); err != nil {
			return err
		}
	}

	for _, change := range changes {
		err := checkChangeGuard(ctx, change, session)
		if err == nil {
			err = session.Apply(ctx, change)
		}
		if err == nil {
			continue
		}
		// Batch errors include the failed change description.
		if len(changes) > 1 {
			err = WrapDatabaseOperation(change.Description, err)
		}
		if session.JoinsUserTransaction {
			session.ReportFailed(err)
		} else {
			// The error reported is the one of the statement, not of a failed rollback.
			rollBackWhatFailed(ctx, session)
		}
		return err
	}

	if !session.JoinsUserTransaction {
		if err := session.Commit(ctx); err != nil {
			// A failed commit can leave the transaction open.
			rollBackWhatFailed(ctx, session)
			return err
		}
	}
	return nil
}

// SideConnection is a lazily opened connection for independent server operations.
type SideConnection[T any] struct {
	open func() (T, error)
	// The mutex permits one connection open at a time.
	guard      sync.Mutex
	connection T
	opened     bool
}

// NewSideConnection holds how a second connection is opened.
func NewSideConnection[T any](open func() (T, error)) *SideConnection[T] {
	return &SideConnection[T]{open: open}
}

// Read returns the second connection, and opens it on the first call.
func (side *SideConnection[T]) Read() (T, error) {
	side.guard.Lock()
	defer side.guard.Unlock()
	if side.opened {
		return side.connection, nil
	}
	connection, err := side.open()
	if err != nil {
		var empty T
		return empty, err
	}
	side.connection = connection
	side.opened = true
	return connection, nil
}

// Find returns the connection if one was opened, for a caller that has to close it.
func (side *SideConnection[T]) Find() (T, bool) {
	side.guard.Lock()
	defer side.guard.Unlock()
	return side.connection, side.opened
}

// ReadAnyText writes any catalog value as text, whatever the driver handed over.
func ReadAnyText(value any) string {
	if written, isText := value.(string); isText {
		return written
	}
	return ReadCatalogText(value)
}

// ReadCatalogText writes a catalog value as text, for the shapes a driver returns.
func ReadCatalogText(value any) string {
	switch held := value.(type) {
	case nil:
		return ""
	case string:
		return held
	case []byte:
		return string(held)
	case int32:
		// PostgreSQL `"char"` values arrive as int32 byte codes. Larger values are numeric counts.
		if held > 0 && held < utf8.RuneSelf {
			return string(rune(held))
		}
		return strconv.FormatInt(int64(held), 10)
	}
	return ""
}

func JoinQuoted(names []string, quote func(string) string) string {
	written := make([]string, 0, len(names))
	for _, name := range names {
		written = append(written, quote(name))
	}
	return strings.Join(written, ", ")
}

// ScanRow reads one row of a result as plain values.
func ScanRow(rows *sql.Rows, width int, binary ...bool) ([]any, error) {
	holders := make([]any, width)
	targets := make([]any, width)
	for at := range holders {
		targets[at] = &holders[at]
	}
	if err := rows.Scan(targets...); err != nil {
		return nil, err
	}
	values := make([]any, width)
	for at, held := range holders {
		values[at] = readTextBytes(held, at < len(binary) && binary[at])
	}
	return values, nil
}

// ReadChangeGuard reads back the guard a change carries. A change built for
// another engine is refused here, not sent to the driver.
func ReadChangeGuard(change Change) (BoundStatement, bool, error) {
	if change.Guard == nil {
		return BoundStatement{}, false, nil
	}
	guard, built := change.Guard.(BoundStatement)
	if !built {
		return BoundStatement{}, false,
			core.NewEditError("this change was not built for this session")
	}
	return guard, true, nil
}

// checkChangeGuard rejects a change if the matched row count differs from the expected count.
func checkChangeGuard(ctx context.Context, change Change, session ChangeApplication) error {
	if session.CountMatches == nil {
		return nil
	}
	guard, guarded, err := ReadChangeGuard(change)
	if err != nil {
		return err
	}
	if !guarded {
		return nil
	}
	matched, err := session.CountMatches(ctx, guard)
	if err != nil {
		return err
	}
	if matched == int64(change.Expect) {
		return nil
	}
	return core.NewEditError(fmt.Sprintf(
		"%s matches %d rows on the server; expected %d. "+
			"The change was not sent. Add a primary key or filter the result.",
		change.Description, matched, change.Expect))
}

// ReadChangeStatement returns the statement of a change, or reports that it was built
// elsewhere.
func ReadChangeStatement(change Change) (BoundStatement, error) {
	statement, built := change.Payload.(BoundStatement)
	if !built {
		return BoundStatement{}, core.NewEditError("this change was not built for this session")
	}
	return statement, nil
}

// binaryColumnTypes is the set of binary column types. Drivers can return both binary and text values as bytes.
var binaryColumnTypes = map[string]bool{
	"binary": true, "varbinary": true, "blob": true, "tinyblob": true,
	"mediumblob": true, "longblob": true, "geometry": true, "bytea": true,
}

// IsBinaryColumnType is true for a column that holds bytes rather than text.
func IsBinaryColumnType(dataType string) bool {
	return binaryColumnTypes[strings.ToLower(strings.TrimSpace(dataType))]
}

// readTextBytes converts valid UTF-8 text bytes to strings. Binary columns and invalid UTF-8 remain bytes.
func readTextBytes(value any, binary bool) any {
	written, isBytes := value.([]byte)
	if !isBytes {
		return value
	}
	if binary || !utf8.Valid(written) {
		return written
	}
	return string(written)
}
