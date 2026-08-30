package agent

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/turanmahmudov/masume/internal/core"
	"github.com/turanmahmudov/masume/internal/db"
)

// stoppableSession is a session that reports what it can do and whether it was told to stop.
type stoppableSession struct {
	db.Session
	cancels bool
	stops   bool
	told    bool
	err     error
}

func (session *stoppableSession) Capabilities() core.Capabilities {
	return core.Capabilities{CancelsRunningQuery: session.cancels}
}

func (session *stoppableSession) CancelRunningQuery(context.Context) (bool, error) {
	session.told = true
	return session.stops, session.err
}

func TestRunStatementWithinAnswersInTime(t *testing.T) {
	session := &stoppableSession{cancels: true, stops: true}
	answered, err := RunStatementWithin(context.Background(), session, time.Second,
		func(context.Context) (db.QueryResult, error) {
			return db.QueryResult{Command: "SELECT"}, nil
		})
	if err != nil {
		t.Fatalf("the statement failed: %v", err)
	}
	if answered.Command != "SELECT" {
		t.Errorf("the result reads %v", answered)
	}
	if session.told {
		t.Error("a statement that answered in time was told to stop")
	}
}

func TestRunStatementWithinCarriesTheFailure(t *testing.T) {
	wanted := errors.New("the server said no")
	_, err := RunStatementWithin(context.Background(), &stoppableSession{}, time.Second,
		func(context.Context) (db.QueryResult, error) { return db.QueryResult{}, wanted })
	if !errors.Is(err, wanted) {
		t.Errorf("the failure reads %v, wanted %v", err, wanted)
	}
}

func TestRunStatementWithinStopsALongStatement(t *testing.T) {
	cases := []struct {
		name    string
		cancels bool
		stops   bool
		err     error
		wanted  string
	}{
		{"a server that cannot be told to stop", false, false, nil,
			"the statement was still running after 10 ms, and this engine cannot be told " +
				"to stop it, so it may be running yet; narrow it, or give it a LIMIT"},
		{"a server that stopped it", true, true, nil,
			"the statement was still running after 10 ms and was cancelled; " +
				"narrow it, or give it a LIMIT"},
		{"a server that refused to stop it", true, false, nil,
			"the statement was still running after 10 ms and was left running, since the " +
				"server refused to cancel it; narrow it, or give it a LIMIT"},
		{"a server that failed to stop it", true, true, errors.New("no"),
			"the statement was still running after 10 ms and was left running, since the " +
				"server refused to cancel it; narrow it, or give it a LIMIT"},
	}
	for _, held := range cases {
		session := &stoppableSession{
			cancels: held.cancels, stops: held.stops, err: held.err,
		}
		dropped := false
		_, err := RunStatementWithin(
			context.Background(), session, 10*time.Millisecond,
			func(running context.Context) (db.QueryResult, error) {
				// The statement runs until the limit drops it, the way a driver that
				// watches its context does.
				<-running.Done()
				dropped = true
				return db.QueryResult{}, running.Err()
			})
		if !dropped {
			t.Errorf("%s: the statement was not dropped when the limit passed", held.name)
		}
		if err == nil {
			t.Errorf("%s: a long statement answered without a failure", held.name)
			continue
		}
		if said := db.DescribeError(err); said != held.wanted {
			t.Errorf("%s: the failure reads\n  %s\n  wanted %s", held.name, said, held.wanted)
		}
		if held.cancels && !session.told {
			t.Errorf("%s: the server was not told to stop", held.name)
		}
	}
}

// A statement that passed its limit is dropped in the driver as well as on the server, and
// the caller waits for it to unwind. Without that the call goes on holding the one
// connection of the profile, and the next call waits behind a statement nobody reads.
func TestRunStatementWithinWaitsForTheStatementItDropped(t *testing.T) {
	session := &stoppableSession{cancels: true, stops: true}
	unwound := make(chan struct{})

	_, err := RunStatementWithin(context.Background(), session, 10*time.Millisecond,
		func(running context.Context) (db.QueryResult, error) {
			<-running.Done()
			close(unwound)
			return db.QueryResult{}, running.Err()
		})
	if err == nil {
		t.Fatal("a statement past its limit answered without a failure")
	}

	// The call came back only once the statement had ended, so the connection is free.
	select {
	case <-unwound:
	default:
		t.Error("the call came back while the statement it dropped was still running")
	}
}
