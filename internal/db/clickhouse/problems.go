package clickhouse

import (
	"errors"
	"regexp"
	"strconv"
	"strings"

	driver "github.com/ClickHouse/clickhouse-go/v2"

	"github.com/turanmahmudov/masume/internal/db"
)

// failedAt matches the place a parse error stopped at, which the server counts from one.
var failedAt = regexp.MustCompile(`failed at position (\d+)`)

// serverFaultCodes are the errors that report a fault of the server, not of the statement.
var serverFaultCodes = map[int32]bool{
	// NETWORK_ERROR, SOCKET_TIMEOUT, TOO_MANY_SIMULTANEOUS_QUERIES, MEMORY_LIMIT_EXCEEDED,
	// TIMEOUT_EXCEEDED, and the read-only refusal.
	210: true, 209: true, 202: true, 241: true, 159: true, 164: true,
}

// describeFailure returns the message of the server without the code the driver adds.
func describeFailure(err error) string {
	var reported *driver.Exception
	if errors.As(err, &reported) && reported.Message != "" {
		return trimServerPrefix(reported.Message)
	}
	return db.DescribeError(err)
}

// trimServerPrefix removes the node the message was received from, which names the server
// and not the fault.
func trimServerPrefix(message string) string {
	const marker = "DB::Exception: "
	if at := strings.LastIndex(message, marker); at != -1 {
		return message[at+len(marker):]
	}
	return message
}

// ReadStatementProblem returns the fault of a statement out of the error of the server. A
// fault of the server itself is not a fault of the statement. The prefix is the text the
// client wrote before the statement, which the place of the fault counts as well.
func ReadStatementProblem(err error, prefix string) (db.StatementProblem, bool) {
	var reported *driver.Exception
	if !errors.As(err, &reported) || reported.Message == "" {
		return db.StatementProblem{}, false
	}
	if serverFaultCodes[reported.Code] {
		return db.StatementProblem{}, false
	}

	message := trimServerPrefix(reported.Message)
	problem := db.StatementProblem{Message: message}
	if offset, found := FindProblemOffset(message); found && offset >= len(prefix) {
		problem.Offset = offset - len(prefix)
		problem.HasOffset = true
	}
	return problem, true
}

// FindProblemOffset returns the offset in the statement the message points at.
func FindProblemOffset(message string) (int, bool) {
	held := failedAt.FindStringSubmatch(message)
	if held == nil {
		return 0, false
	}
	position, err := strconv.Atoi(held[1])
	if err != nil || position < 1 {
		return 0, false
	}
	// The server counts from one.
	return position - 1, true
}
