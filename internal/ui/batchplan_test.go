package ui

import (
	"strings"
	"testing"

	"github.com/turanmahmudov/masume/internal/app"
)

// The plan of a batch belongs to the result on show.
func TestThePlanOfABatchFollowsTheResultOnShow(t *testing.T) {
	const written = "select * from orders;\n\nselect * from customers;"
	_, connection, tab := buildEditingModel(t, written, len(written)-1)
	tab.Results.Start([]string{"select * from orders", "select * from customers"}, 200)

	if held := tab.StatementUnderCaret(connection.Session); !strings.Contains(held, "customers") {
		t.Fatalf("the caret stands in %q, wanted the second statement", held)
	}
	if held := tab.StatementToPlan(connection.Session); !strings.Contains(held, "orders") {
		t.Errorf("the plan reads %q, wanted the statement of the result on show", held)
	}

	tab.Results.SelectNextResult(1)
	if held := tab.StatementToPlan(connection.Session); !strings.Contains(held, "customers") {
		t.Errorf("the second result plans %q, wanted its own statement", held)
	}
}

// One statement plans what the caret sits in, so an edit after the run is planned as typed.
func TestThePlanOfOneStatementFollowsTheCaret(t *testing.T) {
	const written = "select * from orders"
	_, connection, tab := buildEditingModel(t, written, len(written))
	tab.Results.Start([]string{written}, 200)
	tab.Editor = app.NewEditorBuffer("select * from customers", len("select * from customers"))

	if held := tab.StatementToPlan(connection.Session); !strings.Contains(held, "customers") {
		t.Errorf("the plan reads %q, wanted the statement at the caret", held)
	}
}
