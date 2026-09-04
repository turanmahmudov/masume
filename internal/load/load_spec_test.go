package load_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/turanmahmudov/masume/internal/core"
	"github.com/turanmahmudov/masume/internal/db/postgres"
	"github.com/turanmahmudov/masume/internal/db/sqlite"
	"github.com/turanmahmudov/masume/internal/load"
	"github.com/turanmahmudov/masume/internal/query"
)

// writeFile writes one file for a test to read, and returns its path.
func writeFile(t *testing.T, name, written string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, []byte(written), 0o600); err != nil {
		t.Fatalf("the file cannot be written: %v", err)
	}
	return path
}

// ordersCSV is a file of the shape a person exports from another tool: a header, a column
// of every kind, an empty field, and a null of its own.
const ordersCSV = `order_id,placed_at,total_cents,paid,coupon,note
100241,2026-02-11T09:03:00Z,4990,true,,first order
100242,2026-02-12,1200,false,\N,
100243,2026-02-13 10:00:00,99,no,SPRING,third
`

// buildOrdersSample reads the sample of that file.
func buildOrdersSample(t *testing.T, path string) load.Sample {
	t.Helper()
	sample, err := load.ReadSample(path, load.BuildReadOptions(path))
	if err != nil {
		t.Fatalf("the file does not read: %v", err)
	}
	return sample
}

// A sample must name every column of the file and the kind of value each one holds, so the
// form can map them without the user typing a type.
func TestReadSampleReadsTheKindOfEveryColumn(t *testing.T) {
	sample := buildOrdersSample(t, writeFile(t, "orders.csv", ordersCSV))

	if len(sample.Rows) != 3 {
		t.Fatalf("the sample read %d rows, wanted 3", len(sample.Rows))
	}
	if sample.More {
		t.Error("the sample reports more rows than the file holds")
	}

	wanted := []struct {
		name string
		kind core.ColumnKind
	}{
		{"order_id", core.KindInteger},
		{"placed_at", core.KindTimestamp},
		{"total_cents", core.KindInteger},
		{"paid", core.KindBoolean},
		{"coupon", core.KindText},
		{"note", core.KindText},
	}
	if len(sample.Columns) != len(wanted) {
		t.Fatalf("the sample holds %d columns, wanted %d", len(sample.Columns), len(wanted))
	}
	for at, one := range wanted {
		held := sample.Columns[at]
		if held.Name != one.name {
			t.Errorf("column %d is named %q, wanted %q", at, held.Name, one.name)
		}
		if held.Kind != one.kind {
			t.Errorf("%s holds %q, wanted %q", one.name, held.Kind, one.kind)
		}
	}
}

// An empty field and the null text both mean a value that is not there, so the counts hold
// how much of a column a file actually fills.
func TestReadSampleCountsTheValuesThatAreNotThere(t *testing.T) {
	sample := buildOrdersSample(t, writeFile(t, "orders.csv", ordersCSV))

	coupon := sample.Columns[4]
	if coupon.Filled != 1 || coupon.Empty != 2 {
		t.Errorf("coupon holds %d values and misses %d, wanted 1 and 2",
			coupon.Filled, coupon.Empty)
	}
	if coupon.Example != "SPRING" {
		t.Errorf("the example of coupon is %q, wanted SPRING", coupon.Example)
	}
}

// A number written with a zero in front of it is a code and not a number, so a column of
// them is text: read as a number the zero would be gone.
func TestReadSampleReadsACodeWithALeadingZeroAsText(t *testing.T) {
	path := writeFile(t, "codes.csv", "postcode,house\n01234,7\n00999,8\n")
	sample := buildOrdersSample(t, path)

	if sample.Columns[0].Kind != core.KindText {
		t.Errorf("postcode holds %q, wanted text", sample.Columns[0].Kind)
	}
	if sample.Columns[1].Kind != core.KindInteger {
		t.Errorf("house holds %q, wanted integer", sample.Columns[1].Kind)
	}
}

// A column of whole numbers and numbers with a point is read as the kind that holds both.
func TestReadSampleWidensAColumnToTheKindThatHoldsIt(t *testing.T) {
	path := writeFile(t, "totals.csv", "total\n10\n2.5\n")
	sample := buildOrdersSample(t, path)

	if sample.Columns[0].Kind != core.KindNumber {
		t.Errorf("total holds %q, wanted number", sample.Columns[0].Kind)
	}
}

// A file with no header names its columns by position, so it can still be mapped.
func TestReadSampleNamesTheColumnsOfAFileWithNoHeader(t *testing.T) {
	path := writeFile(t, "rows.csv", "1,alpha\n2,beta\n")
	options := load.BuildReadOptions(path)
	options.HasHeader = false

	sample, err := load.ReadSample(path, options)
	if err != nil {
		t.Fatalf("the file does not read: %v", err)
	}
	if len(sample.Columns) != 2 || sample.Columns[0].Name != "column_1" {
		t.Fatalf("the columns read %v, wanted names by position", sample.Columns)
	}
	// The first line holds a row and not a header, so both rows are read.
	if len(sample.Rows) != 2 {
		t.Errorf("the sample read %d rows, wanted both", len(sample.Rows))
	}
}

// A header that names one column twice would write two fields into one column, so every
// repeat is numbered.
func TestReadSampleNumbersARepeatedColumnName(t *testing.T) {
	path := writeFile(t, "twice.csv", "id,id,id\n1,2,3\n")
	sample := buildOrdersSample(t, path)

	names := []string{}
	for _, column := range sample.Columns {
		names = append(names, column.Name)
	}
	if strings.Join(names, ",") != "id,id_2,id_3" {
		t.Errorf("the columns are named %v, wanted each one once", names)
	}
}

// A tab separated file is read with a tab, taken from its extension, so a person does not
// set the delimiter for the format its name already names.
func TestBuildReadOptionsReadsTheFormatOfTheName(t *testing.T) {
	for _, one := range []struct {
		name      string
		format    load.FileFormat
		delimiter string
	}{
		{"orders.csv", load.FileCSV, ","},
		{"orders.tsv", load.FileCSV, "\t"},
		{"orders.json", load.FileJSON, ","},
		{"orders.jsonl", load.FileJSON, ","},
		{"orders", load.FileCSV, ","},
	} {
		options := load.BuildReadOptions(one.name)
		if options.Format != one.format {
			t.Errorf("%s reads as %q, wanted %q", one.name, options.Format, one.format)
		}
		if options.Delimiter != one.delimiter {
			t.Errorf("%s uses %q, wanted %q", one.name, options.Delimiter, one.delimiter)
		}
	}
}

// A JSON file of an array of documents holds its columns in the order the first document
// writes them, and the values keep the kind JSON already gave them.
func TestReadSampleReadsAnArrayOfDocuments(t *testing.T) {
	path := writeFile(t, "orders.json", `[
	  {"id": 1, "total": 49.9, "paid": true, "note": null},
	  {"id": 2, "total": 12, "paid": false, "note": "second", "extra": "late field"}
	]`)
	sample := buildOrdersSample(t, path)

	names := []string{}
	for _, column := range sample.Columns {
		names = append(names, column.Name)
	}
	if strings.Join(names, ",") != "id,total,paid,note,extra" {
		t.Errorf("the columns are %v, wanted the order the file writes", names)
	}
	if sample.Columns[0].Kind != core.KindInteger {
		t.Errorf("id holds %q, wanted integer", sample.Columns[0].Kind)
	}
	if sample.Columns[1].Kind != core.KindNumber {
		t.Errorf("total holds %q, wanted number", sample.Columns[1].Kind)
	}
	if sample.Columns[2].Kind != core.KindBoolean {
		t.Errorf("paid holds %q, wanted boolean", sample.Columns[2].Kind)
	}
	if len(sample.Rows) != 2 {
		t.Fatalf("the sample read %d rows, wanted 2", len(sample.Rows))
	}
}

// One document per line is the other form a tool writes JSON out in.
func TestReadSampleReadsOneDocumentPerLine(t *testing.T) {
	path := writeFile(t, "orders.jsonl",
		"{\"id\": 1, \"note\": \"first\"}\n{\"id\": 2, \"note\": \"second\"}\n")
	sample := buildOrdersSample(t, path)

	if len(sample.Rows) != 2 || len(sample.Columns) != 2 {
		t.Fatalf("the file read as %d rows of %d columns, wanted 2 of 2",
			len(sample.Rows), len(sample.Columns))
	}
	if sample.Rows[1].Values[1] != "second" {
		t.Errorf("the second row holds %v, wanted the second note", sample.Rows[1].Values)
	}
}

// A nested document belongs in one column, so it is kept as the text it was written as.
func TestReadSampleKeepsANestedDocumentWhole(t *testing.T) {
	path := writeFile(t, "orders.json", `[{"id": 1, "address": {"city": "Berlin", "zip": "10115"}}]`)
	sample := buildOrdersSample(t, path)

	held, isText := sample.Rows[0].Values[1].(string)
	if !isText {
		t.Fatalf("the nested document read as %T, wanted text", sample.Rows[0].Values[1])
	}
	if !strings.Contains(held, "Berlin") || !strings.HasPrefix(held, "{") {
		t.Errorf("the nested document reads %q, wanted the document it was written as", held)
	}
}

// A document inside a document belongs in one column whole.
func TestReadSampleKeepsADocumentTwoLevelsDeepWhole(t *testing.T) {
	path := writeFile(t, "orders.json",
		`[{"id": 1, "meta": {"tags": [1, 2], "address": {"city": "Berlin"}}}]`)
	sample := buildOrdersSample(t, path)

	held, isText := sample.Rows[0].Values[1].(string)
	if !isText {
		t.Fatalf("the document read as %T, wanted text", sample.Rows[0].Values[1])
	}

	read := map[string]any{}
	if err := json.Unmarshal([]byte(held), &read); err != nil {
		t.Fatalf("the document %q does not read back as JSON: %v", held, err)
	}
	if _, isList := read["tags"].([]any); !isList {
		t.Errorf("the list two levels down read as %T, wanted a list", read["tags"])
	}
	nested, isObject := read["address"].(map[string]any)
	if !isObject {
		t.Fatalf("the document two levels down read as %T, wanted a document",
			read["address"])
	}
	if nested["city"] != "Berlin" {
		t.Errorf("the city reads %v, wanted Berlin", nested["city"])
	}
}

// A file the reader cannot read must be reported, and not read as a file of no rows.
func TestReadSampleReportsAFileItCannotRead(t *testing.T) {
	for _, one := range []struct{ name, written string }{
		{"empty.csv", ""},
		{"broken.json", "[{"},
		{"scalar.json", "42"},
		{"empty.json", ""},
	} {
		path := writeFile(t, one.name, one.written)
		if _, err := load.ReadSample(path, load.BuildReadOptions(path)); err == nil {
			t.Errorf("%s was read as a file that can be imported", one.name)
		}
	}
	if _, err := load.ReadSample("/no/such/file.csv", load.DefaultReadOptions()); err == nil {
		t.Error("a file that is not there was read")
	}
}

// A delimiter of more than one character cannot separate a field, so it is reported.
func TestReadSampleReportsADelimiterItCannotUse(t *testing.T) {
	path := writeFile(t, "orders.csv", ordersCSV)
	options := load.BuildReadOptions(path)
	options.Delimiter = ";;"

	if _, err := load.ReadSample(path, options); err == nil {
		t.Error("a delimiter of two characters was used")
	}
}

// buildOrdersTable returns the columns of a table the file above can be written into.
func buildOrdersTable() []load.TargetColumn {
	return []load.TargetColumn{
		{Name: "id", DataType: "bigint", Generated: true},
		{Name: "order_id", DataType: "bigint", Optional: true, TakesNull: true},
		{Name: "placed_at", DataType: "timestamptz", Optional: true, TakesNull: true},
		{Name: "total_cents", DataType: "integer", Optional: true, TakesNull: true},
		{Name: "paid", DataType: "boolean", Optional: true, TakesNull: true},
		{Name: "note", DataType: "text", Optional: true, TakesNull: true},
	}
}

// buildOrdersPlan returns the import of the file above into that table.
func buildOrdersPlan(t *testing.T, target []load.TargetColumn) load.Plan {
	t.Helper()
	path := writeFile(t, "orders.csv", ordersCSV)
	return load.BuildPlan(path, load.BuildReadOptions(path), buildOrdersSample(t, path),
		query.QualifiedName{Schema: "public", Name: "orders"}, target)
}

// A column of the file whose name the table holds is mapped onto it by itself, so the usual
// import needs no mapping by hand.
func TestBuildPlanMapsTheColumnsThatShareAName(t *testing.T) {
	plan := buildOrdersPlan(t, buildOrdersTable())

	mapped := map[string]string{}
	for _, mapping := range plan.Mappings {
		mapped[mapping.Source] = mapping.Target
	}
	for _, name := range []string{"order_id", "placed_at", "total_cents", "paid", "note"} {
		if mapped[name] != name {
			t.Errorf("%s is written into %q, wanted the column of the same name",
				name, mapped[name])
		}
	}
	// The table holds no column named coupon, so it is left for the user to map.
	if mapped["coupon"] != "" {
		t.Errorf("coupon is written into %q, wanted no column", mapped["coupon"])
	}
	if plan.CreatesTable {
		t.Error("the plan makes a table that is already there")
	}
}

// The kind a value is cast to is the kind of the column it is written into, not the kind
// the file holds, because the server is what has to take the value.
func TestBuildPlanCastsToTheKindOfTheTable(t *testing.T) {
	target := buildOrdersTable()
	// The table holds the order number as text, so the file must be sent text.
	target[1].DataType = "text"
	plan := buildOrdersPlan(t, target)

	for _, mapping := range plan.Mappings {
		if mapping.Source == "order_id" && mapping.Kind != core.KindText {
			t.Errorf("order_id is cast to %q, wanted the text the column holds", mapping.Kind)
		}
	}
}

// A column the server fills itself must not be written to, because a value sent for it
// takes the place of the one the server would have made.
func TestBuildPlanLeavesOutAColumnTheServerFills(t *testing.T) {
	target := append(buildOrdersTable(), load.TargetColumn{
		Name: "coupon", DataType: "text", Optional: true, TakesNull: true, Generated: true,
	})
	plan := buildOrdersPlan(t, target)

	for _, mapping := range plan.Mappings {
		if mapping.Source == "coupon" && mapping.Target != "" {
			t.Error("a column the server fills itself was written to")
		}
	}
}

// A table that is not there yet is made from the columns of the file, in the kinds the file
// holds.
func TestBuildPlanMakesATableForAFileWithNoTable(t *testing.T) {
	plan := buildOrdersPlan(t, nil)

	if !plan.CreatesTable {
		t.Fatal("the plan writes into a table that is not there")
	}
	if len(plan.ListMappedColumns()) != len(plan.Sample.Columns) {
		t.Errorf("the plan writes %d of %d columns, wanted every one",
			len(plan.ListMappedColumns()), len(plan.Sample.Columns))
	}

	written := load.BuildCreateTable(plan, postgres.Dialect)
	for _, expected := range []string{
		`create table "public"."orders"`,
		"order_id bigint", "placed_at timestamptz", "total_cents bigint",
		"paid boolean", "coupon text", "note text",
	} {
		if !strings.Contains(written, expected) {
			t.Errorf("the table is made as %q, which is missing %q", written, expected)
		}
	}
}

// The user maps a column the names did not match, and leaves out one they do not want.
func TestMapColumnWritesAndLeavesOutAColumn(t *testing.T) {
	plan := buildOrdersPlan(t, buildOrdersTable())

	plan.MapColumn("coupon", "note")
	plan.MapColumn("note", "")

	mapped := map[string]string{}
	for _, mapping := range plan.Mappings {
		mapped[mapping.Source] = mapping.Target
	}
	if mapped["coupon"] != "note" {
		t.Errorf("coupon is written into %q, wanted note", mapped["coupon"])
	}
	if mapped["note"] != "" {
		t.Errorf("note is written into %q, wanted no column", mapped["note"])
	}
}

// An import that cannot run must report why while the form is still open, and not fail at
// the first row the server reads.
func TestFindPlanProblemReportsWhatStopsTheImport(t *testing.T) {
	full := buildOrdersPlan(t, buildOrdersTable())
	if problem := full.FindPlanProblem(postgres.Dialect); problem != "" {
		t.Fatalf("a plan that can run reports %q", problem)
	}

	// Two columns of the file into one column of the table.
	twice := buildOrdersPlan(t, buildOrdersTable())
	twice.MapColumn("coupon", "note")
	if problem := twice.FindPlanProblem(postgres.Dialect); !strings.Contains(problem, "note") {
		t.Errorf("two columns into one report %q", problem)
	}

	// Nothing mapped at all.
	empty := buildOrdersPlan(t, buildOrdersTable())
	for _, mapping := range empty.Mappings {
		empty.MapColumn(mapping.Source, "")
	}
	if problem := empty.FindPlanProblem(postgres.Dialect); problem == "" {
		t.Error("a plan that writes no column reports nothing")
	}

	// A column the server rejects an empty value for, which no column of the file fills.
	required := buildOrdersTable()
	required = append(required, load.TargetColumn{Name: "status", DataType: "text"})
	strict := buildOrdersPlan(t, required)
	if problem := strict.FindPlanProblem(postgres.Dialect); !strings.Contains(problem, "status") {
		t.Errorf("a column that takes no empty value reports %q", problem)
	}
}

// A dry run must read the whole file and report every row the import cannot write, before
// anything is sent to a server.
func TestCheckFileReportsTheRowsItCannotWrite(t *testing.T) {
	path := writeFile(t, "orders.csv", ordersCSV+
		"100244,2026-02-14,n/a,true,,fourth\n"+
		"100245,not a date,10,true,,fifth\n")
	sample := buildOrdersSample(t, path)
	plan := load.BuildPlan(path, load.BuildReadOptions(path), sample,
		query.QualifiedName{Schema: "public", Name: "orders"}, buildOrdersTable())

	report, err := plan.CheckFile()
	if err != nil {
		t.Fatalf("the check does not run: %v", err)
	}
	if report.Rows != 5 {
		t.Errorf("the check read %d rows, wanted 5", report.Rows)
	}
	if report.Refused != 2 {
		t.Fatalf("the check refused %d rows, wanted 2: %v", report.Refused, report.Problems)
	}
	if report.Problems[0].Line != 5 || report.Problems[0].Column != "total_cents" {
		t.Errorf("the first refusal is %v, wanted line 5 of total_cents", report.Problems[0])
	}
	if !strings.Contains(report.Problems[0].Reason, "n/a") {
		t.Errorf("the reason reads %q, wanted the value it refused",
			report.Problems[0].Reason)
	}
	if !strings.Contains(load.DescribeReport(report), "2") {
		t.Errorf("the report reads %q, wanted the count of refused rows",
			load.DescribeReport(report))
	}
}

// A row with more fields than the file names is a row the import cannot place, so it is
// reported with its line and not written.
func TestCheckFileReportsARowOfTheWrongLength(t *testing.T) {
	path := writeFile(t, "orders.csv", "id,note\n1,first\n2,second,extra\n")
	sample := buildOrdersSample(t, path)
	plan := load.BuildPlan(path, load.BuildReadOptions(path), sample,
		query.QualifiedName{Schema: "public", Name: "orders"}, nil)

	report, err := plan.CheckFile()
	if err != nil {
		t.Fatalf("the check does not run: %v", err)
	}
	if report.Refused != 1 || report.Problems[0].Line != 3 {
		t.Fatalf("the check refused %v, wanted the third line", report.Problems)
	}
}

// A file that can be written whole must be reported as such, because a person acts on it.
func TestCheckFileReportsAFileItCanWriteWhole(t *testing.T) {
	plan := buildOrdersPlan(t, buildOrdersTable())

	report, err := plan.CheckFile()
	if err != nil {
		t.Fatalf("the check does not run: %v", err)
	}
	if report.Refused != 0 || report.Rows != 3 {
		t.Fatalf("the check read %d rows and refused %d, wanted 3 and 0",
			report.Rows, report.Refused)
	}
	if !strings.Contains(load.DescribeReport(report), "every one") {
		t.Errorf("the report reads %q", load.DescribeReport(report))
	}
}

// The insert must bind every value, so nothing a file holds can be read as SQL.
func TestBuildInsertBindsEveryValue(t *testing.T) {
	plan := buildOrdersPlan(t, buildOrdersTable())

	values, err := load.BuildRows(plan, plan.Sample.Rows)
	if err != nil {
		t.Fatalf("the rows do not cast: %v", err)
	}
	statement, err := load.BuildInsert(plan, values, postgres.Dialect)
	if err != nil {
		t.Fatalf("the insert does not build: %v", err)
	}

	if !strings.HasPrefix(statement.SQL, `insert into "public"."orders" (`) {
		t.Errorf("the insert reads %q", statement.SQL)
	}
	// Five columns are mapped and three rows are written, so fifteen values are bound.
	if len(statement.Params) != 15 {
		t.Errorf("the insert bound %d values, wanted 15", len(statement.Params))
	}
	if strings.Contains(statement.SQL, "first order") {
		t.Error("a value of the file was written into the statement instead of bound")
	}
	if !strings.Contains(statement.SQL, "$15") {
		t.Errorf("the insert reads %q, wanted a placeholder per value", statement.SQL)
	}
}

// A value is sent as the type its column holds, so the server is not asked to read a
// number or a date out of text.
func TestBuildRowsCastsEveryValueToItsColumn(t *testing.T) {
	plan := buildOrdersPlan(t, buildOrdersTable())

	values, err := load.BuildRows(plan, plan.Sample.Rows)
	if err != nil {
		t.Fatalf("the rows do not cast: %v", err)
	}
	if held, isInteger := values[0][0].(int64); !isInteger || held != 100241 {
		t.Errorf("the order number is %v of %T, wanted a whole number",
			values[0][0], values[0][0])
	}
	if _, isBool := values[0][3].(bool); !isBool {
		t.Errorf("paid is %v of %T, wanted a yes or no", values[0][3], values[0][3])
	}
	// The third row writes `no`, which is a boolean written as a word.
	if held, isBool := values[2][3].(bool); !isBool || held {
		t.Errorf("the third paid is %v, wanted false", values[2][3])
	}
	// The note of the second row is empty, so nothing is sent for it.
	if values[1][4] != nil {
		t.Errorf("the empty note is %v, wanted nothing", values[1][4])
	}
}

// The review shows the SQL before anything is written, so a person reads what will run.
func TestDescribeStatementsWritesTheSQLOfTheImport(t *testing.T) {
	plan := buildOrdersPlan(t, nil)

	written, err := load.DescribeStatements(plan, postgres.Dialect)
	if err != nil {
		t.Fatalf("the review does not build: %v", err)
	}
	if len(written) != 2 {
		t.Fatalf("the review holds %d statements, wanted the table and the insert", len(written))
	}
	if !strings.HasPrefix(written[0], "create table") {
		t.Errorf("the first statement reads %q, wanted the table", written[0])
	}
	// The review is read by a person, so its values are written in and not bound.
	if !strings.Contains(written[1], "'first order'") {
		t.Errorf("the insert reads %q, wanted the values written in", written[1])
	}
	if strings.Contains(written[1], "$1") {
		t.Errorf("the insert reads %q, wanted no placeholder", written[1])
	}
}

// A table that is already there is not made again, so the review holds the insert alone.
func TestDescribeStatementsMakesNoTableThatIsThere(t *testing.T) {
	plan := buildOrdersPlan(t, buildOrdersTable())

	written, err := load.DescribeStatements(plan, postgres.Dialect)
	if err != nil {
		t.Fatalf("the review does not build: %v", err)
	}
	if len(written) != 1 || strings.Contains(written[0], "create table") {
		t.Errorf("the review holds %v, wanted the insert alone", written)
	}
}

// A file of many columns writes fewer rows at a time.
func TestResolveBatchRowsFollowsTheLimitOfTheServer(t *testing.T) {
	for _, one := range []struct {
		mapped int
		rows   int
	}{
		{5, load.BatchRows},
		{65, load.BatchRows},
		{100, 655},
		{40000, 1},
		{0, load.BatchRows},
	} {
		if held := load.ResolveBatchRows(one.mapped, postgres.Dialect); held != one.rows {
			t.Errorf("%d columns write %d rows at a time, wanted %d",
				one.mapped, held, one.rows)
		}
	}

	if held := load.ResolveBatchRows(100, sqlite.Dialect); held != 327 {
		t.Errorf("SQLite writes %d rows at a time for 100 columns, wanted 327", held)
	}
}

// A file wider than the server binds to one statement is refused before anything runs.
func TestFindPlanProblemRefusesAFileWiderThanTheServerBinds(t *testing.T) {
	names := make([]string, 0, 40000)
	for at := range 40000 {
		names = append(names, "c"+strconv.Itoa(at))
	}
	header := strings.Join(names, ",") + "\n" + strings.Repeat("1,", len(names)-1) + "1\n"
	path := writeFile(t, "wide.csv", header)

	sample := buildOrdersSample(t, path)
	plan := load.BuildPlan(path, load.BuildReadOptions(path), sample,
		query.QualifiedName{Schema: "public", Name: "wide"}, nil)

	if problem := plan.FindPlanProblem(sqlite.Dialect); problem == "" {
		t.Error("a file wider than SQLite binds was accepted")
	}
	if problem := plan.FindPlanProblem(postgres.Dialect); problem != "" {
		t.Errorf("a file PostgreSQL can write was refused: %s", problem)
	}
	if held := load.ResolveBatchRows(len(plan.ListMappedColumns()), postgres.Dialect); held != 1 {
		t.Errorf("a file that wide writes %d rows at a time, wanted one", held)
	}
}

// A whole number too large for 64 bits keeps every digit, so an account number or a
// snowflake id survives the import.
func TestReadSampleKeepsAWholeNumberTooLargeForSixtyFourBits(t *testing.T) {
	path := writeFile(t, "ids.csv", "account_id,amount\n12345678901234567890,4990\n")
	sample := buildOrdersSample(t, path)

	if sample.Columns[0].Kind != core.KindText {
		t.Errorf("the column reads %q, wanted text", sample.Columns[0].Kind)
	}
	if held := sample.Rows[0].Values[0]; held != "12345678901234567890" {
		t.Errorf("the value reads %v, wanted every digit", held)
	}
}

// A report of a JSON file names the line an editor shows, and a file that opens with a byte
// order mark still reads.
func TestReadSampleCountsTheLinesOfAJSONFile(t *testing.T) {
	path := writeFile(t, "rows.json", "[\n  {\"a\": 1},\n  {\"a\": 2}\n]\n")
	sample := buildOrdersSample(t, path)

	if sample.Rows[0].Line != 2 || sample.Rows[1].Line != 3 {
		t.Errorf("the rows read lines %d and %d, wanted 2 and 3",
			sample.Rows[0].Line, sample.Rows[1].Line)
	}

	marked := writeFile(t, "marked.jsonl", "\ufeff{\"a\": 1}\n{\"a\": 2}\n")
	if held := buildOrdersSample(t, marked); len(held.Rows) != 2 {
		t.Errorf("a file that opens with the mark read %d rows, wanted 2", len(held.Rows))
	}
}
