package agent

import (
	"context"
	"fmt"
	"time"

	"github.com/turanmahmudov/masume/internal/db"
)

// A time limit for a statement a model asked for. Both callers of the tools need it, because
// nobody watches the connection it holds.

// StoppableSession is what a time limit needs of a connection: what the server does, and the
// way to tell it to stop a statement that has run past the limit.
type StoppableSession interface {
	db.SessionInfo
	db.ServerAdmin
}

// RunStatementWithin gives a statement the time the caller allows, and then stops it on the
// server. A long statement holds the only connection of its profile, and every later call
// waits for it, so a limit only in the client would leave the connection busy.
func RunStatementWithin(
	ctx context.Context, session StoppableSession, timeout time.Duration,
	run func(ctx context.Context) (db.QueryResult, error),
) (db.QueryResult, error) {
	type answer struct {
		result db.QueryResult
		err    error
	}
	// The statement runs on a context of its own, so the limit can drop it in the driver
	// as well as on the server. Without that the call goes on holding the one connection
	// of the profile, and every later call waits behind a statement nobody is reading.
	running, drop := context.WithCancel(ctx)
	defer drop()

	// Buffered, so the statement that passed its limit can still end.
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
		// The server is told first, on the context of the caller, because the cancel
		// opens a second connection and the one above is about to be dropped.
		refusal := stopRunningStatement(ctx, session, timeout)
		drop()
		waitForDroppedStatement(ran)
		return db.QueryResult{}, refusal
	}
}

// droppedStatementWait is how long the client waits for a statement it dropped to unwind.
// A driver that answers in that time gives the connection back; one that does not is left
// to end on its own, and the caller is told the statement may be running yet.
const droppedStatementWait = 5 * time.Second

// waitForDroppedStatement waits for the goroutine of a dropped statement, so the connection
// it holds is free again before the next call asks for it.
func waitForDroppedStatement[T any](ran <-chan T) {
	timer := time.NewTimer(droppedStatementWait)
	defer timer.Stop()
	select {
	case <-ran:
	case <-timer.C:
	}
}

// stopRunningStatement tells the server to drop a statement that passed its limit, and writes
// what became of it.
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
