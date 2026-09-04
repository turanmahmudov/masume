package db

import (
	"strings"
	"testing"
	"time"

	"github.com/turanmahmudov/masume/internal/cfg"
	"github.com/turanmahmudov/masume/internal/query/statement"
	"github.com/turanmahmudov/masume/internal/query/syntax"
)

// A read asks for one row more than the limit, and the extra row is what tells a full page
// from the last one. It is dropped before the rows reach the grid.
func TestBuildCappedResultDropsTheExtraRowAndReportsIt(t *testing.T) {
	rows := func(count int) [][]any {
		built := make([][]any, 0, count)
		for at := range count {
			built = append(built, []any{at})
		}
		return built
	}

	for _, held := range []struct {
		name      string
		read      CappedRead
		wantRows  int
		truncated bool
	}{
		{"a page that is not full", CappedRead{Rows: rows(3), RowLimit: 10}, 3, false},
		{"a page filled exactly", CappedRead{Rows: rows(10), RowLimit: 10}, 10, false},
		{"a page with the extra row", CappedRead{Rows: rows(11), RowLimit: 10}, 10, true},
		{"no rows at all", CappedRead{Rows: nil, RowLimit: 10}, 0, false},
		{"a limit of one", CappedRead{Rows: rows(2), RowLimit: 1}, 1, true},
	} {
		t.Run(held.name, func(t *testing.T) {
			answered := BuildCappedResult(held.read)
			if len(answered.Rows) != held.wantRows {
				t.Errorf("the result holds %d rows, wanted %d", len(answered.Rows), held.wantRows)
			}
			if answered.Truncated != held.truncated {
				t.Errorf("truncated reads %v, wanted %v", answered.Truncated, held.truncated)
			}
		})
	}
}

func TestBuildCappedResultKeepsWhatTheStatementReported(t *testing.T) {
	answered := BuildCappedResult(CappedRead{
		Rows: [][]any{{1}}, RowLimit: 10, Command: "UPDATE",
		Elapsed: 5 * time.Millisecond, Affected: 7, HasAffected: true,
	})
	if answered.Command != "UPDATE" {
		t.Errorf("the command reads %q", answered.Command)
	}
	if !answered.HasAffected || answered.Affected != 7 {
		t.Errorf("the count of changed rows reads %d", answered.Affected)
	}
	if answered.Elapsed != 5*time.Millisecond {
		t.Errorf("the time reads %v", answered.Elapsed)
	}
}

func TestReadOverscanRowLimitAsksForOneMore(t *testing.T) {
	for _, held := range []struct{ limit, want int }{{1, 2}, {10, 11}, {200, 201}} {
		if answered := ReadOverscanRowLimit(held.limit); answered != held.want {
			t.Errorf("a limit of %d asks for %d, wanted %d", held.limit, answered, held.want)
		}
	}
}

// A catalog value arrives in whatever shape the driver chose, and the tree draws text.
func TestReadCatalogTextReadsEveryShapeADriverReturns(t *testing.T) {
	for _, held := range []struct {
		name  string
		value any
		want  string
	}{
		{"nothing", nil, ""},
		{"a text", "orders", "orders"},
		{"bytes", []byte("orders"), "orders"},
		{"empty bytes", []byte{}, ""},

		// A PostgreSQL `"char"` column arrives as the code of one byte, and reads as that
		// letter: 114 is the `r` that marks an ordinary relation.
		{"a char column", int32(114), "r"},
		{"a char column holding v", int32(118), "v"},

		// A wider number is a count, and writing it as a rune would answer a character
		// nobody can see.
		{"a count", int32(4096), "4096"},
		{"a negative count", int32(-7), "-7"},
		{"zero", int32(0), "0"},

		{"a shape nothing reads", 12.5, ""},
	} {
		t.Run(held.name, func(t *testing.T) {
			if answered := ReadCatalogText(held.value); answered != held.want {
				t.Errorf("%v reads as %q, wanted %q", held.value, answered, held.want)
			}
		})
	}
}

func TestReadNonNegativeCountNeverAnswersBelowZero(t *testing.T) {
	for _, held := range []struct {
		value any
		want  int64
	}{
		{nil, 0},
		{int64(42), 42},
		{int64(-1), 0},
		{int32(7), 7},
		{"not a number", 0},
	} {
		if answered := ReadNonNegativeCount(held.value); answered != held.want {
			t.Errorf("%v counts as %d, wanted %d", held.value, answered, held.want)
		}
	}
}

// A batch takes one command at a time through a cursor, so a buffer of several is refused
// before it reaches the server.
func TestHoldsSeveralCommandsReadsABuffer(t *testing.T) {
	for _, held := range []struct {
		sql  string
		want bool
	}{
		{"select 1", false},
		{"select 1;", false},
		{"  select 1 ;  ", false},
		{"select 1; select 2", true},
		{"select 1;\nselect 2;", true},
		// A semicolon inside a string or a comment does not end a statement.
		{"select ';'", false},
		{"select 1 -- ; not a statement", false},
	} {
		if answered := HoldsSeveralCommands(held.sql, syntax.FlavourStandard); answered != held.want {
			t.Errorf("%q holds several = %v, wanted %v", held.sql, answered, held.want)
		}
	}
}

func TestRefuseSeveralCommandsAnswersOnlyForABatch(t *testing.T) {
	if err := RefuseSeveralCommands("select 1", syntax.FlavourStandard); err != nil {
		t.Errorf("one statement was refused: %v", err)
	}
	err := RefuseSeveralCommands("select 1; select 2", syntax.FlavourStandard)
	if err == nil {
		t.Fatal("a batch was not refused")
	}
	if DescribeError(err) == "" {
		t.Error("the refusal is described as an empty text")
	}
}

func TestRefuseSeveralPlansAnswersOnlyForABatch(t *testing.T) {
	if err := RefuseSeveralPlans("select 1", syntax.FlavourStandard); err != nil {
		t.Errorf("one statement was refused: %v", err)
	}
	err := RefuseSeveralPlans("select 1; delete from orders", syntax.FlavourStandard)
	if err == nil {
		t.Fatal("a batch was not refused")
	}
	if !strings.Contains(DescribeError(err), "one statement") {
		t.Errorf("the refusal reads %q", DescribeError(err))
	}
}

// The question the user answers before a write depends on what the profile asks for and on
// what the statement does. `off` asks nothing, and a read is never confirmed.
func TestNeedsConfirmationFollowsTheModeAndTheRisk(t *testing.T) {
	for _, held := range []struct {
		mode cfg.ConfirmWrites
		risk statement.WriteRisk
		want bool
	}{
		{cfg.ConfirmOff, statement.RiskEveryRow, false},
		{cfg.ConfirmOff, statement.RiskDelete, false},

		{cfg.ConfirmWrite, statement.RiskNone, false},
		{cfg.ConfirmWrite, statement.RiskWrite, true},
		{cfg.ConfirmWrite, statement.RiskDelete, true},
		{cfg.ConfirmWrite, statement.RiskEveryRow, true},

		// `destructive` lets an ordinary write through and asks about the rest.
		{cfg.ConfirmDelete, statement.RiskNone, false},
		{cfg.ConfirmDelete, statement.RiskWrite, false},
		{cfg.ConfirmDelete, statement.RiskDelete, true},
		{cfg.ConfirmDelete, statement.RiskEveryRow, true},
	} {
		if answered := NeedsConfirmation(held.mode, held.risk); answered != held.want {
			t.Errorf("mode %q with risk %q asks = %v, wanted %v",
				held.mode, held.risk, answered, held.want)
		}
	}
}

// An edit reaches a relation by the name the statement wrote. Without a schema the name has
// to be unique, or the edit could be written to the wrong relation.
func TestFindTableByNameRefusesAnAmbiguousName(t *testing.T) {
	tables := []TableRef{
		{Schema: "public", Name: "orders"},
		{Schema: "archive", Name: "orders"},
		{Schema: "public", Name: "customers"},
	}

	for _, held := range []struct {
		name       string
		source     statement.SelectSource
		wantSchema string
		wantFound  bool
	}{
		{"a name held once", statement.SelectSource{Name: "customers"}, "public", true},
		{"a name held twice", statement.SelectSource{Name: "orders"}, "public", true},
		{"a name with its schema", statement.SelectSource{
			Name: "orders", Schema: "archive", HasSchema: true}, "archive", true},
		{"a schema that holds no such name", statement.SelectSource{
			Name: "customers", Schema: "archive", HasSchema: true}, "", false},
		{"a name nothing holds", statement.SelectSource{Name: "nothing"}, "", false},
		// The name is matched without regard to case, as a server resolves it.
		{"a name in capitals", statement.SelectSource{Name: "CUSTOMERS"}, "public", true},
	} {
		t.Run(held.name, func(t *testing.T) {
			found, is := FindTableByName(tables, held.source, "public")
			if is != held.wantFound {
				t.Fatalf("found = %v, wanted %v", is, held.wantFound)
			}
			if is && found.Schema != held.wantSchema {
				t.Errorf("the schema reads %q, wanted %q", found.Schema, held.wantSchema)
			}
		})
	}
}

// PostgreSQL holds `Orders` and `orders` in one schema, because a quoted name keeps its
// case. A statement that read one of them must be written back through that one, and a
// match that ignored the case would write the rows of one into the other.
func TestFindTableByNameTellsTwoNamesOfTheSameLettersApart(t *testing.T) {
	tables := []TableRef{
		{Schema: "public", Name: "Orders"},
		{Schema: "public", Name: "orders"},
		{Schema: "public", Name: "customers"},
	}

	for _, held := range []struct {
		name      string
		source    statement.SelectSource
		wantName  string
		wantFound bool
	}{
		{"the quoted one", statement.SelectSource{Name: "Orders"}, "Orders", true},
		{"the plain one", statement.SelectSource{Name: "orders"}, "orders", true},
		{"the quoted one with its schema", statement.SelectSource{
			Name: "Orders", Schema: "public", HasSchema: true}, "Orders", true},
		// A third case matches both and names neither, so the client refuses rather than
		// write the rows of one relation into the other.
		{"a case that matches both", statement.SelectSource{Name: "ORDERS"}, "", false},
		// A name held once still resolves whatever the case, as a server resolves it.
		{"a name held once, in capitals",
			statement.SelectSource{Name: "CUSTOMERS"}, "customers", true},
	} {
		t.Run(held.name, func(t *testing.T) {
			found, is := FindTableByName(tables, held.source, "public")
			if is != held.wantFound {
				t.Fatalf("found = %v, wanted %v", is, held.wantFound)
			}
			if is && found.Name != held.wantName {
				t.Errorf("the name reads %q, wanted %q", found.Name, held.wantName)
			}
		})
	}
}

// Without a default schema to fall back on, a name held twice cannot be resolved.
func TestFindTableByNameRefusesTwoSchemasWithNoDefault(t *testing.T) {
	tables := []TableRef{
		{Schema: "public", Name: "orders"},
		{Schema: "archive", Name: "orders"},
	}
	if _, is := FindTableByName(tables, statement.SelectSource{Name: "orders"}, "shop"); is {
		t.Error("a name held by two schemas resolved with no default among them")
	}
}

func TestIsWriteCommandNamesTheCommandsThatChangeRows(t *testing.T) {
	for _, command := range []string{"INSERT", "update", "delete", "replace", "merge"} {
		if !IsWriteCommand(command) {
			t.Errorf("%q is not a write", command)
		}
	}
	if IsWriteCommand("select") || IsWriteCommand("") {
		t.Error("a read was taken as a write")
	}
}

func TestJoinQuotedJoinsEveryName(t *testing.T) {
	held := JoinQuoted([]string{"id", "name"}, func(name string) string { return "'" + name + "'" })
	if held != "'id', 'name'" {
		t.Errorf("the list reads %q", held)
	}
}

func TestIsBinaryColumnTypeReadsTheDriverType(t *testing.T) {
	if !IsBinaryColumnType("BYTEA") || !IsBinaryColumnType(" blob ") {
		t.Error("a binary type was read as text")
	}
	if IsBinaryColumnType("varchar") || IsBinaryColumnType("") {
		t.Error("a text type was read as bytes")
	}
}

func TestReadAnyTextWritesEveryShape(t *testing.T) {
	if ReadAnyText("public") != "public" {
		t.Error("a string was not kept")
	}
	if ReadAnyText([]byte("orders")) != "orders" {
		t.Error("bytes were not read as text")
	}
	if ReadAnyText(nil) != "" {
		t.Error("nil was not empty")
	}
	if ReadAnyText(int32('c')) != "c" {
		t.Errorf("a postgres char reads %q", ReadAnyText(int32('c')))
	}
	if ReadAnyText(int32(200)) != "200" {
		t.Errorf("a count reads %q", ReadAnyText(int32(200)))
	}
}

func TestBuildMissingDefinitionNamesTheObject(t *testing.T) {
	held := BuildMissingDefinition("orders")
	if len(held) != 1 || !strings.Contains(held[0], "orders") {
		t.Errorf("the definition reads %v", held)
	}
}

func TestFailUnreadablePlanMarksADatabaseError(t *testing.T) {
	err := FailUnreadablePlan()
	if err == nil {
		t.Fatal("no error")
	}
	if DescribeError(err) == "" {
		t.Error("the plan error has no message")
	}
}

func TestReadChangeGuardAndStatementRefuseAForeignPayload(t *testing.T) {
	if _, guarded, err := ReadChangeGuard(Change{}); err != nil || guarded {
		t.Errorf("an unguarded change answered %v, %v", guarded, err)
	}
	if _, _, err := ReadChangeGuard(Change{Guard: "not a statement"}); err == nil {
		t.Error("a guard built elsewhere was accepted")
	}
	guard, guarded, err := ReadChangeGuard(Change{Guard: BoundStatement{SQL: "select 1"}})
	if err != nil || !guarded || guard.SQL != "select 1" {
		t.Errorf("a bound guard answered %+v, %v, %v", guard, guarded, err)
	}
	if _, err := ReadChangeStatement(Change{Payload: "redis"}); err == nil {
		t.Error("a payload built elsewhere was accepted")
	}
	held, err := ReadChangeStatement(Change{Payload: BoundStatement{SQL: "update t set x = 1"}})
	if err != nil || held.SQL != "update t set x = 1" {
		t.Errorf("a bound statement answered %+v, %v", held, err)
	}
}

// A buffer of several statements is answered by the last of them, so that is the statement
// the result is named after.
func TestReadLastCommandWordNamesTheLastStatement(t *testing.T) {
	for _, held := range []struct {
		name   string
		buffer string
		want   string
	}{
		{"one statement", "select 1", "select"},
		{"a write before a read", "insert into orders values (1); select * from orders", "select"},
		{"a read before a write", "select * from orders; insert into orders values (1)", "insert"},
		{"a trailing separator", "update orders set paid = 1;", "update"},
		{"nothing written", "   ", ""},
	} {
		t.Run(held.name, func(t *testing.T) {
			if command := ReadLastCommandWord(
				held.buffer, syntax.FlavourStandard); command != held.want {
				t.Errorf("the command reads %q, wanted %q", command, held.want)
			}
		})
	}
}

func TestHoldsReturningClauseReadsOnlyTheTopLevel(t *testing.T) {
	for _, held := range []struct {
		name string
		sql  string
		want bool
	}{
		{"a plain write", "update orders set paid = 1 where id = 3", false},
		{"a returning write", "delete from orders where id = 3 returning id", true},
		{"returning in upper case", "DELETE FROM orders WHERE id = 3 RETURNING id", true},
		{"returning inside brackets", "update orders set note = (select 'returning')", false},
		{"returning as a quoted name", "update orders set \"returning\" = 1", false},
	} {
		t.Run(held.name, func(t *testing.T) {
			if HoldsReturningClause(held.sql, syntax.FlavourStandard) != held.want {
				t.Errorf("the clause reads %v, wanted %v", !held.want, held.want)
			}
		})
	}
}

func TestReadLastStatementKeepsTheWholeStatement(t *testing.T) {
	buffer := "select * from orders; update orders set paid = 1 returning id"
	last := ReadLastStatement(buffer, syntax.FlavourStandard)
	if last != " update orders set paid = 1 returning id" &&
		last != "update orders set paid = 1 returning id" {
		t.Errorf("the last statement reads %q", last)
	}
}

// A batch is handed over as soon as it is full, and what is left over goes with the flush,
// so an export never holds the whole relation.
func TestRowBatcherHandsOverAFullBatchAndThenTheRest(t *testing.T) {
	columns := []ResultColumn{{Name: "id"}}
	handed := [][][]any{}
	batcher := NewRowBatcher(2, func(rows [][]any, given []ResultColumn) error {
		if len(given) != 1 || given[0].Name != "id" {
			t.Errorf("the batch carried %v", given)
		}
		handed = append(handed, rows)
		return nil
	})

	for at := range 3 {
		if err := batcher.AddRow([]any{at}, columns); err != nil {
			t.Fatalf("row %d answered %v", at, err)
		}
	}
	if len(handed) != 1 || len(handed[0]) != 2 {
		t.Fatalf("the full batch was handed over as %v", handed)
	}
	if err := batcher.FlushRows(columns); err != nil {
		t.Fatalf("the flush answered %v", err)
	}
	if len(handed) != 2 || len(handed[1]) != 1 {
		t.Fatalf("the rest was handed over as %v", handed)
	}
	if batcher.CountRows() != 3 {
		t.Errorf("the batcher counted %d rows, wanted 3", batcher.CountRows())
	}
	if err := batcher.FlushRows(columns); err != nil || len(handed) != 2 {
		t.Errorf("a second flush handed over %v, %v", handed, err)
	}
}

// A result with no rows still has columns, and a reader that writes a format needs them for
// its header. Asking the server again would run the statement twice, so the batcher reports
// them with a batch of no rows.
func TestRowBatcherReportsTheColumnsOfAResultWithNoRows(t *testing.T) {
	columns := []ResultColumn{{Name: "id"}}
	handed := [][][]any{}
	given := [][]ResultColumn{}
	batcher := NewRowBatcher(2, func(rows [][]any, held []ResultColumn) error {
		handed = append(handed, rows)
		given = append(given, held)
		return nil
	})

	if err := batcher.FlushRows(columns); err != nil {
		t.Fatalf("the flush answered %v", err)
	}
	if len(handed) != 1 || len(handed[0]) != 0 {
		t.Fatalf("the columns were reported as %v, wanted one batch of no rows", handed)
	}
	if len(given[0]) != 1 || given[0][0].Name != "id" {
		t.Errorf("the batch carried %v, wanted the columns", given[0])
	}
	if batcher.CountRows() != 0 {
		t.Errorf("the batcher counted %d rows, wanted none", batcher.CountRows())
	}
	// The report is made one time, so a reader does not write two headers.
	if err := batcher.FlushRows(columns); err != nil || len(handed) != 1 {
		t.Errorf("a second flush handed over %v, %v", handed, err)
	}
}

// A statement with no result set has no columns to report, so nothing is handed over: a
// reader that wrote a header for it would write a document of nothing.
func TestRowBatcherReportsNothingForAStatementWithNoResultSet(t *testing.T) {
	handed := 0
	batcher := NewRowBatcher(2, func([][]any, []ResultColumn) error {
		handed++
		return nil
	})

	if err := batcher.FlushRows(nil); err != nil {
		t.Fatalf("the flush answered %v", err)
	}
	if handed != 0 {
		t.Errorf("the batcher handed over %d batches, wanted none", handed)
	}
}

// A result whose rows were handed over must not report its columns again after them.
func TestRowBatcherReportsNoColumnsAfterABatchOfRows(t *testing.T) {
	columns := []ResultColumn{{Name: "id"}}
	handed := [][][]any{}
	batcher := NewRowBatcher(2, func(rows [][]any, _ []ResultColumn) error {
		handed = append(handed, rows)
		return nil
	})

	if err := batcher.AddRow([]any{1}, columns); err != nil {
		t.Fatalf("the row answered %v", err)
	}
	if err := batcher.FlushRows(columns); err != nil {
		t.Fatalf("the flush answered %v", err)
	}
	if err := batcher.FlushRows(columns); err != nil {
		t.Fatalf("the second flush answered %v", err)
	}
	if len(handed) != 1 || len(handed[0]) != 1 {
		t.Errorf("the batcher handed over %v, wanted the one row alone", handed)
	}
}

// A size below one would never fill, so the batcher takes one row at a time instead.
func TestRowBatcherTakesOneRowAtATimeForASizeBelowOne(t *testing.T) {
	handed := 0
	batcher := NewRowBatcher(0, func([][]any, []ResultColumn) error {
		handed++
		return nil
	})
	if err := batcher.AddRow([]any{1}, nil); err != nil {
		t.Fatalf("the row answered %v", err)
	}
	if handed != 1 {
		t.Errorf("the batch was handed over %d times, wanted 1", handed)
	}
}
