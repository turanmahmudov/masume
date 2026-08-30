package db

import (
	"context"

	"github.com/turanmahmudov/masume/internal/cfg"
	"github.com/turanmahmudov/masume/internal/query/statement"
)

// A read-only connection refused in the client, above the driver. PostgreSQL, MySQL and
// SQLite hold a read-only session of their own, and the server refuses the write. MongoDB
// and a key store hold none, so without this the mode would be a promise the connection
// does not keep, and every path that runs a statement would have to check for itself.

// readOnlySession is one session that runs only what changes nothing.
type readOnlySession struct {
	Session
}

// MakeReadOnly returns the session with every write refused, where the profile asked for a
// read-only connection. Any other profile is answered as it is.
func MakeReadOnly(inner Session) Session {
	if inner.Describe().Profile.AccessMode != cfg.AccessReadOnly {
		return inner
	}
	return &readOnlySession{Session: inner}
}

// unwrapSession returns the session inside, so a reader that looks for one kind of
// session can look through this one.
func (session *readOnlySession) unwrapSession() Session { return session.Session }

// buildRefusal returns why the connection refuses this statement, and nothing where it
// changes nothing.
func (session *readOnlySession) buildRefusal(sql string) error {
	if session.Language().ResolveWriteRisk(sql) == statement.RiskNone {
		return nil
	}
	return NewDatabaseError("this connection is read-only, so the statement was not sent")
}

func (session *readOnlySession) RunQuery(
	ctx context.Context, sql string, rowLimit int, params []any,
) (QueryResult, error) {
	if refusal := session.buildRefusal(sql); refusal != nil {
		return QueryResult{}, refusal
	}
	return session.Session.RunQuery(ctx, sql, rowLimit, params)
}

func (session *readOnlySession) StreamQuery(
	ctx context.Context, sql string, params []any, batchSize int,
	onBatch func(rows [][]any, columns []ResultColumn) error,
) (int64, error) {
	if refusal := session.buildRefusal(sql); refusal != nil {
		return 0, refusal
	}
	return session.Session.StreamQuery(ctx, sql, params, batchSize, onBatch)
}

func (session *readOnlySession) ExplainQuery(
	ctx context.Context, sql string, analyze bool,
) (QueryPlan, error) {
	if refusal := session.buildRefusal(sql); refusal != nil {
		return QueryPlan{}, refusal
	}
	return session.Session.ExplainQuery(ctx, sql, analyze)
}

// ApplyChanges is refused whatever it holds, because every staged change is a write.
func (session *readOnlySession) ApplyChanges(context.Context, []Change) error {
	return NewDatabaseError("this connection is read-only, so nothing was written")
}
