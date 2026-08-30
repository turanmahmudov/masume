package result_test

import (
	"encoding/json"
	"math"
	"strings"
	"testing"

	"github.com/turanmahmudov/masume/internal/query"
	"github.com/turanmahmudov/masume/internal/query/result"
)

var shopColumns = []query.ResultColumn{
	{Name: "id", DataType: "integer"},
	{Name: "customer", DataType: "text"},
}

// writeCSV runs a whole export through the writer and answers the file it wrote.
func writeCSV(rows [][]any, options result.CSVOptions) string {
	writer := result.CreateExportWriter(result.ExportCSV, options)
	return writer.Begin(shopColumns) + writer.WriteRows(rows, shopColumns) + writer.End()
}

func TestCsvExportWritesAHeaderAndTheRows(t *testing.T) {
	written := writeCSV([][]any{{int64(1), "ada"}, {int64(2), "grace"}},
		result.DefaultCSVOptions())

	lines := strings.Split(strings.TrimRight(written, "\n"), "\n")
	if len(lines) != 3 {
		t.Fatalf("the file holds %d lines, wanted a header and two rows:\n%s", len(lines), written)
	}
	if lines[0] != "id,customer" {
		t.Errorf("the header reads %q", lines[0])
	}
	if lines[1] != "1,ada" {
		t.Errorf("the first row reads %q", lines[1])
	}
}

func TestCsvExportLeavesOutTheHeaderWhereItIsNotAsked(t *testing.T) {
	options := result.DefaultCSVOptions()
	options.Header = false
	written := writeCSV([][]any{{int64(1), "ada"}}, options)

	if strings.Contains(written, "customer") {
		t.Errorf("the header was written although it was not asked for:\n%s", written)
	}
}

// A field holding the delimiter, a quote or a line break has to be quoted, or the file reads
// back with the wrong number of columns.
func TestCsvExportQuotesWhatWouldBreakTheFile(t *testing.T) {
	for _, held := range []struct {
		name  string
		value string
		want  string
	}{
		{"a comma", "ada,grace", `"ada,grace"`},
		{"a quote", `ada "the" first`, `"ada ""the"" first"`},
		{"a line break", "ada\ngrace", "\"ada\ngrace\""},
		{"a carriage return", "ada\rgrace", "\"ada\rgrace\""},
		{"nothing special", "ada", "ada"},
	} {
		t.Run(held.name, func(t *testing.T) {
			options := result.DefaultCSVOptions()
			options.Header = false
			written := writeOneField(held.value, options)
			if written != held.want {
				t.Errorf("%q writes as %q, wanted %q", held.value, written, held.want)
			}
		})
	}
}

// A value a spreadsheet would run as a formula is made text, because opening an exported file
// must not run what a row of the database happened to hold.
func TestCsvExportMakesAFormulaText(t *testing.T) {
	options := result.DefaultCSVOptions()
	options.Header = false

	for _, held := range []struct {
		name   string
		value  string
		quoted bool
	}{
		{"an equals formula", "=1+1", true},
		{"a command", "=cmd|'/c calc'!A1", true},
		{"a plus", "+1+1", true},
		{"an at sign", "@SUM(A1)", true},
		{"a tab", "\tada", true},

		// A number is left as it is, or every figure in a column would gain a quote.
		{"a plain number", "42", false},
		{"a negative number", "-42", false},
		{"a decimal", "-4.25", false},
		{"a number in exponent form", "-1.5e3", false},
		{"ordinary text", "ada", false},
	} {
		t.Run(held.name, func(t *testing.T) {
			written := writeOneField(held.value, options)
			if strings.HasPrefix(strings.TrimPrefix(written, `"`), "'") != held.quoted {
				t.Errorf("%q writes as %q, wanted a leading quote = %v",
					held.value, written, held.quoted)
			}
		})
	}
}

// The guard can be turned off, for a file that is read by a program rather than a spreadsheet.
func TestCsvExportLeavesAFormulaAloneWhereTheGuardIsOff(t *testing.T) {
	options := result.DefaultCSVOptions()
	options.Header = false
	options.SanitizeFormulas = false

	if written := writeOneField("=1+1", options); strings.Contains(written, "'") {
		t.Errorf("the formula was made text although the guard is off: %q", written)
	}
}

// writeOneField answers the one field a row of one column writes.
func writeOneField(value string, options result.CSVOptions) string {
	columns := []query.ResultColumn{{Name: "customer"}}
	writer := result.CreateExportWriter(result.ExportCSV, options)
	return strings.TrimRight(writer.WriteRows([][]any{{value}}, columns), "\r\n")
}

func TestCsvExportWritesTheLineEndingAsked(t *testing.T) {
	options := result.DefaultCSVOptions()
	options.LineEnding = result.EndingCrlf
	written := writeCSV([][]any{{int64(1), "ada"}}, options)

	if !strings.Contains(written, "\r\n") {
		t.Errorf("the file holds no CRLF:\n%q", written)
	}
}

func TestCsvExportWritesNullAsTheTextAsked(t *testing.T) {
	options := result.DefaultCSVOptions()
	options.Header = false
	options.NullText = "NULL"

	written := strings.TrimRight(writeCSV([][]any{{nil, nil}}, options), "\n")
	if written != "NULL,NULL" {
		t.Errorf("a row of nulls writes as %q", written)
	}
}

// The JSON export has to be one whole document, so a reader can parse the file in one go.
func TestJsonExportWritesOneDocument(t *testing.T) {
	writer := result.CreateExportWriter(result.ExportJSON, result.CSVOptions{})
	written := writer.Begin(shopColumns) +
		writer.WriteRows([][]any{{int64(1), "ada"}, {int64(2), "grace"}}, shopColumns) +
		writer.End()

	var read []map[string]any
	if err := json.Unmarshal([]byte(written), &read); err != nil {
		t.Fatalf("the file does not read back as JSON: %v\n%s", err, written)
	}
	if len(read) != 2 {
		t.Fatalf("the file holds %d records, wanted 2", len(read))
	}
	if read[0]["customer"] != "ada" {
		t.Errorf("the first record reads %v", read[0])
	}
}

// A batch is written as its own call, and the whole file still has to parse.
func TestJsonExportWritesSeveralBatchesAsOneDocument(t *testing.T) {
	writer := result.CreateExportWriter(result.ExportJSON, result.CSVOptions{})
	written := writer.Begin(shopColumns)
	for at := 1; at <= 3; at++ {
		written += writer.WriteRows([][]any{{int64(at), "name"}}, shopColumns)
	}
	written += writer.End()

	var read []map[string]any
	if err := json.Unmarshal([]byte(written), &read); err != nil {
		t.Fatalf("three batches do not read back as JSON: %v\n%s", err, written)
	}
	if len(read) != 3 {
		t.Errorf("the file holds %d records, wanted 3", len(read))
	}
}

func TestJsonExportWritesAnEmptyResultAsAnEmptyList(t *testing.T) {
	writer := result.CreateExportWriter(result.ExportJSON, result.CSVOptions{})
	written := writer.Begin(shopColumns) + writer.End()

	var read []map[string]any
	if err := json.Unmarshal([]byte(written), &read); err != nil {
		t.Fatalf("an empty export does not read back as JSON: %v\n%s", err, written)
	}
	if len(read) != 0 {
		t.Errorf("an empty export holds %d records", len(read))
	}
}

// Two columns of one name would overwrite each other in a record, so the keys are made
// distinct before anything is written.
func TestBuildRecordKeysKeepsEveryColumnApart(t *testing.T) {
	keys := result.BuildRecordKeys([]query.ResultColumn{
		{Name: "id"}, {Name: "name"}, {Name: "name"}, {Name: ""},
	})
	if len(keys) != 4 {
		t.Fatalf("four columns gave %d keys", len(keys))
	}
	seen := map[string]bool{}
	for _, key := range keys {
		if seen[key] {
			t.Errorf("the key %q is used twice", key)
		}
		seen[key] = true
	}
	// The repeat carries a suffix, so the second column of a name is still reachable.
	if keys[1] == keys[2] {
		t.Errorf("two columns named alike answered the same key %q", keys[1])
	}
}

// The name of the file goes onto a disk, so no separator survives and the name stays inside
// the directory the user chose. A dot is kept, because a name may hold one.
func TestBuildExportFilenameHoldsNoUnsafeCharacter(t *testing.T) {
	for _, label := range []string{
		"orders", "public.orders", "a/b", `a\b`, "a b", "../escape", "", "汉字",
	} {
		written := result.BuildExportFilename(label, result.ExportCSV, "20260825")
		if strings.ContainsAny(written, `/\`) {
			t.Errorf("%q became %q, which holds a path separator", label, written)
		}
		if !strings.HasSuffix(written, ".csv") {
			t.Errorf("%q became %q, which does not end in .csv", label, written)
		}
	}
}

// A column that holds a document is written into the file as that document. Written as
// text, every quote in it is escaped and a reader has to unescape the value before it can
// be read as JSON at all.
func TestJSONExportEmbedsADocumentRatherThanEscapingIt(t *testing.T) {
	columns := []query.ResultColumn{
		{Name: "id", DataType: "objectId"},
		{Name: "items", DataType: "array"},
		{Name: "shipping", DataType: "object"},
		{Name: "note", DataType: "string"},
	}
	rows := [][]any{{
		"abc", `[{"sku":"KB-001","qty":1}]`, `{"carrier":"DHL"}`, `{"not":"a document column"}`,
	}}

	writer := result.CreateExportWriter(result.ExportJSON, result.DefaultCSVOptions())
	written := writer.Begin(columns) + writer.WriteRows(rows, columns) + writer.End()

	if !strings.Contains(written, `"items":[{"sku":"KB-001","qty":1}]`) {
		t.Errorf("the array was not embedded:\n%s", written)
	}
	if !strings.Contains(written, `"shipping":{"carrier":"DHL"}`) {
		t.Errorf("the document was not embedded:\n%s", written)
	}
	// A column that holds text keeps its text, whatever the text looks like.
	if !strings.Contains(written, `"note":"{\"not\":\"a document column\"}"`) {
		t.Errorf("a text column was embedded as a document:\n%s", written)
	}

	// The file still reads as JSON.
	var read []map[string]any
	if err := json.Unmarshal([]byte(written), &read); err != nil {
		t.Fatalf("the file does not read as JSON: %v\n%s", err, written)
	}
	if _, isList := read[0]["items"].([]any); !isList {
		t.Errorf("the array reads back as %T, wanted a list", read[0]["items"])
	}
}

// A document column whose value is no document keeps its text, rather than making the
// file unreadable.
func TestJSONExportKeepsAValueThatIsNoDocument(t *testing.T) {
	columns := []query.ResultColumn{{Name: "held", DataType: "object"}}
	writer := result.CreateExportWriter(result.ExportJSON, result.DefaultCSVOptions())
	written := writer.Begin(columns) +
		writer.WriteRows([][]any{{"(not json)"}}, columns) + writer.End()

	var read []map[string]any
	if err := json.Unmarshal([]byte(written), &read); err != nil {
		t.Fatalf("the file does not read as JSON: %v\n%s", err, written)
	}
	if read[0]["held"] != "(not json)" {
		t.Errorf("the value reads %v", read[0]["held"])
	}
}

// JSON has no form for NaN and no form for an infinity, and a server returns both. Written
// as they are, the encoder refuses the record and the row would be missing from the file
// with nothing said, so the cell is written as its text and the row stays.
func TestJSONExportKeepsARowWithAValueJSONCannotHold(t *testing.T) {
	columns := []query.ResultColumn{
		{Name: "id", DataType: "integer"},
		{Name: "rate", DataType: "double precision"},
	}
	rows := [][]any{
		{int64(1), math.NaN()},
		{int64(2), math.Inf(1)},
		{int64(3), 12.5},
	}

	writer := result.CreateExportWriter(result.ExportJSON, result.DefaultCSVOptions())
	written := writer.Begin(columns) + writer.WriteRows(rows, columns) + writer.End()

	var read []map[string]any
	if err := json.Unmarshal([]byte(written), &read); err != nil {
		t.Fatalf("the file does not read as JSON: %v\n%s", err, written)
	}
	if len(read) != len(rows) {
		t.Fatalf("the file holds %d rows, wanted %d:\n%s", len(read), len(rows), written)
	}
	if read[2]["rate"] != 12.5 {
		t.Errorf("a number the encoder holds reads %v", read[2]["rate"])
	}
}
