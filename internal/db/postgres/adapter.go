package postgres

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/turanmahmudov/masume/internal/cfg"
	"github.com/turanmahmudov/masume/internal/core"
	"github.com/turanmahmudov/masume/internal/db"
	"github.com/turanmahmudov/masume/internal/query"
	"github.com/turanmahmudov/masume/internal/query/result"
)

// postgresSession is one session on a PostgreSQL-protocol server, built from its engine
// entry and its flavour.
type postgresSession struct {
	db.SessionFacts

	flavour    Flavour
	connection *pgx.Conn
	// True where the server has the extension that counts statements, read once at connect.
	holdsStatementStats bool
	backendPID          int64
	password            string
	transaction         db.TransactionMark
	side                *db.SideConnection[*pgx.Conn]

	// typeNames names every type the server holds, read once at connect, because the
	// map the driver ships knows the standard types only and a server also holds its
	// own enums, domains and composites.
	typeNames map[uint32]string

	// One queue per connection, because the driver refuses a second call on a
	// connection that is still answering the first.
	mainQueue *db.CallQueue
	sideQueue *db.CallQueue
}

// Capabilities returns what this connection does. Every entry is a fact of the engine except
// the count of statements, which holds only where the extension is installed.
func (session *postgresSession) Capabilities() core.Capabilities {
	held := session.Support.Capabilities
	held.ReportsStatementStats = session.holdsStatementStats
	return held
}

func (session *postgresSession) ReadTransactionState() db.TransactionState {
	return session.transaction.ReadState()
}

// markTransactionFailed records that PostgreSQL refuses every later statement of the
// transaction until it rolls back.
func (session *postgresSession) markTransactionFailed() {
	session.transaction.MarkFailed()
}

// resolveCatalogConnection returns the connection a catalog read uses. Inside a
// transaction it must be the same one, or it reads stale data.
func (session *postgresSession) resolveCatalogConnection() (*pgx.Conn, error) {
	if session.transaction.ReadState() != db.TransactionNone {
		return session.connection, nil
	}
	return session.side.Read()
}

// holdCatalog waits for its turn on the connection a catalog read uses, and returns what
// gives the turn back.
func (session *postgresSession) holdCatalog(
	ctx context.Context,
) (*pgx.Conn, func(), error) {
	if session.transaction.ReadState() != db.TransactionNone {
		giveBack, err := session.mainQueue.Take(ctx)
		if err != nil {
			return nil, nil, err
		}
		return session.connection, giveBack, nil
	}
	connection, err := session.side.Read()
	if err != nil {
		return nil, nil, err
	}
	giveBack, waitErr := session.sideQueue.Take(ctx)
	if waitErr != nil {
		return nil, nil, waitErr
	}
	return connection, giveBack, nil
}

// readTypeName returns the name of a type, so a result column carries it.
func (session *postgresSession) readTypeName(oid uint32) string {
	if name, known := session.typeNames[oid]; known {
		return name
	}
	return fmt.Sprintf("oid:%d", oid)
}

func (session *postgresSession) readResultColumns(
	fields []pgconn.FieldDescription,
) []db.ResultColumn {
	columns := make([]db.ResultColumn, 0, len(fields))
	for _, field := range fields {
		columns = append(columns, db.ResultColumn{
			Name: field.Name, DataType: session.readTypeName(field.DataTypeOID),
		})
	}
	return columns
}

// runOn reads a statement on one connection, capped at rowLimit rows.
func (session *postgresSession) runOn(
	ctx context.Context, connection *pgx.Conn, sql string, rowLimit int, params []any,
) (db.QueryResult, error) {
	startedAt := time.Now()

	// A buffer of several statements goes over the simple protocol, which is the only
	// one the server takes more than one statement on.
	if db.HoldsSeveralCommands(sql, session.Support.Dialect.Syntax) && len(params) == 0 {
		return session.runBatch(ctx, connection, sql, rowLimit, startedAt)
	}

	rows, err := connection.Query(ctx, sql, params...)
	if err != nil {
		return db.QueryResult{}, err
	}
	defer rows.Close()

	columns := session.readResultColumns(rows.FieldDescriptions())
	wanted := db.ReadOverscanRowLimit(rowLimit)
	read := [][]any{}
	for rows.Next() {
		values, valueErr := rows.Values()
		if valueErr != nil {
			return db.QueryResult{}, valueErr
		}
		read = append(read, values)
		if rowLimit >= 0 && len(read) >= wanted {
			break
		}
	}
	rows.Close()
	// pgx reports a cancelled or timed-out fetch on rows.Err after the row loop.
	if rows.Err() != nil {
		return db.QueryResult{}, rows.Err()
	}

	tag := rows.CommandTag()
	command := readCommandName(tag.String())
	result := db.BuildCappedResult(db.CappedRead{
		Rows: read, RowLimit: rowLimit, Columns: columns,
		Elapsed: time.Since(startedAt), Command: command,
	})
	// The driver counts the rows of a read as well, and a read changed none.
	if db.IsWriteCommand(command) {
		result.Affected = tag.RowsAffected()
		result.HasAffected = true
	}
	return result, nil
}

// runBatch reads a buffer of several statements over the simple protocol, and returns
// the result of the last one. Each result is read as it arrives and only the last one is
// kept, and a result past the row limit is read to its end without being held, so a buffer
// that returns millions of rows never puts them all in memory.
func (session *postgresSession) runBatch(
	ctx context.Context, connection *pgx.Conn, sql string, rowLimit int, startedAt time.Time,
) (db.QueryResult, error) {
	reader := connection.PgConn().Exec(ctx, sql)
	wanted := db.ReadOverscanRowLimit(rowLimit)

	answered := false
	var columns []db.ResultColumn
	var read [][]any
	var tag pgconn.CommandTag
	for reader.NextResult() {
		result := reader.ResultReader()
		fields := result.FieldDescriptions()
		rows := [][]any{}
		for result.NextRow() {
			if rowLimit >= 0 && len(rows) >= wanted {
				continue
			}
			rows = append(rows, decodeRawRow(connection, fields, result.Values()))
		}
		commandTag, err := result.Close()
		if err != nil {
			_ = reader.Close()
			return db.QueryResult{}, err
		}
		answered, columns, read, tag = true, session.readResultColumns(fields), rows, commandTag
	}
	if err := reader.Close(); err != nil {
		return db.QueryResult{}, err
	}
	if !answered {
		return db.QueryResult{Elapsed: time.Since(startedAt)}, nil
	}

	command := readCommandName(tag.String())
	result := db.BuildCappedResult(db.CappedRead{
		Rows: read, RowLimit: rowLimit, Columns: columns,
		Elapsed: time.Since(startedAt), Command: command,
	})
	if db.IsWriteCommand(command) {
		result.Affected = tag.RowsAffected()
		result.HasAffected = true
	}
	return result, nil
}

// decodeRawRow reads one row of a simple-protocol result. The reader hands out the bytes
// of a row only until it moves to the next one, so each cell is copied before it is read.
func decodeRawRow(
	connection *pgx.Conn, fields []pgconn.FieldDescription, row [][]byte,
) []any {
	values := make([]any, 0, len(row))
	for at, cell := range row {
		if cell != nil {
			cell = append([]byte(nil), cell...)
		}
		values = append(values, decodeRawValue(connection, fields, at, cell))
	}
	return values
}

// decodeRawValue reads one cell of a simple-protocol result, which arrives as bytes.
func decodeRawValue(
	connection *pgx.Conn, fields []pgconn.FieldDescription, at int, cell []byte,
) any {
	if cell == nil {
		return nil
	}
	if at >= len(fields) {
		return string(cell)
	}
	var value any
	err := connection.TypeMap().Scan(
		fields[at].DataTypeOID, fields[at].Format, cell, &value)
	if err != nil {
		return string(cell)
	}
	return value
}

// readCommandName returns the command word of a tag the server sent, in the case the server
// wrote it, which is upper case.
func readCommandName(tag string) string {
	if tag == "" {
		return ""
	}
	return strings.Fields(tag)[0]
}

func (session *postgresSession) RunQuery(
	ctx context.Context, sql string, rowLimit int, params []any,
) (db.QueryResult, error) {
	giveBack, waitErr := session.mainQueue.Take(ctx)
	if waitErr != nil {
		return db.QueryResult{}, db.WrapDatabaseError(waitErr)
	}
	defer giveBack()

	result, err := session.runOn(ctx, session.connection, sql, rowLimit, params)
	if err != nil {
		session.markTransactionFailed()
		return db.QueryResult{}, db.WrapDatabaseMessage(describeFailure(err), err)
	}
	session.markTransactionFromServer()
	return result, nil
}

// markTransactionFromServer reads the transaction status the server sends with every
// answer. A `begin` or a `commit` written into the editor never reaches BeginTransaction,
// and the server knows what the connection is in whichever way it was asked.
func (session *postgresSession) markTransactionFromServer() {
	switch session.connection.PgConn().TxStatus() {
	case 'I':
		session.transaction.WriteState(db.TransactionNone)
	case 'T':
		session.transaction.WriteState(db.TransactionOpen)
	case 'E':
		session.transaction.WriteState(db.TransactionFailed)
	}
}

func (session *postgresSession) ReadPage(
	ctx context.Context, read db.ComposedRead, window db.ReadWindow,
) (db.QueryResult, error) {
	return db.ReadSQLPage(ctx, session.RunQuery, read, window, session.Support.Dialect.Syntax)
}

func (session *postgresSession) CountRead(
	ctx context.Context, read db.ComposedRead,
) (int64, bool, error) {
	return db.CountSQLRead(ctx, session.RunQuery, read, session.Support.Dialect)
}

// CheckStatement sends Parse and stops, so the server parses the statement, resolves
// every name and plans it, and runs nothing.
func (session *postgresSession) CheckStatement(
	ctx context.Context, sql string,
) (db.StatementProblem, bool) {
	// A statement PostgreSQL refuses to read throws the open transaction away, and a
	// check the user did not ask for must never cost work already done in one.
	if session.transaction.ReadState() != db.TransactionNone {
		return db.StatementProblem{}, false
	}
	if strings.TrimSpace(sql) == "" || db.HoldsSeveralCommands(sql, session.Support.Dialect.Syntax) {
		return db.StatementProblem{}, false
	}

	connection, err := session.side.Read()
	if err != nil {
		return db.StatementProblem{}, false
	}
	giveBack, waitErr := session.sideQueue.Take(ctx)
	if waitErr != nil {
		return db.StatementProblem{}, false
	}
	defer giveBack()

	if _, prepareErr := connection.PgConn().Prepare(ctx, "", sql, nil); prepareErr != nil {
		if reported, ok := errors.AsType[*pgconn.PgError](prepareErr); ok {
			return ReadStatementProblem(reported.Code, reported.Message, int(reported.Position))
		}
		return db.StatementProblem{}, false
	}
	return db.StatementProblem{}, false
}

// StreamQuery reads a batch at a time. The batches are not kept, so a read costs one
// batch, not the whole relation.
func (session *postgresSession) StreamQuery(
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

	rows, err := session.connection.Query(ctx, sql, params...)
	if err != nil {
		session.markTransactionFailed()
		return 0, err
	}
	defer rows.Close()

	columns := session.readResultColumns(rows.FieldDescriptions())
	batcher := db.NewRowBatcher(batchSize, onBatch)

	for rows.Next() {
		values, valueErr := rows.Values()
		if valueErr != nil {
			return batcher.CountRows(), valueErr
		}
		if batchErr := batcher.AddRow(values, columns); batchErr != nil {
			return batcher.CountRows(), batchErr
		}
	}
	if rows.Err() != nil {
		session.markTransactionFailed()
		return batcher.CountRows(), rows.Err()
	}
	if batchErr := batcher.FlushRows(columns); batchErr != nil {
		return batcher.CountRows(), batchErr
	}
	return batcher.CountRows(), nil
}

// readRows returns a catalog read as rows keyed by column name.
func (session *postgresSession) readRows(
	ctx context.Context, sql string, params ...any,
) ([]map[string]any, error) {
	connection, giveBack, err := session.holdCatalog(ctx)
	if err != nil {
		return nil, err
	}
	defer giveBack()

	rows, queryErr := connection.Query(ctx, sql, params...)
	if queryErr != nil {
		return nil, queryErr
	}
	defer rows.Close()

	names := make([]string, 0, len(rows.FieldDescriptions()))
	for _, field := range rows.FieldDescriptions() {
		names = append(names, field.Name)
	}

	read := []map[string]any{}
	for rows.Next() {
		values, valueErr := rows.Values()
		if valueErr != nil {
			return nil, valueErr
		}
		row := map[string]any{}
		for at, name := range names {
			if at < len(values) {
				row[name] = values[at]
			}
		}
		read = append(read, row)
	}
	return read, rows.Err()
}

func (session *postgresSession) ListTables(ctx context.Context) ([]db.TableRef, error) {
	rows, err := session.readRows(ctx, listTablesSQL)
	if err != nil {
		return nil, err
	}
	tables := make([]db.TableRef, 0, len(rows))
	for _, row := range rows {
		tables = append(tables, db.TableRef{
			Schema: db.ReadAnyText(row["schema"]), Name: db.ReadAnyText(row["name"]),
			Kind: MapRelationKind(row["kind"]), EstimatedRows: db.ReadNonNegativeCount(row["estimated_rows"]),
		})
	}
	return tables, nil
}

func (session *postgresSession) ListRoles(ctx context.Context) ([]db.DbRole, error) {
	rows, err := session.readRows(ctx, listRolesSQL)
	if err != nil {
		return nil, err
	}
	roles := make([]db.DbRole, 0, len(rows))
	for _, row := range rows {
		roles = append(roles, db.DbRole{
			Name: db.ReadAnyText(row["name"]), Detail: db.ReadAnyText(row["detail"]),
		})
	}
	return roles, nil
}

func (session *postgresSession) ListSchemaObjects(ctx context.Context) ([]db.SchemaObject, error) {
	rows, err := session.readRows(ctx, listSchemaObjectsSQL)
	if err != nil {
		return nil, err
	}
	objects := make([]db.SchemaObject, 0, len(rows))
	for _, row := range rows {
		kind := db.ReadAnyText(row["kind"])
		if !db.IsSchemaObjectKind(kind) {
			continue
		}
		objects = append(objects, db.SchemaObject{
			Schema: db.ReadAnyText(row["schema"]), Name: db.ReadAnyText(row["name"]),
			Kind: db.SchemaObjectKind(kind), Detail: db.ReadAnyText(row["detail"]),
			Events: db.ReadAnyText(row["events"]), Identity: db.ReadAnyText(row["identity"]),
		})
	}
	return objects, nil
}

// BuildObjectDDL asks Postgres for the definition of the object, which it prints itself.
func (session *postgresSession) BuildObjectDDL(
	ctx context.Context, object db.SchemaObject,
) ([]string, error) {
	statement, known := postgresObjectDDL[object.Kind]
	if !known {
		return db.BuildMissingDefinition(string(object.Kind) + " " + object.Name), nil
	}
	rows, err := session.readRows(ctx, statement, object.Identity)
	if err != nil {
		return nil, err
	}
	if len(rows) == 0 || rows[0]["ddl"] == nil {
		return db.BuildMissingDefinition(string(object.Kind) + " " + object.Name), nil
	}
	return strings.Split(db.ReadAnyText(rows[0]["ddl"]), "\n"), nil
}

func (session *postgresSession) DescribeTable(
	ctx context.Context, table db.TableRef,
) (db.TableDetail, error) {
	qualified := session.Support.Dialect.BuildQualifiedName(table.Qualified())
	columnRows, err := session.readRows(ctx, describeColumnsSQL, qualified)
	if err != nil {
		return db.TableDetail{}, err
	}
	foreignKeyRows, keyErr := session.readRows(ctx, describeForeignKeysSQL, qualified)
	if keyErr != nil {
		return db.TableDetail{}, keyErr
	}

	return db.TableDetail{
		Table:       table,
		Columns:     readColumnDetails(columnRows),
		ForeignKeys: readForeignKeys(foreignKeyRows),
	}, nil
}

func readColumnDetails(rows []map[string]any) []db.ColumnDetail {
	columns := make([]db.ColumnDetail, 0, len(rows))
	for _, row := range rows {
		column := db.ColumnDetail{
			Name: db.ReadAnyText(row["name"]), DataType: db.ReadAnyText(row["data_type"]),
			Nullable: readFlag(row["nullable"]), IsPrimaryKey: readFlag(row["is_primary_key"]),
			IsGenerated: readFlag(row["is_generated"]), Choices: ReadTextArray(row["choices"]),
		}
		if row["default_value"] != nil {
			column.DefaultValue = db.ReadAnyText(row["default_value"])
			column.HasDefault = true
		}
		columns = append(columns, column)
	}
	return columns
}

func readForeignKeys(rows []map[string]any) []db.ForeignKey {
	keys := make([]db.ForeignKey, 0, len(rows))
	for _, row := range rows {
		keys = append(keys, db.ForeignKey{
			Name: db.ReadAnyText(row["name"]), Columns: ReadTextArray(row["columns"]),
			TargetSchema:  db.ReadAnyText(row["target_schema"]),
			TargetTable:   db.ReadAnyText(row["target_table"]),
			TargetColumns: ReadTextArray(row["target_columns"]),
			DeleteRule:    query.ParseDeleteRule(db.ReadAnyText(row["delete_rule"])),
		})
	}
	return keys
}

func (session *postgresSession) ListRelationships(ctx context.Context) ([]db.Relationship, error) {
	rows, err := session.readRows(ctx, listRelationshipsSQL)
	if err != nil {
		return nil, err
	}
	relationships := make([]db.Relationship, 0, len(rows))
	for _, row := range rows {
		relationships = append(relationships, db.Relationship{
			Name: db.ReadAnyText(row["name"]), Columns: ReadTextArray(row["columns"]),
			TargetSchema:  db.ReadAnyText(row["target_schema"]),
			TargetTable:   db.ReadAnyText(row["target_table"]),
			TargetColumns: ReadTextArray(row["target_columns"]),
			DeleteRule:    query.ParseDeleteRule(db.ReadAnyText(row["delete_rule"])),
			Schema:        db.ReadAnyText(row["schema"]), Table: db.ReadAnyText(row["table"]),
		})
	}
	return relationships, nil
}

func (session *postgresSession) ListIndexes(
	ctx context.Context, table db.TableRef,
) ([]db.IndexDetail, error) {
	qualified := session.Support.Dialect.BuildQualifiedName(table.Qualified())
	rows, err := session.readRows(ctx, listIndexesSQL, qualified)
	if err != nil {
		return nil, err
	}
	indexes := make([]db.IndexDetail, 0, len(rows))
	for _, row := range rows {
		indexes = append(indexes, db.IndexDetail{
			Name: db.ReadAnyText(row["name"]), IsUnique: readFlag(row["is_unique"]),
			IsPrimary: readFlag(row["is_primary"]), Definition: db.ReadAnyText(row["definition"]),
		})
	}
	return indexes, nil
}

func (session *postgresSession) ListConstraints(
	ctx context.Context, table db.TableRef,
) ([]db.ConstraintDetail, error) {
	qualified := session.Support.Dialect.BuildQualifiedName(table.Qualified())
	rows, err := session.readRows(ctx, listConstraintsSQL, qualified)
	if err != nil {
		return nil, err
	}
	constraints := make([]db.ConstraintDetail, 0, len(rows))
	for _, row := range rows {
		constraints = append(constraints, db.ConstraintDetail{
			Name: db.ReadAnyText(row["name"]), Kind: MapConstraintKind(row["type"]),
			Definition: db.ReadAnyText(row["definition"]),
		})
	}
	return constraints, nil
}

func (session *postgresSession) BuildTableDDL(
	ctx context.Context, table db.TableRef,
) ([]string, error) {
	detail, err := session.DescribeTable(ctx, table)
	if err != nil {
		return nil, db.WrapDatabaseOperation("reading the columns", err)
	}
	indexes, indexErr := session.ListIndexes(ctx, table)
	if indexErr != nil {
		return nil, db.WrapDatabaseOperation("reading the indexes", indexErr)
	}
	constraints, constraintErr := session.ListConstraints(ctx, table)
	if constraintErr != nil {
		return nil, db.WrapDatabaseOperation("reading the constraints", constraintErr)
	}
	return RenderTableDDL(detail, indexes, constraints, session.Support.Dialect), nil
}

// ExplainQuery asks the server for the plan in the form this server takes.
func (session *postgresSession) ExplainQuery(
	ctx context.Context, sql string, analyze bool,
) (db.QueryPlan, error) {
	if !session.Support.Capabilities.PlansStatement {
		return db.QueryPlan{}, db.NewUnsupportedError("plan a statement")
	}
	if analyze && !session.Support.Capabilities.MeasuresPlan {
		return db.QueryPlan{}, db.NewDatabaseError(
			"this server plans without measuring, so only the estimate is read")
	}
	if err := db.RefuseSeveralPlans(sql, session.Support.Dialect.Syntax); err != nil {
		return db.QueryPlan{}, err
	}

	giveBack, waitErr := session.mainQueue.Take(ctx)
	if waitErr != nil {
		return db.QueryPlan{}, db.WrapDatabaseError(waitErr)
	}
	defer giveBack()

	prefix := session.flavour.BuildExplainPrefix(analyze)
	rows, err := session.connection.Query(ctx, prefix+" "+sql)
	if err != nil {
		session.markTransactionFailed()
		return db.QueryPlan{}, db.WrapDatabaseMessage(describeFailure(err), err)
	}
	defer rows.Close()

	// The server returns one line per row, which is the plan split into lines.
	lines := []string{}
	for rows.Next() {
		values, valueErr := rows.Values()
		if valueErr != nil {
			return db.QueryPlan{}, valueErr
		}
		if len(values) > 0 {
			lines = append(lines, core.FormatCell(values[0], ""))
		}
	}
	if rows.Err() != nil {
		session.markTransactionFailed()
		return db.QueryPlan{}, db.WrapDatabaseMessage(describeFailure(rows.Err()), rows.Err())
	}

	plan, read := result.ParseTextPlan(
		strings.Join(lines, "\n"), analyze, session.Support.Capabilities.MeasuresPlan)
	if !read {
		return db.QueryPlan{}, db.FailUnreadablePlan()
	}
	return plan, nil
}

func (session *postgresSession) BeginTransaction(ctx context.Context) error {
	giveBack, waitErr := session.mainQueue.Take(ctx)
	if waitErr != nil {
		return db.WrapDatabaseError(waitErr)
	}
	defer giveBack()

	if _, err := session.connection.Exec(ctx, "begin"); err != nil {
		return db.WrapDatabaseMessage(describeFailure(err), err)
	}
	session.transaction.WriteState(db.TransactionOpen)
	return nil
}

func (session *postgresSession) CommitTransaction(ctx context.Context) error {
	giveBack, waitErr := session.mainQueue.Take(ctx)
	if waitErr != nil {
		return db.WrapDatabaseError(waitErr)
	}
	defer giveBack()

	if _, err := session.connection.Exec(ctx, "commit"); err != nil {
		return db.WrapDatabaseMessage(describeFailure(err), err)
	}
	session.transaction.WriteState(db.TransactionNone)
	return nil
}

func (session *postgresSession) RollbackTransaction(ctx context.Context) error {
	giveBack, waitErr := session.mainQueue.Take(ctx)
	if waitErr != nil {
		return db.WrapDatabaseError(waitErr)
	}
	defer giveBack()

	if _, err := session.connection.Exec(ctx, "rollback"); err != nil {
		return db.WrapDatabaseMessage(describeFailure(err), err)
	}
	session.transaction.WriteState(db.TransactionNone)
	return nil
}

func (session *postgresSession) ApplyChanges(ctx context.Context, changes []db.Change) error {
	giveBack, waitErr := session.mainQueue.Take(ctx)
	if waitErr != nil {
		return db.WrapDatabaseError(waitErr)
	}
	defer giveBack()

	return db.ApplyChangesInTransaction(ctx, changes, db.ChangeApplication{
		JoinsUserTransaction: session.transaction.ReadState() == db.TransactionOpen,
		Begin: func(ctx context.Context) error {
			_, err := session.connection.Exec(ctx, "begin")
			return err
		},
		Commit: func(ctx context.Context) error {
			_, err := session.connection.Exec(ctx, "commit")
			return err
		},
		Rollback: func(ctx context.Context) error {
			_, err := session.connection.Exec(ctx, "rollback")
			return err
		},
		Apply: func(ctx context.Context, change db.Change) error {
			statement, err := db.ReadChangeStatement(change)
			if err != nil {
				return err
			}
			_, execErr := session.connection.Exec(ctx, statement.SQL, statement.Params...)
			return execErr
		},
		CountMatches: db.BuildGuardCounter(
			func(ctx context.Context, sql string, params ...any) db.ScanOneRow {
				return session.connection.QueryRow(ctx, sql, params...)
			}),
		ReportFailed: func(error) { session.markTransactionFailed() },
	})
}

func (session *postgresSession) ListActivity(ctx context.Context) ([]db.Activity, error) {
	if !session.Support.Capabilities.HasServerSessions {
		return nil, db.NewUnsupportedError("list its sessions")
	}
	rows, err := session.readRows(ctx, listActivitySQL)
	if err != nil {
		return nil, err
	}
	activity := make([]db.Activity, 0, len(rows))
	for _, row := range rows {
		activity = append(activity, db.Activity{
			PID: db.ReadNonNegativeCount(row["pid"]), User: db.ReadAnyText(row["usename"]),
			ApplicationName: db.ReadAnyText(row["application_name"]),
			ClientAddress:   db.ReadAnyText(row["client_addr"]),
			State:           db.ReadAnyText(row["state"]),
			Duration:        time.Duration(db.ReadNonNegativeCount(row["duration_ms"])) * time.Millisecond,
			Query:           db.ReadAnyText(row["query"]),
		})
	}
	return activity, nil
}

func (session *postgresSession) ListLockWaits(ctx context.Context) ([]db.LockWait, error) {
	if !session.Support.Capabilities.ReportsLockWaits {
		return nil, db.NewUnsupportedError("report which sessions wait for a lock")
	}
	rows, err := session.readRows(ctx, listLockWaitsSQL)
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

func (session *postgresSession) ReadServerLoad(ctx context.Context) (db.ServerLoad, error) {
	if !session.Support.Capabilities.ReportsServerLoad {
		return db.ServerLoad{}, db.NewUnsupportedError("report the load it is under")
	}
	rows, err := session.readRows(ctx, readServerLoadSQL)
	if err != nil {
		return db.ServerLoad{}, err
	}
	if len(rows) == 0 {
		return db.ServerLoad{}, nil
	}
	held := rows[0]
	load := db.ServerLoad{
		Connections:    db.ReadNonNegativeCount(held["connections"]),
		MaxConnections: db.ReadNonNegativeCount(held["max_connections"]),
		StartedAt:      readTimestamp(held["started_at"]),
		Transactions:   db.ReadNonNegativeCount(held["transactions"]),
		WalBytes:       db.ReadNonNegativeCount(held["wal_bytes"]),
		TempFiles:      db.ReadNonNegativeCount(held["temp_files"]),
	}
	// The counters are reported together, so one of them being there means the server counts.
	_, countsTransactions := readOptionalFloat(held["transactions"])
	load.HasCounters = countsTransactions

	hit, hasHit := readOptionalFloat(held["blocks_hit"])
	read, hasRead := readOptionalFloat(held["blocks_read"])
	if hasHit && hasRead && hit+read > 0 {
		load.CacheHitRate, load.HasCacheHitRate = hit/(hit+read), true
	}
	if lag, held := readOptionalFloat(held["replication_lag_s"]); held {
		load.ReplicationLag = time.Duration(max(lag, 0) * float64(time.Second))
		load.HasReplicationLag = true
	}
	return load, nil
}

// ListSlowStatements returns the statements this server spent the most time in, the slowest
// by mean time first, narrowed to the database of this connection.
func (session *postgresSession) ListSlowStatements(
	ctx context.Context, limit int,
) ([]db.StatementStat, error) {
	if !session.Capabilities().ReportsStatementStats {
		return nil, db.NewUnsupportedError("report the statements it spends its time in")
	}
	if limit < 1 {
		return nil, nil
	}
	rows, err := session.readRows(ctx, listSlowStatementsSQL, limit, DashboardMark)
	if err != nil {
		return nil, err
	}

	held := make([]db.StatementStat, 0, len(rows))
	for _, row := range rows {
		mean, _ := readOptionalFloat(row["mean_ms"])
		total, _ := readOptionalFloat(row["total_ms"])
		held = append(held, db.StatementStat{
			Query:     db.ReadAnyText(row["query"]),
			Calls:     db.ReadNonNegativeCount(row["calls"]),
			MeanTime:  time.Duration(max(mean, 0) * float64(time.Millisecond)),
			TotalTime: time.Duration(max(total, 0) * float64(time.Millisecond)),
			Rows:      db.ReadNonNegativeCount(row["rows_returned"]),
		})
	}
	return held, nil
}

// CancelBackend stops another session on the second connection, because the one of the
// user can be busy.
func (session *postgresSession) CancelBackend(
	ctx context.Context, pid int64, terminate bool,
) (bool, error) {
	if !session.Support.Capabilities.HasServerSessions {
		return false, db.NewUnsupportedError("stop another session")
	}
	rows, err := session.readRows(ctx,
		"select "+session.flavour.BuildCancelFunction(terminate)+"($1) as ok", pid)
	if err != nil {
		return false, err
	}
	return len(rows) > 0 && readFlag(rows[0]["ok"]), nil
}

// CancelRunningQuery uses a second connection, because the one running the query cannot
// answer until the query ends.
func (session *postgresSession) CancelRunningQuery(ctx context.Context) (bool, error) {
	if !session.Support.Capabilities.CancelsRunningQuery {
		return false, db.NewUnsupportedError("cancel a running statement")
	}
	if session.backendPID <= 0 {
		return false, db.NewDatabaseError(
			"the server did not name this connection, so its statement cannot be cancelled")
	}
	connection, err := session.side.Read()
	if err != nil {
		return false, err
	}
	giveBack, waitErr := session.sideQueue.Take(ctx)
	if waitErr != nil {
		return false, waitErr
	}
	defer giveBack()

	rows, queryErr := connection.Query(ctx,
		"select "+session.flavour.BuildCancelFunction(false)+"($1) as ok", session.backendPID)
	if queryErr != nil {
		return false, queryErr
	}
	defer rows.Close()
	if !rows.Next() {
		return false, rows.Err()
	}
	values, valueErr := rows.Values()
	if valueErr != nil {
		return false, valueErr
	}
	return len(values) > 0 && readFlag(values[0]), nil
}

// Ping asks which backend answered. The driver reopens a dropped socket itself, so
// inside a transaction a statement can reach a connection the transaction was never
// opened on, which the server already rolled back. A connection that is still answering
// a call of its own is left alone: it is answering, so the server is there.
func (session *postgresSession) Ping(ctx context.Context) error {
	if session.transaction.ReadState() == db.TransactionNone {
		connection, err := session.resolveCatalogConnection()
		if err != nil {
			return err
		}
		giveBack, free := session.sideQueue.TryTake()
		if !free {
			return nil
		}
		defer giveBack()

		_, execErr := connection.Exec(ctx, "select 1")
		return execErr
	}

	giveBack, free := session.mainQueue.TryTake()
	if !free {
		return nil
	}
	defer giveBack()

	rows, err := session.connection.Query(ctx, "select pg_backend_pid() as pid")
	if err != nil {
		return err
	}
	answered := int64(0)
	if rows.Next() {
		values, valueErr := rows.Values()
		if valueErr == nil && len(values) > 0 {
			answered = db.ReadNonNegativeCount(values[0])
		}
	}
	rows.Close()
	// Without an id of its own there is nothing to compare, and a connection that answers
	// at all is the server saying it is there.
	if session.backendPID <= 0 || answered == session.backendPID {
		return nil
	}

	session.transaction.WriteState(db.TransactionNone)
	return db.NewDatabaseError(
		"the connection was replaced, so the open transaction was rolled back by the server")
}

// closeWait is how long a close is given to reach the server. A connection still
// answering a call is waited for, and a server that never answers must not hold the
// client open.
const closeWait = 5 * time.Second

// pgx refuses a second caller on a connection that is still answering.
func (session *postgresSession) Close() error {
	ctx, stop := context.WithTimeout(context.Background(), closeWait)
	defer stop()
	if side, opened := session.side.Find(); opened {
		if giveBack, waitErr := session.sideQueue.Take(ctx); waitErr == nil {
			defer giveBack()
		}
		_ = side.Close(ctx)
	}
	if giveBack, waitErr := session.mainQueue.Take(ctx); waitErr == nil {
		defer giveBack()
	}
	return session.connection.Close(ctx)
}

// postgresAdapter opens a connection on a PostgreSQL-protocol server.
type postgresAdapter struct {
	support db.EngineSupport
	flavour Flavour
}

// NewAdapter returns the adapter of one PostgreSQL-protocol engine.
func NewAdapter(support db.EngineSupport, flavour Flavour) db.Adapter {
	return &postgresAdapter{support: support, flavour: flavour}
}

func (adapter *postgresAdapter) Connect(
	ctx context.Context, profile cfg.Profile, password string,
) (db.Session, error) {
	connection, err := openPostgresConnection(ctx, profile, password)
	if err != nil {
		return nil, db.WrapDatabaseMessage(db.BuildConnectMessage(profile, err), err)
	}

	fail := func(reason error) (db.Session, error) {
		_ = connection.Close(ctx)
		return nil, db.WrapDatabaseMessage(db.BuildConnectMessage(profile, reason), reason)
	}

	serverVersion := "unknown"
	rows, versionErr := connection.Query(ctx, "show server_version")
	if versionErr != nil {
		return fail(versionErr)
	}
	if rows.Next() {
		values, valueErr := rows.Values()
		if valueErr == nil && len(values) > 0 {
			serverVersion = db.ReadAnyText(values[0])
		}
	}
	rows.Close()

	if profile.AccessMode == cfg.AccessReadOnly {
		if _, readOnlyErr := connection.Exec(
			ctx, adapter.flavour.ReadOnlyStatement); readOnlyErr != nil {
			return fail(readOnlyErr)
		}
	}

	keepJSONFieldOrder(connection)

	typeNames, typeErr := readTypeNames(ctx, connection)
	if typeErr != nil {
		return fail(typeErr)
	}

	backendPID := int64(0)
	defaultSchema := "public"
	holdsStatementStats := false
	identity, identityErr := connection.Query(ctx, buildIdentityStatement(adapter.flavour))
	if identityErr != nil {
		return fail(identityErr)
	}
	if identity.Next() {
		values, valueErr := identity.Values()
		if valueErr == nil && len(values) > 1 {
			backendPID = db.ReadNonNegativeCount(values[0])
			if written := db.ReadAnyText(values[1]); written != "" {
				defaultSchema = written
			}
		}
		if valueErr == nil && len(values) > 2 {
			holdsStatementStats = readFlag(values[2])
		}
	}
	identity.Close()

	return &postgresSession{
		SessionFacts: db.SessionFacts{
			Descriptor: db.SessionDescriptor{
				Profile: profile, ServerVersion: serverVersion, DefaultSchema: defaultSchema,
			},
			Support: adapter.support,
		},
		flavour: adapter.flavour, connection: connection,
		holdsStatementStats: holdsStatementStats,
		backendPID:          backendPID, password: password, typeNames: typeNames,
		side: db.NewSideConnection(func() (*pgx.Conn, error) {
			return openPostgresConnection(context.Background(), profile, password)
		}),
		mainQueue: db.NewCallQueue(), sideQueue: db.NewCallQueue(),
	}, nil
}

// The compiler reports a part of the port this session has not answered for.
var _ db.Session = (*postgresSession)(nil)

// buildIdentityStatement returns the statement that reads the identity of the connection.
func buildIdentityStatement(flavour Flavour) string {
	written := "select pg_backend_pid() as pid, current_schema() as schema"
	if flavour.HasExtensionCatalog {
		written += `, (select count(*) from pg_extension
		                where extname = 'pg_stat_statements') > 0 as counts_statements`
	}
	return written
}
