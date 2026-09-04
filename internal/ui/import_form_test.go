package ui

import (
	"strings"
	"testing"

	"github.com/turanmahmudov/masume/internal/app"
	"github.com/turanmahmudov/masume/internal/core"
	"github.com/turanmahmudov/masume/internal/db"
	"github.com/turanmahmudov/masume/internal/db/postgres"
	"github.com/turanmahmudov/masume/internal/load"
	"github.com/turanmahmudov/masume/internal/query"
)

// buildImportOverlay returns the import form as it stands once a file has been read and its
// columns mapped onto a table.
func buildImportOverlay() app.Overlay {
	target := []load.TargetColumn{
		{Name: "id", DataType: "bigint", Generated: true},
		{Name: "order_id", DataType: "bigint", Optional: true, TakesNull: true},
		{Name: "note", DataType: "text", Optional: true, TakesNull: true},
	}
	sample := load.Sample{
		Columns: []load.SourceColumn{
			{Name: "order_id", Kind: core.KindInteger, Filled: 2, Example: "100241"},
			{Name: "coupon", Kind: core.KindText, Filled: 1},
		},
		Rows: []load.Row{{Line: 2, Values: []any{"100241", "SPRING"}}},
	}
	plan := load.BuildPlan("/tmp/orders.csv", load.DefaultReadOptions(), sample,
		query.QualifiedName{Schema: "public", Name: "orders"}, target)

	return app.Overlay{
		Kind: app.OverlayImport,
		Import: app.ImportRequest{
			Stage: app.ImportMapping, Plan: plan,
			TargetNames: ListTargetNames(target),
		},
		Draft: app.NewEditorBuffer(plan.Path, len(plan.Path)),
	}
}

// findFieldNamed returns the row of the form with that key.
func findFieldNamed(fields []DialogField, key string) (DialogField, bool) {
	for _, field := range fields {
		if field.Key == key {
			return field, true
		}
	}
	return DialogField{}, false
}

// The form holds one row per setting and one row per column of the file, so everything the
// import does is on the card.
func TestBuildImportFieldsHoldsARowPerColumnOfTheFile(t *testing.T) {
	fields := BuildImportFields(buildImportOverlay())

	for _, key := range []string{
		"path", "table", "format", "delimiter", "header", "null-text",
		"map:order_id", "map:coupon",
	} {
		if _, held := findFieldNamed(fields, key); !held {
			t.Errorf("the form holds no row for %q", key)
		}
	}

	// A column of the file whose name the table holds is written into it, and one it does
	// not is left out until the user maps it.
	mapped, _ := findFieldNamed(fields, "map:order_id")
	if mapped.Value != "order_id" {
		t.Errorf("order_id is written into %q, wanted the column of the same name", mapped.Value)
	}
	left, _ := findFieldNamed(fields, "map:coupon")
	if left.Value != skipColumn {
		t.Errorf("coupon is written into %q, wanted it left out", left.Value)
	}
	// The row carries the kind the column of the file holds, so the mapping reads at a glance.
	if !strings.Contains(mapped.Label, "integer") {
		t.Errorf("the row of order_id reads %q, wanted the kind it holds", mapped.Label)
	}
}

// A mapping steps through the columns of the table, and the answer that leaves the column
// out stands first, so it is one step away.
func TestBuildImportFieldsOffersEveryColumnOfTheTable(t *testing.T) {
	fields := BuildImportFields(buildImportOverlay())
	mapped, _ := findFieldNamed(fields, "map:order_id")

	if len(mapped.Choices) == 0 || mapped.Choices[0] != skipColumn {
		t.Fatalf("the mapping steps through %v, wanted the skip first", mapped.Choices)
	}
	if strings.Join(mapped.Choices, ",") != skipColumn+",order_id,note" {
		t.Errorf("the mapping steps through %v, wanted every column of the table",
			mapped.Choices)
	}
}

// The rows of a CSV belong to a CSV, so a file of documents does not ask for a delimiter.
func TestBuildImportFieldsHidesTheCSVRowsForAFileOfDocuments(t *testing.T) {
	overlay := buildImportOverlay()
	overlay.Import.Plan.Options.Format = load.FileJSON

	fields := BuildImportFields(overlay)
	for _, key := range []string{"delimiter", "header", "null-text"} {
		if _, held := findFieldNamed(fields, key); held {
			t.Errorf("a file of documents asks for %q", key)
		}
	}
	if _, held := findFieldNamed(fields, "map:order_id"); !held {
		t.Error("the mapping rows went with the CSV rows")
	}
}

// A file that was not read yet has no column to map, so the form holds its settings alone.
func TestBuildImportFieldsHoldsNoMappingBeforeTheFileIsRead(t *testing.T) {
	overlay := buildImportOverlay()
	overlay.Import.Stage = app.ImportFile

	for _, field := range BuildImportFields(overlay) {
		if strings.HasPrefix(field.Key, mappingKeyPrefix) {
			t.Errorf("the form maps %q before the file was read", field.Key)
		}
	}
}

// The text typed into a row reaches the import, which runs from the form.
func TestReadImportFieldWritesWhatWasTyped(t *testing.T) {
	overlay := buildImportOverlay()

	for _, one := range []struct {
		key     string
		written string
		read    func(app.Overlay) string
	}{
		{"path", "/tmp/other.csv", func(held app.Overlay) string { return held.Import.Plan.Path }},
		{"delimiter", ";", func(held app.Overlay) string {
			return held.Import.Plan.Options.Delimiter
		}},
		{"null-text", "NULL", func(held app.Overlay) string {
			return held.Import.Plan.Options.NullText
		}},
	} {
		fields := BuildImportFields(overlay)
		for at, field := range fields {
			if field.Key == one.key {
				overlay.Field = at
			}
		}
		ReadImportField(&overlay, one.written)
		if held := one.read(overlay); held != one.written {
			t.Errorf("%s reads %q, wanted %q", one.key, held, one.written)
		}
	}
}

// A table can be named with its schema or without one, and a name without one keeps the
// schema the form opened with.
func TestReadImportFieldReadsTheTableWithAndWithoutASchema(t *testing.T) {
	overlay := buildImportOverlay()
	fields := BuildImportFields(overlay)
	for at, field := range fields {
		if field.Key == "table" {
			overlay.Field = at
		}
	}

	ReadImportField(&overlay, "audit.events")
	if overlay.Import.Plan.Table.Schema != "audit" ||
		overlay.Import.Plan.Table.Name != "events" {
		t.Errorf("the table reads %v, wanted audit.events", overlay.Import.Plan.Table)
	}

	ReadImportField(&overlay, "orders")
	if overlay.Import.Plan.Table.Schema != "audit" ||
		overlay.Import.Plan.Table.Name != "orders" {
		t.Errorf("the table reads %v, wanted the schema it had", overlay.Import.Plan.Table)
	}
}

// Stepping a mapping writes the column of the table, and stepping it back to the skip
// leaves the column of the file out.
func TestStepImportChoiceWritesAndLeavesOutAMapping(t *testing.T) {
	overlay := buildImportOverlay()
	fields := BuildImportFields(overlay)
	for at, field := range fields {
		if field.Key == "map:coupon" {
			overlay.Field = at
		}
	}

	// One step past the skip is the first column of the table.
	StepImportChoice(&overlay, 1)
	mapped, _ := findFieldNamed(BuildImportFields(overlay), "map:coupon")
	if mapped.Value != "order_id" {
		t.Errorf("one step writes %q, wanted the first column offered", mapped.Value)
	}

	StepImportChoice(&overlay, -1)
	mapped, _ = findFieldNamed(BuildImportFields(overlay), "map:coupon")
	if mapped.Value != skipColumn {
		t.Errorf("a step back writes %q, wanted the column left out", mapped.Value)
	}
}

// The file was read in one format, so choosing another reads it again rather than mapping
// the columns of the format it is no longer read as.
func TestStepImportChoiceReadsTheFileAgainForAnotherFormat(t *testing.T) {
	overlay := buildImportOverlay()
	fields := BuildImportFields(overlay)
	for at, field := range fields {
		if field.Key == "format" {
			overlay.Field = at
		}
	}

	StepImportChoice(&overlay, 1)
	if overlay.Import.Plan.Options.Format != load.FileJSON {
		t.Errorf("the format reads %q, wanted json", overlay.Import.Plan.Options.Format)
	}
	if overlay.Import.Stage != app.ImportFile {
		t.Errorf("the stage is %q, wanted the one that reads the file", overlay.Import.Stage)
	}
	if !overlay.Import.FormatChosen {
		t.Error("the format the user chose is not held")
	}
}

// The name of a file names the format, so a person who typed a path does not set it as well.
// A format they chose themselves is kept.
func TestApplyImportPathReadsTheFormatOfTheName(t *testing.T) {
	overlay := buildImportOverlay()
	overlay.Import.Plan.Path = "/tmp/people.json"
	ApplyImportPath(&overlay)
	if overlay.Import.Plan.Options.Format != load.FileJSON {
		t.Errorf("the format reads %q, wanted json", overlay.Import.Plan.Options.Format)
	}

	overlay.Import.Plan.Path = "/tmp/orders.tsv"
	ApplyImportPath(&overlay)
	if overlay.Import.Plan.Options.Delimiter != "\t" {
		t.Errorf("the delimiter reads %q, wanted a tab",
			overlay.Import.Plan.Options.Delimiter)
	}

	overlay.Import.FormatChosen = true
	overlay.Import.Plan.Path = "/tmp/people.json"
	overlay.Import.Plan.Options.Format = load.FileCSV
	ApplyImportPath(&overlay)
	if overlay.Import.Plan.Options.Format != load.FileCSV {
		t.Error("the format the user chose was written over by the name of the file")
	}

	typed := buildImportOverlay()
	typed.Import.Plan.Path = "/tmp/orders.tsv"
	typed.Import.Plan.Options.Delimiter = ";"
	typed.Import.DelimiterChosen = true
	ApplyImportPath(&typed)
	if typed.Import.Plan.Options.Delimiter != ";" {
		t.Errorf("the delimiter reads %q, wanted the one that was typed",
			typed.Import.Plan.Options.Delimiter)
	}
}

// An import that cannot go on reports why on the card, before anything is read or written.
func TestFindImportProblemReportsWhatStopsTheImport(t *testing.T) {
	full := buildImportOverlay()
	if problem := FindImportProblem(full, postgres.Dialect); problem != "" {
		t.Fatalf("a form that can go on reports %q", problem)
	}

	empty := buildImportOverlay()
	empty.Import.Plan.Path = ""
	empty.Draft = app.NewEditorBuffer("", 0)
	if problem := FindImportProblem(empty, postgres.Dialect); problem == "" {
		t.Error("a form with no file reports nothing")
	}

	wide := buildImportOverlay()
	wide.Import.Plan.Options.Delimiter = ";;"
	if problem := FindImportProblem(wide, postgres.Dialect); !strings.Contains(problem, "delimiter") {
		t.Errorf("a delimiter of two characters reports %q", problem)
	}

	// A file that is not read yet is not asked about its mapping.
	unread := buildImportOverlay()
	unread.Import.Stage = app.ImportFile
	unread.Import.Plan.Options.Delimiter = ","
	for _, mapping := range unread.Import.Plan.Mappings {
		unread.Import.Plan.MapColumn(mapping.Source, "")
	}
	if problem := FindImportProblem(unread, postgres.Dialect); problem != "" {
		t.Errorf("a file that is not read yet reports %q", problem)
	}
}

// The cursor moves through the rows and keeps what was typed on the row it left.
func TestStepImportFieldKeepsWhatWasTyped(t *testing.T) {
	overlay := buildImportOverlay()
	overlay.Field = 0
	overlay.Draft = app.NewEditorBuffer("/tmp/typed.csv", 0)

	StepImportField(&overlay, 1)
	if overlay.Import.Plan.Path != "/tmp/typed.csv" {
		t.Errorf("the path reads %q, wanted what was typed", overlay.Import.Plan.Path)
	}
	if overlay.Field != 1 {
		t.Errorf("the cursor stands on row %d, wanted the next one", overlay.Field)
	}
	// The draft holds the value of the row the cursor moved to.
	if overlay.Draft.Text != describeImportTable(overlay.Import) {
		t.Errorf("the field holds %q, wanted the table", overlay.Draft.Text)
	}

	// The cursor wraps, so every row is reachable with one key.
	overlay.Field = len(BuildImportFields(overlay)) - 1
	StepImportField(&overlay, 1)
	if overlay.Field != 0 {
		t.Errorf("the cursor stands on row %d, wanted the first one", overlay.Field)
	}
}

// Every row of the form holds its own value, so the cursor moving away and back leaves each
// row reading what it held: the draft belongs to the row the cursor stands on and to no
// other.
func TestStepImportFieldKeepsEachRowsOwnValue(t *testing.T) {
	overlay := buildImportOverlay()
	overlay.Field = 0
	overlay.Draft = app.NewEditorBuffer(overlay.Import.Plan.Path, 0)
	path := overlay.Import.Plan.Path
	table := describeImportTable(overlay.Import)

	// Down to the table and back up to the file.
	StepImportField(&overlay, 1)
	if overlay.Draft.Text != table {
		t.Fatalf("the table row holds %q, wanted %q", overlay.Draft.Text, table)
	}
	StepImportField(&overlay, -1)
	if overlay.Draft.Text != path {
		t.Errorf("the file row holds %q, wanted %q", overlay.Draft.Text, path)
	}
	if overlay.Import.Plan.Path != path {
		t.Errorf("the path reads %q, wanted %q", overlay.Import.Plan.Path, path)
	}
	if held := describeImportTable(overlay.Import); held != table {
		t.Errorf("the table reads %q, wanted %q", held, table)
	}

	// Around the whole form and back to the file, which every row is reached by.
	for range len(BuildImportFields(overlay)) {
		StepImportField(&overlay, 1)
	}
	if overlay.Field != 0 || overlay.Draft.Text != path {
		t.Errorf("the file row holds %q on row %d, wanted %q on the first row",
			overlay.Draft.Text, overlay.Field, path)
	}
}

// A column the server fills itself takes no value from a file, and a column with a default
// takes an empty one, so the import reads both from the table.
func TestBuildImportTargetReadsWhatEachColumnTakes(t *testing.T) {
	target := buildImportTarget(db.TableDetail{Columns: []db.ColumnDetail{
		{Name: "id", DataType: "bigint", IsGenerated: true},
		{Name: "status", DataType: "text", HasDefault: true, DefaultValue: "'new'"},
		{Name: "note", DataType: "text", Nullable: true},
		{Name: "total", DataType: "integer"},
	}})

	if len(target) != 4 {
		t.Fatalf("the table reads as %d columns, wanted 4", len(target))
	}
	if !target[0].Generated {
		t.Error("a column the server fills is not marked")
	}
	// A row may leave out a column with a default, and the column still refuses an empty
	// value written into it. The two are read apart.
	if !target[1].Optional || target[1].TakesNull {
		t.Error("a column with a default is read as one that holds an empty value")
	}
	if !target[2].Optional || !target[2].TakesNull {
		t.Error("a column that holds an empty value is not marked")
	}
	if target[3].Optional || target[3].TakesNull {
		t.Error("a column that takes no empty value is marked as if it did")
	}
	// A column the server fills itself is never offered as somewhere to write.
	if strings.Join(ListTargetNames(target), ",") != "status,note,total" {
		t.Errorf("the names read %v, wanted the generated column left out",
			ListTargetNames(target))
	}
}

// The card names the next step, so a person reads the step and not the key alone.
func TestDescribeImportStepNamesTheNextStep(t *testing.T) {
	held := app.ImportRequest{Stage: app.ImportFile}
	if describeImportStep(held) != "read the file" {
		t.Errorf("the first step reads %q", describeImportStep(held))
	}
	held.Stage = app.ImportMapping
	if describeImportStep(held) != "review" {
		t.Errorf("the second step reads %q", describeImportStep(held))
	}
	held.Running = true
	if !strings.Contains(describeImportStep(held), "…") {
		t.Errorf("a step that is running reads %q", describeImportStep(held))
	}
}

// The review counts the rows it would write, which is what a person decides on.
func TestDescribeImportSummaryCountsTheRowsItWouldWrite(t *testing.T) {
	held := buildImportOverlay().Import
	held.Report = load.CheckReport{Rows: 48210, Refused: 3}

	said := DescribeImportSummary(held)
	for _, expected := range []string{"48,207", "48,210", "orders"} {
		if !strings.Contains(said, expected) {
			t.Errorf("the summary reads %q, which is missing %q", said, expected)
		}
	}
}
