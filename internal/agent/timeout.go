package agent

import (
	"context"
	"fmt"
	"time"

	"github.com/turanmahmudov/masume/internal/db"
)

// Time limits for statements started by a model.

// StoppableSession is the connection interface for statement cancellation.
type StoppableSession interface {
	db.SessionInfo
	db.ServerAdmin
}

// RunStatementWithin runs a statement with a timeout and requests server cancellation when the timeout expires.
func RunStatementWithin(
	ctx context.Context, session StoppableSession, timeout time.Duration,
	run func(ctx context.Context) (db.QueryResult, error),
) (db.QueryResult, error) {
	type answer struct {
		result db.QueryResult
		err    error
	}
	// Driver cancellation uses a separate context from server cancellation.
	running, drop := context.WithCancel(ctx)
	defer drop()

	// The buffered channel accepts a response after timeout.
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
		// Server cancellation opens a second connection and requires an active context.
		refusal := stopRunningStatement(ctx, session, timeout)
		drop()
		waitForDroppedStatement(ran)
		return db.QueryResult{}, refusal
	}
}

// droppedStatementWait is the maximum wait for the driver after cancellation.
const droppedStatementWait = 5 * time.Second

// waitForDroppedStatement waits for the statement goroutine until the cancellation timeout.
func waitForDroppedStatement[T any](ran <-chan T) {
	timer := time.NewTimer(droppedStatementWait)
	defer timer.Stop()
	select {
	case <-ran:
	case <-timer.C:
	}
}

// stopRunningStatement requests server cancellation and returns a timeout error.
func stopRunningStatement(
	ctx context.Context, session StoppableSession, timeout time.Duration,
) error {
	waited := fmt.Sprintf("the statement was still running after %d ms", timeout.Milliseconds())
	advice := "; use a more specific predicate or a query LIMIT where appropriate"
	if !session.Capabilities().CancelsRunningQuery {
		return db.NewDatabaseError("%s", waited+
			"; this engine does not support cancellation, and the statement may still be running"+advice)
	}
	stopped, err := session.CancelRunningQuery(ctx)
	if err != nil || !stopped {
		return db.NewDatabaseError("%s", waited+
			"; server cancellation failed, and the statement may still be running"+advice)
	}
	return db.NewDatabaseError("%s", waited+" and was cancelled"+advice)
}
