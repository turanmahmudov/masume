package statement_test

import (
	"testing"

	"github.com/turanmahmudov/masume/internal/query/statement"
	"github.com/turanmahmudov/masume/internal/query/syntax"
)

func TestIsPageableIsTrueOnlyForAStatementThatOnlyReads(t *testing.T) {
	for _, held := range []struct {
		name string
		sql  string
		want bool
	}{
		{"a select", "select * from users", true},
		{"a common table expression", "with c as (select 1) select * from c", true},
		{"a table statement", "table users", true},
		{"a values statement", "values (1), (2)", true},
		{"a comment before the statement", "-- a name\nselect 1", true},
		{"a comment after the statement", "select 1;\n-- trailing", true},
		{"a select that already pages itself", "select * from t limit 5", true},
		{"a select of a locking read", "select * from t for update", false},
		{"a select that writes a table", "select * into new_t from t", true},
		{"a select holding a write word in text", "select 'insert' from t", true},
		{"an insert", "insert into t (a) values (1)", false},
		{"an update", "update t set a = 1 where id = 2", false},
		{"a delete", "delete from t", false},
		{"an explain", "explain select 1", false},
		{"a create from a query", "create table t as select * from u", false},
		{"a call", "call proc()", false},
		{"nothing", "", false},
		{"blank text", "   ", false},
		{"a semicolon alone", ";", false},
		{"a comment alone", "/* only a comment */", false},
	} {
		t.Run(held.name, func(t *testing.T) {
			if got := statement.IsPageable(held.sql, syntax.FlavourStandard); got != held.want {
				t.Errorf("statement.IsPageable(%q) = %v, want %v", held.sql, got, held.want)
			}
		})
	}
}

func TestIsPageableRefusesAReadThatCarriesAWriteWord(t *testing.T) {
	// `for update` locks the rows it reads, so a second page would lock more of the table.
	if statement.IsPageable("select * from t for update", syntax.FlavourStandard) {
		t.Error("a locking read must not be paged")
	}
	if statement.IsPageable("with c as (delete from t returning *) select * from c",
		syntax.FlavourStandard) {
		t.Error("a common table expression that deletes must not be paged")
	}
}

func TestCanExplainIsTrueForEveryStatementTheServerPlans(t *testing.T) {
	for _, held := range []struct {
		name string
		sql  string
		want bool
	}{
		{"a select", "select * from users", true},
		{"a common table expression", "with c as (select 1) select * from c", true},
		{"a table statement", "table users", true},
		{"a values statement", "values (1)", true},
		{"an insert", "insert into t (a) values (1)", true},
		{"an update", "update t set a = 1", true},
		{"a delete", "delete from t", true},
		{"a merge", "merge into t using u on t.id = u.id", true},
		{"a replace", "replace into t values (1)", true},
		{"a create table from a query", "create table t as select * from u", true},
		{"a materialized view from a query", "create materialized view mv as select 1", true},
		{"a create table with no query", "create table t (a int)", false},
		{"a create index", "create index i on t (a)", false},
		{"a truncate", "truncate table t", false},
		{"a drop", "drop table t", false},
		{"an alter", "alter table t add column c int", false},
		{"a grant", "grant select on t to r", false},
		{"an explain", "explain select 1", false},
		{"a call", "call proc()", false},
		{"a refresh", "refresh materialized view mv", false},
		{"a copy", "copy t from stdin", false},
		{"nothing", "", false},
		{"a comment alone", "/* only a comment */", false},
	} {
		t.Run(held.name, func(t *testing.T) {
			if got := statement.CanExplain(held.sql, syntax.FlavourStandard); got != held.want {
				t.Errorf("statement.CanExplain(%q) = %v, want %v", held.sql, got, held.want)
			}
		})
	}
}

func TestCanExplainReadsACreateOnlyWhereTheObjectComesBeforeTheQuery(t *testing.T) {
	// `create table … as select` is planned; a `create` whose `as` names something else
	// is not.
	if !statement.CanExplain("create table t as select * from u", syntax.FlavourStandard) {
		t.Error("a create table from a query must be planned")
	}
	if statement.CanExplain("create or replace function f() returns int as $$ select 1 $$ language sql",
		syntax.FlavourStandard) {
		t.Error("a create function must not be planned")
	}
}

func TestChangesCatalogIsTrueOnlyWhereAnObjectIsMadeOrChanged(t *testing.T) {
	for _, held := range []struct {
		name string
		sql  string
		want bool
	}{
		{"a create", "create table t (a int)", true},
		{"a create from a query", "create table t as select * from u", true},
		{"a materialized view", "create materialized view mv as select 1", true},
		{"a drop", "drop table t", true},
		{"a drop schema", "drop schema s cascade", true},
		{"an alter", "alter table t add column c int", true},
		{"a comment on", "comment on table t is 'x'", true},
		{"a select into a new table", "select * into new_t from t", true},
		{"a plain select", "select * from users", false},
		{"an insert", "insert into t (a) values (1)", false},
		{"an update", "update t set a = 1", false},
		{"a delete", "delete from t", false},
		{"a truncate", "truncate table t", false},
		{"a grant", "grant select on t to r", false},
		{"a refresh", "refresh materialized view mv", false},
		{"nothing", "", false},
		{"a comment alone", "/* only a comment */", false},
	} {
		t.Run(held.name, func(t *testing.T) {
			if got := statement.ChangesCatalog(held.sql, syntax.FlavourStandard); got != held.want {
				t.Errorf("statement.ChangesCatalog(%q) = %v, want %v", held.sql, got, held.want)
			}
		})
	}
}

func TestTheKindOfAStatementIsReadTheSameInBothFlavours(t *testing.T) {
	// A backtick quotes a name in MySQL and opens nothing in the standard flavour, so a
	// statement that carries one must still be read as the same kind.
	for _, sql := range []string{
		"select `back` from `t`",
		"select # hash\n1",
		"insert into t (a) values (1)",
		"drop table t",
	} {
		for _, held := range []struct {
			name string
			read func(string, syntax.SyntaxFlavour) bool
		}{
			{"IsPageable", statement.IsPageable},
			{"CanExplain", statement.CanExplain},
			{"ChangesCatalog", statement.ChangesCatalog},
		} {
			standard := held.read(sql, syntax.FlavourStandard)
			mysql := held.read(sql, syntax.FlavourMysql)
			if standard != mysql {
				t.Errorf("%s(%q): standard = %v, mysql = %v",
					held.name, sql, standard, mysql)
			}
		}
	}
}
