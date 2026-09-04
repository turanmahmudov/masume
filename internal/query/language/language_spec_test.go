package language_test

import (
	"testing"

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

// Every command of a batch runs, so a batch holding one that writes is a write.

// A statement that bounds its own result already holds how many rows it wants, so a reader
// gives it every row it returns instead of one page of them.
func TestHoldsRowLimitReadsTheClauseThatBoundsAResult(t *testing.T) {
	held := language.SQL

	for _, one := range []struct {
		sql    string
		bounds bool
	}{
		{"select * from orders", false},
		{"select * from orders order by id", false},
		{"select * from orders limit 250", true},
		{"select * from orders LIMIT 250", true},
		{"select * from orders order by id limit 250 offset 10", true},
		{"select * from orders offset 10", false},
		{"select * from orders fetch first 10 rows only", true},
		// A word inside a name or a string is not the clause.
		{"select limit_cents from orders", false},
		{"select * from orders where note = 'limit'", false},
		{"update orders set status = 'paid'", false},
	} {
		if held.HoldsRowLimit(one.sql) != one.bounds {
			t.Errorf("%q bounds itself: %v, wanted %v",
				one.sql, held.HoldsRowLimit(one.sql), one.bounds)
		}
	}
}
