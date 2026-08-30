package build_test

import (
	"strings"
	"testing"

	"github.com/turanmahmudov/masume/internal/core"
	"github.com/turanmahmudov/masume/internal/db/postgres"
	"github.com/turanmahmudov/masume/internal/query"
	"github.com/turanmahmudov/masume/internal/query/build"
)

// buildOrdersTarget answers a write against a small relation with one key column.
func buildOrdersTarget(keys ...string) build.WriteTarget {
	return build.WriteTarget{
		Table: query.QualifiedName{Schema: "public", Name: "orders"},
		Columns: []query.ResultColumn{
			{Name: "id", DataType: "integer"},
			{Name: "customer", DataType: "text"},
			{Name: "total", DataType: "numeric"},
		},
		KeyColumns: keys,
		Dialect:    postgres.Dialect,
	}
}

// A value the user typed must be bound, never written into the statement, or a value holding
// a quote would change what the statement does.
func TestBuildUpdateStatementBindsEveryValue(t *testing.T) {
	target := buildOrdersTarget("id")
	row := []any{int64(7), "ada", "12.50"}

	answered, err := build.BuildUpdateStatement(target, row, []build.CellAssignment{
		{ColumnIndex: 1, Value: core.CellValue{Kind: core.CellText, Text: "grace'; drop table orders --"}},
	})
	if err != nil {
		t.Fatalf("the update answered %v", err)
	}

	// The text the user typed appears nowhere in the statement.
	if strings.Contains(answered.SQL, "drop table") {
		t.Errorf("the value was written into the statement:\n%s", answered.SQL)
	}
	if len(answered.Params) != 2 {
		t.Fatalf("the update binds %d values, wanted the new value and the key", len(answered.Params))
	}
	if answered.Params[0] != "grace'; drop table orders --" {
		t.Errorf("the first bound value reads %v", answered.Params[0])
	}
	// The key of the row is bound too, so the update reaches one row.
	if !strings.Contains(strings.ToLower(answered.SQL), "where") {
		t.Errorf("the update carries no where clause:\n%s", answered.SQL)
	}
}

// Two cells of one row are one statement, so both land or neither does.
func TestBuildUpdateStatementWritesTwoCellsAsOneStatement(t *testing.T) {
	target := buildOrdersTarget("id")
	row := []any{int64(7), "ada", "12.50"}

	answered, err := build.BuildUpdateStatement(target, row, []build.CellAssignment{
		{ColumnIndex: 1, Value: core.CellValue{Kind: core.CellText, Text: "grace"}},
		{ColumnIndex: 2, Value: core.CellValue{Kind: core.CellText, Text: "99.00"}},
	})
	if err != nil {
		t.Fatalf("the update answered %v", err)
	}
	if len(answered.Params) != 3 {
		t.Errorf("the update binds %d values, wanted two new ones and the key", len(answered.Params))
	}
	written := strings.ToLower(answered.SQL)
	if strings.Count(written, "update ") != 1 {
		t.Errorf("two cells became more than one statement:\n%s", answered.SQL)
	}
}

// A null is bound as a value with nothing in it, which is how a driver sets a column to null.
func TestBuildUpdateStatementBindsANull(t *testing.T) {
	target := buildOrdersTarget("id")
	row := []any{int64(7), "ada", "12.50"}

	answered, err := build.BuildUpdateStatement(target, row, []build.CellAssignment{
		{ColumnIndex: 1, Value: core.CellValue{Kind: core.CellNull}},
	})
	if err != nil {
		t.Fatalf("the update answered %v", err)
	}
	if len(answered.Params) != 2 {
		t.Fatalf("the update binds %d values, wanted the null and the key",
			len(answered.Params))
	}
	if answered.Params[0] != nil {
		t.Errorf("the bound value reads %v, wanted nothing", answered.Params[0])
	}
}

// DEFAULT cannot be bound: a driver would send the word as text and the column would hold
// that text instead of what the server chose. So it is written into the statement.
func TestBuildUpdateStatementWritesDefaultIntoTheStatement(t *testing.T) {
	target := buildOrdersTarget("id")
	row := []any{int64(7), "ada", "12.50"}

	answered, err := build.BuildUpdateStatement(target, row, []build.CellAssignment{
		{ColumnIndex: 1, Value: core.CellValue{Kind: core.CellDefault}},
	})
	if err != nil {
		t.Fatalf("the update answered %v", err)
	}
	if !strings.Contains(strings.ToLower(answered.SQL), "default") {
		t.Errorf("the statement does not hold the word default:\n%s", answered.SQL)
	}
	// Only the key is bound, because the word is not a value.
	if len(answered.Params) != 1 {
		t.Errorf("the statement binds %d values, wanted the key alone", len(answered.Params))
	}
}

// An empty text is a value, and must not be turned into a null.
func TestBuildUpdateStatementKeepsAnEmptyTextApartFromNull(t *testing.T) {
	target := buildOrdersTarget("id")
	row := []any{int64(7), "ada", "12.50"}

	answered, err := build.BuildUpdateStatement(target, row, []build.CellAssignment{
		{ColumnIndex: 1, Value: core.CellValue{Kind: core.CellEmpty}},
	})
	if err != nil {
		t.Fatalf("the update answered %v", err)
	}
	if strings.Contains(strings.ToLower(answered.SQL), "null") {
		t.Errorf("an empty text was written as null:\n%s", answered.SQL)
	}
	if len(answered.Params) != 2 || answered.Params[0] != "" {
		t.Errorf("the bound values read %v, wanted the empty text and the key", answered.Params)
	}
}

func TestBuildUpdateStatementRefusesNothingToAssign(t *testing.T) {
	target := buildOrdersTarget("id")
	if _, err := build.BuildUpdateStatement(target, []any{int64(1)}, nil); err == nil {
		t.Error("an update with nothing assigned was built")
	}
}

// Without a key column the whole row identifies itself, so every column has to be matched or
// the write could reach a row the user never saw.
func TestBuildUpdateStatementMatchesEveryColumnWithNoKey(t *testing.T) {
	target := buildOrdersTarget()
	row := []any{int64(7), "ada", "12.50"}

	answered, err := build.BuildUpdateStatement(target, row, []build.CellAssignment{
		{ColumnIndex: 1, Value: core.CellValue{Kind: core.CellText, Text: "grace"}},
	})
	if err != nil {
		t.Fatalf("the update answered %v", err)
	}
	// One new value, and every column of the row matched.
	if len(answered.Params) != 1+len(target.Columns) {
		t.Errorf("the update binds %d values, wanted one and the whole row",
			len(answered.Params))
	}
}

func TestBuildInsertStatementBindsEveryValueAndOrdersTheColumns(t *testing.T) {
	answered, err := build.BuildInsertStatement(
		query.QualifiedName{Schema: "public", Name: "orders"},
		map[string]any{"customer": "ada", "total": "12.50"},
		postgres.Dialect)
	if err != nil {
		t.Fatalf("the insert answered %v", err)
	}
	if len(answered.Params) != 2 {
		t.Fatalf("the insert binds %d values, wanted 2", len(answered.Params))
	}
	// The columns are named in a settled order, so the same row always writes the same
	// statement and the bound values line up with it.
	written := answered.SQL
	if strings.Index(written, "customer") > strings.Index(written, "total") {
		t.Errorf("the columns are not in order:\n%s", written)
	}
	if answered.Params[0] != "ada" {
		t.Errorf("the first bound value reads %v, wanted the one of the first column",
			answered.Params[0])
	}
}

func TestBuildInsertStatementRefusesARowWithNoValue(t *testing.T) {
	_, err := build.BuildInsertStatement(
		query.QualifiedName{Name: "orders"}, map[string]any{}, postgres.Dialect)
	if err == nil {
		t.Error("an insert with no column was built")
	}
}

// A delete of many rows is split, because a server takes only so many bound values in one
// statement.
func TestBuildDeleteStatementsSplitOnTheParameterCap(t *testing.T) {
	target := buildOrdersTarget("id")
	rows := make([][]any, 0, 10)
	for at := range 10 {
		rows = append(rows, []any{int64(at), "ada", "1"})
	}

	// A cap of four bound values takes four rows per statement, so ten rows need three.
	answered, err := build.BuildDeleteStatements(target, rows, 4)
	if err != nil {
		t.Fatalf("the delete answered %v", err)
	}
	if len(answered) != 3 {
		t.Fatalf("ten rows became %d statements, wanted 3", len(answered))
	}
	total := 0
	for _, statement := range answered {
		if len(statement.Params) > 4 {
			t.Errorf("a statement binds %d values, over the cap of 4", len(statement.Params))
		}
		total += len(statement.Params)
	}
	if total != 10 {
		t.Errorf("the statements bind %d values in all, wanted one per row", total)
	}
}

func TestBuildDeleteStatementsAnswersNothingForNoRows(t *testing.T) {
	answered, err := build.BuildDeleteStatements(buildOrdersTarget("id"), nil, 0)
	if err != nil {
		t.Fatalf("the delete answered %v", err)
	}
	if len(answered) != 0 {
		t.Errorf("no rows became %d statements", len(answered))
	}
}

// The order the staged work runs in matters: a row inserted and then edited has to exist
// before the edit, and a row deleted must not be updated first.
func TestBuildChangeStatementsRunsInsertsThenUpdatesThenDeletes(t *testing.T) {
	target := buildOrdersTarget("id")
	rows := [][]any{
		{int64(1), "ada", "1"},
		{int64(2), "grace", "2"},
	}

	pending := core.NewPendingChanges()
	pending.Inserts = append(pending.Inserts, map[string]any{"customer": "alan"})
	pending.Edits[core.BuildEditKey(0, 1)] = core.CellEdit{
		RowIndex: 0, ColumnIndex: 1,
		Value: core.CellValue{Kind: core.CellText, Text: "ada2"},
	}
	pending.DeletedRows[1] = true

	answered, err := build.BuildChangeStatements(target, rows, pending)
	if err != nil {
		t.Fatalf("the changes answered %v", err)
	}
	if len(answered) != 3 {
		t.Fatalf("the staged work became %d statements, wanted 3", len(answered))
	}
	for at, wanted := range []string{"insert", "update", "delete"} {
		if !strings.HasPrefix(strings.ToLower(answered[at].Statement.SQL), wanted) {
			t.Errorf("statement %d is %q, wanted it to open with %q",
				at, answered[at].Statement.SQL, wanted)
		}
	}
}

// A row marked for deletion skips its own updates: writing to a row that is about to go is
// work the server would only undo.
func TestBuildChangeStatementsSkipsAnUpdateToADeletedRow(t *testing.T) {
	target := buildOrdersTarget("id")
	rows := [][]any{{int64(1), "ada", "1"}}

	pending := core.NewPendingChanges()
	pending.Edits[core.BuildEditKey(0, 1)] = core.CellEdit{
		RowIndex: 0, ColumnIndex: 1,
		Value: core.CellValue{Kind: core.CellText, Text: "grace"},
	}
	pending.DeletedRows[0] = true

	answered, err := build.BuildChangeStatements(target, rows, pending)
	if err != nil {
		t.Fatalf("the changes answered %v", err)
	}
	if len(answered) != 1 {
		t.Fatalf("the staged work became %d statements, wanted the delete alone", len(answered))
	}
	if !strings.HasPrefix(strings.ToLower(answered[0].Statement.SQL), "delete") {
		t.Errorf("the one statement is %q, wanted the delete", answered[0].Statement.SQL)
	}
}

// A key column is what makes an edit reach one row. Without one the client falls back to the
// whole row, and the columns it can match on must be the ones a server can compare.
func TestFindIdentityColumnsSkipsWhatCannotBeCompared(t *testing.T) {
	columns := []query.ResultColumn{
		{Name: "id", DataType: "integer"},
		{Name: "customer", DataType: "text"},
		{Name: "notes", DataType: "json"},
	}
	found := build.FindIdentityColumns(columns, postgres.Dialect)
	for _, column := range found {
		if column.DataType == "json" {
			t.Error("a json column was taken as part of the identity of a row")
		}
	}
	if len(found) == 0 {
		t.Error("no column was taken as the identity of a row")
	}
}

func TestBuildRowCountStatementCountsWhatTheKeyMatches(t *testing.T) {
	// A table with no primary key can hold the same values twice, so the client counts
	// before it writes and refuses where a write would land on more than one row.
	target := buildOrdersTarget("id")
	answered, err := build.BuildRowCountStatement(target, []any{int64(7), "ada", "12.50"})
	if err != nil {
		t.Fatalf("the count answered %v", err)
	}
	want := `select count(*)::int8 as matched from "public"."orders" where "id" = $1`
	if answered.SQL != want {
		t.Errorf("SQL = %q, want %q", answered.SQL, want)
	}
	if len(answered.Params) != 1 || answered.Params[0] != int64(7) {
		t.Errorf("Params = %v, want [7]", answered.Params)
	}
	if answered.Description != "count rows of orders where id=7" {
		t.Errorf("Description = %q", answered.Description)
	}
}

func TestBuildRowCountStatementTakesTheWholeRowAsTheKeyWhereThereIsNone(t *testing.T) {
	target := buildOrdersTarget()
	answered, err := build.BuildRowCountStatement(target, []any{int64(7), "ada", "12.50"})
	if err != nil {
		t.Fatalf("the count answered %v", err)
	}
	if !strings.Contains(answered.SQL, `"id" = $1`) ||
		!strings.Contains(answered.SQL, `"customer" = $2`) ||
		!strings.Contains(answered.SQL, `"total" = $3`) {
		t.Errorf("SQL = %q, want every column in the predicate", answered.SQL)
	}
}

func TestBuildRowCountStatementAsksForANullWhereTheRowHoldsNoKeyValue(t *testing.T) {
	// `= null` matches nothing on any server, so a missing key value is asked for as
	// `is null` and the count answers what is really there.
	target := buildOrdersTarget("id")
	answered, err := build.BuildRowCountStatement(target, nil)
	if err != nil {
		t.Fatalf("the count answered %v", err)
	}
	if !strings.Contains(answered.SQL, `"id" is null`) {
		t.Errorf("SQL = %q, want an is null test", answered.SQL)
	}
	if len(answered.Params) != 0 {
		t.Errorf("Params = %v, want none bound", answered.Params)
	}
}

func TestBuildRowCountStatementRefusesAKeyColumnTheResultHasNot(t *testing.T) {
	target := buildOrdersTarget("missing_column")
	if _, err := build.BuildRowCountStatement(target, []any{int64(7)}); err == nil {
		t.Error("a key column outside the result was counted")
	}
}

func TestFindForeignKeyTargetAnswersTheColumnTheKeyPointsAt(t *testing.T) {
	keys := []query.ForeignKey{
		{
			Columns:      []string{"customer_id"},
			TargetSchema: "public", TargetTable: "customers",
			TargetColumns: []string{"id"},
		},
		{
			Columns:      []string{"shop_id", "sku"},
			TargetSchema: "public", TargetTable: "stock",
			TargetColumns: []string{"shop", "code"},
		},
	}
	for _, held := range []struct {
		name   string
		column string
		want   query.ForeignKeyTarget
	}{
		{"a key of one column", "customer_id",
			query.ForeignKeyTarget{Schema: "public", Table: "customers", Column: "id"}},
		{"the first column of a key of two", "shop_id",
			query.ForeignKeyTarget{Schema: "public", Table: "stock", Column: "shop"}},
		{"the second column of a key of two", "sku",
			query.ForeignKeyTarget{Schema: "public", Table: "stock", Column: "code"}},
		{"the case of the name is not read", "CUSTOMER_ID",
			query.ForeignKeyTarget{Schema: "public", Table: "customers", Column: "id"}},
	} {
		t.Run(held.name, func(t *testing.T) {
			got, found := build.FindForeignKeyTarget(keys, held.column)
			if !found {
				t.Fatalf("no target for %q", held.column)
			}
			if got != held.want {
				t.Errorf("got %v, want %v", got, held.want)
			}
		})
	}
}

func TestFindForeignKeyTargetAnswersNothingWhereNoKeyReachesTheColumn(t *testing.T) {
	keys := []query.ForeignKey{{
		Columns:      []string{"customer_id"},
		TargetSchema: "public", TargetTable: "customers",
		TargetColumns: []string{"id"},
	}}
	for _, held := range []struct {
		name   string
		keys   []query.ForeignKey
		column string
	}{
		{"a column no key names", keys, "total"},
		{"no keys at all", nil, "customer_id"},
		{"a key that names fewer targets than columns",
			[]query.ForeignKey{{Columns: []string{"a", "b"}, TargetColumns: []string{"x"}}}, "b"},
	} {
		t.Run(held.name, func(t *testing.T) {
			if _, found := build.FindForeignKeyTarget(held.keys, held.column); found {
				t.Errorf("a target was answered for %q", held.column)
			}
		})
	}
}

func TestRenderLiteralWritesEachKindOfValueAsTheServerReadsIt(t *testing.T) {
	for _, held := range []struct {
		name  string
		value any
		want  string
	}{
		{"an integer", int64(7), "7"},
		{"a small integer", int8(-3), "-3"},
		{"an unsigned integer", uint32(9), "9"},
		{"a float", 12.5, "12.5"},
		{"a 32 bit float", float32(1.5), "1.5"},
		{"a boolean", true, "true"},
		{"text", "ada", "'ada'"},
		{"text holding the quote mark", "it's", "'it''s'"},
		{"nothing", nil, "'NULL'"},
	} {
		t.Run(held.name, func(t *testing.T) {
			got := build.RenderLiteral(held.value, postgres.Dialect, "text")
			if got != held.want {
				t.Errorf("RenderLiteral(%v) = %q, want %q", held.value, got, held.want)
			}
		})
	}
}

func TestBuildFilterSQLWritesEveryTestOfAStep(t *testing.T) {
	// A null needs its own test, because `= null` matches nothing on any server.
	for _, held := range []struct {
		name string
		step core.FilterStep
		want string
	}{
		{"equals", core.FilterStep{
			Kind: core.FilterCompare, Column: "customer",
			Test: core.FilterEquals, Value: "ada",
		}, `"customer" = $1`},
		{"differs", core.FilterStep{
			Kind: core.FilterCompare, Column: "customer",
			Test: core.FilterDiffers, Value: "ada",
		}, `"customer" <> $1`},
		{"is null", core.FilterStep{
			Kind: core.FilterCompare, Column: "customer", Test: core.FilterIsNull,
		}, `"customer" is null`},
		{"is not null", core.FilterStep{
			Kind: core.FilterCompare, Column: "customer", Test: core.FilterIsNotNull,
		}, `"customer" is not null`},
		{"text the user typed", core.FilterStep{
			Kind: core.FilterRaw, Text: "total > 100",
		}, "total > 100"},
		{"a test the client does not know", core.FilterStep{
			Kind: core.FilterCompare, Column: "customer", Test: core.FilterTest("odd"),
		}, ""},
	} {
		t.Run(held.name, func(t *testing.T) {
			built := build.ComposeFilter([]core.FilterStep{held.step}, postgres.Dialect, 1)
			if built.Text != held.want {
				t.Errorf("Text = %q, want %q", built.Text, held.want)
			}
		})
	}
}
