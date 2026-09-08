package sqlserver

import (
	"errors"
	"regexp"
	"strings"

	mssql "github.com/microsoft/go-mssqldb"

	"github.com/turanmahmudov/masume/internal/db"
	"github.com/turanmahmudov/masume/internal/query/syntax"
)

// analyzerErrorFloor is the first error number the statement analyzer writes about itself.
// A number below it comes from the statement.
const analyzerErrorFloor = 11500

// syntaxErrorNumber is the error of a statement the server could not parse.
const syntaxErrorNumber = 102

// nearText matches the text a parse error stopped on.
var nearText = regexp.MustCompile(`near '([^']*)'`)

// quotedName matches the name an error names, which can carry its schema.
var quotedName = regexp.MustCompile(`'([^']+)'`)

// describeFailure returns the server message without the driver prefix.
func describeFailure(err error) string {
	var reported mssql.Error
	if errors.As(err, &reported) && reported.Message != "" {
		return reported.Message
	}
	return db.DescribeError(err)
}

// ReadStatementProblem returns the fault of a statement out of the rows the analyzer wrote.
// A statement the user is still typing is not a fault.
func ReadStatementProblem(
	sql string, rows []map[string]any,
) (db.StatementProblem, bool) {
	for _, row := range rows {
		number := db.ReadNonNegativeCount(row["error_number"])
		message := db.ReadAnyText(row["error_message"])
		if number >= analyzerErrorFloor || message == "" {
			continue
		}
		if number == syntaxErrorNumber && IsUnfinishedStatement(sql, message) {
			return db.StatementProblem{}, false
		}
		problem := db.StatementProblem{Message: message}
		if offset, found := FindProblemOffset(sql, message); found {
			problem.Offset = offset
			problem.HasOffset = true
		}
		return problem, true
	}
	return db.StatementProblem{}, false
}

// IsUnfinishedStatement is true where a parse error stopped on the last word of the
// statement, which is the word the user is still writing.
func IsUnfinishedStatement(sql, message string) bool {
	near := nearText.FindStringSubmatch(message)
	if near == nil {
		return false
	}
	tokens := syntax.ReadCodeTokens(sql, syntax.FlavourSqlserver)
	if len(tokens) == 0 {
		return false
	}
	last := tokens[len(tokens)-1]
	return strings.EqualFold(sql[last.Start:last.End], near[1])
}

// FindProblemOffset returns the offset in the statement the message points at.
func FindProblemOffset(sql, message string) (int, bool) {
	if near := nearText.FindStringSubmatch(message); near != nil && near[1] != "" {
		if found := strings.Index(sql, near[1]); found != -1 {
			return found, true
		}
	}
	// Other errors name an object or a column, which can carry a schema.
	if named := quotedName.FindStringSubmatch(message); named != nil {
		parts := strings.Split(named[1], ".")
		last := parts[len(parts)-1]
		if found := strings.Index(sql, last); found != -1 {
			return found, true
		}
	}
	return 0, false
}
