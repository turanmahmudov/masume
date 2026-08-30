package db

import (
	"context"
	"errors"
	"fmt"
	"testing"
)

type driverFault struct{ code int }

func (f *driverFault) Error() string { return fmt.Sprintf("driver fault %d", f.code) }

func TestDatabaseErrorKeepsTheChainAndTheText(t *testing.T) {
	inner := &driverFault{code: 42}

	wrapped := WrapDatabaseError(inner)
	if !errors.Is(wrapped, ErrDatabase) {
		t.Error("the wrap does not read as a database error")
	}
	var reached *driverFault
	if !errors.As(wrapped, &reached) || reached.code != 42 {
		t.Error("errors.As cannot reach the driver error through the wrap")
	}
	if DescribeError(wrapped) != "driver fault 42" {
		t.Errorf("the user reads %q, wanted the bare driver text", DescribeError(wrapped))
	}

	// Wrapping twice must not double the mark.
	if DescribeError(WrapDatabaseError(wrapped)) != "driver fault 42" {
		t.Error("a second wrap changed the message")
	}

	named := WrapDatabaseOperation("reading the indexes", wrapped)
	if !errors.Is(named, ErrDatabase) {
		t.Error("the named wrap lost the mark")
	}
	if !errors.As(named, &reached) {
		t.Error("the named wrap lost the driver error")
	}
	if DescribeError(named) != "reading the indexes: driver fault 42" {
		t.Errorf("the user reads %q", DescribeError(named))
	}

	if WrapDatabaseError(nil) != nil || WrapDatabaseOperation("x", nil) != nil {
		t.Error("no error must stay no error")
	}

	// A context reason still reads through the wrap, which the read loops rely on.
	cancelled := WrapDatabaseError(context.Canceled)
	if !errors.Is(cancelled, context.Canceled) {
		t.Error("the wrap hides context.Canceled")
	}

	// A message this client writes itself keeps the cause reachable.
	spoken := WrapDatabaseMessage("cannot connect to host", inner)
	if DescribeError(spoken) != "cannot connect to host" {
		t.Errorf("the user reads %q", DescribeError(spoken))
	}
	if !errors.As(spoken, &reached) {
		t.Error("the written message dropped the driver error")
	}
}
