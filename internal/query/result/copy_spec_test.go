package result_test

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/turanmahmudov/masume/internal/db/postgres"
	"github.com/turanmahmudov/masume/internal/query"
	"github.com/turanmahmudov/masume/internal/query/result"
)

func TestBuildMarkdownWritesAHeaderARuleAndOneRowPerRow(t *testing.T) {
	got := result.BuildMarkdown(shopColumns, [][]any{
		{1, "Ada"},
		{2, "Grace"},
	})
	want := strings.Join([]string{
		"| id | customer |",
		"| --- | --- |",
		"| 1 | Ada |",
		"| 2 | Grace |",
	}, "\n")
	if got != want {
		t.Errorf("got\n%s\nwant\n%s", got, want)
	}
}

func TestBuildMarkdownKeepsAValueInsideItsOwnCell(t *testing.T) {
	// A pipe would open a cell of its own, and a newline would end the row, so both are
	// written so the table still reads as one row per row.
	got := result.BuildMarkdown(
		[]query.ResultColumn{{Name: "a|b", DataType: "text"}},
		[][]any{{"one|two"}, {"one\ntwo"}, {"one \n two"}},
	)
	want := strings.Join([]string{
		`| a\|b |`,
		"| --- |",
		`| one\|two |`,
		"| one two |",
		"| one two |",
	}, "\n")
	if got != want {
		t.Errorf("got\n%s\nwant\n%s", got, want)
	}
}

func TestBuildMarkdownWritesAHeaderAloneForAnEmptyResult(t *testing.T) {
	got := result.BuildMarkdown(shopColumns, nil)
	want := "| id | customer |\n| --- | --- |"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestBuildMarkdownWritesAShortRowAsFarAsItGoes(t *testing.T) {
	got := result.BuildMarkdown(shopColumns, [][]any{{1}})
	if want := "| id | customer |\n| --- | --- |\n| 1 |"; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestBuildInClauseWritesOneColumnAsAListForAWhere(t *testing.T) {
	got := result.BuildInClause(shopColumns, [][]any{
		{1, "Ada"},
		{2, "Grace"},
	}, 1, postgres.Dialect)
	if want := "('Ada', 'Grace')"; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestBuildInClauseWritesEachValueOnce(t *testing.T) {
	got := result.BuildInClause(shopColumns, [][]any{
		{1, "Ada"},
		{2, "Ada"},
		{3, "Grace"},
	}, 1, postgres.Dialect)
	if want := "('Ada', 'Grace')"; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestBuildInClauseLeavesOutANull(t *testing.T) {
	// IN never matches a null, so a null in the column would be a value that finds nothing.
	got := result.BuildInClause(shopColumns, [][]any{
		{1, "Ada"},
		{2, nil},
		{3, "Grace"},
	}, 1, postgres.Dialect)
	if want := "('Ada', 'Grace')"; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestBuildInClauseAnswersNothingWhereNoValueIsLeft(t *testing.T) {
	for _, held := range []struct {
		name   string
		rows   [][]any
		column int
	}{
		{"no rows", nil, 1},
		{"every value null", [][]any{{1, nil}, {2, nil}}, 1},
		{"a column below the row", [][]any{{1, "Ada"}}, -1},
		{"a column past the row", [][]any{{1, "Ada"}}, 5},
	} {
		t.Run(held.name, func(t *testing.T) {
			got := result.BuildInClause(shopColumns, held.rows, held.column, postgres.Dialect)
			if got != "" {
				t.Errorf("got %q, want nothing", got)
			}
		})
	}
}

func TestBuildInsertScriptWritesOneStatementPerRow(t *testing.T) {
	got := result.BuildInsertScript(shopColumns, [][]any{
		{1, "Ada"},
		{2, "Grace"},
	}, query.QualifiedName{Schema: "public", Name: "customers"}, postgres.Dialect)
	want := `insert into "public"."customers" ("id", "customer") values (1, 'Ada');` + "\n" +
		`insert into "public"."customers" ("id", "customer") values (2, 'Grace');` + "\n"
	if got != want {
		t.Errorf("got\n%s\nwant\n%s", got, want)
	}
}

func TestBuildInsertScriptWritesAMissingValueAsANull(t *testing.T) {
	got := result.BuildInsertScript(shopColumns, [][]any{
		{1, nil},
		{2},
	}, query.QualifiedName{Schema: "public", Name: "customers"}, postgres.Dialect)
	want := `insert into "public"."customers" ("id", "customer") values (1, null);` + "\n" +
		`insert into "public"."customers" ("id", "customer") values (2, null);` + "\n"
	if got != want {
		t.Errorf("got\n%s\nwant\n%s", got, want)
	}
}

func TestBuildInsertScriptAnswersNothingForAnEmptyResult(t *testing.T) {
	got := result.BuildInsertScript(shopColumns, nil,
		query.QualifiedName{Schema: "public", Name: "customers"}, postgres.Dialect)
	if got != "" {
		t.Errorf("got %q, want nothing", got)
	}
}

func TestBuildInsertScriptQuotesAValueSoItCannotEndTheStatement(t *testing.T) {
	got := result.BuildInsertScript(
		[]query.ResultColumn{{Name: "note", DataType: "text"}},
		[][]any{{"it's; drop table t --"}},
		query.QualifiedName{Schema: "public", Name: "notes"}, postgres.Dialect)
	want := `insert into "public"."notes" ("note") values ('it''s; drop table t --');` + "\n"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestJSONExportKeepsTheTypeOfEveryValue(t *testing.T) {
	// A number written as text would come back as text, so a reader of the file would
	// have to know the column to read it right.
	columns := []query.ResultColumn{
		{Name: "id", DataType: "integer"},
		{Name: "paid", DataType: "boolean"},
		{Name: "total", DataType: "numeric"},
		{Name: "note", DataType: "text"},
		{Name: "missing", DataType: "text"},
	}
	writer := result.CreateExportWriter(result.ExportJSON, result.CSVOptions{})
	written := writer.Begin(columns) +
		writer.WriteRows([][]any{{int64(7), true, 12.5, "ada", nil}}, columns) +
		writer.End()

	var read []map[string]any
	if err := json.Unmarshal([]byte(written), &read); err != nil {
		t.Fatalf("the file is not JSON: %v\n%s", err, written)
	}
	row := read[0]
	if row["id"] != float64(7) {
		t.Errorf("id = %#v, want a number", row["id"])
	}
	if row["paid"] != true {
		t.Errorf("paid = %#v, want a boolean", row["paid"])
	}
	if row["total"] != 12.5 {
		t.Errorf("total = %#v, want a number", row["total"])
	}
	if row["note"] != "ada" {
		t.Errorf("note = %#v, want text", row["note"])
	}
	if row["missing"] != nil {
		t.Errorf("missing = %#v, want null", row["missing"])
	}
}

func TestJSONExportWritesBytesAsTextAndNotAsAListOfNumbers(t *testing.T) {
	columns := []query.ResultColumn{{Name: "blob", DataType: "bytea"}}
	writer := result.CreateExportWriter(result.ExportJSON, result.CSVOptions{})
	written := writer.Begin(columns) +
		writer.WriteRows([][]any{{[]byte{0x00, 0xff}}}, columns) +
		writer.End()

	var read []map[string]any
	if err := json.Unmarshal([]byte(written), &read); err != nil {
		t.Fatalf("the file is not JSON: %v\n%s", err, written)
	}
	if _, isText := read[0]["blob"].(string); !isText {
		t.Errorf("blob = %#v, want text", read[0]["blob"])
	}
}

func TestJSONExportWritesARowShorterThanTheColumnsAsNulls(t *testing.T) {
	writer := result.CreateExportWriter(result.ExportJSON, result.CSVOptions{})
	written := writer.Begin(shopColumns) +
		writer.WriteRows([][]any{{int64(1)}}, shopColumns) +
		writer.End()

	var read []map[string]any
	if err := json.Unmarshal([]byte(written), &read); err != nil {
		t.Fatalf("the file is not JSON: %v\n%s", err, written)
	}
	if read[0]["customer"] != nil {
		t.Errorf("customer = %#v, want null", read[0]["customer"])
	}
}

func TestJSONExportWritesNothingForABatchOfNoRows(t *testing.T) {
	// A read answers batch after batch, and an empty one must add no comma of its own or
	// the file stops being JSON.
	writer := result.CreateExportWriter(result.ExportJSON, result.CSVOptions{})
	written := writer.Begin(shopColumns) +
		writer.WriteRows(nil, shopColumns) +
		writer.WriteRows([][]any{{int64(1), "Ada"}}, shopColumns) +
		writer.WriteRows([][]any{}, shopColumns) +
		writer.End()

	var read []map[string]any
	if err := json.Unmarshal([]byte(written), &read); err != nil {
		t.Fatalf("the file is not JSON: %v\n%s", err, written)
	}
	if len(read) != 1 {
		t.Errorf("the file holds %d rows, want 1", len(read))
	}
}
