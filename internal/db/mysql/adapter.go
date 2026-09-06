package mysql

import (
	"context"
	"database/sql"
	"errors"
	"strconv"
	"strings"
	"time"

	driver "github.com/go-sql-driver/mysql"

	"github.com/turanmahmudov/masume/internal/cfg"
	"github.com/turanmahmudov/masume/internal/db"
	"github.com/turanmahmudov/masume/internal/query/statement"
	"github.com/turanmahmudov/masume/internal/query/syntax"
)

type mysqlSession struct {
	db.NoServerLoad
	db.SessionFacts

	flavour     Flavour
	connection  *sql.Conn
	pool        *sql.DB
	side        *db.SideConnection[*sql.DB]
	threadID    int64
	profile     cfg.Profile
	password    string
	transaction db.TransactionMark
	// The row cap of the connection, kept so it is not set twice.
	selectLimit    int
	hasSelectLimit bool
	// True if innodb_rollback_on_timeout enables full rollback after a lock timeout.
	rollsBackOnTimeout bool

	// mainQueue serializes user connection calls. The single-connection catalog pool serializes its own calls.
	mainQueue *db.CallQueue
}

func (session *mysqlSession) ReadTransactionState() db.TransactionState {
	return session.transaction.ReadState()
}

// markTransactionFailed records full transaction rollback errors. Ordinary MySQL errors leave the transaction open.
// Lock timeouts roll back only the statement unless innodb_rollback_on_timeout is enabled. BEGIN commits an existing MySQL transaction.
func (session *mysqlSession) markTransactionFailed(err error) {
	var reported *driver.MySQLError
	if !errors.As(err, &reported) {
		return
	}
	if reported.Number == mysqlDeadlock ||
		(reported.Number == mysqlLockTimeout && session.rollsBackOnTimeout) {
		session.transaction.MarkFailed()
	}
}

// resolveCatalogRunner uses the user connection during a transaction and the catalog pool otherwise.
func (session *mysqlSession) resolveCatalogRunner() (queryRunner, error) {
	if session.transaction.ReadState() != db.TransactionNone {
		return session.connection, nil
	}
	side, err := session.side.Read()
	if err != nil {
		return nil, err
	}
	return side, nil
}

// holdCatalogPool reserves the user connection for transactional catalog reads and returns a release callback.
func (session *mysqlSession) holdCatalogPool(
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

// queryRunner is what a catalog read needs of a connection or a pool.
type queryRunner interface {
	QueryContext(ctx context.Context, sql string, params ...any) (*sql.Rows, error)
	ExecContext(ctx context.Context, sql string, params ...any) (sql.Result, error)
}

// applySelectLimit sets the session row limit.
func (session *mysqlSession) applySelectLimit(ctx context.Context, cap int) error {
	if session.hasSelectLimit && session.selectLimit == cap {
		return nil
	}
	written := "default"
	if cap >= 0 {
		if cap < 1 {
			cap = 1
		}
		written = strconv.Itoa(cap)
	}
	if _, err := session.connection.ExecContext(
		ctx, "set session sql_select_limit = "+written); err != nil {
		return err
	}
	session.selectLimit = cap
	session.hasSelectLimit = true
	return nil
}

// buildMysqlColumns reads result column names, types, and binary flags.
func buildMysqlColumns(rows *sql.Rows) ([]db.ResultColumn, []bool, error) {
	names, err := rows.Columns()
	if err != nil {
		return nil, nil, err
	}
	if len(names) == 0 {
		return nil, nil, nil
	}
	// ColumnTypes names the scan type used to read bytes as hex.
	types, typeErr := rows.ColumnTypes()
	if typeErr != nil {
		return nil, nil, typeErr
	}

	columns := make([]db.ResultColumn, 0, len(names))
	// Binary column values remain bytes for hexadecimal display.
	binary := make([]bool, 0, len(names))
	for at, name := range names {
		dataType := ""
		if at < len(types) && types[at] != nil {
			dataType = strings.ToLower(types[at].DatabaseTypeName())
		}
		columns = append(columns, db.ResultColumn{Name: name, DataType: dataType})
		binary = append(binary, db.IsBinaryColumnType(dataType))
	}
	return columns, binary, nil
}

// readMysqlRows reads a result as rows of plain values, with its columns.
func readMysqlRows(rows *sql.Rows, cap int) ([][]any, []db.ResultColumn, error) {
	columns, binary, err := buildMysqlColumns(rows)
	if err != nil || columns == nil {
		return nil, nil, err
	}

	read := [][]any{}
	for rows.Next() {
		values, scanErr := db.ScanRow(rows, len(columns), binary...)
		if scanErr != nil {
			return nil, nil, scanErr
		}
		read = append(read, values)
		if cap >= 0 && len(read) >= cap {
			break
		}
	}
	return read, columns, rows.Err()
}

func (session *mysqlSession) RunQuery(
	ctx context.Context, statement string, rowLimit int, params []any,
) (db.QueryResult, error) {
	startedAt := time.Now()
	giveBack, waitErr := session.mainQueue.Take(ctx)
	if waitErr != nil {
		return db.QueryResult{}, db.WrapDatabaseError(waitErr)
	}
	defer giveBack()

	if err := session.applySelectLimit(ctx, db.ReadOverscanRowLimit(rowLimit)); err != nil {
		return db.QueryResult{}, db.WrapDatabaseError(err)
	}

	flavour := session.Support.Dialect.Syntax
	lastStatement := db.ReadLastStatement(statement, flavour)
	command := syntax.ReadCommandWord(lastStatement, flavour)
	rows, err := session.connection.QueryContext(ctx, statement, params...)
	if err != nil {
		session.markTransactionFailed(err)
		return db.QueryResult{}, db.WrapDatabaseError(err)
	}
	defer func() { _ = rows.Close() }()

	// The driver exposes only row results. The final result here belongs to the last row-returning statement.
	read, columns, readErr := readMysqlRows(rows, db.ReadOverscanRowLimit(rowLimit))
	if readErr != nil {
		session.markTransactionFailed(readErr)
		return db.QueryResult{}, db.WrapDatabaseError(readErr)
	}
	for rows.NextResultSet() {
		next, nextColumns, nextErr := readMysqlRows(rows, db.ReadOverscanRowLimit(rowLimit))
		if nextErr != nil {
			session.markTransactionFailed(nextErr)
			return db.QueryResult{}, db.WrapDatabaseError(nextErr)
		}
		read, columns = next, nextColumns
	}

	writes := db.IsWriteCommand(command)
	if writes && !db.HoldsReturningClause(lastStatement, flavour) {
		// A final write uses its affected count instead of an earlier statement's rows.
		read, columns = nil, nil
	}
	result := db.BuildCappedResult(db.CappedRead{
		Rows: read, RowLimit: rowLimit, Columns: columns,
		Elapsed: time.Since(startedAt), Command: strings.ToUpper(command),
	})
	if writes {
		result.Affected, result.HasAffected = session.countAffected(ctx)
	}
	session.markTransactionFromStatement(statement)
	return result, nil
}

// markTransactionFromStatement updates transaction state after statements from the editor.
func (session *mysqlSession) markTransactionFromStatement(sql string) {
	session.transaction.ApplyStatementEffect(
		statement.ResolveTransactionEffect(sql, session.Support.Dialect.Syntax))
}

// countAffected returns the last write count and distinguishes an unavailable count from zero.
func (session *mysqlSession) countAffected(ctx context.Context) (int64, bool) {
	row := session.connection.QueryRowContext(ctx, "select row_count() as changed")
	var changed int64
	if row.Scan(&changed) != nil {
		return 0, false
	}
	if changed < 0 {
		return 0, true
	}
	return changed, true
}

func (session *mysqlSession) ReadPage(
	ctx context.Context, read db.ComposedRead, window db.ReadWindow,
) (db.QueryResult, error) {
	return db.ReadSQLPage(ctx, session.RunQuery, read, window, session.Support.Dialect.Syntax)
}

func (session *mysqlSession) CountRead(
	ctx context.Context, read db.ComposedRead,
) (int64, bool, error) {
	return db.CountSQLRead(ctx, session.RunQuery, read, session.Support.Dialect)
}

// CheckStatement prepares the statement, which makes the server parse it and resolve
// every name in it.
func (session *mysqlSession) CheckStatement(
	ctx context.Context, statement string,
) (db.StatementProblem, bool) {
	if session.transaction.ReadState() != db.TransactionNone {
		return db.StatementProblem{}, false
	}
	if strings.TrimSpace(statement) == "" ||
		db.HoldsSeveralCommands(statement, session.Support.Dialect.Syntax) {
		return db.StatementProblem{}, false
	}

	side, err := session.side.Read()
	if err != nil {
		return db.StatementProblem{}, false
	}
	prepared, prepareErr := side.PrepareContext(ctx, statement)
	if prepareErr == nil {
		_ = prepared.Close()
		return db.StatementProblem{}, false
	}

	var reported *driver.MySQLError
	if !errors.As(prepareErr, &reported) || reported.Message == "" {
		// A broken connection reports no server message, and the statement is not at fault.
		return db.StatementProblem{}, false
	}
	if reported.Number == mysqlCannotPrepare || isUnfinishedMysqlStatement(reported.Message) {
		return db.StatementProblem{}, false
	}
	problem := db.StatementProblem{Message: reported.Message}
	if offset, found := findMysqlOffset(statement, reported.Message); found {
		problem.Offset = offset
		problem.HasOffset = true
	}
	return problem, true
}

// StreamQuery reads a batch at a time, so an export never holds the whole relation.
func (session *mysqlSession) StreamQuery(
	ctx context.Context, statement string, params []any, batchSize int,
	onBatch func(rows [][]any, columns []db.ResultColumn) error,
) (int64, error) {
	if err := db.RefuseSeveralCommands(statement, session.Support.Dialect.Syntax); err != nil {
		return 0, err
	}
	giveBack, waitErr := session.mainQueue.Take(ctx)
	if waitErr != nil {
		return 0, waitErr
	}
	defer giveBack()

	// The row cap of a page would cut the export short.
	if err := session.applySelectLimit(ctx, -1); err != nil {
		return 0, err
	}

	rows, err := session.connection.QueryContext(ctx, statement, params...)
	if err != nil {
		session.markTransactionFailed(err)
		return 0, err
	}
	defer func() { _ = rows.Close() }()

	columns, _, columnErr := buildMysqlColumns(rows)
	if columnErr != nil {
		return 0, columnErr
	}
	if columns == nil {
		return 0, nil
	}

	batcher := db.NewRowBatcher(batchSize, onBatch)
	for rows.Next() {
		values, scanErr := db.ScanRow(rows, len(columns))
		if scanErr != nil {
			return batcher.CountRows(), scanErr
		}
		if batchErr := batcher.AddRow(values, columns); batchErr != nil {
			return batcher.CountRows(), batchErr
		}
	}
	if rows.Err() != nil {
		session.markTransactionFailed(rows.Err())
		return batcher.CountRows(), rows.Err()
	}
	if batchErr := batcher.FlushRows(columns); batchErr != nil {
		return batcher.CountRows(), batchErr
	}
	return batcher.CountRows(), nil
}

// readNamedRows reads a catalog result as rows keyed by column name, and returns the
// order the server named them in.
func (session *mysqlSession) readNamedRows(
	ctx context.Context, statement string, params ...any,
) ([]map[string]any, []string, error) {
	runner, giveBack, err := session.holdCatalogPool(ctx)
	if err != nil {
		return nil, nil, err
	}
	defer giveBack()

	rows, queryErr := runner.QueryContext(ctx, statement, params...)
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

// ExplainQuery returns the plan as the server writes it, asked for in the form this
// server takes.
func (session *mysqlSession) ExplainQuery(
	ctx context.Context, statement string, analyze bool,
) (db.QueryPlan, error) {
	if err := db.RefuseSeveralPlans(statement, session.Support.Dialect.Syntax); err != nil {
		return db.QueryPlan{}, err
	}
	giveBack, waitErr := session.mainQueue.Take(ctx)
	if waitErr != nil {
		return db.QueryPlan{}, db.WrapDatabaseError(waitErr)
	}
	defer giveBack()

	// Planning requires the original statement without the session row cap.
	if err := session.applySelectLimit(ctx, -1); err != nil {
		return db.QueryPlan{}, db.WrapDatabaseError(err)
	}

	// An analyzed plan runs the statement, so it uses the connection of the user.
	rows, err := session.connection.QueryContext(
		ctx, session.flavour.BuildExplainStatement(statement, analyze))
	if err != nil {
		session.markTransactionFailed(err)
		return db.QueryPlan{}, db.WrapDatabaseError(err)
	}
	defer func() { _ = rows.Close() }()

	read, columns, readErr := readMysqlRows(rows, -1)
	if readErr != nil {
		return db.QueryPlan{}, db.WrapDatabaseError(readErr)
	}
	plan, built := session.flavour.ReadPlan(
		db.QueryResult{Columns: columns, Rows: read}, analyze)
	if !built {
		return db.QueryPlan{}, db.FailUnreadablePlan()
	}
	return plan, nil
}

// runPlain runs a statement of the client itself, which binds nothing and returns nothing.
func (session *mysqlSession) runPlain(ctx context.Context, statement string) error {
	_, err := session.connection.ExecContext(ctx, statement)
	return err
}

func (session *mysqlSession) BeginTransaction(ctx context.Context) error {
	giveBack, waitErr := session.mainQueue.Take(ctx)
	if waitErr != nil {
		return db.WrapDatabaseError(waitErr)
	}
	defer giveBack()

	if err := session.runPlain(ctx, "start transaction"); err != nil {
		return db.WrapDatabaseError(err)
	}
	session.transaction.WriteState(db.TransactionOpen)
	return nil
}

func (session *mysqlSession) CommitTransaction(ctx context.Context) error {
	giveBack, waitErr := session.mainQueue.Take(ctx)
	if waitErr != nil {
		return db.WrapDatabaseError(waitErr)
	}
	defer giveBack()

	if err := session.runPlain(ctx, "commit"); err != nil {
		return db.WrapDatabaseError(err)
	}
	session.transaction.WriteState(db.TransactionNone)
	return nil
}

func (session *mysqlSession) RollbackTransaction(ctx context.Context) error {
	giveBack, waitErr := session.mainQueue.Take(ctx)
	if waitErr != nil {
		return db.WrapDatabaseError(waitErr)
	}
	defer giveBack()

	if err := session.runPlain(ctx, "rollback"); err != nil {
		return db.WrapDatabaseError(err)
	}
	session.transaction.WriteState(db.TransactionNone)
	return nil
}

func (session *mysqlSession) ApplyChanges(ctx context.Context, changes []db.Change) error {
	giveBack, waitErr := session.mainQueue.Take(ctx)
	if waitErr != nil {
		return db.WrapDatabaseError(waitErr)
	}
	defer giveBack()

	return db.ApplyChangesInTransaction(ctx, changes, db.ChangeApplication{
		JoinsUserTransaction: session.transaction.ReadState() == db.TransactionOpen,
		Begin: func(ctx context.Context) error {
			return session.runPlain(ctx, "start transaction")
		},
		Commit:   func(ctx context.Context) error { return session.runPlain(ctx, "commit") },
		Rollback: func(ctx context.Context) error { return session.runPlain(ctx, "rollback") },
		Apply: func(ctx context.Context, change db.Change) error {
			statement, err := db.ReadChangeStatement(change)
			if err != nil {
				return err
			}
			_, execErr := session.connection.ExecContext(ctx, statement.SQL, statement.Params...)
			return execErr
		},
		CountMatches: db.BuildGuardCounter(
			func(ctx context.Context, sql string, params ...any) db.ScanOneRow {
				return session.connection.QueryRowContext(ctx, sql, params...)
			}),
		ReportFailed: session.markTransactionFailed,
	})
}

func (session *mysqlSession) ListActivity(ctx context.Context) ([]db.Activity, error) {
	if !session.Support.Capabilities.HasServerSessions {
		return nil, db.NewUnsupportedError("list sessions")
	}
	rows, _, err := session.readNamedRows(ctx, listMysqlActivitySQL)
	if err != nil {
		return nil, err
	}
	activity := make([]db.Activity, 0, len(rows))
	for _, row := range rows {
		activity = append(activity, db.Activity{
			PID: db.ReadNonNegativeCount(row["pid"]), User: db.ReadAnyText(row["user"]),
			ApplicationName: db.ReadAnyText(row["db"]), ClientAddress: db.ReadAnyText(row["host"]),
			State:    db.ReadAnyText(row["command"]),
			Duration: time.Duration(db.ReadNonNegativeCount(row["duration_ms"])) * time.Millisecond,
			Query:    db.ReadAnyText(row["query"]),
		})
	}
	return activity, nil
}

// ReadServerLoad returns load statistics and calculates server start time from uptime.
func (session *mysqlSession) ReadServerLoad(ctx context.Context) (db.ServerLoad, error) {
	if !session.Support.Capabilities.ReportsServerLoad {
		return db.ServerLoad{}, db.NewUnsupportedError("report server load")
	}
	rows, _, err := session.readNamedRows(ctx, readMysqlServerLoadSQL)
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

// CancelBackend stops another session through the catalog connection.
func (session *mysqlSession) CancelBackend(
	ctx context.Context, pid int64, terminate bool,
) (bool, error) {
	statement := session.flavour.BuildKillStatement(pid, terminate)
	if statement == "" {
		return false, db.NewUnsupportedError("stop another session")
	}
	side, err := session.side.Read()
	if err != nil {
		return false, err
	}
	if _, execErr := side.ExecContext(ctx, statement); execErr != nil {
		return false, execErr
	}
	return true, nil
}

// CancelRunningQuery needs a second connection, because the busy one cannot answer.
func (session *mysqlSession) CancelRunningQuery(ctx context.Context) (bool, error) {
	if !session.Support.Capabilities.CancelsRunningQuery {
		return false, db.NewUnsupportedError("cancel a running statement")
	}
	if session.threadID <= 0 {
		return false, db.NewDatabaseError(
			"cannot cancel the statement: the connection ID is unavailable")
	}
	statement := session.flavour.BuildKillStatement(session.threadID, false)
	if statement == "" {
		return false, db.NewUnsupportedError("cancel a running statement")
	}
	side, err := session.side.Read()
	if err != nil {
		return false, err
	}
	if _, execErr := side.ExecContext(ctx, statement); execErr != nil {
		return false, execErr
	}
	return true, nil
}

// Ping checks the catalog connection outside transactions. Transactional checks use the user connection and skip busy connections.
func (session *mysqlSession) Ping(ctx context.Context) error {
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

// The user connection is one socket the driver takes no second caller on.
func (session *mysqlSession) Close() error {
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

// mysqlAdapter opens a connection on a MySQL-protocol server.
type mysqlAdapter struct {
	support db.EngineSupport
	flavour Flavour
}

// NewAdapter returns the adapter of one MySQL-protocol engine.
func NewAdapter(support db.EngineSupport, flavour Flavour) db.Adapter {
	return &mysqlAdapter{support: support, flavour: flavour}
}

func (adapter *mysqlAdapter) Connect(
	ctx context.Context, profile cfg.Profile, password string,
) (db.Session, error) {
	pool, err := openMysqlPool(profile, password)
	if err != nil {
		return nil, db.WrapDatabaseMessage(db.BuildConnectMessage(profile, err), err)
	}

	// One connection retains the session transaction and row limit.
	connection, connectionErr := pool.Conn(ctx)
	if connectionErr != nil {
		_ = pool.Close()
		return nil, db.WrapDatabaseMessage(db.BuildConnectMessage(profile, connectionErr), connectionErr)
	}

	fail := func(reason error) (db.Session, error) {
		_ = connection.Close()
		_ = pool.Close()
		return nil, db.WrapDatabaseMessage(db.BuildConnectMessage(profile, reason), reason)
	}

	if profile.AccessMode == cfg.AccessReadOnly {
		// Read-only profiles require server support.
		if adapter.flavour.ReadOnlyStatement == "" {
			return fail(errors.New("this server does not support read-only sessions"))
		}
		if _, readOnlyErr := connection.ExecContext(
			ctx, adapter.flavour.ReadOnlyStatement); readOnlyErr != nil {
			return fail(readOnlyErr)
		}
	}

	serverVersion := "unknown"
	var written string
	if connection.QueryRowContext(
		ctx, "select version() as version").Scan(&written) == nil && written != "" {
		serverVersion = written
	}

	// Cancellation requires the server connection ID. A missing ID disables cancellation.
	threadID := int64(0)
	if connection.QueryRowContext(
		ctx, "select connection_id() as id").Scan(&threadID) != nil {
		threadID = 0
	}

	return &mysqlSession{
		rollsBackOnTimeout: readRollsBackOnTimeout(ctx, connection),
		SessionFacts: db.SessionFacts{
			Descriptor: db.SessionDescriptor{
				Profile: profile, ServerVersion: serverVersion,
				// A MySQL schema is a database. The connected one is the default.
				DefaultSchema: profile.Database,
			},
			Support: adapter.support,
		},
		flavour:    adapter.flavour,
		connection: connection, pool: pool, threadID: threadID,
		profile: profile, password: password,
		side: db.NewSideConnection(func() (*sql.DB, error) {
			return openMysqlPool(profile, password)
		}),
		mainQueue: db.NewCallQueue(),
	}, nil
}

// readRollsBackOnTimeout reads innodb_rollback_on_timeout. An unavailable value uses the MySQL and MariaDB default: statement-only rollback.
func readRollsBackOnTimeout(ctx context.Context, connection *sql.Conn) bool {
	var written string
	if err := connection.QueryRowContext(
		ctx, "select @@innodb_rollback_on_timeout as held").Scan(&written); err != nil {
		return false
	}
	return written == "1" || strings.EqualFold(written, "on")
}

// Compile-time Session interface check.
var _ db.Session = (*mysqlSession)(nil)
