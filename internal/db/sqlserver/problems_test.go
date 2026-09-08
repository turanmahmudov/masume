package sqlserver

import "testing"

// buildAnalyzerRows returns the rows the statement analyzer writes for one fault.
func buildAnalyzerRows(pairs ...any) []map[string]any {
	rows := []map[string]any{}
	for at := 0; at+1 < len(pairs); at += 2 {
		rows = append(rows, map[string]any{
			"error_number": pairs[at], "error_message": pairs[at+1],
		})
	}
	return rows
}

// The analyzer writes the fault of the statement first and then its own faults about the
// analysis, so the first row is the one the editor shows.
func TestReadStatementProblemTakesTheFaultOfTheStatement(t *testing.T) {
	rows := buildAnalyzerRows(
		int64(208), "Invalid object name 'dbo.nope'.",
		int64(11529), "The metadata could not be determined.")

	problem, faulty := ReadStatementProblem("select * from dbo.nope", rows)
	if !faulty {
		t.Fatal("a read of a table that is not there was checked as good")
	}
	if problem.Message != "Invalid object name 'dbo.nope'." {
		t.Errorf("the fault reads %q", problem.Message)
	}
	if !problem.HasOffset {
		t.Fatal("the fault carries no place in the statement")
	}
	if problem.Offset != len("select * from dbo.") {
		t.Errorf("the fault points at offset %d, wanted the name", problem.Offset)
	}
}

// A statement the user has not finished stops the parser on its last word, and that is not
// a fault the editor marks.
func TestReadStatementProblemLeavesAnUnfinishedStatementAlone(t *testing.T) {
	rows := buildAnalyzerRows(int64(102), "Incorrect syntax near 'from'.")
	if _, faulty := ReadStatementProblem("select customer from", rows); faulty {
		t.Error("an unfinished statement was checked as faulty")
	}
}

// A parse error inside the statement is a fault, because the user has written past it.
func TestReadStatementProblemReportsAFaultInsideTheStatement(t *testing.T) {
	rows := buildAnalyzerRows(int64(102), "Incorrect syntax near '*'.")
	problem, faulty := ReadStatementProblem("selct * from dbo.orders", rows)
	if !faulty {
		t.Fatal("a statement the server cannot parse was checked as good")
	}
	if problem.Offset != len("selct ") {
		t.Errorf("the fault points at offset %d, wanted the star", problem.Offset)
	}
}

// The analyzer numbers its own faults from 11500 up. One of those alone says nothing about
// the statement.
func TestReadStatementProblemIgnoresTheFaultsOfTheAnalyzer(t *testing.T) {
	rows := buildAnalyzerRows(int64(11501), "The batch could not be analyzed.")
	if _, faulty := ReadStatementProblem("select 1", rows); faulty {
		t.Error("a fault of the analyzer was reported as a fault of the statement")
	}
}
