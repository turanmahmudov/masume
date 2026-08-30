package editor_test

import (
	"strings"
	"testing"

	"github.com/turanmahmudov/masume/internal/query/editor"
	"github.com/turanmahmudov/masume/internal/query/statement"
)

// The list over the statement offers relations in one place and columns in another, so where
// the caret stands decides what the user is shown.
func TestResolveNamePositionReadsWhatTheStatementExpects(t *testing.T) {
	for _, held := range []struct {
		name string
		// The caret stands where the bar is.
		sql  string
		want editor.NamePosition
	}{
		{"after from", "select * from |", editor.PositionRelation},
		{"after join", "select * from orders join |", editor.PositionRelation},
		{"after update", "update |", editor.PositionRelation},
		{"after insert into", "insert into |", editor.PositionRelation},
		{"after delete from", "delete from |", editor.PositionRelation},

		{"in the select list", "select |", editor.PositionColumn},
		{"after where", "select * from orders where |", editor.PositionColumn},
		{"after order by", "select * from orders order by |", editor.PositionColumn},
		{"after group by", "select * from orders group by |", editor.PositionColumn},
		{"after a comma in the select list", "select id, |", editor.PositionColumn},

		{"on nothing at all", "|", editor.PositionNone},
	} {
		t.Run(held.name, func(t *testing.T) {
			sql, offset := splitCaret(held.sql)
			if answered := editor.ResolveNamePosition(sql, offset); answered != held.want {
				t.Errorf("%q expects %q at %d, wanted %q", sql, answered, offset, held.want)
			}
		})
	}
}

// The caret can stand inside a comment or an unfinished string, and the list must not offer a
// name there: what the user is typing is text, not SQL.
func TestResolveNamePositionOffersNothingInsideTextOrAComment(t *testing.T) {
	for _, written := range []string{
		"select * from orders where name = 'ad|",
		"select * from orders -- from |",
	} {
		sql, offset := splitCaret(written)
		if answered := editor.ResolveNamePosition(sql, offset); answered == editor.PositionRelation {
			t.Errorf("%q offers a relation inside text or a comment", sql)
		}
	}
}

func TestResolveNamePositionHoldsAnOffsetPastTheEnd(t *testing.T) {
	const sql = "select * from "
	// An offset past the buffer must be read as the end of it rather than reach outside.
	if answered := editor.ResolveNamePosition(sql, len(sql)+50); answered != editor.PositionRelation {
		t.Errorf("an offset past the end reads %q", answered)
	}
}

// A server refuses a qualified name on the left of an assignment, so `set a.name = …` is
// rejected before it is sent.
func TestIsUpdateSetTargetKnowsTheLeftOfAnAssignment(t *testing.T) {
	for _, held := range []struct {
		name string
		sql  string
		want bool
	}{
		{"on the target of a set", "update orders set |", true},
		{"on a later target", "update orders set paid = true, |", true},
		{"in the value of a set", "update orders set paid = |", false},
		{"after where", "update orders set paid = true where |", false},
		{"in a plain read", "select | from orders", false},
		{"on the relation of an update", "update |", false},
	} {
		t.Run(held.name, func(t *testing.T) {
			sql, offset := splitCaret(held.sql)
			if answered := editor.IsUpdateSetTarget(sql, offset); answered != held.want {
				t.Errorf("%q at %d reads %v, wanted %v", sql, offset, answered, held.want)
			}
		})
	}
}

// What the user has typed so far is the word the list filters on, and it stops at whatever is
// not part of a name.
func TestReadPrefixAnswersTheWordBeingTyped(t *testing.T) {
	for _, held := range []struct {
		sql  string
		want string
	}{
		{"select cust|", "cust"},
		{"select * from ord|", "ord"},
		{"select |", ""},
		{"select id,|", ""},
		{"select customer_i|", "customer_i"},
		// A qualified name keeps its qualifier, so the list can resolve the alias before it
		// filters the columns of the relation behind it.
		{"select o.cust|", "o.cust"},
		{"|", ""},
	} {
		sql, offset := splitCaret(held.sql)
		if answered := editor.ReadPrefix(sql, offset); answered != held.want {
			t.Errorf("%q at %d reads the prefix %q, wanted %q", sql, offset, answered, held.want)
		}
	}
}

func TestReadPrefixHoldsAnOffsetPastTheEnd(t *testing.T) {
	const sql = "select cust"
	if answered := editor.ReadPrefix(sql, len(sql)+50); answered != "cust" {
		t.Errorf("an offset past the end reads %q", answered)
	}
}

// splitCaret answers the statement with the bar taken out, and where the bar stood.
func splitCaret(written string) (string, int) {
	at := strings.IndexByte(written, '|')
	if at < 0 {
		return written, len(written)
	}
	return written[:at] + written[at+1:], at
}

func TestIsUpdateSetTargetReadsOnlyTheTopLevelOfTheStatement(t *testing.T) {
	// A name inside a bracket belongs to an expression, not to the assignment list, so
	// the qualified form is allowed there.
	for _, held := range []struct {
		name   string
		sql    string
		offset int
		want   bool
	}{
		{"the first assigned name", "update t set ", 13, true},
		{"a second assigned name after a comma", "update t set a = 1, ", 20, true},
		{"the right of an equals sign", "update t set a = ", 17, false},
		{"inside a bracket on the right", "update t set a = coalesce(", 26, false},
		{"after that bracket closes", "update t set a = coalesce(b, 1), ", 33, true},
		{"after the set list ends", "update t set a = 1 where ", 25, false},
		{"before the set word", "update t ", 9, false},
		{"after a semicolon ends the statement", "update t set a = 1; select ", 27, false},
		{"a select is no assignment list", "select ", 7, false},
	} {
		t.Run(held.name, func(t *testing.T) {
			if got := editor.IsUpdateSetTarget(held.sql, held.offset); got != held.want {
				t.Errorf("IsUpdateSetTarget(%q, %d) = %v, want %v",
					held.sql, held.offset, got, held.want)
			}
		})
	}
}

func TestNothingKnownReportsNoRelationAsUnknown(t *testing.T) {
	// The catalog has not been read, so every name has to pass.
	knowledge := editor.NothingKnown()
	if knowledge.Loaded {
		t.Error("an unread catalog says it was read")
	}
	if !knowledge.IsKnownTable(statement.TableReference{Name: "anything"}) {
		t.Error("a relation was called unknown before the catalog was read")
	}
	if len(knowledge.ColumnsByQualifier) != 0 {
		t.Errorf("the columns hold %d entries, want none", len(knowledge.ColumnsByQualifier))
	}
}

func TestResolveNamePositionOffersARelationAfterTheWordsThatNameOne(t *testing.T) {
	for _, held := range []struct {
		name string
		sql  string
		want editor.NamePosition
	}{
		{"after from", "select * from ", editor.PositionRelation},
		{"after join", "select * from a join ", editor.PositionRelation},
		{"after into", "insert into ", editor.PositionRelation},
		{"after update", "update ", editor.PositionRelation},
		{"after table", "table ", editor.PositionRelation},
		{"after select", "select ", editor.PositionColumn},
		{"after where", "select * from t where ", editor.PositionColumn},
		{"after and", "select * from t where a = 1 and ", editor.PositionColumn},
		{"after on", "select * from a join b on ", editor.PositionColumn},
		{"after having", "select 1 having ", editor.PositionColumn},
		{"after order by", "select * from t order by ", editor.PositionColumn},
		{"after returning", "update t set a = 1 returning ", editor.PositionColumn},
		{"after a comma", "select a, ", editor.PositionColumn},
		{"after an opening bracket", "select count(", editor.PositionColumn},
		{"after an equals sign", "select * from t where a = ", editor.PositionColumn},
		{"after a star, which needs a from clause", "select * ", editor.PositionNone},
		{"at the very start", "", editor.PositionNone},
	} {
		t.Run(held.name, func(t *testing.T) {
			got := editor.ResolveNamePosition(held.sql, len(held.sql))
			if got != held.want {
				t.Errorf("ResolveNamePosition(%q) = %q, want %q", held.sql, got, held.want)
			}
		})
	}
}
