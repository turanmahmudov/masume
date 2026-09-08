package sqlserver

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"strings"
	"time"

	"github.com/turanmahmudov/masume/internal/cfg"
	"github.com/turanmahmudov/masume/internal/core"
	"github.com/turanmahmudov/masume/internal/db"
	"github.com/turanmahmudov/masume/internal/query/statement"
	"github.com/turanmahmudov/masume/internal/query/syntax"
)

// sqlserverSession is one session on a SQL Server.
type sqlserverSession struct {
	db.SessionFacts

	connection *sql.Conn
	pool       *sql.DB
	side       *db.SideConnection[*sql.DB]
	// True where the connection may read the statement counters of the server.
	holdsStatementStats bool
	// True where the connection cannot take another statement. A stopped statement leaves
	// the socket of this driver unusable.
	broken      bool
	transaction db.TransactionMark

	// mainQueue serializes user connection calls. The single-connection catalog pool
	// serializes its own calls.
	mainQueue *db.CallQueue
}

// Capabilities returns engine support plus statement statistics availability from the
// connected server.
func (session *sqlserverSession) Capabilities() core.Capabilities {
	held := session.Support.Capabilities
	held.ReportsStatementStats = session.holdsStatementStats
	return held
}

func (session *sqlserverSession) ReadTransactionState() db.TransactionState {
	return session.transaction.ReadState()
}

// readServerTransaction reads the transaction state of the server back after a failure.
// An uncommittable transaction takes no further statement, and a statement of its own can
// have rolled the whole transaction back.
func (session *sqlserverSession) readServerTransaction(ctx context.Context) {
	if session.transaction.ReadState() == db.TransactionNone {
		return
	}
	held, stop := context.WithTimeout(context.WithoutCancel(ctx), stateWait)
	defer stop()

	var state int64
	if session.connection.QueryRowContext(
		held, "select xact_state() as state").Scan(&state) != nil {
		return
	}
	switch state {
	case transactionUncommittable:
		session.transaction.MarkFailed()
	case transactionNone:
		session.transaction.WriteState(db.TransactionNone)
	}
}

// The states xact_state() returns.
const (
	transactionUncommittable = -1
	transactionNone          = 0
)

// markBroken records a connection that cannot take another statement, and opens it again at
// once. A statement the client stopped leaves the socket unusable, and so does a connection
// the server closed. The statement of the user has failed already, so the wait belongs to
// this call and not to the next one, which carries a time limit of its own.
func (session *sqlserverSession) markBroken(ctx context.Context, err error) {
	if !db.IsBrokenConnection(ctx, err) {
		return
	}
	session.broken = true
	if session.transaction.ReadState() != db.TransactionNone {
		// The next call reports the transaction that was lost with the connection.
		return
	}
	// A connection that does not open now is opened by the next call.
	_ = session.reopenBrokenConnection(ctx)
}

// reopenBrokenConnection opens the user connection again. A transaction is lost with the
// connection, so one that was open is reported instead of carried on.
func (session *sqlserverSession) reopenBrokenConnection(ctx context.Context) error {
	if !session.broken {
		return nil
	}
	lost := session.transaction.ReadState() != db.TransactionNone
	session.transaction.WriteState(db.TransactionNone)

	// The pool takes the old connection back and hands it out again, because a driver does
	// not always report one as bad. This marks it bad itself, so the pool drops it and the
	// connection that follows is a new one. The call takes a time limit of its own: the one
	// of the statement it failed in is spent.
	_ = session.connection.Raw(func(any) error { return driver.ErrBadConn })
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
	if lost {
		return db.NewDatabaseError("the connection closed and the transaction was lost")
	}
	return nil
}

// stateWait is the time limit for one call the client makes after a failure.
const stateWait = 5 * time.Second

// reopenWait is the time limit for opening the connection again after a failure.
const reopenWait = 15 * time.Second

// queryRunner is what a catalog read needs of a connection or a pool.
type queryRunner interface {
	QueryContext(ctx context.Context, sql string, params ...any) (*sql.Rows, error)
	ExecContext(ctx context.Context, sql string, params ...any) (sql.Result, error)
}

// resolveCatalogRunner uses the user connection during a transaction and the catalog pool
// otherwise.
func (session *sqlserverSession) resolveCatalogRunner() (queryRunner, error) {
	if session.transaction.ReadState() != db.TransactionNone {
		return session.connection, nil
	}
	side, err := session.side.Read()
	if err != nil {
		return nil, err
	}
	return side, nil
}

// holdCatalogPool reserves the user connection for transactional catalog reads and returns
// a release callback.
func (session *sqlserverSession) holdCatalogPool(
	ctx context.Context,
) (queryRunner, func(), error) {
	runner, err := session.resolveCatalogRunner()
	if err != nil {
		return nil, nil, err
	}
	if session.transaction.ReadState() == db.TransactionNone {
		return runner, func() {}, nil
	}
	giveBack, waitErr := session.mainQueue.Take(ctx)
	if waitErr != nil {
		return nil, nil, waitErr
	}
	return runner, giveBack, nil
}

func (session *sqlserverSession) RunQuery(
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
	lastStatement := db.ReadLastStatement(sql, flavour)
	command := syntax.ReadCommandWord(lastStatement, flavour)
	writes := db.IsWriteCommand(command) && !HoldsOutputClause(lastStatement, flavour)
	batch := sql
	if writes {
		// The count of a write belongs to the batch that wrote it: a later batch reads
		// the count of its own last statement.
		batch += rowCountProbe
	}

	answered, err := session.connection.QueryContext(ctx, batch, params...)
	if err != nil {
		session.markBroken(ctx, err)
		session.readServerTransaction(ctx)
		return db.QueryResult{}, db.WrapDatabaseMessage(describeFailure(err), err)
	}
	defer func() { _ = answered.Close() }()

	// The last result set of the batch is the one the pane shows.
	read, columns, readErr := readLastResultSet(answered, db.ReadOverscanRowLimit(rowLimit))
	if readErr != nil {
		session.markBroken(ctx, readErr)
		session.readServerTransaction(ctx)
		return db.QueryResult{}, db.WrapDatabaseMessage(describeFailure(readErr), readErr)
	}

	result := db.QueryResult{
		Elapsed: time.Since(startedAt), Command: strings.ToUpper(command),
	}
	if writes {
		result.Affected, result.HasAffected = readAffected(read)
	} else {
		result = db.BuildCappedResult(db.CappedRead{
			Rows: read, RowLimit: rowLimit, Columns: columns,
			Elapsed: result.Elapsed, Command: result.Command,
		})
	}
	session.markTransactionFromStatement(sql)
	return result, nil
}

// rowCountProbe is the statement that reads the count of the write before it.
const rowCountProbe = "\n;select @@rowcount as affected"

// readAffected reads the count of a write out of the rows the probe answered.
func readAffected(rows [][]any) (int64, bool) {
	if len(rows) == 0 || len(rows[0]) == 0 {
		return 0, false
	}
	return db.ReadNonNegativeCount(rows[0][0]), true
}

// markTransactionFromStatement updates transaction state after statements from the editor.
func (session *sqlserverSession) markTransactionFromStatement(sql string) {
	session.transaction.ApplyStatementEffect(
		statement.ResolveTransactionEffect(sql, session.Support.Dialect.Syntax))
}

func (session *sqlserverSession) ReadPage(
	ctx context.Context, read db.ComposedRead, window db.ReadWindow,
) (db.QueryResult, error) {
	return db.ReadSQLPage(ctx, session.RunQuery, read, window, session.Support.Dialect)
}

func (session *sqlserverSession) CountRead(
	ctx context.Context, read db.ComposedRead,
) (int64, bool, error) {
	return db.CountSQLRead(ctx, session.RunQuery, read, session.Support.Dialect)
}

// CheckStatement asks the server to compile the statement without running it. The server
// returns the fault as a row, so no statement of the client is refused for it.
func (session *sqlserverSession) CheckStatement(
	ctx context.Context, sql string,
) (db.StatementProblem, bool) {
	if strings.TrimSpace(sql) == "" ||
		db.HoldsSeveralCommands(sql, session.Support.Dialect.Syntax) {
		return db.StatementProblem{}, false
	}
	rows, _, err := session.readNamedRows(ctx, describeStatementSQL, sql)
	if err != nil || len(rows) == 0 {
		return db.StatementProblem{}, false
	}
	return ReadStatementProblem(sql, rows)
}

// StreamQuery reads a batch at a time, so an export never holds the whole relation.
func (session *sqlserverSession) StreamQuery(
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

	rows, err := session.connection.QueryContext(ctx, sql, params...)
	if err != nil {
		session.markBroken(ctx, err)
		session.readServerTransaction(ctx)
		return 0, db.WrapDatabaseMessage(describeFailure(err), err)
	}
	defer func() { _ = rows.Close() }()

	columns, reader, columnErr := buildSqlserverColumns(rows)
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
		session.markBroken(ctx, rows.Err())
		session.readServerTransaction(ctx)
		return batcher.CountRows(), rows.Err()
	}
	if batchErr := batcher.FlushRows(columns); batchErr != nil {
		return batcher.CountRows(), batchErr
	}
	return batcher.CountRows(), nil
}

// readNamedRows reads a catalog result as rows keyed by column name, and returns the order
// the server named them in.
func (session *sqlserverSession) readNamedRows(
	ctx context.Context, sql string, params ...any,
) ([]map[string]any, []string, error) {
	runner, giveBack, err := session.holdCatalogPool(ctx)
	if err != nil {
		return nil, nil, err
	}
	defer giveBack()

	rows, queryErr := runner.QueryContext(ctx, sql, params...)
	if queryErr != nil {
		return nil, nil, queryErr
	}
	defer func() { _ = rows.Close() }()

	names, nameErr := rows.Columns()
	if nameErr != nil {
		return nil, nil, nameErr
	}

	read := []map[string]any{}
	for rows.Next() {
		values, scanErr := db.ScanRow(rows, len(names))
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

// ExplainQuery returns the plan the server writes as rows. SHOWPLAN_ALL estimates the plan,
// and STATISTICS PROFILE runs the statement and counts the rows of every step.
func (session *sqlserverSession) ExplainQuery(
	ctx context.Context, sql string, analyze bool,
) (db.QueryPlan, error) {
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

	setting := planSetting(analyze)
	if _, err := session.connection.ExecContext(ctx, "set "+setting+" on"); err != nil {
		return db.QueryPlan{}, db.WrapDatabaseMessage(describeFailure(err), err)
	}
	defer session.stopPlanSetting(ctx, setting)

	rows, err := session.connection.QueryContext(ctx, sql)
	if err != nil {
		session.markBroken(ctx, err)
		session.readServerTransaction(ctx)
		return db.QueryPlan{}, db.WrapDatabaseMessage(describeFailure(err), err)
	}
	defer func() { _ = rows.Close() }()

	read, columns, readErr := readLastResultSet(rows, -1)
	if readErr != nil {
		session.readServerTransaction(ctx)
		return db.QueryPlan{}, db.WrapDatabaseMessage(describeFailure(readErr), readErr)
	}

	plan, built := BuildPlan(db.QueryResult{Columns: columns, Rows: read}, analyze)
	if !built {
		return db.QueryPlan{}, db.FailUnreadablePlan()
	}
	return plan, nil
}

// stopPlanSetting turns the plan setting off again, whatever the statement did. The setting
// stays on the connection until it does.
func (session *sqlserverSession) stopPlanSetting(ctx context.Context, setting string) {
	held, stop := context.WithTimeout(context.WithoutCancel(ctx), stateWait)
	defer stop()
	_, _ = session.connection.ExecContext(held, "set "+setting+" off")
}

// runPlain runs a statement of the client itself, which binds nothing and returns nothing.
func (session *sqlserverSession) runPlain(ctx context.Context, sql string) error {
	_, err := session.connection.ExecContext(ctx, sql)
	session.markBroken(ctx, err)
	return err
}

func (session *sqlserverSession) BeginTransaction(ctx context.Context) error {
	giveBack, waitErr := session.mainQueue.Take(ctx)
	if waitErr != nil {
		return db.WrapDatabaseError(waitErr)
	}
	defer giveBack()

	if err := session.reopenBrokenConnection(ctx); err != nil {
		return err
	}
	if err := session.runPlain(ctx, "begin transaction"); err != nil {
		return db.WrapDatabaseMessage(describeFailure(err), err)
	}
	session.transaction.WriteState(db.TransactionOpen)
	return nil
}

func (session *sqlserverSession) CommitTransaction(ctx context.Context) error {
	giveBack, waitErr := session.mainQueue.Take(ctx)
	if waitErr != nil {
		return db.WrapDatabaseError(waitErr)
	}
	defer giveBack()

	if err := session.runPlain(ctx, "commit"); err != nil {
		return db.WrapDatabaseMessage(describeFailure(err), err)
	}
	session.transaction.WriteState(db.TransactionNone)
	return nil
}

func (session *sqlserverSession) RollbackTransaction(ctx context.Context) error {
	giveBack, waitErr := session.mainQueue.Take(ctx)
	if waitErr != nil {
		return db.WrapDatabaseError(waitErr)
	}
	defer giveBack()

	if err := session.runPlain(ctx, "rollback"); err != nil {
		return db.WrapDatabaseMessage(describeFailure(err), err)
	}
	session.transaction.WriteState(db.TransactionNone)
	return nil
}

func (session *sqlserverSession) ApplyChanges(ctx context.Context, changes []db.Change) error {
	giveBack, waitErr := session.mainQueue.Take(ctx)
	if waitErr != nil {
		return db.WrapDatabaseError(waitErr)
	}
	defer giveBack()

	if err := session.reopenBrokenConnection(ctx); err != nil {
		return err
	}

	return db.ApplyChangesInTransaction(ctx, changes, db.ChangeApplication{
		JoinsUserTransaction: session.transaction.ReadState() == db.TransactionOpen,
		Begin: func(ctx context.Context) error {
			return session.runPlain(ctx, "begin transaction")
		},
		Commit:   func(ctx context.Context) error { return session.runPlain(ctx, "commit") },
		Rollback: func(ctx context.Context) error { return session.runPlain(ctx, "rollback") },
		Apply: func(ctx context.Context, change db.Change) error {
			held, err := db.ReadChangeStatement(change)
			if err != nil {
				return err
			}
			_, execErr := session.connection.ExecContext(ctx, held.SQL, held.Params...)
			if execErr == nil {
				return nil
			}
			session.markBroken(ctx, execErr)
			return db.WrapDatabaseMessage(describeFailure(execErr), execErr)
		},
		CountMatches: db.BuildGuardCounter(
			func(ctx context.Context, sql string, params ...any) db.ScanOneRow {
				return session.connection.QueryRowContext(ctx, sql, params...)
			}),
		ReportFailed: func(error) { session.readServerTransaction(ctx) },
	})
}

func (session *sqlserverSession) ListActivity(ctx context.Context) ([]db.Activity, error) {
	rows, _, err := session.readNamedRows(ctx, listActivitySQL)
	if err != nil {
		return nil, err
	}
	activity := make([]db.Activity, 0, len(rows))
	for _, row := range rows {
		activity = append(activity, db.Activity{
			PID: db.ReadNonNegativeCount(row["pid"]), User: db.ReadAnyText(row["user"]),
			ApplicationName: db.ReadAnyText(row["application_name"]),
			ClientAddress:   db.ReadAnyText(row["client_address"]),
			State:           db.ReadAnyText(row["state"]),
			Duration: time.Duration(db.ReadNonNegativeCount(row["duration_ms"])) *
				time.Millisecond,
			Query: db.ReadAnyText(row["query"]),
		})
	}
	return activity, nil
}

// ListLockWaits returns every session waiting for a lock another session holds.
func (session *sqlserverSession) ListLockWaits(ctx context.Context) ([]db.LockWait, error) {
	rows, _, err := session.readNamedRows(ctx, listLockWaitsSQL)
	if err != nil {
		return nil, err
	}
	waits := make([]db.LockWait, 0, len(rows))
	for _, row := range rows {
		waits = append(waits, db.LockWait{
			BlockedPID:   db.ReadNonNegativeCount(row["blocked_pid"]),
			BlockedQuery: db.ReadAnyText(row["blocked_query"]),
			Waiting: time.Duration(db.ReadNonNegativeCount(row["waiting_ms"])) *
				time.Millisecond,
			Mode:          db.ReadAnyText(row["mode"]),
			Relation:      db.ReadAnyText(row["relation"]),
			BlockingPID:   db.ReadNonNegativeCount(row["blocking_pid"]),
			BlockingQuery: db.ReadAnyText(row["blocking_query"]),
			BlockingFor: time.Duration(db.ReadNonNegativeCount(row["blocking_ms"])) *
				time.Millisecond,
		})
	}
	return waits, nil
}

// ReadServerLoad returns the connections of the server, its connection limit and the time
// it started.
func (session *sqlserverSession) ReadServerLoad(ctx context.Context) (db.ServerLoad, error) {
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
	if started, isTime := rows[0]["started_at"].(time.Time); isTime {
		load.StartedAt = started
	}
	return load, nil
}

// ListSlowStatements returns the statements the server spent the most time in, the slowest
// by mean time first.
func (session *sqlserverSession) ListSlowStatements(
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
			MeanTime: time.Duration(db.ReadNonNegativeCount(row["mean_us"])) *
				time.Microsecond,
			TotalTime: time.Duration(db.ReadNonNegativeCount(row["total_us"])) *
				time.Microsecond,
			Rows: db.ReadNonNegativeCount(row["rows_back"]),
		})
	}
	return stats, nil
}

// CancelBackend stops another session through the catalog connection. KILL ends the whole
// session, and T-SQL has no statement that stops one statement of it.
func (session *sqlserverSession) CancelBackend(
	ctx context.Context, pid int64, terminate bool,
) (bool, error) {
	if !terminate {
		return false, db.NewUnsupportedError("cancel one statement of another session")
	}
	side, err := session.side.Read()
	if err != nil {
		return false, err
	}
	if _, execErr := side.ExecContext(ctx, BuildKillStatement(pid)); execErr != nil {
		return false, execErr
	}
	return true, nil
}

// CancelRunningQuery is refused, because the driver cancels through the context.
func (session *sqlserverSession) CancelRunningQuery(context.Context) (bool, error) {
	return false, db.NewUnsupportedError("cancel a running statement")
}

// Ping checks the catalog connection outside transactions. Transactional checks use the
// user connection and skip busy connections.
func (session *sqlserverSession) Ping(ctx context.Context) error {
	runner, err := session.resolveCatalogRunner()
	if err != nil {
		return err
	}
	if session.transaction.ReadState() != db.TransactionNone {
		giveBack, free := session.mainQueue.TryTake()
		if !free {
			return nil
		}
		defer giveBack()
	}
	_, execErr := runner.ExecContext(ctx, "select 1")
	return execErr
}

// closeWait is the time limit for waiting to close the connection.
const closeWait = 5 * time.Second

func (session *sqlserverSession) Close() error {
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

// sqlserverAdapter opens a connection on a SQL Server.
type sqlserverAdapter struct {
	support db.EngineSupport
}

// NewAdapter returns the adapter of the SQL Server engine.
func NewAdapter(support db.EngineSupport) db.Adapter {
	return &sqlserverAdapter{support: support}
}

func (adapter *sqlserverAdapter) Connect(
	ctx context.Context, profile cfg.Profile, password string,
) (db.Session, error) {
	pool := openSqlserverPool(profile, password)

	// One connection retains the transaction of the user.
	connection, connectionErr := pool.Conn(ctx)
	if connectionErr != nil {
		_ = pool.Close()
		return nil, db.WrapDatabaseMessage(
			db.BuildConnectMessage(profile, connectionErr), connectionErr)
	}

	return &sqlserverSession{
		SessionFacts: db.SessionFacts{
			Descriptor: db.SessionDescriptor{
				Profile: profile, ServerVersion: readServerVersion(ctx, connection),
				DefaultSchema: readDefaultSchema(ctx, connection),
			},
			Support: adapter.support,
		},
		connection: connection, pool: pool,
		holdsStatementStats: readsStatementStats(ctx, connection),
		side: db.NewSideConnection(func() (*sql.DB, error) {
			return openSqlserverPool(profile, password), nil
		}),
		mainQueue: db.NewCallQueue(),
	}, nil
}

// readServerVersion returns the version of the server, as it writes it itself.
func readServerVersion(ctx context.Context, connection *sql.Conn) string {
	var written string
	if connection.QueryRowContext(ctx,
		"select cast(serverproperty('productversion') as nvarchar(128)) as version").
		Scan(&written) != nil || written == "" {
		return "unknown"
	}
	return written
}

// readDefaultSchema returns the schema the connection reads an unqualified name in.
func readDefaultSchema(ctx context.Context, connection *sql.Conn) string {
	var written string
	if connection.QueryRowContext(
		ctx, "select schema_name() as name").Scan(&written) != nil || written == "" {
		return "dbo"
	}
	return written
}

// readsStatementStats is true where this connection may read the statement counters of the
// server, which needs the VIEW SERVER STATE permission.
func readsStatementStats(ctx context.Context, connection *sql.Conn) bool {
	var counted int64
	return connection.QueryRowContext(ctx,
		"select count_big(*) as held from sys.dm_exec_query_stats").Scan(&counted) == nil
}

// Compile-time Session interface check.
var _ db.Session = (*sqlserverSession)(nil)
