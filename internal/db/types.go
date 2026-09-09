// Package db is the port to a server: what one open connection returns for, one
// entry per engine, and the driver that opens each.
package db

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/turanmahmudov/masume/internal/cfg"
	"github.com/turanmahmudov/masume/internal/core"
	"github.com/turanmahmudov/masume/internal/query"
	"github.com/turanmahmudov/masume/internal/query/language"
	"github.com/turanmahmudov/masume/internal/query/statement"
)

// The pure data types the query tier already holds, named here as the db tier reads
// them.
type (
	// ResultColumn is one column of a result.
	ResultColumn = query.ResultColumn
	// ForeignKey is a foreign key of a table.
	ForeignKey = query.ForeignKey
	// QueryPlan is a plan as the server explained it.
	QueryPlan = query.QueryPlan
	// PlanNode is one step of a plan.
	PlanNode = query.PlanNode
	// BoundStatement is a statement with the values it binds.
	BoundStatement = query.BoundStatement
)

// RelationKind is a kind of relation the client draws.
type RelationKind string

const (
	RelationTable            RelationKind = "table"
	RelationView             RelationKind = "view"
	RelationMaterializedView RelationKind = "materialized-view"
)

// TransactionState is the state of the transaction of the user.
type TransactionState string

const (
	TransactionNone   TransactionState = "none"
	TransactionOpen   TransactionState = "open"
	TransactionFailed TransactionState = "failed"
)

// TableRef is a relation, named by schema and name.
type TableRef struct {
	Schema        string
	Name          string
	Kind          RelationKind
	EstimatedRows int64
}

// Qualified returns the relation as the dialect writes a name.
func (table TableRef) Qualified() query.QualifiedName {
	return query.QualifiedName{Schema: table.Schema, Name: table.Name}
}

// ColumnDetail is one column of a table.
type ColumnDetail struct {
	Name         string
	DataType     string
	Nullable     bool
	DefaultValue string
	HasDefault   bool
	IsPrimaryKey bool
	// The server rejects any value sent for this column.
	IsGenerated bool
	// Only an enum column has these.
	Choices []string
}

// TableDetail is a table with its columns and foreign keys.
type TableDetail struct {
	Table       TableRef
	Columns     []ColumnDetail
	ForeignKeys []ForeignKey
}

// Relationship is a foreign key with the table it starts from.
type Relationship struct {
	ForeignKey
	Schema string
	Table  string
}

// IndexDetail is one index of a table.
type IndexDetail struct {
	Name       string
	IsUnique   bool
	IsPrimary  bool
	Definition string
}

// ConstraintKind is a kind of constraint a table can have.
type ConstraintKind string

const (
	ConstraintPrimaryKey ConstraintKind = "primary key"
	ConstraintForeignKey ConstraintKind = "foreign key"
	ConstraintUnique     ConstraintKind = "unique"
	ConstraintCheck      ConstraintKind = "check"
	ConstraintExclusion  ConstraintKind = "exclusion"
)

// ConstraintDetail is one constraint of a table.
type ConstraintDetail struct {
	Name       string
	Kind       ConstraintKind
	Definition string
}

// QueryResult is what a statement answered.
type QueryResult struct {
	// Empty for a statement with no result set, which is not the same as no rows.
	Columns []ResultColumn
	// HoldsResultSet marks a read whose result names no column. A MongoDB collection
	// holds no schema, so a find that matched nothing names none.
	HoldsResultSet bool
	Rows           [][]any
	Elapsed        time.Duration
	Truncated      bool
	Command        string
	// DDL changes no rows and reports nothing.
	Affected    int64
	HasAffected bool
}

// Change is a staged edit, built by the engine that applies it.
type Change struct {
	Description string
	Display     string
	Params      []any
	// Only the engine that built this change reads it.
	Payload any
	// Guard is a count that must return Expect before this change runs. It is set only
	// for a write on a table with no key of its own, where the same row can be held twice
	// and the write would otherwise take every copy. Only the engine that built it reads it.
	Guard  any
	Expect int
}

// ChangeTarget is the result a change is staged against.
type ChangeTarget struct {
	Table   TableRef
	Columns []ResultColumn
	Rows    [][]any
	// An empty list means no column identifies one row, so the whole row is the key.
	KeyColumns []string
}

// Activity is one server connection, from the activity list.
type Activity struct {
	PID             int64
	User            string
	ApplicationName string
	ClientAddress   string
	State           string
	Duration        time.Duration
	Query           string
}

// LockWait is one session waiting for a lock another session holds. It carries the
// statement of both sides.
type LockWait struct {
	BlockedPID   int64
	BlockedQuery string
	Waiting      time.Duration
	Mode         string
	Relation     string
	// The session that holds the lock, and how long its statement has run.
	BlockingPID   int64
	BlockingQuery string
	BlockingFor   time.Duration
}

// ServerLoad is what the server itself is carrying now. A field the server does not report
// is left at zero.
type ServerLoad struct {
	Connections    int64
	MaxConnections int64
	StartedAt      time.Time

	// Counters that count up from the moment the server started, so a rate needs two
	// readings. HasCounters is false where the server reports none of them.
	Transactions int64
	WalBytes     int64
	TempFiles    int64
	HasCounters  bool

	// The share of block reads the server answered from its own cache, from 0 to 1, and
	// unset where it has read no block.
	CacheHitRate    float64
	HasCacheHitRate bool

	// How far behind a standby is, or the worst of the standbys this server feeds, and
	// unset where there is neither.
	ReplicationLag    time.Duration
	HasReplicationLag bool
}

// StatementStat is one statement the server has been keeping count of. It is a shape of a
// statement and not one run of it, so one row stands for every run.
type StatementStat struct {
	Query     string
	Calls     int64
	MeanTime  time.Duration
	TotalTime time.Duration
	Rows      int64
}

// SchemaObjectKind is a kind of object a schema holds beside its relations.
type SchemaObjectKind string

const (
	ObjectFunction SchemaObjectKind = "function"
	ObjectSequence SchemaObjectKind = "sequence"
	ObjectType     SchemaObjectKind = "type"
	ObjectTrigger  SchemaObjectKind = "trigger"
)

// SchemaObjectKinds lists the kinds in the order the tree groups them.
var SchemaObjectKinds = []SchemaObjectKind{
	ObjectFunction, ObjectSequence, ObjectType, ObjectTrigger,
}

// IsSchemaObjectKind is true where the text names a kind of object.
func IsSchemaObjectKind(written string) bool {
	for _, kind := range SchemaObjectKinds {
		if string(kind) == written {
			return true
		}
	}
	return false
}

// SchemaObject is a function, sequence, type or trigger held by a schema.
type SchemaObject struct {
	Schema string
	Name   string
	Kind   SchemaObjectKind
	Detail string
	// The writes a trigger runs for, as `insert`, `update`, `delete` or `truncate`, more
	// than one of them separated by commas. Only a trigger has them, and only where the
	// catalog of the server names them.
	Events string
	// A DDL lookup identifies a function by its argument types.
	Identity string
}

// DbRole is a role or a user on the server.
type DbRole struct {
	Name   string
	Detail string
}

// StatementProblem is a fault the server found in a statement.
type StatementProblem struct {
	Message   string
	Offset    int
	HasOffset bool
}

// ComposedRead is a relation or a statement, composed into the read the engine runs.
type ComposedRead struct {
	Text   string
	Params []any
	// The values are written in rather than bound.
	Display  string
	Pageable bool
	// An engine that reads a cursor keeps its state here.
	Payload any
}

// BoundText is text with the values it binds.
type BoundText struct {
	Text   string
	Params []any
}

// ReadWindow is the rows one page of a read covers.
type ReadWindow struct {
	Limit  int
	Offset int
}

// SessionDescriptor is a connection as plain data, so it can be stored and read back.
type SessionDescriptor struct {
	Profile       cfg.Profile
	ServerVersion string
	DefaultSchema string
}

// SessionInfo returns what a connection is, and what its server does and does not do.
// Nothing here reaches the server.
type SessionInfo interface {
	// Describe returns the connection as plain data.
	Describe() SessionDescriptor
	// Dialect returns how SQL is written for this server.
	Dialect() *query.Dialect
	// Language returns how the buffer of a tab is read.
	Language() language.Language
	// Capabilities returns what this server does and does not do.
	Capabilities() core.Capabilities
	Composer() Composer
}

// CatalogReader returns what the server holds: its relations, the objects beside them, and
// the definition of each.
type CatalogReader interface {
	// ListSchemas returns every schema the tree draws, including one that holds nothing.
	// A MySQL-protocol or ClickHouse profile that names a database lists that database
	// alone. The other engines list the schemas of the connected database.
	ListSchemas(ctx context.Context) ([]string, error)
	// ListTables returns every relation the server holds.
	ListTables(ctx context.Context) ([]TableRef, error)
	ListRoles(ctx context.Context) ([]DbRole, error)
	ListSchemaObjects(ctx context.Context) ([]SchemaObject, error)
	ListRelationships(ctx context.Context) ([]Relationship, error)
	DescribeTable(ctx context.Context, table TableRef) (TableDetail, error)
	ListIndexes(ctx context.Context, table TableRef) ([]IndexDetail, error)
	ListConstraints(ctx context.Context, table TableRef) ([]ConstraintDetail, error)
	BuildTableDDL(ctx context.Context, table TableRef) ([]string, error)
	BuildObjectDDL(ctx context.Context, object SchemaObject) ([]string, error)
}

// QueryRunner runs what the user wrote and reads the answer back, a page or a batch at a
// time.
type QueryRunner interface {
	// RunQuery takes one row more than rowLimit, and reports it as truncated. The limit
	// must be above zero: there is no value that means every row, because the extra row
	// is what tells a full page from the last one.
	RunQuery(ctx context.Context, sql string, rowLimit int, params []any) (QueryResult, error)
	// ReadPage returns one page of a read. A read that is not pageable returns its
	// whole result.
	ReadPage(ctx context.Context, read ComposedRead, window ReadWindow) (QueryResult, error)
	// CountRead returns nothing where the server cannot count without running the
	// read twice.
	CountRead(ctx context.Context, read ComposedRead) (int64, bool, error)
	// CheckStatement returns nothing if the statement is good, or if the server could
	// not check it.
	CheckStatement(ctx context.Context, sql string) (StatementProblem, bool)
	// StreamQuery reads a batch at a time, so an export never holds the whole relation.
	// A statement with a result set hands over at least one batch, so a caller learns the
	// columns even where there are no rows. One with no result set hands over none.
	StreamQuery(
		ctx context.Context, sql string, params []any, batchSize int,
		onBatch func(rows [][]any, columns []ResultColumn) error,
	) (int64, error)
	// ExplainQuery returns the plan. Only an engine whose PlansStatement is true has one.
	ExplainQuery(ctx context.Context, sql string, analyze bool) (QueryPlan, error)
}

// TransactionKeeper holds the transaction the user opened, and applies the staged work.
type TransactionKeeper interface {
	ReadTransactionState() TransactionState
	BeginTransaction(ctx context.Context) error
	CommitTransaction(ctx context.Context) error
	RollbackTransaction(ctx context.Context) error
	// ApplyChanges applies every change, or none.
	ApplyChanges(ctx context.Context, changes []Change) error
}

// ServerAdmin reaches the connections of the server itself, rather than the data it holds.
// Only an engine whose HasServerSessions is true returns these.
type ServerAdmin interface {
	ListActivity(ctx context.Context) ([]Activity, error)
	CancelBackend(ctx context.Context, pid int64, terminate bool) (bool, error)
	CancelRunningQuery(ctx context.Context) (bool, error)
	// ListLockWaits returns every session waiting for a lock another session holds. Only
	// an engine whose ReportsLockWaits is true returns these.
	ListLockWaits(ctx context.Context) ([]LockWait, error)
	// ListSlowStatements returns the statements the server spent the most time in, the
	// slowest by mean time first. Only an engine whose ReportsStatementStats is true
	// answers it, and that is known only once the connection is open.
	ListSlowStatements(ctx context.Context, limit int) ([]StatementStat, error)
	// ReadServerLoad returns what the server is carrying now. Only an engine whose
	// ReportsServerLoad is true returns it.
	ReadServerLoad(ctx context.Context) (ServerLoad, error)
}

type SessionLifecycle interface {
	Ping(ctx context.Context) error
	Close() error
}

// Session is one open connection, and everything the app asks of a server through it. It is
// the six parts above joined, so a caller that needs one of them can name that part alone.
//
// The servers differ in three ways:
//
//   - PostgreSQL refuses every further statement of a transaction one statement
//     failed in. MySQL continues, apart from a deadlock or a lock-wait timeout.
//   - PostgreSQL caps a read through a cursor. MySQL has no cursor a client can stop
//     early, so the cap is set on the session with `sql_select_limit`.
//   - MySQL returns the CREATE statement of a table. PostgreSQL has none, so the
//     client builds one from the catalog.
type Session interface {
	SessionInfo
	CatalogReader
	QueryRunner
	TransactionKeeper
	ServerAdmin
	SessionLifecycle
}

// Adapter opens a connection for an engine.
type Adapter interface {
	Connect(ctx context.Context, profile cfg.Profile, password string) (Session, error)
}

// NeedsConfirmation applies the configured confirmation level to statement risk. The caller validates the source of the confirmation.
func NeedsConfirmation(mode cfg.ConfirmWrites, risk statement.WriteRisk) bool {
	if mode == cfg.ConfirmOff || risk == statement.RiskNone {
		return false
	}
	return mode == cfg.ConfirmWrite || mode == cfg.ConfirmAgent ||
		risk == statement.RiskDelete || risk == statement.RiskEveryRow
}

// ErrDatabase marks an error from a driver or a server.
var ErrDatabase = errors.New("database")

// NewDatabaseError builds a database error from client text. WrapDatabaseError preserves an existing driver error.
func NewDatabaseError(format string, parts ...any) error {
	return fmt.Errorf("%w: %s", ErrDatabase, fmt.Sprintf(format, parts...))
}

// databaseError is a driver error with an optional operation and display message. The original error remains in the chain.
type databaseError struct {
	// operation is the step that failed, and is empty for a call that takes only one.
	operation string
	// The optional display message in place of the driver text.
	message string
	cause   error
}

func (err *databaseError) Error() string {
	written := err.message
	if written == "" {
		written = err.cause.Error()
	}
	if err.operation == "" {
		return ErrDatabase.Error() + ": " + written
	}
	return ErrDatabase.Error() + ": " + err.operation + ": " + written
}

func (err *databaseError) Unwrap() error { return err.cause }

// Is matches the ErrDatabase sentinel.
func (err *databaseError) Is(target error) bool { return target == ErrDatabase }

// WrapDatabaseError adds the database marker. Nil and already marked errors remain unchanged.
func WrapDatabaseError(err error) error {
	if err == nil {
		return err
	}
	if errors.Is(err, ErrDatabase) {
		return err
	}
	return &databaseError{cause: err}
}

// WrapDatabaseOperation adds the failed operation to a database error.
func WrapDatabaseOperation(operation string, err error) error {
	if err == nil {
		return nil
	}
	if marked, ok := errors.AsType[*databaseError](err); ok {
		return &databaseError{
			operation: operation, message: marked.message, cause: marked.cause,
		}
	}
	return &databaseError{operation: operation, cause: err}
}

// WrapDatabaseMessage adds a display message and preserves the driver error in the chain.
func WrapDatabaseMessage(message string, cause error) error {
	if cause == nil {
		return nil
	}
	return &databaseError{message: message, cause: cause}
}

// NewUnsupportedError builds the error for a part of a session the engine of the
// connection does not have.
func NewUnsupportedError(what string) error {
	return NewDatabaseError("this server does not %s", what)
}

// markers are the errors this client wraps a reason in. Each one writes its own name as the
// prefix of the message, which the user is not shown.
var markers = []error{ErrDatabase, core.ErrEdit, statement.ErrParameter}

// DescribeError removes the prefix for a recognized error sentinel. Unmarked messages remain unchanged.
func DescribeError(err error) string {
	if err == nil {
		return ""
	}
	message := err.Error()
	for _, marker := range markers {
		if errors.Is(err, marker) {
			return strings.TrimPrefix(message, marker.Error()+": ")
		}
	}
	return message
}

// BuildConnectMessage writes why a connection could not be opened.
func BuildConnectMessage(profile cfg.Profile, err error) string {
	return fmt.Sprintf("cannot connect to %s: %s",
		cfg.DescribeProfileTarget(profile), DescribeError(err))
}

// FindTableByName resolves a unique relation, with exact names before case-insensitive matches and the default schema for ambiguous unqualified names.
func FindTableByName(
	tables []TableRef, source statement.SelectSource, defaultSchema string,
) (TableRef, bool) {
	if source.HasSchema {
		return pickSingleTable(
			narrowToTableName(narrowToSchemaName(tables, source.Schema), source.Name))
	}
	named := narrowToTableName(tables, source.Name)
	if len(named) == 1 {
		return named[0], true
	}
	return pickSingleTable(narrowToSchemaName(named, defaultSchema))
}

// narrowToTableName returns the relations of that name: the ones that match it exactly, or
// the ones that match it whatever the case where none matches exactly.
func narrowToTableName(tables []TableRef, name string) []TableRef {
	exact, folded := []TableRef{}, []TableRef{}
	lowered := strings.ToLower(name)
	for _, table := range tables {
		switch {
		case table.Name == name:
			exact = append(exact, table)
		case strings.ToLower(table.Name) == lowered:
			folded = append(folded, table)
		}
	}
	if len(exact) > 0 {
		return exact
	}
	return folded
}

// narrowToSchemaName returns the relations of that schema, by the same rule.
func narrowToSchemaName(tables []TableRef, schema string) []TableRef {
	exact, folded := []TableRef{}, []TableRef{}
	lowered := strings.ToLower(schema)
	for _, table := range tables {
		switch {
		case table.Schema == schema:
			exact = append(exact, table)
		case strings.ToLower(table.Schema) == lowered:
			folded = append(folded, table)
		}
	}
	if len(exact) > 0 {
		return exact
	}
	return folded
}

// pickSingleTable returns the one relation left, and nothing where the name reached none
// or more than one.
func pickSingleTable(tables []TableRef) (TableRef, bool) {
	if len(tables) == 1 {
		return tables[0], true
	}
	return TableRef{}, false
}
