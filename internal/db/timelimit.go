package db

import (
	"context"
	"time"
)

// A time limit on every statement of one profile, from `statement_timeout_ms`. It sits
// above the driver, so the limit holds for every engine and not only for the two that
// have a timeout of their own. The driver cancels the statement on the server when the
// limit passes, because the context it was given is done.

// timeLimitedSession is one session with a limit on each statement it runs. It embeds the
// session it wraps, so a call that runs no statement passes straight through.
type timeLimitedSession struct {
	Session
	timeout time.Duration
}

// MakeTimeLimited returns the session with the limit its profile named. A profile that
// named none, or named zero, is answered as it is.
func MakeTimeLimited(inner Session) Session {
	timeout := inner.Describe().Profile.StatementTimeout
	if timeout <= 0 {
		return inner
	}
	return &timeLimitedSession{Session: inner, timeout: timeout}
}

// unwrapSession returns the session inside, so a reader that looks for one kind of
// session can look through this one.
func (session *timeLimitedSession) unwrapSession() Session { return session.Session }

// buildLimitedContext returns the context one statement runs under, and what gives its
// time back.
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
