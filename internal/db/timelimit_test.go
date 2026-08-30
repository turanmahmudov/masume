package db

import (
	"context"
	"testing"
	"time"

	"github.com/turanmahmudov/masume/internal/cfg"
)

// slowSession answers a statement that runs until its context is done, and records the
// deadline the caller gave it.
type slowSession struct {
	Session
	profile  cfg.Profile
	deadline bool
}

func (session *slowSession) Describe() SessionDescriptor {
	return SessionDescriptor{Profile: session.profile}
}

func (session *slowSession) RunQuery(
	ctx context.Context, _ string, _ int, _ []any,
) (QueryResult, error) {
	_, session.deadline = ctx.Deadline()
	<-ctx.Done()
	return QueryResult{}, ctx.Err()
}

// A profile that named no limit runs as it is, so nothing is wrapped and no statement is
// cut short.
func TestMakeTimeLimitedLeavesAProfileWithNoLimit(t *testing.T) {
	inner := &slowSession{profile: cfg.Profile{Name: "held"}}
	if wrapped := MakeTimeLimited(inner); wrapped != Session(inner) {
		t.Error("a profile with no limit was wrapped")
	}
}

// A statement that runs past the limit of its profile is stopped, because a long statement
// holds the only connection of the profile and every later call waits for it.
func TestMakeTimeLimitedStopsAStatementThatPassesTheLimit(t *testing.T) {
	inner := &slowSession{
		profile: cfg.Profile{Name: "held", StatementTimeout: 20 * time.Millisecond},
	}
	session := MakeTimeLimited(inner)

	started := time.Now()
	if _, err := session.RunQuery(context.Background(), "select 1", 10, nil); err == nil {
		t.Fatal("the statement answered although it never ended")
	}
	if !inner.deadline {
		t.Error("the statement ran with no deadline")
	}
	if held := time.Since(started); held > time.Second {
		t.Errorf("the statement ran for %v, wanted it stopped at the limit", held)
	}
}

// A reconnecting session inside the limit is still found, so a lost connection is opened
// again under the tabs that use it.
func TestFindReconnectableLooksThroughTheLimit(t *testing.T) {
	profile := cfg.Profile{
		Name: "held", Keepalive: time.Second, StatementTimeout: time.Second,
	}
	inner := &stubSession{profile: profile}
	session := MakeTimeLimited(MakeReconnectable(inner, &stubAdapter{}, ""))
	if _, found := FindReconnectable(session); !found {
		t.Error("the reconnecting session was hidden by the limit")
	}
}
