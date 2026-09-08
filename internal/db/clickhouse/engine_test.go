package clickhouse

import (
	"testing"

	"github.com/turanmahmudov/masume/internal/core"
	"github.com/turanmahmudov/masume/internal/query"
	"github.com/turanmahmudov/masume/internal/query/build"
)

// A name is quoted with backticks, and a backtick inside a name is doubled, or the name
// would end early and the rest of it would be read as SQL.
func TestQuoteIdentifierWritesBackticks(t *testing.T) {
	for _, held := range []struct {
		name string
		want string
	}{
		{"orders", "`orders`"},
		{"order", "`order`"},
		{"a`b", "`a``b`"},
	} {
		if written := Dialect.QuoteIdentifier(held.name); written != held.want {
			t.Errorf("%q quotes as %q, wanted %q", held.name, written, held.want)
		}
	}
}

// The server numbers no placeholder, so every one of them is the same mark and the order of
// the values is what lines them up.
func TestBuildPlaceholderWritesOneMark(t *testing.T) {
	bound := query.NewBoundValues(Dialect, 1)
	if first, second := bound.Bind("ada"), bound.Bind("grace"); first != "?" || second != "?" {
		t.Errorf("the placeholders read %q and %q, wanted a mark each", first, second)
	}
}

// A page is taken with the trailing window of standard SQL.
func TestBuildPageWindowWritesLimitAndOffset(t *testing.T) {
	if written := Dialect.BuildPageWindow(10, 0); written != "limit 10" {
		t.Errorf("the first page writes %q", written)
	}
	if written := Dialect.BuildPageWindow(10, 20); written != "limit 10 offset 20" {
		t.Errorf("a later page writes %q", written)
	}
}

// A write of one row is a mutation of the table, which is the only form the server takes.
func TestBuildUpdateRowWritesAMutation(t *testing.T) {
	written := Dialect.BuildUpdateRow("`shop`.`orders`", "`total` = ?", "`id` = ?")
	want := "alter table `shop`.`orders` update `total` = ? where `id` = ?"
	if written != want {
		t.Errorf("the write reads %q, wanted %q", written, want)
	}
}

// A new table needs an engine and the order it keeps its rows in.
func TestGenerateCreateTableNamesTheEngine(t *testing.T) {
	written := build.GenerateCreateTable("shop", Dialect)
	want := "create table `shop`.`new_table` (\n" +
		"    id UInt64,\n    name String not null\n)\n" +
		"engine = MergeTree\norder by id;"
	if written != want {
		t.Errorf("the table reads as %q, wanted %q", written, want)
	}
}

// A function of the user belongs to the server and not to one database, so a drop names it
// alone.
func TestBuildDropRoutineNamesTheFunctionAlone(t *testing.T) {
	if written := Dialect.BuildDropRoutine("shop", "twice", "twice"); written !=
		"drop function `twice`;" {
		t.Errorf("a function drops as %q", written)
	}
}

// A rename of a relation is a statement of its own on this server.
func TestBuildRenameTableWritesRenameTable(t *testing.T) {
	written := Dialect.BuildRenameTable(
		query.QualifiedName{Schema: "shop", Name: "orders"}, "orders_renamed")
	want := "rename table `shop`.`orders`\n  to `shop`.`orders_renamed`;"
	if written != want {
		t.Errorf("the rename reads %q, wanted %q", written, want)
	}
}

// An imported column takes a type this server stores.
func TestBuildColumnTypeWritesTheTypesOfThisServer(t *testing.T) {
	for _, held := range []struct {
		kind core.ColumnKind
		want string
	}{
		{core.KindText, "String"},
		{core.KindInteger, "Int64"},
		{core.KindBoolean, "Bool"},
		{core.KindTimestamp, "DateTime64(3)"},
	} {
		if written := Dialect.BuildColumnType(held.kind); written != held.want {
			t.Errorf("%q writes as %q, wanted %q", held.kind, written, held.want)
		}
	}
}
