package language_test

import (
	"testing"

	"github.com/turanmahmudov/masume/internal/db/redis"
	"github.com/turanmahmudov/masume/internal/query/language"
	"github.com/turanmahmudov/masume/internal/query/statement"
)

// Every statement of a buffer runs, so a buffer that opens with a read is as risky as
// the worst statement after it.
func TestResolveWriteRiskOfABufferTakesTheWorstStatement(t *testing.T) {
	for _, held := range []struct {
		name string
		sql  string
		want statement.WriteRisk
	}{
		{"reads only", "select 1; select 2", statement.RiskNone},
		{"a write after a read", "select 1; insert into orders (id) values (1)",
			statement.RiskWrite},
		{"a call after a read", "select 1; call proc()", statement.RiskWrite},
		{"a copy after a read", "select 1; copy orders from stdin", statement.RiskWrite},
		{"a vacuum after a read", "select 1; vacuum full", statement.RiskWrite},
		{"a setting that opens writes after a read",
			"select 1; set default_transaction_read_only = off", statement.RiskWrite},
		{"a delete after a read", "select 1; delete from orders", statement.RiskEveryRow},
	} {
		t.Run(held.name, func(t *testing.T) {
			if got := language.SQL.ResolveWriteRisk(held.sql); got != held.want {
				t.Errorf("%q reads as %q, wanted %q", held.sql, got, held.want)
			}
		})
	}
}

// A batch is as risky as the worst statement in it, whatever order they were written in,
// because every one of them runs.
func TestResolveBatchRiskTakesTheWorstOfABatch(t *testing.T) {
	for _, held := range []struct {
		name       string
		statements []string
		want       statement.WriteRisk
	}{
		{"nothing", nil, statement.RiskNone},
		{"reads only", []string{"select 1", "select 2"}, statement.RiskNone},
		{"a write among reads", []string{"select 1", "insert into orders (id) values (1)"},
			statement.RiskWrite},
		{"the worst is not the last",
			[]string{"delete from orders", "select 1"}, statement.RiskEveryRow},
		{"a delete with a where among writes",
			[]string{"insert into orders (id) values (1)", "delete from orders where id = 1"},
			statement.RiskDelete},
	} {
		t.Run(held.name, func(t *testing.T) {
			answered := language.ResolveBatchRisk(held.statements, language.SQL)
			if answered != held.want {
				t.Errorf("the batch reads %q, wanted %q", answered, held.want)
			}
		})
	}
}

// The two SQL languages differ in how they read a buffer, and both have to answer every part
// of the contract, because the tab above them does not know which it holds.
func TestEverySqlLanguageAnswersTheWholeContract(t *testing.T) {
	for _, held := range []struct {
		name  string
		spoke language.Language
	}{
		{"standard", language.SQL},
		{"mysql", language.Mysql},
	} {
		t.Run(held.name, func(t *testing.T) {
			const sql = "select id from orders where id = 1"

			if len(held.spoke.Tokenize(sql)) == 0 {
				t.Error("the buffer read as no tokens")
			}
			if got := held.spoke.SplitStatements(sql); len(got) != 1 {
				t.Errorf("one statement split into %d", len(got))
			}
			if got := held.spoke.ReadStatementAtOffset(sql, 3); got == "" {
				t.Error("the caret answered no statement")
			}
			if got := held.spoke.ResolveWriteRisk(sql); got != statement.RiskNone {
				t.Errorf("a read reads as %q", got)
			}
			if !held.spoke.CanExplain(sql) {
				t.Error("a read cannot be planned")
			}
			if held.spoke.ChangesCatalog(sql) {
				t.Error("a read was said to make the catalog stale")
			}
			if got := held.spoke.FormatStatement(sql); got == "" {
				t.Error("the buffer formatted to nothing")
			}
		})
	}
}

// A statement that changes the shape of the database makes the catalog stale, so the tree is
// read again rather than drawn from what it held.
func TestChangesCatalogNamesTheStatementsThatReshape(t *testing.T) {
	for _, held := range []struct {
		sql  string
		want bool
	}{
		{"create table orders (id int)", true},
		{"drop table orders", true},
		{"alter table orders add column paid boolean", true},
		{"create index orders_idx on orders (id)", true},

		{"select * from orders", false},
		{"insert into orders (id) values (1)", false},
		{"update orders set id = 2", false},
		{"delete from orders", false},
	} {
		if answered := language.SQL.ChangesCatalog(held.sql); answered != held.want {
			t.Errorf("%q makes the catalog stale = %v, wanted %v", held.sql, answered, held.want)
		}
	}
}

// A server cannot plan every statement, and asking it to plan one it refuses answers an error
// where the pane wanted a plan.
func TestCanExplainRefusesWhatNoServerPlans(t *testing.T) {
	for _, held := range []struct {
		sql  string
		want bool
	}{
		{"select * from orders", true},
		{"insert into orders (id) values (1)", true},
		{"update orders set id = 1", true},
		{"delete from orders", true},

		{"drop table orders", false},
		{"create table orders (id int)", false},
		{"", false},
	} {
		if answered := language.SQL.CanExplain(held.sql); answered != held.want {
			t.Errorf("%q can be planned = %v, wanted %v", held.sql, answered, held.want)
		}
	}
}

// A key store takes commands, not SQL, so its language reads a buffer its own way and must
// still answer everything the tab asks.
func TestTheCommandLanguageAnswersTheWholeContract(t *testing.T) {
	spoke := redis.Support.Language
	if spoke == nil {
		t.Fatal("the key store answers no language")
	}

	const command = "GET order:1"
	if len(spoke.SplitStatements(command)) != 1 {
		t.Errorf("one command split into %d", len(spoke.SplitStatements(command)))
	}
	if spoke.ResolveWriteRisk(command) != statement.RiskNone {
		t.Errorf("a read of one key reads as %q", spoke.ResolveWriteRisk(command))
	}
	// A command that removes a key is a write, and the confirmation depends on it.
	if held := spoke.ResolveWriteRisk("DEL order:1"); held == statement.RiskNone {
		t.Error("a command that removes a key reads as a read")
	}
	// No key store plans a command.
	if spoke.CanExplain(command) {
		t.Error("a key store reports that it plans a command")
	}
}

// Every command of a batch runs, so a batch holding one that writes is a write.
func TestResolveBatchRiskReadsACommandBatch(t *testing.T) {
	spoke := redis.Support.Language
	held := language.ResolveBatchRisk([]string{"GET order:1", "DEL order:2"}, spoke)
	if held == statement.RiskNone {
		t.Error("a batch holding a command that removes a key reads as a read")
	}
}
