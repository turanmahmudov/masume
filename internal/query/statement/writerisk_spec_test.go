// Spec tests reach the package only through what it exports, so they pin the contract and
// not the way it is built. They live beside the source in the `_test` package Go gives each
// directory for exactly this.
package statement_test

import (
	"strings"
	"testing"

	"github.com/turanmahmudov/masume/internal/query/statement"
	"github.com/turanmahmudov/masume/internal/query/syntax"
)

// The risk of a statement decides whether an agent may run it and whether the user is asked
// first, so a statement read as safer than it is opens the door to a write nobody allowed.
func TestResolveWriteRiskReadsWhatAStatementDoes(t *testing.T) {
	for _, held := range []struct {
		name    string
		sql     string
		flavour syntax.SyntaxFlavour
		want    statement.WriteRisk
	}{
		{"a plain read", "select * from orders", syntax.FlavourStandard, statement.RiskNone},
		{"a read of a count", "select count(*) from orders", syntax.FlavourStandard, statement.RiskNone},
		{"a read that locks", "select * from orders for update", syntax.FlavourStandard, statement.RiskNone},
		{"a read that locks without a key", "select * from orders for no key update",
			syntax.FlavourStandard, statement.RiskNone},
		{"an insert", "insert into orders (id) values (1)", syntax.FlavourStandard, statement.RiskWrite},
		{"an update with a where", "update orders set paid = true where id = 1",
			syntax.FlavourStandard, statement.RiskWrite},
		{"an update with no where", "update orders set paid = true",
			syntax.FlavourStandard, statement.RiskEveryRow},
		{"a delete with a where", "delete from orders where id = 1",
			syntax.FlavourStandard, statement.RiskDelete},
		{"a delete with no where", "delete from orders", syntax.FlavourStandard, statement.RiskEveryRow},
		{"a truncate", "truncate orders", syntax.FlavourStandard, statement.RiskEveryRow},
		{"a drop", "drop table orders", syntax.FlavourStandard, statement.RiskDelete},
		{"a create", "create table orders (id int)", syntax.FlavourStandard, statement.RiskWrite},

		// A word that opens a statement is also a name and a function, and the risk must
		// follow which of those it is here.
		{"truncate as a function", "select truncate(1.234, 2)", syntax.FlavourMysql, statement.RiskNone},
		{"a column named delete", "select \"delete\" from audit", syntax.FlavourStandard, statement.RiskNone},

		// A statement this client does not know may be one the server writes with, so it
		// is never read as a read.
		{"a load of a file", "load data infile '/tmp/rows.csv' into table orders",
			syntax.FlavourMysql, statement.RiskWrite},
		{"an import", "import foreign schema public from server other into local",
			syntax.FlavourStandard, statement.RiskWrite},
		{"a prepared statement being run", "execute plan", syntax.FlavourStandard,
			statement.RiskWrite},
		{"a vacuum", "vacuum full orders", syntax.FlavourStandard, statement.RiskWrite},
		// A read that opens with a bracket is still a read.
		{"a read in brackets", "(select 1) union (select 2)", syntax.FlavourStandard,
			statement.RiskNone},
		// A session setting changes nothing another connection reads. A server setting
		// does, and every later connection reads it.
		{"a session setting", "set time zone 'UTC'", syntax.FlavourStandard, statement.RiskNone},
		{"a server setting", "set global max_connections = 500", syntax.FlavourMysql,
			statement.RiskWrite},
		{"a pragma that reads", "pragma table_info(orders)", syntax.FlavourStandard,
			statement.RiskNone},
		{"a pragma that writes", "pragma journal_mode = wal", syntax.FlavourStandard,
			statement.RiskWrite},

		// MySQL runs what stands inside `/*! … */`, so the text of an executable comment
		// is a statement and not a comment.
		{"a write inside an executable comment", "/*! delete from orders */",
			syntax.FlavourMysql, statement.RiskDelete},
		{"a write inside a version-gated comment", "/*!80000 delete from orders where id = 1 */",
			syntax.FlavourMysql, statement.RiskDelete},
		{"a write hidden behind a read", "select 1 /*! ; drop table orders */",
			syntax.FlavourMysql, statement.RiskDelete},
		// Only MySQL runs it. Every other server reads it as a comment.
		{"an executable comment on another server", "/*! delete from orders */",
			syntax.FlavourStandard, statement.RiskNone},
		// A plain block comment stays a comment on MySQL too.
		{"a plain block comment", "select 1 /* delete from orders */",
			syntax.FlavourMysql, statement.RiskNone},

		// A CTE writes through the statement that follows it, not through the WITH.
		{"a read behind a CTE", "with recent as (select 1) select * from recent",
			syntax.FlavourStandard, statement.RiskNone},
		{"a delete behind a CTE", "with old as (select id from orders) delete from orders",
			syntax.FlavourStandard, statement.RiskEveryRow},
	} {
		t.Run(held.name, func(t *testing.T) {
			if answered := statement.ResolveWriteRisk(held.sql, held.flavour); answered != held.want {
				t.Errorf("%q reads as %q, wanted %q", held.sql, answered, held.want)
			}
		})
	}
}

// A batch is as risky as the worst statement in it, because every one of them runs.
func TestResolveStrongestRiskTakesTheWorstOfASet(t *testing.T) {
	for _, held := range []struct {
		name  string
		risks []statement.WriteRisk
		want  statement.WriteRisk
	}{
		{"nothing", nil, statement.RiskNone},
		{"reads only", []statement.WriteRisk{statement.RiskNone, statement.RiskNone},
			statement.RiskNone},
		{"a write among reads", []statement.WriteRisk{statement.RiskNone, statement.RiskWrite},
			statement.RiskWrite},
		{"a delete among writes", []statement.WriteRisk{statement.RiskWrite, statement.RiskDelete},
			statement.RiskDelete},
		{"every row among the rest", []statement.WriteRisk{
			statement.RiskDelete, statement.RiskEveryRow, statement.RiskNone}, statement.RiskEveryRow},
		{"the worst is not the last", []statement.WriteRisk{
			statement.RiskEveryRow, statement.RiskNone}, statement.RiskEveryRow},
	} {
		t.Run(held.name, func(t *testing.T) {
			if answered := statement.ResolveStrongestRisk(held.risks); answered != held.want {
				t.Errorf("the set reads as %q, wanted %q", answered, held.want)
			}
		})
	}
}

// Every risk is described for one statement and for several, because the question the user
// answers names how many are about to run.
func TestDescribeRiskAnswersForEveryRisk(t *testing.T) {
	risks := []statement.WriteRisk{
		statement.RiskNone, statement.RiskWrite, statement.RiskDelete, statement.RiskEveryRow,
	}
	for _, risk := range risks {
		for _, count := range []int{1, 2} {
			if written := statement.DescribeRisk(risk, count); written == "" {
				t.Errorf("%q with %d statements is described as an empty text", risk, count)
			}
		}
	}
}

func TestResolveWriteRiskReadsAStoredRoutineBodyAsAWriteAlone(t *testing.T) {
	// A routine body is stored, not run, so a DELETE inside it removes nothing now.
	for _, held := range []struct {
		name    string
		sql     string
		flavour syntax.SyntaxFlavour
		want    statement.WriteRisk
	}{
		{"a procedure that deletes", "create procedure p() delete from t",
			syntax.FlavourMysql, statement.RiskWrite},
		{"a function that deletes", "create function f() returns int begin delete from t; end",
			syntax.FlavourMysql, statement.RiskWrite},
		{"a trigger that deletes", "create trigger tr after insert on t delete from u",
			syntax.FlavourMysql, statement.RiskWrite},
		{"an altered routine", "alter procedure p() delete from t",
			syntax.FlavourMysql, statement.RiskWrite},
		{"a table is no routine", "create table t as select * from u",
			syntax.FlavourMysql, statement.RiskWrite},
		{"a drop is no routine", "drop procedure p", syntax.FlavourMysql, statement.RiskDelete},
	} {
		t.Run(held.name, func(t *testing.T) {
			if got := statement.ResolveWriteRisk(held.sql, held.flavour); got != held.want {
				t.Errorf("ResolveWriteRisk(%q) = %v, want %v", held.sql, got, held.want)
			}
		})
	}
}

func TestResolveWriteRiskWeighsTheRestOfTheWritingStatements(t *testing.T) {
	for _, held := range []struct {
		name string
		sql  string
		want statement.WriteRisk
	}{
		{"a replace removes the row it lands on", "replace into t values (1)",
			statement.RiskDelete},
		{"a merge", "merge into t using u on t.id = u.id", statement.RiskWrite},
		{"a select into a new table", "select * into new_t from t", statement.RiskWrite},
		{"a copy", "copy t from stdin", statement.RiskWrite},
		{"a refresh", "refresh materialized view mv", statement.RiskWrite},
		{"a call", "call proc()", statement.RiskWrite},
		{"a do block", "do $$ begin end $$", statement.RiskWrite},
		{"a grant", "grant select on t to r", statement.RiskWrite},
		{"a revoke", "revoke select on t from r", statement.RiskWrite},
		{"an alter", "alter table t add column c int", statement.RiskWrite},
		{"a lock", "lock table t in access exclusive mode", statement.RiskNone},
		// An EXPLAIN runs nothing, but the word is weighed anyway, so the client asks
		// rather than misses a delete.
		{"an explain of a delete", "explain delete from t", statement.RiskDelete},
		{"nothing", "", statement.RiskNone},
		{"a comment alone", "/* only a comment */", statement.RiskNone},
	} {
		t.Run(held.name, func(t *testing.T) {
			if got := statement.ResolveWriteRisk(held.sql, syntax.FlavourStandard); got != held.want {
				t.Errorf("ResolveWriteRisk(%q) = %v, want %v", held.sql, got, held.want)
			}
		})
	}
}

func TestBuildConfirmationNamesTheProfileTheEnvironmentAndWhatWillRun(t *testing.T) {
	one := statement.BuildConfirmation("shop-prod", "prod", statement.RiskEveryRow,
		[]string{"delete from orders"})
	if want := "confirm on shop-prod"; one.Title != want {
		t.Errorf("Title = %q, want %q", one.Title, want)
	}
	want := "This statement names no rows, so it lands on every row on prod.\n\n" +
		"delete from orders"
	if one.Body != want {
		t.Errorf("Body = %q, want %q", one.Body, want)
	}
}

func TestBuildConfirmationCountsTheStatementsAndJoinsThem(t *testing.T) {
	many := statement.BuildConfirmation("shop", "dev", statement.RiskWrite,
		[]string{"insert into t values (1)", "update t set a = 2"})
	want := "These 2 statements write to the database on dev.\n\n" +
		"insert into t values (1);\nupdate t set a = 2"
	if many.Body != want {
		t.Errorf("Body = %q, want %q", many.Body, want)
	}
	none := statement.BuildConfirmation("shop", "dev", statement.RiskNone, nil)
	if !strings.HasPrefix(none.Body, "These 0 statements read only on dev.") {
		t.Errorf("Body = %q", none.Body)
	}
}

// A SET that a read-only connection may run must not be one that takes the connection out
// of read-only. PostgreSQL and MySQL both hold that state in a setting, so a SET read as
// safe would let every statement after it write while each one still reads as safe.
func TestResolveWriteRiskRefusesASettingItDoesNotKnow(t *testing.T) {
	for _, held := range []struct {
		name    string
		sql     string
		flavour syntax.SyntaxFlavour
		want    statement.WriteRisk
	}{
		// The two that open a read-only connection to writes.
		{"the PostgreSQL read-only setting", "set default_transaction_read_only = off",
			syntax.FlavourStandard, statement.RiskWrite},
		{"the same one reset", "reset default_transaction_read_only",
			syntax.FlavourStandard, statement.RiskWrite},
		{"every setting reset", "reset all", syntax.FlavourStandard, statement.RiskWrite},
		{"the MySQL transaction mode", "set session transaction read write",
			syntax.FlavourMysql, statement.RiskWrite},
		{"the same mode of the session", "set session characteristics as transaction read write",
			syntax.FlavourStandard, statement.RiskWrite},

		// A setting this client does not know may be another one of the same kind.
		{"a setting of the server", "set global max_connections = 10",
			syntax.FlavourMysql, statement.RiskWrite},
		{"a role", "set role postgres", syntax.FlavourStandard, statement.RiskWrite},

		// The settings a reader sets to read.
		{"the search path", "set search_path to public",
			syntax.FlavourStandard, statement.RiskNone},
		{"the time zone", "set time zone 'UTC'", syntax.FlavourStandard, statement.RiskNone},
		{"the same one as one word", "set timezone = 'UTC'",
			syntax.FlavourStandard, statement.RiskNone},
		{"the timeout of a statement", "set statement_timeout = 5000",
			syntax.FlavourStandard, statement.RiskNone},
		{"a planner switch", "set enable_seqscan = off",
			syntax.FlavourStandard, statement.RiskNone},
		{"the encoding", "set names utf8mb4", syntax.FlavourMysql, statement.RiskNone},
		{"a transaction that only reads", "set transaction read only",
			syntax.FlavourStandard, statement.RiskNone},
		{"the isolation of a transaction", "set transaction isolation level serializable",
			syntax.FlavourStandard, statement.RiskNone},
	} {
		t.Run(held.name, func(t *testing.T) {
			answered := statement.ResolveWriteRisk(held.sql, held.flavour)
			if answered != held.want {
				t.Errorf("%q reads as %q, wanted %q", held.sql, answered, held.want)
			}
		})
	}
}

// MariaDB runs what an executable comment holds, and only MariaDB writes it as `/*M!`. Read
// as a plain comment, a whole DELETE would pass every check this client makes and land on
// the server all the same.
func TestResolveWriteRiskReadsAnExecutableComment(t *testing.T) {
	for _, held := range []struct {
		name    string
		sql     string
		flavour syntax.SyntaxFlavour
		want    statement.WriteRisk
	}{
		{"the MariaDB form", "/*M! delete from orders */",
			syntax.FlavourMysql, statement.RiskDelete},
		{"the MariaDB form with a version", "/*M!100000 delete from orders */",
			syntax.FlavourMysql, statement.RiskDelete},
		{"the MariaDB form in lower case", "/*m! drop table orders */",
			syntax.FlavourMysql, statement.RiskDelete},
		{"the MySQL form", "/*! delete from orders */",
			syntax.FlavourMysql, statement.RiskDelete},
		{"the MySQL form with a version", "/*!40000 delete from orders */",
			syntax.FlavourMysql, statement.RiskDelete},

		// A plain block comment holds nothing the server runs.
		{"a plain comment", "/* delete from orders */",
			syntax.FlavourMysql, statement.RiskNone},
		// No other engine runs one, so the text stays a comment there.
		{"the MariaDB form on another engine", "/*M! delete from orders */",
			syntax.FlavourStandard, statement.RiskNone},
	} {
		t.Run(held.name, func(t *testing.T) {
			answered := statement.ResolveWriteRisk(held.sql, held.flavour)
			if answered != held.want {
				t.Errorf("%q reads as %q, wanted %q", held.sql, answered, held.want)
			}
		})
	}
}

// `begin read write` opens a transaction that writes, whatever the connection was set
// read-only for. Read as a plain `begin`, it would let every statement after it write while
// each one still reads as safe.
func TestResolveWriteRiskReadsATransactionThatOpensForWrites(t *testing.T) {
	for _, held := range []struct {
		sql  string
		want statement.WriteRisk
	}{
		{"begin read write", statement.RiskWrite},
		{"begin transaction read write", statement.RiskWrite},
		{"start transaction read write", statement.RiskWrite},

		{"begin", statement.RiskNone},
		{"begin read only", statement.RiskNone},
		{"start transaction", statement.RiskNone},
		{"start transaction read only", statement.RiskNone},
		{"begin transaction isolation level serializable", statement.RiskNone},
	} {
		t.Run(held.sql, func(t *testing.T) {
			answered := statement.ResolveWriteRisk(held.sql, syntax.FlavourStandard)
			if answered != held.want {
				t.Errorf("%q reads as %q, wanted %q", held.sql, answered, held.want)
			}
		})
	}
}
