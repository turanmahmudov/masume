package agent

import (
	"context"
	"fmt"
	"time"

	"github.com/turanmahmudov/masume/internal/db"
)

// A time limit for a statement started by a model. Both callers of the tools need it,
// because no user watches the connection.

// StoppableSession is the part of a connection a time limit needs: the capabilities of the
// server, and the call that stops a statement above the limit.
type StoppableSession interface {
	db.SessionInfo
	db.ServerAdmin
}

// RunStatementWithin runs a statement with the time limit of the caller and then stops it on
// the server. A long statement holds the only connection of its profile, and every later
// call waits for it, so a limit in the client alone would leave the connection busy.
func RunStatementWithin(
	ctx context.Context, session StoppableSession, timeout time.Duration,
	run func(ctx context.Context) (db.QueryResult, error),
) (db.QueryResult, error) {
	type answer struct {
		result db.QueryResult
		err    error
	}
	// The statement runs on its own context, so the limit stops it in the driver and on
	// the server. Without this the call keeps the only connection of the profile, and
	// every later call waits for a statement that nobody reads.
	running, drop := context.WithCancel(ctx)
	defer drop()

	// Buffered, so a statement above its limit can still finish.
	ran := make(chan answer, 1)
	go func() {
		result, err := run(running)
		ran <- answer{result: result, err: err}
	}()

	timer := time.NewTimer(timeout)
	defer timer.Stop()
	select {
	case held := <-ran:
		return held.result, held.err
	case <-timer.C:
		// The server is stopped first, on the context of the caller, because the cancel
		// opens a second connection and the context above is about to be cancelled.
		refusal := stopRunningStatement(ctx, session, timeout)
		drop()
		waitForDroppedStatement(ran)
		return db.QueryResult{}, refusal
	}
}

// droppedStatementWait is the time the client waits for a cancelled statement to finish. A
// driver that returns in that time gives the connection back. A driver that does not is left
// to finish on its own, and the caller is told the statement can still be running.
const droppedStatementWait = 5 * time.Second

// waitForDroppedStatement waits for the goroutine of a cancelled statement, so its
// connection is free before the next call needs it.
func waitForDroppedStatement[T any](ran <-chan T) {
	timer := time.NewTimer(droppedStatementWait)
	defer timer.Stop()
	select {
	case <-ran:
	case <-timer.C:
	}
}

// stopRunningStatement asks the server to stop a statement above its limit and returns the
// result.
func stopRunningStatement(
	ctx context.Context, session StoppableSession, timeout time.Duration,
) error {
	waited := fmt.Sprintf("the statement was still running after %d ms", timeout.Milliseconds())
	advice := "; narrow it, or give it a LIMIT"
	if !session.Capabilities().CancelsRunningQuery {
		return db.NewDatabaseError("%s", waited+
			", and this engine cannot be told to stop it, so it may be running yet"+advice)
	}
	stopped, err := session.CancelRunningQuery(ctx)
	if err != nil || !stopped {
		return db.NewDatabaseError("%s", waited+
			" and was left running, since the server refused to cancel it"+advice)
	}
	return db.NewDatabaseError("%s", waited+" and was cancelled"+advice)
}
