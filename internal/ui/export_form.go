package ui

import (
	"strings"

	"github.com/turanmahmudov/masume/internal/app"
	"github.com/turanmahmudov/masume/internal/present"
	"github.com/turanmahmudov/masume/internal/query/result"
)

// The form the export dialog draws: one row per setting, each typed into or stepped
// through. The CSV settings only stand where CSV is the format.

// The two sets of rows an export writes.
const (
	exportLoaded = "loaded so far"
	exportEvery  = "every row"
)

// exportPathField is the row of the form the file path is written on.
const exportPathField = 0

// BuildExportFields returns the rows of the form as the overlay stands now.
func BuildExportFields(overlay app.Overlay) []DialogField {
	// The count says what "loaded so far" means, because the grid holds one page and
	// not the whole relation.
	loaded := exportLoaded + " (" + present.FormatCount(int64(overlay.Export.RowCount)) + ")"
	rows := loaded
	if overlay.Export.WholeRead {
		rows = exportEvery
	}

	formats := make([]string, 0, len(result.ExportFormats))
	for _, format := range result.ExportFormats {
		formats = append(formats, string(format))
	}

	fields := []DialogField{
		{Key: "path", Label: "file", Value: readExportPath(overlay)},
		{Key: "format", Label: "format", Value: string(overlay.Export.Format), Choices: formats},
		{Key: "scope", Label: "rows", Value: rows, Choices: []string{loaded, exportEvery}},
	}
	if overlay.Export.Format != result.ExportCSV {
		return fields
	}

	quotings := make([]string, 0, len(result.CSVQuotings))
	for _, quoting := range result.CSVQuotings {
		quotings = append(quotings, string(quoting))
	}
	endings := make([]string, 0, len(result.CSVLineEndings))
	for _, ending := range result.CSVLineEndings {
		endings = append(endings, string(ending))
	}
	return append(fields,
		DialogField{Key: "delimiter", Label: "delimiter", Value: overlay.Export.CSV.Delimiter},
		DialogField{
			Key: "header", Label: "header",
			Value: describeYesOrNo(overlay.Export.CSV.Header), Choices: yesOrNo,
		},
		DialogField{
			Key: "quoting", Label: "quote",
			Value: string(overlay.Export.CSV.Quoting), Choices: quotings,
		},
		DialogField{
			Key: "line-ending", Label: "line ending",
			Value: string(overlay.Export.CSV.LineEnding), Choices: endings,
		},
		DialogField{Key: "null-text", Label: "null as", Value: overlay.Export.CSV.NullText},
		DialogField{
			Key: "formulas", Label: "guard formulas",
			Value: describeYesOrNo(overlay.Export.CSV.SanitizeFormulas), Choices: yesOrNo,
		},
	)
}

// readExportPath returns the path the form holds. The buffer follows the cursor, so it holds
// the path only while the cursor stands on that row.
func readExportPath(overlay app.Overlay) string {
	if overlay.Draft != nil && overlay.Field == exportPathField {
		return overlay.Draft.Text
	}
	return overlay.Export.Path
}

// FindExportProblem returns why the export cannot be written, and nothing where it can.
func FindExportProblem(overlay app.Overlay) string {
	if strings.TrimSpace(readExportPath(overlay)) == "" {
		return "the file cannot be empty"
	}
	if overlay.Export.Format == result.ExportCSV && len([]rune(overlay.Export.CSV.Delimiter)) != 1 {
		return "the delimiter has to be one character"
	}
	return ""
}

// StepExportChoice returns the overlay with the next value of the field under the
// cursor. A field that is typed into steps through nothing.
func StepExportChoice(overlay *app.Overlay, step int) {
	fields := BuildExportFields(*overlay)
	if overlay.Field < 0 || overlay.Field >= len(fields) {
		return
	}
	field := fields[overlay.Field]
	if len(field.Choices) == 0 {
		return
	}

	// A value that is none of the choices would step from -1 and always land on the
	// first choice.
	at := 0
	for index, choice := range field.Choices {
		if choice == field.Value {
			at = index
		}
	}
	next := field.Choices[((at+step)%len(field.Choices)+len(field.Choices))%len(field.Choices)]

	switch field.Key {
	case "format":
		applyExportFormat(overlay, result.ExportFormat(next))
	case "scope":
		overlay.Export.WholeRead = next == exportEvery
	case "header":
		overlay.Export.CSV.Header = next == "yes"
	case "formulas":
		overlay.Export.CSV.SanitizeFormulas = next == "yes"
	case "quoting":
		overlay.Export.CSV.Quoting = result.CSVQuoting(next)
	case "line-ending":
		overlay.Export.CSV.LineEnding = result.CSVLineEnding(next)
	}
}

// applyExportFormat takes the new format, and changes the extension of the file with
// it unless the user typed their own.
func applyExportFormat(overlay *app.Overlay, format result.ExportFormat) {
	if format == overlay.Export.Format {
		return
	}
	path := readExportPath(*overlay)
	if strings.HasSuffix(path, "."+string(overlay.Export.Format)) {
		path = strings.TrimSuffix(path, string(overlay.Export.Format)) + string(format)
	}
	overlay.Export.Format = format
	overlay.Export.Path = path
	if overlay.Draft != nil {
		overlay.Draft = app.NewEditorBuffer(path, len(path))
	}
}

// ReadExportField writes what the user typed into the field under the cursor.
func ReadExportField(overlay *app.Overlay, written string) {
	fields := BuildExportFields(*overlay)
	if overlay.Field < 0 || overlay.Field >= len(fields) {
		return
	}
	switch fields[overlay.Field].Key {
	case "path":
		overlay.Export.Path = written
	case "delimiter":
		overlay.Export.CSV.Delimiter = written
	case "null-text":
		overlay.Export.CSV.NullText = written
	}
}

// StepExportField moves the cursor to another row of the form, and wraps at each end.
// The buffer follows the cursor, because only one field is typed into at a time.
func StepExportField(overlay *app.Overlay, step int) {
	fields := BuildExportFields(*overlay)
	if len(fields) == 0 {
		return
	}
	overlay.Field = wrap(overlay.Field+step, len(fields))
	value := fields[overlay.Field].Value
	overlay.Draft = app.NewEditorBuffer(value, len(value))
}
