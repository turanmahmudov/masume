package db

import (
	"context"

	"github.com/turanmahmudov/masume/internal/cfg"
	"github.com/turanmahmudov/masume/internal/query/statement"
)

// Read-only checks reject writes before driver calls, including engines without server-side read-only sessions.

// readOnlySession is a session wrapper that rejects write operations.
type readOnlySession struct {
	Session
}

// MakeReadOnly wraps read-only profiles. Other sessions remain unchanged.
func MakeReadOnly(inner Session) Session {
	if inner.Describe().Profile.AccessMode != cfg.AccessReadOnly {
		return inner
	}
	return &readOnlySession{Session: inner}
}

// unwrapSession returns the wrapped session.
func (session *readOnlySession) unwrapSession() Session { return session.Session }

// buildRefusal returns an error for write risk and nil for read-only statements.
func (session *readOnlySession) buildRefusal(sql string) error {
	if session.Language().ResolveWriteRisk(sql) == statement.RiskNone {
		return nil
	}
	return NewDatabaseError("this connection is read-only; the statement was not sent")
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

// ApplyChanges rejects all staged changes.
func (session *readOnlySession) ApplyChanges(context.Context, []Change) error {
	return NewDatabaseError("this connection is read-only; the changes were not sent")
}
