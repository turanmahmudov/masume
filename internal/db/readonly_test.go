package db

import (
	"context"
	"strings"
	"testing"

	"github.com/turanmahmudov/masume/internal/cfg"
	"github.com/turanmahmudov/masume/internal/core"
	"github.com/turanmahmudov/masume/internal/query/language"
)

// countingSession records what reached the server behind the read-only check.
type countingSession struct {
	Session
	profile cfg.Profile
	ran     []string
	applied int
}

func (session *countingSession) Describe() SessionDescriptor {
	return SessionDescriptor{Profile: session.profile}
}

func (session *countingSession) Language() language.Language { return language.SQL }

func (session *countingSession) RunQuery(
	_ context.Context, sql string, _ int, _ []any,
) (QueryResult, error) {
	session.ran = append(session.ran, sql)
	return QueryResult{}, nil
}

func (session *countingSession) StreamQuery(
	_ context.Context, sql string, _ []any, _ int,
	_ func(rows [][]any, columns []ResultColumn) error,
) (int64, error) {
	session.ran = append(session.ran, sql)
	return 0, nil
}

func (session *countingSession) ApplyChanges(context.Context, []Change) error {
	session.applied++
	return nil
}

func (session *countingSession) ExplainQuery(
	_ context.Context, sql string, _ bool,
) (QueryPlan, error) {
	session.ran = append(session.ran, sql)
	return QueryPlan{}, nil
}

func buildReadOnlyProfile(mode cfg.AccessMode) cfg.Profile {
	return cfg.Profile{Name: "shop", Engine: core.EngineMongo, AccessMode: mode}
}

// A profile that asks for a write connection is answered as it is.
func TestMakeReadOnlyLeavesAWriteConnection(t *testing.T) {
	inner := &countingSession{profile: buildReadOnlyProfile(cfg.AccessWrite)}
	if wrapped := MakeReadOnly(inner); wrapped != Session(inner) {
		t.Error("a write connection was wrapped")
	}
}

// A read-only connection reads and never writes. MongoDB and a key store hold no read-only
// session of their own, so the client refuses the write itself.
func TestMakeReadOnlyRefusesEveryWrite(t *testing.T) {
	inner := &countingSession{profile: buildReadOnlyProfile(cfg.AccessReadOnly)}
	session := MakeReadOnly(inner)

	if _, err := session.RunQuery(context.Background(), "select * from orders", 10, nil); err != nil {
		t.Errorf("a read was refused: %v", err)
	}

	for _, sql := range []string{
		"delete from orders",
		"update orders set paid = true where id = 1",
		"load data infile '/tmp/rows.csv' into table orders",
	} {
		_, err := session.RunQuery(context.Background(), sql, 10, nil)
		if err == nil {
			t.Errorf("%q ran on a read-only connection", sql)
			continue
		}
		if !strings.Contains(DescribeError(err), "read-only") {
			t.Errorf("%q was refused with %q", sql, DescribeError(err))
		}
	}

	if _, err := session.StreamQuery(context.Background(), "delete from orders", nil, 10,
		nil); err == nil {
		t.Error("a write was exported from a read-only connection")
	}
	if err := session.ApplyChanges(context.Background(), []Change{{}}); err == nil {
		t.Error("a staged change was written on a read-only connection")
	}

	if len(inner.ran) != 1 {
		t.Errorf("the server was asked %v, wanted only the one read", inner.ran)
	}
	if inner.applied != 0 {
		t.Errorf("%d staged changes reached the server", inner.applied)
	}
}

// A plan of a write still sends the statement, so a read-only connection refuses it
// the same way it refuses a run. A batch that opens with a read is a write too.
func TestMakeReadOnlyRefusesToExplainAWrite(t *testing.T) {
	inner := &countingSession{profile: buildReadOnlyProfile(cfg.AccessReadOnly)}
	session := MakeReadOnly(inner)

	if _, err := session.ExplainQuery(context.Background(), "select * from orders", false); err != nil {
		t.Errorf("a plan of a read was refused: %v", err)
	}

	for _, sql := range []string{
		"delete from orders",
		"select 1; delete from orders",
		"select 1; set default_transaction_read_only = off",
	} {
		_, err := session.ExplainQuery(context.Background(), sql, false)
		if err == nil {
			t.Errorf("%q was planned on a read-only connection", sql)
			continue
		}
		if !strings.Contains(DescribeError(err), "read-only") {
			t.Errorf("%q was refused with %q", sql, DescribeError(err))
		}
	}

	if len(inner.ran) != 1 {
		t.Errorf("the server was asked %v, wanted only the one read", inner.ran)
	}
}
