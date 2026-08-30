package sqlite

import (
	"regexp"
	"strings"

	"github.com/turanmahmudov/masume/internal/db"
)

// unfinishedStatement is the server's message for a statement that is unfinished, not
// wrong.
const unfinishedStatement = "incomplete input"

// The driver wraps the message of the server. `SQL logic error: ` opens it and ` (N)`
// closes it, and neither is part of what the server said.
var (
	driverPrefix = regexp.MustCompile(`^SQL logic error:\s*`)
	driverCode   = regexp.MustCompile(`\s*\(\d+\)$`)
	// nearToken names the word the parser stopped at, which places the fault.
	nearToken = regexp.MustCompile(`near "([^"]*)"`)
	// namedObject names the column, table or function the server could not find, which
	// places the fault as well.
	namedObject = regexp.MustCompile(
		`no such (?:column|table|function|module|collation sequence|index|trigger|view|` +
			`savepoint|schema): (\S+)`)
)

// ReadStatementProblem returns the problem the server reported when it refused to
// prepare the statement, and nothing where the statement is good or unfinished.
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

// findNamedFault returns where in the statement the name the server wrote stands. A parser
// that stopped names the word it stopped at, and a name it could not resolve is named as it
// was written, which may carry the schema before it.
func findNamedFault(sql, message string) (int, bool) {
	named := ""
	if found := nearToken.FindStringSubmatch(message); found != nil {
		named = found[1]
	} else if found := namedObject.FindStringSubmatch(message); found != nil {
		named = found[1]
		// A name the server wrote as `main.orders` stands in the statement under its last
		// part, because that is the part every writing of it holds.
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
