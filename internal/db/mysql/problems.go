package mysql

import (
	"regexp"
	"strings"
)

// mysqlCannotPrepare is the code MySQL reports for a statement it cannot prepare, which
// is not a statement that is wrong.
const mysqlCannotPrepare = 1295

// mysqlDeadlock is the code the server reports where it chose this connection to break a
// deadlock. It always rolls the whole transaction back.
const mysqlDeadlock uint16 = 1213

// mysqlLockTimeout is the code the server reports where a lock was waited for too long. It
// rolls back the statement alone and leaves the transaction open, unless the server was
// started with `innodb_rollback_on_timeout`, which is off by default.
const mysqlLockTimeout uint16 = 1205

var atLine = regexp.MustCompile(`at line (\d+)`)
var nearText = regexp.MustCompile(`near '([\s\S]*)' at line \d+`)
var quotedName = regexp.MustCompile(`'([^']+)'`)
var unfinishedNear = regexp.MustCompile(`near '' at line \d+`)

// findMysqlOffset returns the offset in the statement the message points at.
func findMysqlOffset(sql, message string) (int, bool) {
	lines := atLine.FindStringSubmatch(message)
	line := 1
	if lines != nil {
		line = readInt(lines[1])
	}

	lineStart := 0
	for counted := 1; counted < line; counted++ {
		broke := strings.IndexByte(sql[lineStart:], '\n')
		if broke == -1 {
			break
		}
		lineStart += broke + 1
	}

	// A parse error quotes the text it stopped on, which gives the exact place.
	if near := nearText.FindStringSubmatch(message); near != nil {
		if found := strings.Index(sql[lineStart:], near[1]); found != -1 {
			return lineStart + found, true
		}
	}

	// Every other error names what it could not find, sometimes with the database in
	// front. The statement holds the last part of that name.
	if named := quotedName.FindStringSubmatch(message); named != nil {
		parts := strings.Split(named[1], ".")
		last := parts[len(parts)-1]
		if found := strings.Index(sql[lineStart:], last); found != -1 {
			return lineStart + found, true
		}
	}

	if lines == nil {
		return 0, false
	}
	return lineStart, true
}

func readInt(written string) int {
	value := 0
	for _, character := range written {
		if character < '0' || character > '9' {
			break
		}
		value = value*10 + int(character-'0')
	}
	return value
}

// isUnfinishedMysqlStatement is true where MySQL quoted nothing, which it does at the
// end of the input.
func isUnfinishedMysqlStatement(message string) bool {
	return unfinishedNear.MatchString(message)
}
