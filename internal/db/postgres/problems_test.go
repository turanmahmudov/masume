package postgres

import (
	"errors"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"
)

// The message the server wrote is what the user needs. The driver wraps it in text of its
// own, which says nothing a reader can act on.
func TestDescribeFailureAnswersTheMessageOfTheServer(t *testing.T) {
	reported := &pgconn.PgError{
		Code: "42P01", Message: `relation "nothing_here" does not exist`,
	}
	if held := describeFailure(reported); held != reported.Message {
		t.Errorf("the message reads %q, wanted the one the server wrote", held)
	}

	// The message reads through a wrap too, because the port marks what a driver reported.
	wrapped := errors.Join(errors.New("running the statement"), reported)
	if held := describeFailure(wrapped); held != reported.Message {
		t.Errorf("a wrapped error reads %q, wanted the message of the server", held)
	}
}

// An error the driver never gave a message to still has to read as something.
func TestDescribeFailureFallsBackWhereTheServerSaidNothing(t *testing.T) {
	for _, held := range []error{
		errors.New("the connection closed"),
		&pgconn.PgError{Code: "08006"},
	} {
		if describeFailure(held) == "" {
			t.Errorf("%v is described as an empty text", held)
		}
	}
}

// A fault in the statement is marked in the editor. A fault of the server is not: the
// statement may be perfect and the mark would send the reader looking in the wrong place.
func TestReadStatementProblemTellsAStatementFaultFromAServerFault(t *testing.T) {
	for _, held := range []struct {
		name     string
		code     string
		message  string
		position int
		reported bool
	}{
		{"a relation that is not there", "42P01", `relation "x" does not exist`, 0, true},
		{"a column that is not there", "42703", `column "x" does not exist`, 15, true},
		{"a syntax fault", "42601", "syntax error at or near \"slect\"", 1, true},

		// A class the server uses for its own troubles: the statement is not at fault.
		{"the connection broke", "08006", "connection failure", 0, false},
		{"out of memory", "53200", "out of memory", 0, false},
		{"the server is shutting down", "57P01", "terminating connection", 0, false},
		{"an internal fault", "XX000", "internal error", 0, false},

		// A statement still being typed is not wrong yet, so nothing is marked while the
		// reader is mid-word.
		{"a statement not finished", "42601", "syntax error at end of input", 0, false},

		{"no code at all", "", "something", 0, false},
		{"a code of the wrong length", "42", "something", 0, false},
		{"no message", "42601", "", 0, false},
	} {
		t.Run(held.name, func(t *testing.T) {
			problem, is := ReadStatementProblem(held.code, held.message, held.position)
			if is != held.reported {
				t.Fatalf("reported = %v, wanted %v", is, held.reported)
			}
			if !is {
				return
			}
			if problem.Message != held.message {
				t.Errorf("the message reads %q", problem.Message)
			}
		})
	}
}

// The server counts the place of a fault from one, and the editor counts from zero, so the
// mark would land one character along.
func TestReadStatementProblemCountsTheOffsetFromZero(t *testing.T) {
	problem, is := ReadStatementProblem("42601", "syntax error", 8)
	if !is {
		t.Fatal("a syntax fault was not reported")
	}
	if !problem.HasOffset {
		t.Fatal("the fault carries no place")
	}
	if problem.Offset != 7 {
		t.Errorf("the place reads %d, wanted the eighth character at seven", problem.Offset)
	}
}

// A fault the server gave no place to is still reported, and the editor marks the statement
// rather than one character of it.
func TestReadStatementProblemCarriesNoOffsetWhereTheServerGaveNone(t *testing.T) {
	problem, is := ReadStatementProblem("42P01", `relation "x" does not exist`, 0)
	if !is {
		t.Fatal("the fault was not reported")
	}
	if problem.HasOffset {
		t.Errorf("the fault carries a place of %d, wanted none", problem.Offset)
	}
}

// The catalog answers a code for what a relation is, and the tree draws each kind its own way.
func TestMapRelationKindReadsTheCodeOfTheCatalog(t *testing.T) {
	// A `"char"` column arrives as the code of one byte: r is an ordinary relation, v a view,
	// m a view the server keeps the rows of.
	seen := map[string]bool{}
	for _, code := range []any{int32('r'), int32('v'), int32('m')} {
		held := MapRelationKind(code)
		if held == "" {
			t.Errorf("the code %v reads as no kind", code)
		}
		seen[string(held)] = true
	}
	if len(seen) != 3 {
		t.Errorf("three codes read as %d kinds, wanted one each", len(seen))
	}
}

// The driver decodes an array of the server into a slice, so the members arrive already
// apart. The text form the server prints never reaches here.
func TestReadTextArrayReadsTheSlicesTheDriverGives(t *testing.T) {
	for _, held := range []struct {
		name  string
		value any
		count int
	}{
		{"nothing", nil, 0},
		{"an empty slice of text", []string{}, 0},
		{"members as text", []string{"pending", "packed", "sent"}, 3},
		// A driver hands over a slice of values where it could not settle on one type.
		{"members as values", []any{"pending", "packed"}, 2},
		{"a member holding a comma", []string{"a,b", "c"}, 2},
		// A shape the driver never gives answers no members rather than one wrong one.
		{"the text form of an array", "{pending,packed}", 0},
	} {
		t.Run(held.name, func(t *testing.T) {
			if answered := ReadTextArray(held.value); len(answered) != held.count {
				t.Errorf("%v reads as %q, wanted %d members",
					held.value, answered, held.count)
			}
		})
	}

	// The members keep the text they carried, because they name the values a column takes.
	if held := ReadTextArray([]string{"pending", "packed"}); held[0] != "pending" {
		t.Errorf("the first member reads %q", held[0])
	}
}
