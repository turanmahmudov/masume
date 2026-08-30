package editor_test

import (
	"strings"
	"testing"

	"github.com/turanmahmudov/masume/internal/query/editor"
	"github.com/turanmahmudov/masume/internal/query/statement"
	"github.com/turanmahmudov/masume/internal/query/syntax"
)

// buildKnowledge answers a catalog that holds one relation and its columns.
func buildKnowledge() editor.SchemaKnowledge {
	return editor.SchemaKnowledge{
		Loaded: true,
		IsKnownTable: func(reference statement.TableReference) bool {
			return strings.EqualFold(reference.Name, "orders")
		},
		ColumnsByQualifier: map[string][]string{
			"orders": {"id", "customer", "total"},
			"o":      {"id", "customer", "total"},
		},
	}
}

// A fault the client finds itself is marked before the statement is sent, so the user is not
// waiting on a server to be told about a bracket.
func TestFindLocalDiagnosticsMarksAnUnclosedPart(t *testing.T) {
	for _, held := range []struct {
		name  string
		sql   string
		holds string
	}{
		{"an unclosed string", "select * from orders where customer = 'ada", "string"},
		{"an unclosed name", `select * from "orders`, "name"},
		{"an unclosed comment", "select 1 /* why", "comment"},
	} {
		t.Run(held.name, func(t *testing.T) {
			found := editor.FindLocalDiagnostics(held.sql, buildKnowledge(), syntax.FlavourStandard)
			if len(found) == 0 {
				t.Fatalf("%q was marked with nothing", held.sql)
			}
			if !strings.Contains(strings.ToLower(found[0].Message), held.holds) {
				t.Errorf("the message reads %q, wanted it to name the %s",
					found[0].Message, held.holds)
			}
			// The mark has to point somewhere inside the buffer, because the editor draws it.
			if found[0].Start < 0 || found[0].End > len(held.sql) {
				t.Errorf("the mark covers %d to %d of %d cells",
					found[0].Start, found[0].End, len(held.sql))
			}
		})
	}
}

// An unclosed part takes the rest of the buffer, so every bracket and name after it would read
// as wrong. Only the one fault that matters is reported.
func TestFindLocalDiagnosticsReportsTheUnclosedPartAlone(t *testing.T) {
	const sql = "select * from nothing_here where a = 'ada and ((("
	found := editor.FindLocalDiagnostics(sql, buildKnowledge(), syntax.FlavourStandard)
	if len(found) != 1 {
		t.Errorf("the buffer was marked %d times, wanted the unclosed string alone: %+v",
			len(found), found)
	}
}

func TestFindLocalDiagnosticsMarksABracketThatNeverCloses(t *testing.T) {
	found := editor.FindLocalDiagnostics(
		"select count( from orders", buildKnowledge(), syntax.FlavourStandard)
	if len(found) == 0 {
		t.Fatal("an unclosed bracket was marked with nothing")
	}
}

// A relation the catalog does not hold is a name the server will refuse, so it is marked while
// the user is still typing.
func TestFindLocalDiagnosticsMarksARelationTheCatalogHasNot(t *testing.T) {
	found := editor.FindLocalDiagnostics(
		"select * from nothing_here", buildKnowledge(), syntax.FlavourStandard)
	if len(found) == 0 {
		t.Fatal("a relation the catalog has not was marked with nothing")
	}
	if !strings.Contains(found[0].Message, "nothing_here") {
		t.Errorf("the message reads %q and does not name the relation", found[0].Message)
	}
}

// A relation the catalog holds is not marked, or the editor would cry wolf on every statement.
func TestFindLocalDiagnosticsLeavesAGoodStatementAlone(t *testing.T) {
	for _, sql := range []string{
		"select * from orders",
		"select id, customer from orders where total > 0",
		"select o.id from orders o",
		// A name the statement makes itself is not in the catalog and is still good.
		"with recent as (select 1) select * from recent",
	} {
		if found := editor.FindLocalDiagnostics(
			sql, buildKnowledge(), syntax.FlavourStandard); len(found) != 0 {
			t.Errorf("%q was marked: %+v", sql, found)
		}
	}
}

// Nothing is reported until the catalog has been read, because every name would read as one
// the server has not.
func TestFindLocalDiagnosticsSaysNothingBeforeTheCatalogIsRead(t *testing.T) {
	found := editor.FindLocalDiagnostics(
		"select * from nothing_here", editor.NothingKnown(), syntax.FlavourStandard)
	if len(found) != 0 {
		t.Errorf("a statement was marked against a catalog never read: %+v", found)
	}
}

func TestFindLocalDiagnosticsSaysNothingForAnEmptyBuffer(t *testing.T) {
	for _, sql := range []string{"", "   ", "\n\n"} {
		if found := editor.FindLocalDiagnostics(
			sql, buildKnowledge(), syntax.FlavourStandard); len(found) != 0 {
			t.Errorf("%q was marked: %+v", sql, found)
		}
	}
}

// The marks come out in the order they stand in the buffer, because the editor draws them in
// one pass and the first one is the one reported in the bar.
func TestFindLocalDiagnosticsAnswersInTheOrderOfTheBuffer(t *testing.T) {
	const sql = "select * from nothing_here, also_missing"
	found := editor.FindLocalDiagnostics(sql, buildKnowledge(), syntax.FlavourStandard)
	if len(found) < 2 {
		t.Skipf("the buffer was marked %d times, so there is no order to check", len(found))
	}
	for at := 1; at < len(found); at++ {
		if found[at].Start < found[at-1].Start {
			t.Errorf("mark %d starts at %d, behind the %d before it",
				at, found[at].Start, found[at-1].Start)
		}
	}
}
