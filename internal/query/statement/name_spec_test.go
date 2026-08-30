package statement_test

import (
	"testing"

	"github.com/turanmahmudov/masume/internal/query/statement"
)

func TestFindQueryNameReadsTheLineCommentAtTheStart(t *testing.T) {
	for _, held := range []struct {
		name string
		sql  string
		want string
	}{
		{"a name on the first line", "-- a name\nselect 1", "a name"},
		{"a name with no space after the marks", "--a name\nselect 1", "a name"},
		{"a name behind blanks", "   -- a name\nselect 1", "a name"},
		{"blanks around the name", "--   a name   \nselect 1", "a name"},
		{"a name and nothing else", "-- a name", "a name"},
		{"no comment", "select * from users", ""},
		{"a comment on a later line", "select 1;\n-- trailing", ""},
		{"a comment at the end of the first line", "select 1 -- trailing", ""},
		{"a block comment", "/* only a comment */", ""},
		{"an empty comment", "--\nselect 1", ""},
		{"nothing", "", ""},
	} {
		t.Run(held.name, func(t *testing.T) {
			if got := statement.FindQueryName(held.sql); got != held.want {
				t.Errorf("statement.FindQueryName(%q) = %q, want %q", held.sql, got, held.want)
			}
		})
	}
}

func TestApplyQueryNameWritesTheNameAboveTheStatement(t *testing.T) {
	for _, held := range []struct {
		name  string
		sql   string
		given string
		want  string
	}{
		{"a statement with no name", "select * from users", "given name",
			"-- given name\nselect * from users"},
		{"a statement that already has one", "-- a name\nselect 1", "given name",
			"-- given name\nselect 1"},
		{"blanks around the given name", "select 1", "  given name  ",
			"-- given name\nselect 1"},
		{"an empty buffer", "", "given name", "-- given name\n"},
		{"a block comment is not a name", "/* only a comment */", "given name",
			"-- given name\n/* only a comment */"},
		{"a statement over several lines", "select 1\nfrom t", "given name",
			"-- given name\nselect 1\nfrom t"},
	} {
		t.Run(held.name, func(t *testing.T) {
			if got := statement.ApplyQueryName(held.sql, held.given); got != held.want {
				t.Errorf("statement.ApplyQueryName(%q, %q) = %q, want %q",
					held.sql, held.given, got, held.want)
			}
		})
	}
}

func TestApplyQueryNameWithAnEmptyNameTakesTheLineAway(t *testing.T) {
	for _, held := range []struct {
		name  string
		sql   string
		given string
		want  string
	}{
		{"a statement that has a name", "-- a name\nselect 1", "", "select 1"},
		{"a given name of blanks", "-- a name\nselect 1", "   ", "select 1"},
		{"a statement that has none", "select 1", "", "select 1"},
	} {
		t.Run(held.name, func(t *testing.T) {
			if got := statement.ApplyQueryName(held.sql, held.given); got != held.want {
				t.Errorf("statement.ApplyQueryName(%q, %q) = %q, want %q",
					held.sql, held.given, got, held.want)
			}
		})
	}
}

func TestApplyQueryNameAndFindQueryNameAgree(t *testing.T) {
	for _, given := range []string{"a name", "one-two", "a name with  two spaces", "--"} {
		written := statement.ApplyQueryName("select 1", given)
		if got := statement.FindQueryName(written); got != given {
			t.Errorf("wrote %q as %q, read back %q", given, written, got)
		}
	}
}
