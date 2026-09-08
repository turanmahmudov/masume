package sqlserver

import (
	"testing"

	"github.com/turanmahmudov/masume/internal/core"
	"github.com/turanmahmudov/masume/internal/query"
)

// A name is quoted with brackets, and a bracket inside a name is doubled, or the name would
// end early and the rest of it would be read as SQL.
func TestQuoteIdentifierWritesBrackets(t *testing.T) {
	for _, held := range []struct {
		name string
		want string
	}{
		{"orders", "[orders]"},
		{"order", "[order]"},
		{"a]b", "[a]]b]"},
	} {
		if written := Dialect.QuoteIdentifier(held.name); written != held.want {
			t.Errorf("%q quotes as %q, wanted %q", held.name, written, held.want)
		}
	}
}

// The server names every placeholder, counted from one, and the values line up with the
// names rather than with their order.
func TestBuildPlaceholderNamesEveryValue(t *testing.T) {
	bound := query.NewBoundValues(Dialect, 1)
	first := bound.Bind("ada")
	second := bound.Bind("grace")
	if first != "@p1" || second != "@p2" {
		t.Errorf("the placeholders read %q and %q, wanted @p1 and @p2", first, second)
	}
}

// A page after the first is taken with OFFSET and FETCH, which the server reads after a
// sort only. The first page takes no window, so a read the server refuses to sort still
// runs, and the client caps its rows as it reads them.
func TestBuildPageWindowWritesOffsetAndFetch(t *testing.T) {
	for _, held := range []struct {
		limit  int
		offset int
		want   string
	}{
		{10, 0, ""},
		{10, 20, "offset 20 rows fetch next 10 rows only"},
	} {
		if written := Dialect.BuildPageWindow(held.limit, held.offset); written != held.want {
			t.Errorf("a page of %d from %d writes %q, wanted %q",
				held.limit, held.offset, written, held.want)
		}
	}
}

// A generated read caps its rows with TOP, because a trailing window would need a sort.
func TestBuildCappedReadWritesTop(t *testing.T) {
	written := Dialect.BuildCappedRead("[dbo].[orders]", 100)
	want := "select top 100 *\n  from [dbo].[orders];"
	if written != want {
		t.Errorf("the read writes %q, wanted %q", written, want)
	}
}

// A drop of a routine names the kind the server stores it as, because DROP FUNCTION refuses
// a procedure.
func TestBuildDropRoutineNamesTheKind(t *testing.T) {
	function := Dialect.BuildDropRoutine("dbo", "double_it",
		BuildRoutineIdentity(RoutineFunction, "dbo", "double_it"))
	if function != "drop function [dbo].[double_it];" {
		t.Errorf("a function drops as %q", function)
	}
	procedure := Dialect.BuildDropRoutine("dbo", "touch_it",
		BuildRoutineIdentity(RoutineProcedure, "dbo", "touch_it"))
	if procedure != "drop procedure [dbo].[touch_it];" {
		t.Errorf("a procedure drops as %q", procedure)
	}
}

// A trigger of this server carries its own qualified name, so a drop needs no table.
func TestBuildDropTriggerNamesTheTriggerAlone(t *testing.T) {
	written := Dialect.BuildDropTrigger("dbo", "users_touch", "users")
	if written != "drop trigger [dbo].[users_touch];" {
		t.Errorf("a trigger drops as %q", written)
	}
}

// A filter of the grid compares with `=`, and these types have no equality operator, so a
// filter must not be offered on them.
func TestCanCompareTypeRefusesTheTypesWithNoEquality(t *testing.T) {
	for _, held := range []struct {
		dataType string
		compares bool
	}{
		{"nvarchar(64)", true},
		{"bigint", true},
		{"decimal(10,2)", true},
		{"uniqueidentifier", true},
		{"text", false},
		{"ntext", false},
		{"xml", false},
		{"geography", false},
	} {
		if answered := Dialect.CanCompareType(held.dataType); answered != held.compares {
			t.Errorf("%q compares %v, wanted %v", held.dataType, answered, held.compares)
		}
	}
}

// An imported column takes a type this server stores, and no other dialect writes these.
func TestBuildColumnTypeWritesTheTypesOfThisServer(t *testing.T) {
	for _, held := range []struct {
		kind core.ColumnKind
		want string
	}{
		{core.KindText, "nvarchar(max)"},
		{core.KindInteger, "bigint"},
		{core.KindBoolean, "bit"},
		{core.KindTimestamp, "datetime2"},
	} {
		if written := Dialect.BuildColumnType(held.kind); written != held.want {
			t.Errorf("%q writes as %q, wanted %q", held.kind, written, held.want)
		}
	}
}
