package statement_test

import (
	"testing"

	"github.com/turanmahmudov/masume/internal/query/statement"
	"github.com/turanmahmudov/masume/internal/query/syntax"
)

func equalStatements(held, wanted []string) bool {
	if len(held) != len(wanted) {
		return false
	}
	for at := range held {
		if held[at] != wanted[at] {
			return false
		}
	}
	return true
}

// Where a statement ends decides what runs and what its risk is. A semicolon inside a string
// or a comment that ended a statement early would hand the server half a statement, and would
// hide the rest from the read that decides whether a write is about to happen.
func TestSplitStatementsEndsAStatementOnlyAtARealSemicolon(t *testing.T) {
	for _, held := range []struct {
		name    string
		sql     string
		flavour syntax.SyntaxFlavour
		want    []string
	}{
		{"one statement", "select 1", syntax.FlavourStandard, []string{"select 1"}},
		{"one statement with its semicolon", "select 1;", syntax.FlavourStandard,
			[]string{"select 1"}},
		{"two statements", "select 1; select 2", syntax.FlavourStandard,
			[]string{"select 1", "select 2"}},
		{"two over several lines", "select 1;\n\nselect 2;\n", syntax.FlavourStandard,
			[]string{"select 1", "select 2"}},

		{"nothing", "", syntax.FlavourStandard, nil},
		{"only space", "   \n  ", syntax.FlavourStandard, nil},
		{"only semicolons", ";;;", syntax.FlavourStandard, nil},

		// A comment holds nothing to run, so it is no statement.
		{"only a line comment", "-- a note\n", syntax.FlavourStandard, nil},
		{"only a block comment", "/* a note */", syntax.FlavourStandard, nil},
		{"a comment after the last statement", "select 1;\n-- a note",
			syntax.FlavourStandard, []string{"select 1"}},
		{"a comment before the statement", "-- a note\nselect 1",
			syntax.FlavourStandard, []string{"-- a note\nselect 1"}},

		// A semicolon that is text, not the end of a statement.
		{"a semicolon in a string", "select ';'", syntax.FlavourStandard,
			[]string{"select ';'"}},
		{"a semicolon in a line comment", "select 1 -- ; not the end\n", syntax.FlavourStandard,
			[]string{"select 1 -- ; not the end"}},
		{"a semicolon in a block comment", "select /* ; */ 1", syntax.FlavourStandard,
			[]string{"select /* ; */ 1"}},
		{"a semicolon in a quoted name", `select "a;b" from t`, syntax.FlavourStandard,
			[]string{`select "a;b" from t`}},

		// MySQL reads a backslash inside a string as an escape, so the string runs on and
		// the semicolon inside it is text.
		{"a semicolon after an escaped quote", `select 'a\'; b'`, syntax.FlavourMysql,
			[]string{`select 'a\'; b'`}},

		// A trigger body and a stored routine hold their statements between `begin` and
		// `end`, and the block keeps its own semicolon.
		{
			"a trigger body",
			"create trigger t after insert on orders begin update c set n=n; end;\n" +
				"select 1;",
			syntax.FlavourStandard,
			[]string{
				"create trigger t after insert on orders begin update c set n=n; end;",
				"select 1",
			},
		},
		{
			"two statements in a body",
			"create trigger t after insert on orders begin insert into a values (1); " +
				"insert into b values (2); end;",
			syntax.FlavourStandard,
			[]string{
				"create trigger t after insert on orders begin insert into a values (1); " +
					"insert into b values (2); end;",
			},
		},
		{
			"a nested block",
			"create procedure p() begin begin insert into a values (1); end; end;",
			syntax.FlavourMysql,
			[]string{
				"create procedure p() begin begin insert into a values (1); end; end;",
			},
		},
		// `begin` on its own opens a transaction, and the statements after it are their
		// own.
		{"a transaction", "begin; insert into a values (1); commit;", syntax.FlavourStandard,
			[]string{"begin", "insert into a values (1)", "commit"}},
		// A `case` expression closes with `end` and holds no semicolon of its own.
		{
			"a case expression",
			"select case when a then 1 else 2 end from t; select 2",
			syntax.FlavourStandard,
			[]string{"select case when a then 1 else 2 end from t", "select 2"},
		},
		// A statement that ends in a `case` keeps no semicolon: only the block of a
		// routine needs its terminator.
		{
			"a case at the end of the statement",
			"select case when a then 1 end; select 2",
			syntax.FlavourStandard,
			[]string{"select case when a then 1 end", "select 2"},
		},
		// `end if` and `end loop` close what opened no block of its own, so the body runs
		// on to its own `end`.
		{
			"a routine holding an if",
			"create procedure p() begin if a then insert into t values (1); end if; end;\n" +
				"select 1;",
			syntax.FlavourMysql,
			[]string{
				"create procedure p() begin if a then insert into t values (1); " +
					"end if; end;",
				"select 1",
			},
		},
		{
			"a routine holding a loop",
			"create procedure p() begin my_loop: loop insert into t values (1); " +
				"end loop; end;",
			syntax.FlavourMysql,
			[]string{
				"create procedure p() begin my_loop: loop insert into t values (1); " +
					"end loop; end;",
			},
		},
		{
			"a routine holding a case",
			"create procedure p() begin case a when 1 then insert into t values (1); " +
				"end case; end;\nselect 1;",
			syntax.FlavourMysql,
			[]string{
				"create procedure p() begin case a when 1 then insert into t values (1); " +
					"end case; end;",
				"select 1",
			},
		},
		{
			"a routine holding a loop and a statement after it",
			"create procedure p() begin my_loop: loop insert into t values (1); " +
				"end loop; end;\nselect 1;",
			syntax.FlavourMysql,
			[]string{
				"create procedure p() begin my_loop: loop insert into t values (1); " +
					"end loop; end;",
				"select 1",
			},
		},
		{
			"two routines",
			"create procedure a() begin insert into t values (1); end;\n" +
				"create procedure b() begin insert into t values (2); end;",
			syntax.FlavourMysql,
			[]string{
				"create procedure a() begin insert into t values (1); end;",
				"create procedure b() begin insert into t values (2); end;",
			},
		},
	} {
		t.Run(held.name, func(t *testing.T) {
			answered := statement.SplitStatements(held.sql, held.flavour)
			if !equalStatements(answered, held.want) {
				t.Errorf("%q splits into\n  %q\nwanted\n  %q", held.sql, answered, held.want)
			}
		})
	}
}

// The ranges cover the statements in the order they run, and each range points at the text it
// came from, which is how the editor marks the statement the caret stands in.
func TestSplitStatementRangesPointAtTheTextTheyCoverFrom(t *testing.T) {
	const sql = "select 1;\nselect 2;\nselect 3"

	ranges := statement.SplitStatementRanges(sql, syntax.FlavourStandard)
	if len(ranges) != 3 {
		t.Fatalf("the buffer split into %d ranges, wanted 3", len(ranges))
	}
	at := 0
	for index, held := range ranges {
		if held.Start < at {
			t.Errorf("range %d starts at %d, behind %d", index, held.Start, at)
		}
		if held.End < held.Start || held.End > len(sql) {
			t.Errorf("range %d covers %d to %d, outside the buffer", index, held.Start, held.End)
		}
		at = held.End
	}
}

// The caret decides which statement a run takes, so the statement it stands in must be the one
// the user sees it in, including at the edges.
func TestReadStatementAtOffsetAnswersTheStatementTheCaretIsIn(t *testing.T) {
	const sql = "select 1;\nselect 2;"

	for _, held := range []struct {
		name   string
		offset int
		want   string
	}{
		{"at the start", 0, "select 1"},
		{"inside the first", 4, "select 1"},
		{"on the semicolon of the first", 8, "select 1"},
		{"at the start of the second", 10, "select 2"},
		{"inside the second", 14, "select 2"},
		{"at the end of the buffer", len(sql), "select 2"},
		// An offset outside the buffer must answer something rather than reach past it.
		{"past the end", len(sql) + 50, "select 2"},
		{"before the start", -5, "select 1"},
	} {
		t.Run(held.name, func(t *testing.T) {
			if answered := statement.ReadStatementAtOffset(
				sql, held.offset, syntax.FlavourStandard); answered != held.want {
				t.Errorf("the caret at %d reads %q, wanted %q", held.offset, answered, held.want)
			}
		})
	}
}

func TestReadStatementAtOffsetAnswersNothingForAnEmptyBuffer(t *testing.T) {
	for _, sql := range []string{"", "   "} {
		if answered := statement.ReadStatementAtOffset(sql, 0, syntax.FlavourStandard); answered != "" {
			t.Errorf("%q answered %q, wanted nothing", sql, answered)
		}
	}
}

// A buffer of semicolons alone splits into no statement, and yet the caret answers the raw
// text, because one statement is answered whole and none reads the same way. The run that
// follows sends that text to the server rather than reporting that there is nothing to run.
// This records what the client does today; the recorded answers hold the same.
func TestReadStatementAtOffsetAnswersTheRawTextOfASemicolonAlone(t *testing.T) {
	if answered := statement.ReadStatementAtOffset(";", 0, syntax.FlavourStandard); answered != ";" {
		t.Errorf("a semicolon alone answered %q, wanted the raw text", answered)
	}
	if held := statement.SplitStatements(";", syntax.FlavourStandard); len(held) != 0 {
		t.Errorf("a semicolon alone splits into %q, wanted no statement", held)
	}
}
