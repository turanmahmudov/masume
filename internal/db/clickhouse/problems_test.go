package clickhouse

import (
	"errors"
	"testing"

	driver "github.com/ClickHouse/clickhouse-go/v2"

	"github.com/turanmahmudov/masume/internal/db"
)

// buildServerError returns the error of the server, as the driver hands it over.
func buildServerError(code int32, message string) error {
	return &driver.Exception{Code: code, Message: message}
}

// The message of the server names the node it came from, which says nothing about the
// fault. The client shows the fault alone.
func TestDescribeFailureTakesTheMessageAlone(t *testing.T) {
	held := buildServerError(60,
		"Received from localhost:9000. DB::Exception: Table shop.nope does not exist.")
	if written := describeFailure(held); written != "Table shop.nope does not exist." {
		t.Errorf("the failure reads %q", written)
	}
}

// A parse error names the place it stopped at, counted from one and from the start of the
// text the client sent, which opens with a prefix of its own.
func TestReadStatementProblemCountsTheOffsetFromTheStatement(t *testing.T) {
	const prefix = "explain plan "
	problem, faulty := ReadStatementProblem(
		buildServerError(62, "Syntax error: failed at position 22 (*)"), prefix)
	if !faulty {
		t.Fatal("a statement the server cannot parse was checked as good")
	}
	if !problem.HasOffset {
		t.Fatal("the fault carries no place in the statement")
	}
	if problem.Offset != 21-len(prefix) {
		t.Errorf("the fault points at offset %d", problem.Offset)
	}
}

// A fault of the server itself says nothing about the statement.
func TestReadStatementProblemIgnoresAFaultOfTheServer(t *testing.T) {
	if _, faulty := ReadStatementProblem(
		buildServerError(241, "Memory limit exceeded"), ""); faulty {
		t.Error("a fault of the server was reported as a fault of the statement")
	}
}

// An error the driver did not read from the server is no fault of the statement.
func TestReadStatementProblemIgnoresAnErrorOfItsOwn(t *testing.T) {
	if _, faulty := ReadStatementProblem(errors.New("broken pipe"), ""); faulty {
		t.Error("an error of the client was reported as a fault of the statement")
	}
	if written := describeFailure(db.NewDatabaseError("held")); written != "held" {
		t.Errorf("the failure reads %q", written)
	}
}
