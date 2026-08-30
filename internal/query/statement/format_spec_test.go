package statement_test

import (
	"testing"

	"github.com/turanmahmudov/masume/internal/query/statement"
	"github.com/turanmahmudov/masume/internal/query/syntax"
)

func TestFormatStatementPutsEveryTopLevelClauseOnItsOwnLine(t *testing.T) {
	for _, held := range []struct {
		name string
		sql  string
		want string
	}{
		{
			"a select with every clause",
			"select count(*) from t group by a having count(*) > 1",
			"select count(*)\nfrom t\ngroup by a\nhaving count(*) > 1",
		},
		{
			"a join and the paging clauses",
			"select * from a join b on a.id=b.id order by a.id limit 10 offset 5",
			"select *\nfrom a\njoin b on a.id=b.id\norder by a.id\nlimit 10\noffset 5",
		},
		{
			"a common table expression before the select",
			"with a as (select 1), b as (select 2) select * from a, b",
			"with a as (select 1), b as (select 2)\nselect *\nfrom a, b",
		},
		{
			"a union",
			"select 1 union all select 2",
			"select 1\nunion all\nselect 2",
		},
		{
			"a comma list stays on the clause it belongs to",
			"select a, b from t where a = 1 and b = 2 order by a desc, b asc",
			"select a, b\nfrom t\nwhere a = 1 and b = 2\norder by a desc, b asc",
		},
	} {
		t.Run(held.name, func(t *testing.T) {
			got := statement.FormatStatement(held.sql, syntax.FlavourStandard)
			if got != held.want {
				t.Errorf("statement.FormatStatement(%q)\n got %q\nwant %q", held.sql, got, held.want)
			}
		})
	}
}

func TestFormatStatementLeavesTheTextOfALiteralAndACommentAlone(t *testing.T) {
	// A run of blanks between tokens is layout, and a run inside a literal or a comment
	// is content. Only the first is collapsed.
	for _, held := range []struct {
		name string
		sql  string
		want string
	}{
		{
			"blanks inside a text literal",
			"select 'two  spaces' from t",
			"select 'two  spaces'\nfrom t",
		},
		{
			"blanks inside a quoted name",
			`select "two  spaces" from t`,
			"select \"two  spaces\"\nfrom t",
		},
		{
			"blanks inside a block comment",
			"select * from t /* two  spaces */ where x = 1",
			"select *\nfrom t /* two  spaces */\nwhere x = 1",
		},
		{
			"an escaped quote inside a literal",
			"select 'it''s' as x",
			"select 'it''s' as x",
		},
		{
			"a clause word inside a literal is not a clause",
			"select 'from t' as x",
			"select 'from t' as x",
		},
		{
			"a clause word inside a comment is not a clause",
			"select 1 /* order by a */",
			"select 1 /* order by a */",
		},
		{
			"a line comment keeps the rest of its line",
			"select * from t where a = 1 -- comment\n and b = 2",
			"select *\nfrom t\nwhere a = 1 -- comment\n and b = 2",
		},
	} {
		t.Run(held.name, func(t *testing.T) {
			got := statement.FormatStatement(held.sql, syntax.FlavourStandard)
			if got != held.want {
				t.Errorf("statement.FormatStatement(%q)\n got %q\nwant %q", held.sql, got, held.want)
			}
		})
	}
}

func TestFormatStatementAnswersTheTrimmedTextWhereThereIsNoClause(t *testing.T) {
	for _, held := range []struct {
		name string
		sql  string
		want string
	}{
		{"nothing", "", ""},
		{"blank text", "   ", ""},
		{"a comment alone", "/* only a comment */", "/* only a comment */"},
		{"an insert", "insert into t (a) values (1)", "insert into t (a) values (1)"},
		{"blanks around a statement", "  drop table t  ", "drop table t"},
	} {
		t.Run(held.name, func(t *testing.T) {
			got := statement.FormatStatement(held.sql, syntax.FlavourStandard)
			if got != held.want {
				t.Errorf("statement.FormatStatement(%q) = %q, want %q", held.sql, got, held.want)
			}
		})
	}
}

func TestFormatStatementKeepsTheTextBeforeTheFirstClause(t *testing.T) {
	got := statement.FormatStatement("-- a name\nselect 1", syntax.FlavourStandard)
	if want := "-- a name\nselect 1"; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestFormatStatementKeepsTheCaseOfEveryWord(t *testing.T) {
	got := statement.FormatStatement("SELECT\n  a\nFROM t\nWHERE a > 1", syntax.FlavourStandard)
	if want := "SELECT\n a\nFROM t\nWHERE a > 1"; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}
