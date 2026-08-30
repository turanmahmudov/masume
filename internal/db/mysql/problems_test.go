package mysql

import (
	"errors"
	"testing"

	driver "github.com/go-sql-driver/mysql"

	"github.com/turanmahmudov/masume/internal/db"
)

// MySQL says where a statement went wrong by quoting the text it stopped on, and the editor
// marks that place. A place read wrong sends the reader looking at the wrong character.
func TestFindMysqlOffsetPointsAtTheTextTheServerQuoted(t *testing.T) {
	for _, held := range []struct {
		name    string
		sql     string
		message string
		want    int
		is      bool
	}{
		{
			name:    "the text it stopped on",
			sql:     "select * form orders",
			message: "You have an error in your SQL syntax near 'form orders' at line 1",
			want:    9,
			is:      true,
		},
		{
			name:    "a relation it could not find",
			sql:     "select * from nothing_here",
			message: "Table 'shop.nothing_here' doesn't exist",
			want:    14,
			is:      true,
		},
		{
			name:    "a column it could not find",
			sql:     "select nothing_here from orders",
			message: "Unknown column 'nothing_here' in 'field list'",
			want:    7,
			is:      true,
		},
		{
			name:    "a fault on the second line",
			sql:     "select *\nform orders",
			message: "You have an error in your SQL syntax near 'form orders' at line 2",
			want:    9,
			is:      true,
		},
		{
			name:    "a message that names no place",
			sql:     "select 1",
			message: "Lost connection to MySQL server",
			want:    0,
			is:      false,
		},
	} {
		t.Run(held.name, func(t *testing.T) {
			answered, is := findMysqlOffset(held.sql, held.message)
			if is != held.is {
				t.Fatalf("read = %v, wanted %v", is, held.is)
			}
			if is && answered != held.want {
				t.Errorf("the place reads %d, wanted %d (%q)",
					answered, held.want, held.sql[min(answered, len(held.sql)):])
			}
		})
	}
}

// The place has to stay inside the statement, because the editor slices the text with it.
func TestFindMysqlOffsetStaysInsideTheStatement(t *testing.T) {
	for _, held := range []struct{ sql, message string }{
		{"select 1", "syntax error near 'nothing in the statement' at line 1"},
		{"select 1", "syntax error near 'x' at line 99"},
		{"", "syntax error near 'x' at line 1"},
		{"select 1", "Table 'shop.nothing_here' doesn't exist"},
	} {
		answered, is := findMysqlOffset(held.sql, held.message)
		if !is {
			continue
		}
		if answered < 0 || answered > len(held.sql) {
			t.Errorf("%q with %q answered %d, outside a statement of %d",
				held.sql, held.message, answered, len(held.sql))
		}
	}
}

// A statement still being typed is not wrong yet. MySQL quotes nothing at the end of the
// input, and the editor must not mark a fault while the reader is mid-word.
func TestIsUnfinishedMysqlStatementReadsWhatTheServerQuotedNothingFor(t *testing.T) {
	for _, held := range []struct {
		message string
		want    bool
	}{
		{"You have an error in your SQL syntax near '' at line 1", true},
		{"You have an error in your SQL syntax near '' at line 12", true},

		{"You have an error in your SQL syntax near 'form' at line 1", false},
		{"Table 'shop.orders' doesn't exist", false},
		{"", false},
	} {
		if answered := isUnfinishedMysqlStatement(held.message); answered != held.want {
			t.Errorf("%q reads as unfinished = %v, wanted %v",
				held.message, answered, held.want)
		}
	}
}

// A deadlock rolls the whole transaction back, and a lock timeout rolls back the statement
// alone unless the server was started to do more. A transaction marked failed while the
// server still holds it open would be left behind, and the next `begin` would commit it.
func TestTheServerRollsBackOnADeadlockAndOnATimeoutOnlyWhereItSaysSo(t *testing.T) {
	deadlock := &driver.MySQLError{Number: mysqlDeadlock}
	timeout := &driver.MySQLError{Number: mysqlLockTimeout}
	ordinary := &driver.MySQLError{Number: 1146}

	for _, held := range []struct {
		name             string
		err              error
		rollsBackTimeout bool
		want             db.TransactionState
	}{
		{"a deadlock", deadlock, false, db.TransactionFailed},
		{"a lock timeout on a server that keeps the transaction", timeout, false,
			db.TransactionOpen},
		{"a lock timeout on a server that rolls it back", timeout, true,
			db.TransactionFailed},
		{"a table that is not there", ordinary, false, db.TransactionOpen},
		{"a fault the driver did not report", errors.New("gone"), true,
			db.TransactionOpen},
	} {
		t.Run(held.name, func(t *testing.T) {
			session := &mysqlSession{rollsBackOnTimeout: held.rollsBackTimeout}
			session.transaction.WriteState(db.TransactionOpen)
			session.markTransactionFailed(held.err)
			if answered := session.transaction.ReadState(); answered != held.want {
				t.Errorf("the state reads %q, wanted %q", answered, held.want)
			}
		})
	}
}

func TestReadIntReadsTheDigitsAndStopsAtTheRest(t *testing.T) {
	for _, held := range []struct {
		written string
		want    int
	}{
		{"1", 1},
		{"42", 42},
		{"12abc", 12},
		{"", 0},
		{"abc", 0},
		{"0", 0},
	} {
		if answered := readInt(held.written); answered != held.want {
			t.Errorf("%q reads as %d, wanted %d", held.written, answered, held.want)
		}
	}
}
