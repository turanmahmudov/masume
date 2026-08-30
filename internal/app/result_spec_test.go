package app_test

import (
	"testing"

	"github.com/turanmahmudov/masume/internal/app"
	"github.com/turanmahmudov/masume/internal/db"
)

func TestSkipRestMarksTheStatementsABatchNeverReached(t *testing.T) {
	// A statement that failed stops the batch. The ones after it must not sit waiting for
	// a server that is never asked.
	store := &app.ResultStore{}
	store.Start([]string{"select 1", "select 2", "select 3"}, 100)
	store.Fail(0, "boom")
	store.SkipRest(1, "not run: an earlier statement failed")

	results := store.Results()
	if results[0].State.Message != "boom" {
		t.Errorf("the failed statement says %q", results[0].State.Message)
	}
	for _, at := range []int{1, 2} {
		if results[at].State.Kind != app.QueryFailed {
			t.Errorf("statement %d is %v, want failed", at, results[at].State.Kind)
		}
		if results[at].State.Message != "not run: an earlier statement failed" {
			t.Errorf("statement %d says %q", at, results[at].State.Message)
		}
	}
	if store.IsRunning() {
		t.Error("the store still reports a run in flight")
	}
}

func TestSkipRestLeavesAnAnsweredStatementAlone(t *testing.T) {
	store := &app.ResultStore{}
	store.Start([]string{"select 1", "select 2"}, 100)
	store.Succeed(0, db.ComposedRead{}, db.QueryResult{})
	store.SkipRest(0, "not run")

	if store.Results()[0].State.Kind != app.QuerySucceeded {
		t.Error("a statement that answered was marked as never run")
	}
	if store.Results()[1].State.Kind != app.QueryFailed {
		t.Error("the statement after it was not marked")
	}
}
