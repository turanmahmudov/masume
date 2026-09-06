package db

import (
	"context"
	"time"
)

// The profile statement_timeout_ms setting is the context time limit passed to driver operations.

// timeLimitedSession is a session wrapper with per-operation timeouts. Other methods use the embedded session.
type timeLimitedSession struct {
	Session
	timeout time.Duration
}

// MakeTimeLimited applies a positive profile timeout. Other sessions remain unchanged.
func MakeTimeLimited(inner Session) Session {
	timeout := inner.Describe().Profile.StatementTimeout
	if timeout <= 0 {
		return inner
	}
	return &timeLimitedSession{Session: inner, timeout: timeout}
}

// unwrapSession returns the wrapped session.
func (session *timeLimitedSession) unwrapSession() Session { return session.Session }

// buildLimitedContext returns a timed context and its cancellation function.
func (session *timeLimitedSession) buildLimitedContext(
	ctx context.Context,
) (context.Context, context.CancelFunc) {
	return context.WithTimeout(ctx, session.timeout)
}

func (session *timeLimitedSession) RunQuery(
	ctx context.Context, sql string, rowLimit int, params []any,
) (QueryResult, error) {
	bound, giveBack := session.buildLimitedContext(ctx)
	defer giveBack()
	return session.Session.RunQuery(bound, sql, rowLimit, params)
}

func (session *timeLimitedSession) ReadPage(
	ctx context.Context, read ComposedRead, window ReadWindow,
) (QueryResult, error) {
	bound, giveBack := session.buildLimitedContext(ctx)
	defer giveBack()
	return session.Session.ReadPage(bound, read, window)
}

func (session *timeLimitedSession) CountRead(
	ctx context.Context, read ComposedRead,
) (int64, bool, error) {
	bound, giveBack := session.buildLimitedContext(ctx)
	defer giveBack()
	return session.Session.CountRead(bound, read)
}

func (session *timeLimitedSession) CheckStatement(
	ctx context.Context, sql string,
) (StatementProblem, bool) {
	bound, giveBack := session.buildLimitedContext(ctx)
	defer giveBack()
	return session.Session.CheckStatement(bound, sql)
}

func (session *timeLimitedSession) StreamQuery(
	ctx context.Context, sql string, params []any, batchSize int,
	onBatch func(rows [][]any, columns []ResultColumn) error,
) (int64, error) {
	bound, giveBack := session.buildLimitedContext(ctx)
	defer giveBack()
	return session.Session.StreamQuery(bound, sql, params, batchSize, onBatch)
}

func (session *timeLimitedSession) ExplainQuery(
	ctx context.Context, sql string, analyze bool,
) (QueryPlan, error) {
	bound, giveBack := session.buildLimitedContext(ctx)
	defer giveBack()
	return session.Session.ExplainQuery(bound, sql, analyze)
}

func (session *timeLimitedSession) ApplyChanges(ctx context.Context, changes []Change) error {
	bound, giveBack := session.buildLimitedContext(ctx)
	defer giveBack()
	return session.Session.ApplyChanges(bound, changes)
}
