package postgres

import (
	"errors"

	"github.com/jackc/pgx/v5/pgconn"

	"github.com/turanmahmudov/masume/internal/db"
)

// describeFailure writes a failure of the server as the message the server wrote, without
// the wrapper the driver puts around it.
func describeFailure(err error) string {
	var reported *pgconn.PgError
	if errors.As(err, &reported) && reported.Message != "" {
		return reported.Message
	}
	return db.DescribeError(err)
}

// serverFaultClasses are the SQLSTATE classes that report a server fault, not a fault
// in the statement.
var serverFaultClasses = map[string]bool{
	"08": true, "53": true, "57": true, "58": true, "XX": true,
}

// isUnfinishedStatement is true if the statement is unfinished, not wrong.
func isUnfinishedStatement(message string) bool {
	return message == "syntax error at end of input"
}

// ReadStatementProblem returns the problem the server reported for a Describe, and
// nothing where the fault is the server's own.
func ReadStatementProblem(code, message string, position int) (db.StatementProblem, bool) {
	if len(code) != 5 || serverFaultClasses[code[:2]] {
		return db.StatementProblem{}, false
	}
	if message == "" || isUnfinishedStatement(message) {
		return db.StatementProblem{}, false
	}
	// The server counts from one.
	if position > 0 {
		return db.StatementProblem{Message: message, Offset: position - 1, HasOffset: true}, true
	}
	return db.StatementProblem{Message: message}, true
}
