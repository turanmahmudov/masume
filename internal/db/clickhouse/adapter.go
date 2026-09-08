package clickhouse

import (
	"context"
	"database/sql"
	stddriver "database/sql/driver"
	"strings"
	"sync"
	"time"

	driver "github.com/ClickHouse/clickhouse-go/v2"

	"github.com/turanmahmudov/masume/internal/cfg"
	"github.com/turanmahmudov/masume/internal/core"
	"github.com/turanmahmudov/masume/internal/db"
	"github.com/turanmahmudov/masume/internal/query/statement"
	"github.com/turanmahmudov/masume/internal/query/syntax"
)

// clickhouseSession is one session on a ClickHouse.
type clickhouseSession struct {
	db.SessionFacts
	// The server holds no transaction of the user.
	db.NoUserTransactions

	connection *sql.Conn
	pool       *sql.DB
	side       *db.SideConnection[*sql.DB]
	// True where the server keeps the log this client reads statement counters from.
	holdsStatementStats bool
	// True where the connection cannot take another statement. A stopped statement leaves
	// the socket of this driver unusable.
	broken bool

	// The guard protects the id of the running statement and the ids of the activity list.
	guard sync.Mutex
	// The id of the statement the user connection runs now, which a stop needs.
	runningID string
	// The query the client listed at each row of the activity list. The server names a
	// query by a text id, and the list of this client counts its rows instead.
	listedIDs map[int64]string

	// mainQueue serializes user connection calls. The single-connection catalog pool
	// serializes its own calls.
	mainQueue *db.CallQueue
}

// Capabilities returns engine support plus statement statistics availability from the
// connected server.
func (session *clickhouseSession) Capabilities() core.Capabilities {
	held := session.Support.Capabilities
	held.ReportsStatementStats = session.holdsStatementStats
	return held
}

// queryRunner is what a catalog read needs of a connection or a pool.
type queryRunner interface {
	QueryContext(ctx context.Context, sql string, params ...any) (*sql.Rows, error)
	ExecContext(ctx context.Context, sql string, params ...any) (sql.Result, error)
}

// readCatalogRunner returns the pool the catalog reads run on.
func (session *clickhouseSession) readCatalogRunner() (queryRunner, error) {
	side, err := session.side.Read()
	if err != nil {
		return nil, err
	}
	return side, nil
}

// holdRunningID records the statement the connection runs, and gives it back afterwards.
func (session *clickhouseSession) holdRunningID(id string) func() {
	session.guard.Lock()
	session.runningID = id
	session.guard.Unlock()
	return func() {
		session.guard.Lock()
		if session.runningID == id {
			session.runningID = ""
		}
		session.guard.Unlock()
	}
}

// readRunningID returns the id of the statement the connection runs now.
func (session *clickhouseSession) readRunningID() string {
	session.guard.Lock()
	defer session.guard.Unlock()
	return session.runningID
}

// noteFailedStatement replaces the connection a statement failed on.
//
// A statement that ends before its answer does leaves the rest of that answer on the
// socket, and the next statement of the user reads it instead of its own. This driver
// reports such a connection in three ways and none of them reliably: a context that is
// done, a bad connection, or the timeout of the socket it set from the deadline of the
// call. So a failed statement replaces the connection, whatever the reason it failed for.
func (session *clickhouseSession) noteFailedStatement(ctx context.Context) {
	session.broken = true
	// A connection that does not open now is opened by the next call.
	_ = session.reopenBrokenConnection(ctx)
}

// reopenBrokenConnection opens the user connection again, with the settings the session
// holds. The server keeps no transaction, so nothing of the user is lost with it.
func (session *clickhouseSession) reopenBrokenConnection(ctx context.Context) error {
	if !session.broken {
		return nil
	}
	// The pool takes the old connection back and hands it out again, because a driver does
	// not always report one as bad. This marks it bad itself, so the pool drops it and the
	// connection that follows is a new one. The call takes a time limit of its own: the one
	// of the statement it failed in is spent.
	_ = session.connection.Raw(func(any) error { return stddriver.ErrBadConn })
	_ = session.connection.Close()
	held, stop := context.WithTimeout(context.WithoutCancel(ctx), reopenWait)
	defer stop()
	opened, err := session.pool.Conn(held)
	if err != nil {
		return db.WrapDatabaseMessage(
			db.BuildConnectMessage(session.Descriptor.Profile, err), err)
	}
	session.connection = opened
	session.broken = false
	if session.Descriptor.Profile.AccessMode == cfg.AccessReadOnly {
		if _, setErr := opened.ExecContext(held, readOnlyStatement); setErr != nil {
			return db.WrapDatabaseMessage(describeFailure(setErr), setErr)
		}
	}
	return nil
}

// buildStatementContext returns the context that carries the id of one statement.
func buildStatementContext(ctx context.Context, id string) context.Context {
	if id == "" {
		return ctx
	}
	return driver.Context(ctx, driver.WithQueryID(id))
}

func (session *clickhouseSession) RunQuery(
	ctx context.Context, sql string, rowLimit int, params []any,
) (db.QueryResult, error) {
	startedAt := time.Now()
	giveBack, waitErr := session.mainQueue.Take(ctx)
	if waitErr != nil {
		return db.QueryResult{}, db.WrapDatabaseError(waitErr)
	}
	defer giveBack()

	if err := session.reopenBrokenConnection(ctx); err != nil {
		return db.QueryResult{}, err
	}

	flavour := session.Support.Dialect.Syntax
	statements := statement.SplitStatements(sql, flavour)
	if len(statements) == 0 {
		return db.QueryResult{}, nil
	}
	// The protocol takes one statement per call, so a buffer of several runs one at a time
	// and the values of the user belong to a buffer that holds one.
	if len(statements) > 1 && len(params) > 0 {
		return db.QueryResult{}, db.NewDatabaseError(
			"only one statement at a time can bind values on this server")
	}

	result := db.QueryResult{}
	for _, held := range statements {
		answered, err := session.runOne(ctx, held, rowLimit, params)
		if err != nil {
			return db.QueryResult{}, err
		}
		result = answered
	}
	result.Elapsed = time.Since(startedAt)
	return result, nil
}

// runOne runs one statement of a buffer and returns what it answered.
func (session *clickhouseSession) runOne(
	ctx context.Context, written string, rowLimit int, params []any,
) (db.QueryResult, error) {
	flavour := session.Support.Dialect.Syntax
	command := syntax.ReadCommandWord(written, flavour)
	id := buildQueryID()
	release := session.holdRunningID(id)
	defer release()
	held := buildStatementContext(ctx, id)

	if !ReadsRows(command) {
		if _, err := session.connection.ExecContext(held, written, params...); err != nil {
			session.noteFailedStatement(ctx)
			return db.QueryResult{}, db.WrapDatabaseMessage(describeFailure(err), err)
		}
		// The server reports no count for a write, so the result names the command alone.
		return db.QueryResult{Command: strings.ToUpper(command)}, nil
	}

	rows, err := session.connection.QueryContext(held, written, params...)
	if err != nil {
		session.noteFailedStatement(ctx)
		return db.QueryResult{}, db.WrapDatabaseMessage(describeFailure(err), err)
	}
	defer func() { _ = rows.Close() }()

	read, columns, readErr := readClickhouseRows(rows, db.ReadOverscanRowLimit(rowLimit))
	if readErr != nil {
		session.noteFailedStatement(ctx)
		return db.QueryResult{}, db.WrapDatabaseMessage(describeFailure(readErr), readErr)
	}
	return db.BuildCappedResult(db.CappedRead{
		Rows: read, RowLimit: rowLimit, Columns: columns,
		Command: strings.ToUpper(command),
	}), nil
}

func (session *clickhouseSession) ReadPage(
	ctx context.Context, read db.ComposedRead, window db.ReadWindow,
) (db.QueryResult, error) {
	return db.ReadSQLPage(ctx, session.RunQuery, read, window, session.Support.Dialect)
}

func (session *clickhouseSession) CountRead(
	ctx context.Context, read db.ComposedRead,
) (int64, bool, error) {
	return db.CountSQLRead(ctx, session.RunQuery, read, session.Support.Dialect)
}

// CheckStatement asks the server to plan the statement, which reads every name in it and
// runs nothing. Only a read is planned.
func (session *clickhouseSession) CheckStatement(
	ctx context.Context, sql string,
) (db.StatementProblem, bool) {
	trimmed := strings.TrimSpace(sql)
	flavour := session.Support.Dialect.Syntax
	if trimmed == "" || db.HoldsSeveralCommands(sql, flavour) {
		return db.StatementProblem{}, false
	}
	if !PlansStatement(syntax.ReadCommandWord(sql, flavour)) {
		return db.StatementProblem{}, false
	}

	side, err := session.side.Read()
	if err != nil {
		return db.StatementProblem{}, false
	}
	rows, planErr := side.QueryContext(ctx, planPrefix+trimmed)
	if planErr == nil {
		_ = rows.Close()
		return db.StatementProblem{}, false
	}
	return ReadStatementProblem(planErr, planPrefix)
}

// StreamQuery reads a batch at a time, so an export never holds the whole relation.
func (session *clickhouseSession) StreamQuery(
	ctx context.Context, sql string, params []any, batchSize int,
	onBatch func(rows [][]any, columns []db.ResultColumn) error,
) (int64, error) {
	if err := db.RefuseSeveralCommands(sql, session.Support.Dialect.Syntax); err != nil {
		return 0, err
	}
	giveBack, waitErr := session.mainQueue.Take(ctx)
	if waitErr != nil {
		return 0, waitErr
	}
	defer giveBack()

	if err := session.reopenBrokenConnection(ctx); err != nil {
		return 0, err
	}

	id := buildQueryID()
	release := session.holdRunningID(id)
	defer release()

	rows, err := session.connection.QueryContext(
		buildStatementContext(ctx, id), sql, params...)
	if err != nil {
		session.noteFailedStatement(ctx)
		return 0, db.WrapDatabaseMessage(describeFailure(err), err)
	}
	defer func() { _ = rows.Close() }()

	columns, reader, columnErr := buildClickhouseColumns(rows)
	if columnErr != nil {
		return 0, columnErr
	}
	if columns == nil {
		return 0, nil
	}

	batcher := db.NewRowBatcher(batchSize, onBatch)
	for rows.Next() {
		values, scanErr := reader.readRow(rows)
		if scanErr != nil {
			return batcher.CountRows(), scanErr
		}
		if batchErr := batcher.AddRow(values, columns); batchErr != nil {
			return batcher.CountRows(), batchErr
		}
	}
	if rows.Err() != nil {
		session.noteFailedStatement(ctx)
		return batcher.CountRows(), rows.Err()
	}
	if batchErr := batcher.FlushRows(columns); batchErr != nil {
		return batcher.CountRows(), batchErr
	}
	return batcher.CountRows(), nil
}

// readNamedRows reads a catalog result as rows keyed by column name, and returns the order
// the server named them in.
func (session *clickhouseSession) readNamedRows(
	ctx context.Context, sql string, params ...any,
) ([]map[string]any, []string, error) {
	runner, err := session.readCatalogRunner()
	if err != nil {
		return nil, nil, err
	}
	rows, queryErr := runner.QueryContext(ctx, sql, params...)
	if queryErr != nil {
		return nil, nil, queryErr
	}
	defer func() { _ = rows.Close() }()

	names, nameErr := rows.Columns()
	if nameErr != nil {
		return nil, nil, nameErr
	}
	_, reader, readerErr := buildClickhouseColumns(rows)
	if readerErr != nil {
		return nil, nil, readerErr
	}

	read := []map[string]any{}
	for rows.Next() {
		values, scanErr := reader.readRow(rows)
		if scanErr != nil {
			return nil, nil, scanErr
		}
		row := map[string]any{}
		for at, name := range names {
			row[name] = values[at]
		}
		read = append(read, row)
	}
	return read, names, rows.Err()
}

// ExplainQuery returns the plan of a read, which the server writes as one line per step.
func (session *clickhouseSession) ExplainQuery(
	ctx context.Context, sql string, analyze bool,
) (db.QueryPlan, error) {
	if analyze {
		return db.QueryPlan{}, db.NewDatabaseError(
			"measured query plans are unsupported; request an estimated plan")
	}
	if err := db.RefuseSeveralPlans(sql, session.Support.Dialect.Syntax); err != nil {
		return db.QueryPlan{}, err
	}
	giveBack, waitErr := session.mainQueue.Take(ctx)
	if waitErr != nil {
		return db.QueryPlan{}, db.WrapDatabaseError(waitErr)
	}
	defer giveBack()

	if err := session.reopenBrokenConnection(ctx); err != nil {
		return db.QueryPlan{}, err
	}

	rows, err := session.connection.QueryContext(ctx, BuildExplainStatement(sql))
	if err != nil {
		session.noteFailedStatement(ctx)
		return db.QueryPlan{}, db.WrapDatabaseMessage(describeFailure(err), err)
	}
	defer func() { _ = rows.Close() }()

	read, columns, readErr := readClickhouseRows(rows, -1)
	if readErr != nil {
		return db.QueryPlan{}, db.WrapDatabaseMessage(describeFailure(readErr), readErr)
	}
	plan, built := BuildPlan(db.QueryResult{Columns: columns, Rows: read})
	if !built {
		return db.QueryPlan{}, db.FailUnreadablePlan()
	}
	return plan, nil
}

// ApplyChanges applies changes in order. The server holds no transaction, so a change that
// fails leaves the changes before it applied, and the error names the change that failed.
func (session *clickhouseSession) ApplyChanges(ctx context.Context, changes []db.Change) error {
	if len(changes) == 0 {
		return nil
	}
	giveBack, waitErr := session.mainQueue.Take(ctx)
	if waitErr != nil {
		return db.WrapDatabaseError(waitErr)
	}
	defer giveBack()

	if err := session.reopenBrokenConnection(ctx); err != nil {
		return err
	}

	return db.ApplyChangesInTransaction(ctx, changes, db.ChangeApplication{
		// Every change stands on its own, so nothing opens, commits or rolls back.
		Begin:    func(context.Context) error { return nil },
		Commit:   func(context.Context) error { return nil },
		Rollback: func(context.Context) error { return nil },
		Apply: func(ctx context.Context, change db.Change) error {
			held, err := db.ReadChangeStatement(change)
			if err != nil {
				return err
			}
			if _, execErr := session.connection.ExecContext(
				ctx, held.SQL, held.Params...); execErr != nil {
				session.noteFailedStatement(ctx)
				return db.WrapDatabaseMessage(describeFailure(execErr), execErr)
			}
			return nil
		},
		CountMatches: db.BuildGuardCounter(
			func(ctx context.Context, sql string, params ...any) db.ScanOneRow {
				return session.connection.QueryRowContext(ctx, sql, params...)
			}),
		ReportFailed: func(error) {},
	})
}

func (session *clickhouseSession) ListActivity(ctx context.Context) ([]db.Activity, error) {
	rows, _, err := session.readNamedRows(ctx, listActivitySQL)
	if err != nil {
		return nil, err
	}

	activity := make([]db.Activity, 0, len(rows))
	listed := map[int64]string{}
	for at, row := range rows {
		// The server names a query with a text of its own, so the list counts its rows and
		// keeps the name of each one for a stop.
		id := int64(at + 1)
		listed[id] = db.ReadAnyText(row["query_id"])
		activity = append(activity, db.Activity{
			PID: id, User: db.ReadAnyText(row["user"]),
			ApplicationName: db.ReadAnyText(row["client"]),
			ClientAddress:   db.ReadAnyText(row["address"]),
			State:           db.ReadAnyText(row["state"]),
			Duration: time.Duration(db.ReadNonNegativeCount(row["duration_ms"])) *
				time.Millisecond,
			Query: db.ReadAnyText(row["query"]),
		})
	}
	session.guard.Lock()
	session.listedIDs = listed
	session.guard.Unlock()
	return activity, nil
}

// findListedID returns the query the activity list held at that row.
func (session *clickhouseSession) findListedID(pid int64) (string, bool) {
	session.guard.Lock()
	defer session.guard.Unlock()
	id, held := session.listedIDs[pid]
	return id, held && id != ""
}

// ReadServerLoad returns the connections of the server, its connection limit and the time
// it started.
func (session *clickhouseSession) ReadServerLoad(ctx context.Context) (db.ServerLoad, error) {
	rows, _, err := session.readNamedRows(ctx, readServerLoadSQL)
	if err != nil {
		return db.ServerLoad{}, err
	}
	if len(rows) == 0 {
		return db.ServerLoad{}, nil
	}
	load := db.ServerLoad{
		Connections:    db.ReadNonNegativeCount(rows[0]["connections"]),
		MaxConnections: db.ReadNonNegativeCount(rows[0]["max_connections"]),
	}
	if seconds := db.ReadNonNegativeCount(rows[0]["uptime_seconds"]); seconds > 0 {
		load.StartedAt = time.Now().Add(-time.Duration(seconds) * time.Second)
	}
	return load, nil
}

// ListLockWaits is refused, because the server takes no lock a session waits for.
func (session *clickhouseSession) ListLockWaits(context.Context) ([]db.LockWait, error) {
	return nil, db.NewUnsupportedError("report lock waits")
}

// ListSlowStatements returns the statements the server spent the most time in, the slowest
// by mean time first.
func (session *clickhouseSession) ListSlowStatements(
	ctx context.Context, limit int,
) ([]db.StatementStat, error) {
	if !session.holdsStatementStats {
		return nil, db.NewUnsupportedError("report slow statements")
	}
	rows, _, err := session.readNamedRows(ctx, listSlowStatementsSQL, limit)
	if err != nil {
		return nil, err
	}
	stats := make([]db.StatementStat, 0, len(rows))
	for _, row := range rows {
		stats = append(stats, db.StatementStat{
			Query: db.ReadAnyText(row["query"]),
			Calls: db.ReadNonNegativeCount(row["calls"]),
			MeanTime: time.Duration(db.ReadNonNegativeCount(row["mean_ms"])) *
				time.Millisecond,
			TotalTime: time.Duration(db.ReadNonNegativeCount(row["total_ms"])) *
				time.Millisecond,
			Rows: db.ReadNonNegativeCount(row["rows_read"]),
		})
	}
	return stats, nil
}

// CancelBackend stops the statement the activity list held at that row. A query of this
// server belongs to no session that can be closed, so a stop ends the statement alone.
func (session *clickhouseSession) CancelBackend(
	ctx context.Context, pid int64, _ bool,
) (bool, error) {
	id, held := session.findListedID(pid)
	if !held {
		return false, db.NewDatabaseError("this statement is no longer in the list")
	}
	return session.killQuery(ctx, id)
}

// CancelRunningQuery stops the statement of this connection through a second connection,
// because the busy one cannot answer.
func (session *clickhouseSession) CancelRunningQuery(ctx context.Context) (bool, error) {
	id := session.readRunningID()
	if id == "" {
		return false, db.NewDatabaseError("this connection runs no statement")
	}
	return session.killQuery(ctx, id)
}

// killQuery stops one statement of the server, named by its id.
func (session *clickhouseSession) killQuery(
	ctx context.Context, id string,
) (bool, error) {
	side, err := session.side.Read()
	if err != nil {
		return false, err
	}
	if _, execErr := side.ExecContext(ctx, killQuerySQL, id); execErr != nil {
		return false, db.WrapDatabaseMessage(describeFailure(execErr), execErr)
	}
	return true, nil
}

// Ping reads the catalog pool, which answers while the user connection is busy.
func (session *clickhouseSession) Ping(ctx context.Context) error {
	runner, err := session.readCatalogRunner()
	if err != nil {
		return err
	}
	_, execErr := runner.ExecContext(ctx, "select 1")
	return execErr
}

// readOnlyStatement opens a session the server refuses every write on. Mode 2 refuses a
// write and takes a setting; mode 1 refuses a setting as well, and the driver sends one for
// every statement that carries a time limit.
const readOnlyStatement = "set readonly = 2"

// reopenWait is the time limit for opening the connection again after a failure.
const reopenWait = 15 * time.Second

// closeWait is the time limit for waiting to close the connection.
const closeWait = 5 * time.Second

func (session *clickhouseSession) Close() error {
	ctx, stop := context.WithTimeout(context.Background(), closeWait)
	defer stop()
	if side, opened := session.side.Find(); opened {
		_ = side.Close()
	}
	if giveBack, waitErr := session.mainQueue.Take(ctx); waitErr == nil {
		defer giveBack()
	}
	err := session.connection.Close()
	_ = session.pool.Close()
	return err
}

// clickhouseAdapter opens a connection on a ClickHouse.
type clickhouseAdapter struct {
	support db.EngineSupport
}

// NewAdapter returns the adapter of the ClickHouse engine.
func NewAdapter(support db.EngineSupport) db.Adapter {
	return &clickhouseAdapter{support: support}
}

func (adapter *clickhouseAdapter) Connect(
	ctx context.Context, profile cfg.Profile, password string,
) (db.Session, error) {
	pool := openClickhousePool(profile, password)

	// One connection holds the settings of the session. The connection takes no deadline
	// of the caller: this driver would set that deadline on the socket for the life of the
	// connection, and every later statement would find it closed. The dial time limit of
	// the driver bounds this call.
	connection, connectionErr := pool.Conn(context.WithoutCancel(ctx))
	if connectionErr != nil {
		_ = pool.Close()
		return nil, db.WrapDatabaseMessage(
			db.BuildConnectMessage(profile, connectionErr), connectionErr)
	}

	fail := func(reason error) (db.Session, error) {
		_ = connection.Close()
		_ = pool.Close()
		return nil, db.WrapDatabaseMessage(db.BuildConnectMessage(profile, reason), reason)
	}
	if profile.AccessMode == cfg.AccessReadOnly {
		if _, err := connection.ExecContext(ctx, readOnlyStatement); err != nil {
			return fail(err)
		}
	}

	return &clickhouseSession{
		SessionFacts: db.SessionFacts{
			Descriptor: db.SessionDescriptor{
				Profile: profile, ServerVersion: readServerVersion(ctx, connection),
				// A ClickHouse schema is a database. The connected one is the default.
				DefaultSchema: profile.Database,
			},
			Support: adapter.support,
		},
		connection: connection, pool: pool,
		holdsStatementStats: readsStatementStats(ctx, connection),
		listedIDs:           map[int64]string{},
		side: db.NewSideConnection(func() (*sql.DB, error) {
			return openClickhousePool(profile, password), nil
		}),
		mainQueue: db.NewCallQueue(),
	}, nil
}

// readServerVersion returns the version of the server, as it writes it itself.
func readServerVersion(ctx context.Context, connection *sql.Conn) string {
	var written string
	if connection.QueryRowContext(
		ctx, "select version()").Scan(&written) != nil || written == "" {
		return "unknown"
	}
	return written
}

// readsStatementStats is true where the server keeps the log of the statements it ran.
func readsStatementStats(ctx context.Context, connection *sql.Conn) bool {
	var counted uint64
	return connection.QueryRowContext(
		ctx, "select count() from system.query_log limit 1").Scan(&counted) == nil
}

// Compile-time Session interface check.
var _ db.Session = (*clickhouseSession)(nil)
