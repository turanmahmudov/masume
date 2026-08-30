package db

import (
	"context"
	"sync"

	"github.com/turanmahmudov/masume/internal/core"
	"github.com/turanmahmudov/masume/internal/query"
	"github.com/turanmahmudov/masume/internal/query/language"
)

// A session that can be replaced under the tabs that use it, so a reconnect does not rebuild
// every tab. It never runs a statement again, because a statement that failed can still have
// reached the server.

// ReconnectOutcome is the result of one attempt at reconnecting.
type ReconnectOutcome struct {
	Reconnected bool
	// True where a transaction was open. The server drops it whether or not the
	// connection comes back.
	TransactionLost bool
	Problem         string
}

// heldSession is one session with the calls inside it counted, so a session a reconnect
// replaced closes only once the last call leaves it.
type heldSession struct {
	session Session
	calls   int
	retired bool
}

// ReconnectingSession wraps one session and can open another in its place.
type ReconnectingSession struct {
	adapter  Adapter
	password string

	// guard holds the swap of the session and the count of the calls inside each one,
	// because a read of the frame and a reconnect run on their own goroutines.
	guard sync.Mutex
	inner *heldSession
	// True while a reconnect runs, so two failures start only one.
	reconnecting bool
}

// MakeReconnectable wraps the session so it can be replaced. A profile with the keepalive
// turned off is answered as it is, because nothing checks it and nothing reconnects it.
func MakeReconnectable(inner Session, adapter Adapter, password string) Session {
	if inner.Describe().Profile.Keepalive <= 0 {
		return inner
	}
	return &ReconnectingSession{
		adapter: adapter, password: password, inner: &heldSession{session: inner},
	}
}

// FindReconnectable returns the session as a reconnecting one, and reports whether it is one.
// A session inside another is found too, so a wrapper above it hides nothing.
func FindReconnectable(session Session) (*ReconnectingSession, bool) {
	for session != nil {
		if held, is := session.(*ReconnectingSession); is {
			return held, true
		}
		wrapper, wraps := session.(interface{ unwrapSession() Session })
		if !wraps {
			return nil, false
		}
		session = wrapper.unwrapSession()
	}
	return nil, false
}

// hold returns the session every call goes to, and what gives it back. A call is counted
// while it runs, so a reconnect cannot close the connection it is reading.
func (session *ReconnectingSession) hold() (Session, func()) {
	session.guard.Lock()
	defer session.guard.Unlock()
	held := session.inner
	held.calls++
	return held.session, func() { session.leave(held) }
}

// leave gives back one call, and closes a retired session once its last call is out.
func (session *ReconnectingSession) leave(held *heldSession) {
	session.guard.Lock()
	held.calls--
	closing := held.retired && held.calls == 0
	session.guard.Unlock()

	if closing {
		_ = held.session.Close()
	}
}

// Reconnect opens a connection in place of the one that stopped answering. The old session
// is retired at once and closes when the last call inside it leaves, so a read already
// under way returns before its connection goes.
func (session *ReconnectingSession) Reconnect(ctx context.Context) ReconnectOutcome {
	session.guard.Lock()
	if session.reconnecting {
		session.guard.Unlock()
		return ReconnectOutcome{Problem: "already reconnecting"}
	}
	session.reconnecting = true
	old := session.inner
	lost := old.session.ReadTransactionState() != TransactionNone
	profile := old.session.Describe().Profile
	session.guard.Unlock()

	fresh, err := session.adapter.Connect(ctx, profile, session.password)

	session.guard.Lock()
	session.reconnecting = false
	if err != nil {
		session.guard.Unlock()
		return ReconnectOutcome{TransactionLost: lost, Problem: DescribeError(err)}
	}
	session.inner = &heldSession{session: fresh}
	old.retired = true
	closing := old.calls == 0
	session.guard.Unlock()

	if closing {
		_ = old.session.Close()
	}
	return ReconnectOutcome{Reconnected: true, TransactionLost: lost}
}

func (session *ReconnectingSession) Describe() SessionDescriptor {
	held, done := session.hold()
	defer done()
	return held.Describe()
}

func (session *ReconnectingSession) Dialect() *query.Dialect {
	held, done := session.hold()
	defer done()
	return held.Dialect()
}

func (session *ReconnectingSession) Language() language.Language {
	held, done := session.hold()
	defer done()
	return held.Language()
}

func (session *ReconnectingSession) Capabilities() core.Capabilities {
	held, done := session.hold()
	defer done()
	return held.Capabilities()
}

func (session *ReconnectingSession) Composer() Composer {
	held, done := session.hold()
	defer done()
	return held.Composer()
}

func (session *ReconnectingSession) ListTables(ctx context.Context) ([]TableRef, error) {
	held, done := session.hold()
	defer done()
	return held.ListTables(ctx)
}

func (session *ReconnectingSession) ListRoles(ctx context.Context) ([]DbRole, error) {
	held, done := session.hold()
	defer done()
	return held.ListRoles(ctx)
}

func (session *ReconnectingSession) ListSchemaObjects(
	ctx context.Context,
) ([]SchemaObject, error) {
	held, done := session.hold()
	defer done()
	return held.ListSchemaObjects(ctx)
}

func (session *ReconnectingSession) ListRelationships(
	ctx context.Context,
) ([]Relationship, error) {
	held, done := session.hold()
	defer done()
	return held.ListRelationships(ctx)
}

func (session *ReconnectingSession) DescribeTable(
	ctx context.Context, table TableRef,
) (TableDetail, error) {
	held, done := session.hold()
	defer done()
	return held.DescribeTable(ctx, table)
}

func (session *ReconnectingSession) ListIndexes(
	ctx context.Context, table TableRef,
) ([]IndexDetail, error) {
	held, done := session.hold()
	defer done()
	return held.ListIndexes(ctx, table)
}

func (session *ReconnectingSession) ListConstraints(
	ctx context.Context, table TableRef,
) ([]ConstraintDetail, error) {
	held, done := session.hold()
	defer done()
	return held.ListConstraints(ctx, table)
}

func (session *ReconnectingSession) BuildTableDDL(
	ctx context.Context, table TableRef,
) ([]string, error) {
	held, done := session.hold()
	defer done()
	return held.BuildTableDDL(ctx, table)
}

func (session *ReconnectingSession) BuildObjectDDL(
	ctx context.Context, object SchemaObject,
) ([]string, error) {
	held, done := session.hold()
	defer done()
	return held.BuildObjectDDL(ctx, object)
}

func (session *ReconnectingSession) RunQuery(
	ctx context.Context, sql string, rowLimit int, params []any,
) (QueryResult, error) {
	held, done := session.hold()
	defer done()
	return held.RunQuery(ctx, sql, rowLimit, params)
}

func (session *ReconnectingSession) ReadPage(
	ctx context.Context, read ComposedRead, window ReadWindow,
) (QueryResult, error) {
	held, done := session.hold()
	defer done()
	return held.ReadPage(ctx, read, window)
}

func (session *ReconnectingSession) CountRead(
	ctx context.Context, read ComposedRead,
) (int64, bool, error) {
	held, done := session.hold()
	defer done()
	return held.CountRead(ctx, read)
}

func (session *ReconnectingSession) CheckStatement(
	ctx context.Context, sql string,
) (StatementProblem, bool) {
	held, done := session.hold()
	defer done()
	return held.CheckStatement(ctx, sql)
}

func (session *ReconnectingSession) StreamQuery(
	ctx context.Context, sql string, params []any, batchSize int,
	onBatch func(rows [][]any, columns []ResultColumn) error,
) (int64, error) {
	held, done := session.hold()
	defer done()
	return held.StreamQuery(ctx, sql, params, batchSize, onBatch)
}

func (session *ReconnectingSession) ExplainQuery(
	ctx context.Context, sql string, analyze bool,
) (QueryPlan, error) {
	held, done := session.hold()
	defer done()
	return held.ExplainQuery(ctx, sql, analyze)
}

func (session *ReconnectingSession) ReadTransactionState() TransactionState {
	held, done := session.hold()
	defer done()
	return held.ReadTransactionState()
}

func (session *ReconnectingSession) BeginTransaction(ctx context.Context) error {
	held, done := session.hold()
	defer done()
	return held.BeginTransaction(ctx)
}

func (session *ReconnectingSession) CommitTransaction(ctx context.Context) error {
	held, done := session.hold()
	defer done()
	return held.CommitTransaction(ctx)
}

func (session *ReconnectingSession) RollbackTransaction(ctx context.Context) error {
	held, done := session.hold()
	defer done()
	return held.RollbackTransaction(ctx)
}

func (session *ReconnectingSession) ApplyChanges(
	ctx context.Context, changes []Change,
) error {
	held, done := session.hold()
	defer done()
	return held.ApplyChanges(ctx, changes)
}

func (session *ReconnectingSession) ListActivity(ctx context.Context) ([]Activity, error) {
	held, done := session.hold()
	defer done()
	return held.ListActivity(ctx)
}

func (session *ReconnectingSession) CancelBackend(
	ctx context.Context, pid int64, terminate bool,
) (bool, error) {
	held, done := session.hold()
	defer done()
	return held.CancelBackend(ctx, pid, terminate)
}

func (session *ReconnectingSession) CancelRunningQuery(ctx context.Context) (bool, error) {
	held, done := session.hold()
	defer done()
	return held.CancelRunningQuery(ctx)
}

func (session *ReconnectingSession) Ping(ctx context.Context) error {
	held, done := session.hold()
	defer done()
	return held.Ping(ctx)
}

func (session *ReconnectingSession) Close() error {
	held, done := session.hold()
	defer done()
	return held.Close()
}
