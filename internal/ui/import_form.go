package ui

import (
	"strconv"
	"strings"

	"github.com/turanmahmudov/masume/internal/app"
	"github.com/turanmahmudov/masume/internal/db/postgres"
	"github.com/turanmahmudov/masume/internal/load"
	"github.com/turanmahmudov/masume/internal/present"
	"github.com/turanmahmudov/masume/internal/query"
)

// The form the import dialog draws: how the file is read, and which column of the file goes
// into which column of the table.

// skipColumn is what a mapping shows for a column of the file the import leaves out.
const skipColumn = "(skip)"

// importFileField is the row of the form the path of the file is written on.
const importFileField = 0

// mappingKeyPrefix marks a row of the form that maps one column of the file. The name of
// the column follows it.
const mappingKeyPrefix = "map:"

// BuildImportFields returns the rows of the form as the overlay stands now.
func BuildImportFields(overlay app.Overlay) []DialogField {
	held := overlay.Import
	options := held.Plan.Options

	formats := make([]string, 0, len(load.FileFormats))
	for _, format := range load.FileFormats {
		formats = append(formats, string(format))
	}

	fields := []DialogField{
		{Key: "path", Label: "file", Value: readImportPath(overlay)},
		{Key: "table", Label: "table", Value: describeImportTable(held)},
		{Key: "format", Label: "format", Value: string(options.Format), Choices: formats},
	}
	if options.Format == load.FileCSV {
		fields = append(fields,
			DialogField{Key: "delimiter", Label: "delimiter", Value: options.Delimiter},
			DialogField{
				Key: "header", Label: "header",
				Value: describeYesOrNo(options.HasHeader), Choices: yesOrNo,
			},
			DialogField{Key: "null-text", Label: "null as", Value: options.NullText},
		)
	}
	return append(fields, buildMappingFields(held)...)
}

// buildMappingFields returns one row per column of the file, which steps through the
// columns of the table.
func buildMappingFields(held app.ImportRequest) []DialogField {
	if held.Stage == app.ImportFile {
		return nil
	}

	choices := append([]string{skipColumn}, held.TargetNames...)
	fields := make([]DialogField, 0, len(held.Plan.Mappings))
	for at, mapping := range held.Plan.Mappings {
		value := skipColumn
		if mapping.Target != "" {
			value = mapping.Target
		}
		fields = append(fields, DialogField{
			Key:   mappingKeyPrefix + mapping.Source,
			Label: describeSourceColumn(held.Plan.Sample.Columns[at]),
			Value: value, Choices: choices,
		})
	}
	return fields
}

// describeSourceColumn names one column of the file and what it holds.
func describeSourceColumn(column load.SourceColumn) string {
	return column.Name + " " + string(column.Kind)
}

// describeImportTable returns the table the rows are written into.
func describeImportTable(held app.ImportRequest) string {
	return held.Plan.Table.Schema + "." + held.Plan.Table.Name
}

// readImportPath returns the path the form holds.
func readImportPath(overlay app.Overlay) string {
	if overlay.Draft != nil && overlay.Field == importFileField {
		return overlay.Draft.Text
	}
	return overlay.Import.Plan.Path
}

// FindImportProblem returns why the import cannot go on, and nothing where it can.
func FindImportProblem(overlay app.Overlay, dialect *query.Dialect) string {
	if strings.TrimSpace(readImportPath(overlay)) == "" {
		return "the file cannot be empty"
	}
	options := overlay.Import.Plan.Options
	if options.Format == load.FileCSV && len([]rune(options.Delimiter)) != 1 {
		return "the delimiter has to be one character"
	}
	if overlay.Import.Stage == app.ImportFile {
		return ""
	}
	return overlay.Import.Plan.FindPlanProblem(dialect)
}

// readActiveDialect returns how the server on screen writes SQL, or the default dialect
// where no connection is open.
func (model *Model) readActiveDialect() *query.Dialect {
	if connection := model.Active(); connection != nil {
		return connection.Session.Dialect()
	}
	return postgres.Dialect
}

// readFieldKey returns the key of the row the cursor stands on.
func readFieldKey(overlay app.Overlay) string {
	fields := BuildImportFields(overlay)
	if overlay.Field < 0 || overlay.Field >= len(fields) {
		return ""
	}
	return fields[overlay.Field].Key
}

// ReadImportField writes what was typed into the field under the cursor.
func ReadImportField(overlay *app.Overlay, written string) {
	fields := BuildImportFields(*overlay)
	if overlay.Field < 0 || overlay.Field >= len(fields) {
		return
	}
	switch fields[overlay.Field].Key {
	case "path":
		overlay.Import.Plan.Path = written
	case "table":
		readImportTable(overlay, written)
	case "delimiter":
		overlay.Import.Plan.Options.Delimiter = written
		overlay.Import.DelimiterChosen = true
	case "null-text":
		overlay.Import.Plan.Options.NullText = written
	}
}

// readImportTable writes the table the rows go into. A name with a dot in it names its
// schema as well, and a name without one keeps the schema the form opened with.
func readImportTable(overlay *app.Overlay, written string) {
	name := strings.TrimSpace(written)
	if schema, table, held := strings.Cut(name, "."); held {
		overlay.Import.Plan.Table.Schema = strings.TrimSpace(schema)
		overlay.Import.Plan.Table.Name = strings.TrimSpace(table)
		return
	}
	overlay.Import.Plan.Table.Name = name
}

// StepImportChoice returns the overlay with the next value of the field under the cursor. A
// field that is typed into steps through nothing.
func StepImportChoice(overlay *app.Overlay, step int) {
	fields := BuildImportFields(*overlay)
	if overlay.Field < 0 || overlay.Field >= len(fields) {
		return
	}
	field := fields[overlay.Field]
	if len(field.Choices) == 0 {
		return
	}

	at := 0
	for index, choice := range field.Choices {
		if choice == field.Value {
			at = index
		}
	}
	next := field.Choices[wrap(at+step, len(field.Choices))]

	if source, held := strings.CutPrefix(field.Key, mappingKeyPrefix); held {
		target := next
		if next == skipColumn {
			target = ""
		}
		overlay.Import.Plan.MapColumn(source, target)
		return
	}
	switch field.Key {
	case "format":
		if format, known := load.FindFormatNamed(next); known {
			overlay.Import.Plan.Options.Format = format
		}
		overlay.Import.FormatChosen = true
		overlay.Import.Stage = app.ImportFile
	case "header":
		overlay.Import.Plan.Options.HasHeader = next == "yes"
		overlay.Import.Stage = app.ImportFile
	}
}

// StepImportField moves the cursor to another row of the form and hands it the value of
// that row.
func StepImportField(overlay *app.Overlay, step int) {
	fields := BuildImportFields(*overlay)
	if len(fields) == 0 {
		return
	}
	if overlay.Draft != nil {
		ReadImportField(overlay, overlay.Draft.Text)
	}
	overlay.Field = wrap(overlay.Field+step, len(fields))

	// The draft belongs to the row the cursor left, so it is dropped before the value of
	// the row it moved to is read.
	overlay.Draft = nil
	value := BuildImportFields(*overlay)[overlay.Field].Value
	overlay.Draft = app.NewEditorBuffer(value, len(value))
}

// ApplyImportPath sets the format and the delimiter from the name of the file. A format the
// user chose themselves is kept.
func ApplyImportPath(overlay *app.Overlay) {
	held := load.BuildReadOptions(overlay.Import.Plan.Path)
	if !overlay.Import.FormatChosen {
		overlay.Import.Plan.Options.Format = held.Format
	}
	if !overlay.Import.DelimiterChosen {
		overlay.Import.Plan.Options.Delimiter = held.Delimiter
	}
}

// DescribeImportSummary returns what the import would do, in one line, for the review.
func DescribeImportSummary(held app.ImportRequest) string {
	mapped := len(held.Plan.ListMappedColumns())
	written := present.FormatCount(int64(held.Report.Rows - held.Report.Refused))
	return written + " of " + present.FormatCount(int64(held.Report.Rows)) +
		" rows into " + held.Plan.Table.Name + ", " + strconv.Itoa(mapped) + " columns"
}
