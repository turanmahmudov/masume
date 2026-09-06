package sqlite

import (
	"regexp"
	"strings"

	"github.com/turanmahmudov/masume/internal/db"
)

// unfinishedStatement is the SQLite error text for incomplete input.
const unfinishedStatement = "incomplete input"

// The driver adds a `SQL logic error: ` prefix and a ` (N)` suffix to the SQLite message.
var (
	driverPrefix = regexp.MustCompile(`^SQL logic error:\s*`)
	driverCode   = regexp.MustCompile(`\s*\(\d+\)$`)
	// nearToken names the word the parser stopped at, which places the fault.
	nearToken = regexp.MustCompile(`near "([^"]*)"`)
	// namedObject is the missing column, table, or function name.
	namedObject = regexp.MustCompile(
		`no such (?:column|table|function|module|collation sequence|index|trigger|view|` +
			`savepoint|schema): (\S+)`)
)

// ReadStatementProblem returns preparation errors and excludes valid or incomplete statements.
func ReadStatementProblem(sql string, err error) (db.StatementProblem, bool) {
	if err == nil {
		return db.StatementProblem{}, false
	}
	message := db.DescribeError(err)
	if message == "" || strings.Contains(message, unfinishedStatement) {
		return db.StatementProblem{}, false
	}
	message = driverCode.ReplaceAllString(driverPrefix.ReplaceAllString(message, ""), "")
	if message == "" {
		return db.StatementProblem{}, false
	}

	// The driver hands over no offset, so the name the server wrote places the fault.
	problem := db.StatementProblem{Message: message}
	if at, found := findNamedFault(sql, message); found {
		problem.Offset, problem.HasOffset = at, true
	}
	return problem, true
}

// findNamedFault locates the token or object name from the SQLite error.
func findNamedFault(sql, message string) (int, bool) {
	named := ""
	if found := nearToken.FindStringSubmatch(message); found != nil {
		named = found[1]
	} else if found := namedObject.FindStringSubmatch(message); found != nil {
		named = found[1]
		// Qualified names match by their final segment.
		if at := strings.LastIndexByte(named, '.'); at != -1 {
			named = named[at+1:]
		}
	}
	if named == "" {
		return 0, false
	}
	at := strings.Index(sql, named)
	if at == -1 {
		return 0, false
	}
	return len([]rune(sql[:at])), true
}
