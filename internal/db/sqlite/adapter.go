package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"io/fs"
	"os"
	"strconv"
	"strings"
	"time"

	_ "modernc.org/sqlite"

	"github.com/turanmahmudov/masume/internal/cfg"
	"github.com/turanmahmudov/masume/internal/core"
	"github.com/turanmahmudov/masume/internal/db"
	"github.com/turanmahmudov/masume/internal/query/statement"
	"github.com/turanmahmudov/masume/internal/query/syntax"
)

// mainSchema is the database every SQLite file holds. A bare name is looked up here.
const mainSchema = "main"

// sqliteBusyTimeout is how long a statement waits for another process to release the
// file.
const sqliteBusyTimeout = 5 * time.Second

// generatedMarks name a generated column, which SQLite hides from a write.
var generatedMarks = map[int64]bool{2: true, 3: true}

// sqliteSession is one session on a SQLite file.
type sqliteSession struct {
	db.PlainCatalog
	db.NoServerSessions
	db.SessionFacts

	file        *sql.DB
	transaction db.TransactionMark
	// mainQueue holds the file for one caller at a time. The pool opens one connection,
	// so a statement of another goroutine would otherwise take that connection between
	// the `begin` of a staged set and its `commit`, and run inside that transaction.
	mainQueue *db.CallQueue
}

// holdFile waits for its turn on the file and returns what gives the turn back.
func (session *sqliteSession) holdFile(ctx context.Context) (func(), error) {
	giveBack, err := session.mainQueue.Take(ctx)
	if err != nil {
		return nil, db.WrapDatabaseError(err)
	}
	return giveBack, nil
}

func (session *sqliteSession) ReadTransactionState() db.TransactionState {
	return session.transaction.ReadState()
}

// markTransactionFailed records a transaction the server may already have ended.
func (session *sqliteSession) markTransactionFailed() {
	session.transaction.MarkFailed()
}

// rawRead is the result of one statement, of any kind.
type rawRead struct {
	rows    [][]any
	columns []db.ResultColumn
}

// read runs one statement and reads at most cap rows, or every row when cap is below
// zero.
func (session *sqliteSession) read(
	ctx context.Context, statement string, params []any, cap int,
) (rawRead, error) {
	rows, err := session.file.QueryContext(ctx, statement, params...)
	if err != nil {
		session.markTransactionFailed()
		return rawRead{}, err
	}
	defer func() { _ = rows.Close() }()

	names, nameErr := rows.Columns()
	if nameErr != nil {
		return rawRead{}, nameErr
	}
	// A statement with no result set changed rows instead of returning them.
	if len(names) == 0 {
		return rawRead{}, nil
	}
	types, typeErr := rows.ColumnTypes()
	if typeErr != nil {
		return rawRead{}, typeErr
	}

	read := [][]any{}
	for rows.Next() {
		values, scanErr := db.ScanRow(rows, len(names))
		if scanErr != nil {
			return rawRead{}, scanErr
		}
		read = append(read, values)
		if cap >= 0 && len(read) >= cap {
			break
		}
	}
	if rows.Err() != nil {
		session.markTransactionFailed()
		return rawRead{}, rows.Err()
	}

	var first []any
	if len(read) > 0 {
		first = read[0]
	}
	return rawRead{rows: read, columns: readColumns(names, types, first)}, nil
}

// markTransactionFromStatement records what the statement left the transaction as. A
// `begin` or a `commit` written into the editor never reaches BeginTransaction, and
// without this the mark and the file would drift apart.
func (session *sqliteSession) markTransactionFromStatement(sql string) {
	session.transaction.ApplyStatementEffect(
		statement.ResolveTransactionEffect(sql, session.Support.Dialect.Syntax))
}

// countChanges returns how many rows the last write changed. The server keeps it until
// the next write.
func (session *sqliteSession) countChanges(ctx context.Context) int64 {
	read, err := session.read(ctx, "select changes() as changed", nil, 1)
	if err != nil || len(read.rows) == 0 || len(read.rows[0]) == 0 {
		return 0
	}
	return db.ReadNonNegativeCount(read.rows[0][0])
}

// RunQuery runs the buffer statement by statement. The driver prepares only the first
// one, so the buffer is split and run in order, and the last statement gives the result.
func (session *sqliteSession) RunQuery(
	ctx context.Context, sql string, rowLimit int, params []any,
) (db.QueryResult, error) {
	giveBack, waitErr := session.holdFile(ctx)
	if waitErr != nil {
		return db.QueryResult{}, waitErr
	}
	defer giveBack()

	startedAt := time.Now()
	statements := statement.SplitStatements(sql, session.Support.Dialect.Syntax)
	if len(statements) == 0 {
		return db.QueryResult{Elapsed: time.Since(startedAt)}, nil
	}

	read := rawRead{}
	for at, statement := range statements {
		cap := -1
		if at == len(statements)-1 {
			cap = db.ReadOverscanRowLimit(rowLimit)
		}
		// Every statement of a batch takes the same values, because the client binds a
		// value only for a statement it wrote itself.
		one, err := session.read(ctx, statement, params, cap)
		if err != nil {
			return db.QueryResult{}, db.WrapDatabaseError(err)
		}
		session.markTransactionFromStatement(statement)
		read = one
	}

	command := syntax.ReadCommandWord(statements[len(statements)-1], session.Support.Dialect.Syntax)
	result := db.BuildCappedResult(db.CappedRead{
		Rows: read.rows, RowLimit: rowLimit, Columns: read.columns,
		Elapsed: time.Since(startedAt), Command: strings.ToUpper(command),
	})
	if db.IsWriteCommand(command) {
		result.Affected = session.countChanges(ctx)
		result.HasAffected = true
	}
	return result, nil
}

func (session *sqliteSession) ReadPage(
	ctx context.Context, read db.ComposedRead, window db.ReadWindow,
) (db.QueryResult, error) {
	return db.ReadSQLPage(ctx, session.RunQuery, read, window, session.Support.Dialect.Syntax)
}

func (session *sqliteSession) CountRead(
	ctx context.Context, read db.ComposedRead,
) (int64, bool, error) {
	return db.CountSQLRead(ctx, session.RunQuery, read, session.Support.Dialect)
}

// CheckStatement prepares the statement, which makes the server read it and resolve
// every name in it.
func (session *sqliteSession) CheckStatement(
	ctx context.Context, statement string,
) (db.StatementProblem, bool) {
	if strings.TrimSpace(statement) == "" ||
		db.HoldsSeveralCommands(statement, session.Support.Dialect.Syntax) {
		return db.StatementProblem{}, false
	}
	giveBack, waitErr := session.holdFile(ctx)
	if waitErr != nil {
		return db.StatementProblem{}, false
	}
	defer giveBack()

	prepared, err := session.file.PrepareContext(ctx, statement)
	if err == nil {
		_ = prepared.Close()
		return db.StatementProblem{}, false
	}
	return ReadStatementProblem(statement, err)
}

// StreamQuery reads one batch at a time, so a relation larger than memory can be
// exported.
func (session *sqliteSession) StreamQuery(
	ctx context.Context, statement string, params []any, batchSize int,
	onBatch func(rows [][]any, columns []db.ResultColumn) error,
) (int64, error) {
	if err := db.RefuseSeveralCommands(statement, session.Support.Dialect.Syntax); err != nil {
		return 0, err
	}
	giveBack, waitErr := session.holdFile(ctx)
	if waitErr != nil {
		return 0, waitErr
	}
	defer giveBack()

	rows, err := session.file.QueryContext(ctx, statement, params...)
	if err != nil {
		session.markTransactionFailed()
		return 0, err
	}
	defer func() { _ = rows.Close() }()

	names, nameErr := rows.Columns()
	if nameErr != nil {
		return 0, nameErr
	}
	if len(names) == 0 {
		return 0, nil
	}
	types, typeErr := rows.ColumnTypes()
	if typeErr != nil {
		return 0, typeErr
	}

	var columns []db.ResultColumn
	batcher := db.NewRowBatcher(batchSize, onBatch)

	for rows.Next() {
		values, scanErr := db.ScanRow(rows, len(names))
		if scanErr != nil {
			return batcher.CountRows(), scanErr
		}
		if columns == nil {
			columns = readColumns(names, types, values)
		}
		if batchErr := batcher.AddRow(values, columns); batchErr != nil {
			return batcher.CountRows(), batchErr
		}
	}
	if rows.Err() != nil {
		session.markTransactionFailed()
		return batcher.CountRows(), rows.Err()
	}
	if columns == nil {
		columns = readColumns(names, types, nil)
	}
	if batchErr := batcher.FlushRows(columns); batchErr != nil {
		return batcher.CountRows(), batchErr
	}
	return batcher.CountRows(), nil
}

// ExplainQuery returns the plan SQLite writes, which it never measures.
func (session *sqliteSession) ExplainQuery(
	ctx context.Context, statement string, analyze bool,
) (db.QueryPlan, error) {
	if analyze && !session.Support.Capabilities.MeasuresPlan {
		return db.QueryPlan{}, db.NewDatabaseError(
			"sqlite measures no plan, so only the estimated plan can be read")
	}
	if err := db.RefuseSeveralPlans(statement, session.Support.Dialect.Syntax); err != nil {
		return db.QueryPlan{}, err
	}
	giveBack, waitErr := session.holdFile(ctx)
	if waitErr != nil {
		return db.QueryPlan{}, waitErr
	}
	defer giveBack()

	read, err := session.read(ctx, "explain query plan "+statement, nil, -1)
	if err != nil {
		return db.QueryPlan{}, db.WrapDatabaseError(err)
	}
	if len(read.rows) == 0 {
		return db.QueryPlan{}, db.FailUnreadablePlan()
	}
	return buildSqlitePlan(read.rows, session.Support.Capabilities.MeasuresPlan), nil
}

func (session *sqliteSession) BeginTransaction(ctx context.Context) error {
	giveBack, waitErr := session.holdFile(ctx)
	if waitErr != nil {
		return waitErr
	}
	defer giveBack()

	if _, err := session.file.ExecContext(ctx, "begin"); err != nil {
		return db.WrapDatabaseError(err)
	}
	session.transaction.WriteState(db.TransactionOpen)
	return nil
}

func (session *sqliteSession) CommitTransaction(ctx context.Context) error {
	giveBack, waitErr := session.holdFile(ctx)
	if waitErr != nil {
		return waitErr
	}
	defer giveBack()

	if _, err := session.file.ExecContext(ctx, "commit"); err != nil {
		return db.WrapDatabaseError(err)
	}
	session.transaction.WriteState(db.TransactionNone)
	return nil
}

// RollbackTransaction ends the transaction. The server can roll back by itself, so
// there may be nothing left to do.
func (session *sqliteSession) RollbackTransaction(ctx context.Context) error {
	giveBack, waitErr := session.holdFile(ctx)
	if waitErr != nil {
		return waitErr
	}
	defer giveBack()

	_, err := session.file.ExecContext(ctx, "rollback")
	session.transaction.WriteState(db.TransactionNone)
	if err != nil && !strings.Contains(db.DescribeError(err), "no transaction") {
		return db.WrapDatabaseError(err)
	}
	return nil
}

func (session *sqliteSession) ApplyChanges(ctx context.Context, changes []db.Change) error {
	giveBack, waitErr := session.holdFile(ctx)
	if waitErr != nil {
		return waitErr
	}
	defer giveBack()

	return db.ApplyChangesInTransaction(ctx, changes, db.ChangeApplication{
		JoinsUserTransaction: session.transaction.ReadState() == db.TransactionOpen,
		Begin: func(ctx context.Context) error {
			_, err := session.file.ExecContext(ctx, "begin")
			return err
		},
		Commit: func(ctx context.Context) error {
			_, err := session.file.ExecContext(ctx, "commit")
			return err
		},
		Rollback: func(ctx context.Context) error {
			_, err := session.file.ExecContext(ctx, "rollback")
			return err
		},
		Apply: func(ctx context.Context, change db.Change) error {
			statement, err := db.ReadChangeStatement(change)
			if err != nil {
				return err
			}
			_, execErr := session.file.ExecContext(ctx, statement.SQL, statement.Params...)
			return execErr
		},
		CountMatches: db.BuildGuardCounter(
			func(ctx context.Context, sql string, params ...any) db.ScanOneRow {
				return session.file.QueryRowContext(ctx, sql, params...)
			}),
		ReportFailed: func(error) { session.markTransactionFailed() },
	})
}

// Ping asks whether the file still answers. The pool holds one connection, so a ping
// while a statement runs would wait for that statement and then look like a dead
// server. A file that is still answering a call of its own is left alone: it is
// answering, so the file is there.
func (session *sqliteSession) Ping(ctx context.Context) error {
	giveBack, free := session.mainQueue.TryTake()
	if !free {
		return nil
	}
	defer giveBack()
	return session.file.PingContext(ctx)
}

func (session *sqliteSession) Close() error {
	return session.file.Close()
}

// sqliteAdapter opens a SQLite file.
type sqliteAdapter struct{ support db.EngineSupport }

// NewAdapter returns the adapter that opens a SQLite file.
func NewAdapter(support db.EngineSupport) db.Adapter {
	return &sqliteAdapter{support: support}
}

func (adapter *sqliteAdapter) Connect(
	ctx context.Context, profile cfg.Profile, _ string,
) (db.Session, error) {
	path := core.ExpandHomePath(profile.Database)
	// The driver creates a missing file, so a wrong path would open an empty database.
	if _, err := os.Stat(path); err != nil {
		reason := err
		if errors.Is(err, fs.ErrNotExist) {
			reason = errors.New("there is no database file at this path")
		}
		return nil, db.WrapDatabaseMessage(db.BuildConnectMessage(profile, reason), err)
	}

	readOnly := profile.AccessMode == cfg.AccessReadOnly
	settings := []string{
		"_pragma=busy_timeout(" + strconv.FormatInt(sqliteBusyTimeout.Milliseconds(), 10) + ")",
	}
	if readOnly {
		settings = append(settings, "mode=ro")
	} else {
		// SQLite checks foreign keys only if the connection asks for it, so a write that
		// breaks one is refused rather than kept.
		settings = append(settings, "_pragma=foreign_keys(1)")
	}

	file, err := sql.Open("sqlite", "file:"+path+"?"+strings.Join(settings, "&"))
	if err != nil {
		return nil, db.WrapDatabaseMessage(db.BuildConnectMessage(profile, err), err)
	}
	// One connection, so a transaction and the pragmas of this session stay on it.
	file.SetMaxOpenConns(1)

	if pingErr := file.PingContext(ctx); pingErr != nil {
		_ = file.Close()
		return nil, db.WrapDatabaseMessage(db.BuildConnectMessage(profile, pingErr), pingErr)
	}

	serverVersion := "unknown"
	row := file.QueryRowContext(ctx, "select sqlite_version() as version")
	var written string
	if row.Scan(&written) == nil && written != "" {
		serverVersion = written
	}

	return &sqliteSession{
		SessionFacts: db.SessionFacts{
			Descriptor: db.SessionDescriptor{
				Profile: profile, ServerVersion: serverVersion,
				// A bare name is looked up in the file itself.
				DefaultSchema: mainSchema,
			},
			Support: adapter.support,
		},
		file: file, mainQueue: db.NewCallQueue(),
	}, nil
}

// The compiler reports a part of the port this session has not answered for.
var _ db.Session = (*sqliteSession)(nil)
