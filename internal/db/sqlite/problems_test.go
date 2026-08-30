package sqlite

import (
	"errors"
	"testing"
)

func TestReadStatementProblem(t *testing.T) {
	cases := []struct {
		name      string
		sql       string
		message   string
		reports   bool
		wanted    string
		offset    int
		hasOffset bool
	}{
		{"a column the server cannot find", "select nope from seeded",
			"SQL logic error: no such column: nope (1)", true, "no such column: nope", 7, true},
		{"a table the server cannot find", "select * from nosuch",
			"no such table: nosuch", true, "no such table: nosuch", 14, true},
		{"a table the server named with its schema", "select * from main.nosuch",
			"no such table: main.nosuch", true, "no such table: main.nosuch", 19, true},
		{"a word the parser stopped at", "select * form seeded",
			`near "form": syntax error`, true, `near "form": syntax error`, 9, true},
		{"a statement that is unfinished", "select * from",
			"incomplete input", false, "", 0, false},
		{"a fault the server placed nowhere", "select 1",
			"database is locked", true, "database is locked", 0, false},
	}
	for _, held := range cases {
		problem, reported := ReadStatementProblem(held.sql, errors.New(held.message))
		if reported != held.reports {
			t.Errorf("%s: reported %v, wanted %v", held.name, reported, held.reports)
			continue
		}
		if !reported {
			continue
		}
		if problem.Message != held.wanted {
			t.Errorf("%s: the message reads %q, wanted %q",
				held.name, problem.Message, held.wanted)
		}
		if problem.HasOffset != held.hasOffset || problem.Offset != held.offset {
			t.Errorf("%s: the fault sits at %d (given %v), wanted %d (given %v)",
				held.name, problem.Offset, problem.HasOffset, held.offset, held.hasOffset)
		}
	}
}
