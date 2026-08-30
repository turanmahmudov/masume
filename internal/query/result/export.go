package result

import (
	"encoding/json"
	"fmt"
	"math"
	"regexp"
	"strings"

	"github.com/turanmahmudov/masume/internal/core"
	"github.com/turanmahmudov/masume/internal/query"
	"github.com/turanmahmudov/masume/internal/query/build"
)

// ExportFormat is a format an export can write.
type ExportFormat string

// The two formats a reader may choose.
const (
	ExportCSV  ExportFormat = "csv"
	ExportJSON ExportFormat = "json"
)

// ExportFormats lists the formats in the order the form steps through them.
var ExportFormats = []ExportFormat{ExportCSV, ExportJSON}

// BuildRecordKeys names each column once. A join can return one column name twice,
// so a repeat gets a suffix.
func BuildRecordKeys(columns []query.ResultColumn) []string {
	used := map[string]bool{}
	keys := make([]string, 0, len(columns))
	for _, column := range columns {
		key := column.Name
		suffix := 1
		for used[key] {
			suffix++
			key = fmt.Sprintf("%s_%d", column.Name, suffix)
		}
		used[key] = true
		keys = append(keys, key)
	}
	return keys
}

// CSVQuoting is the quoting rule: every field, or only the ones that need it.
type CSVQuoting string

// The two quoting rules a reader may choose.
const (
	QuoteAsNeeded CSVQuoting = "as-needed"
	QuoteAlways   CSVQuoting = "always"
)

// CSVQuotings lists the rules in the order the form steps through them.
var CSVQuotings = []CSVQuoting{QuoteAsNeeded, QuoteAlways}

// CSVLineEnding is the line ending a CSV export writes.
type CSVLineEnding string

// The two line endings a reader may choose.
const (
	EndingLf   CSVLineEnding = "lf"
	EndingCrlf CSVLineEnding = "crlf"
)

// CSVLineEndings lists the endings in the order the form steps through them.
var CSVLineEndings = []CSVLineEnding{EndingLf, EndingCrlf}

// CSVOptions says how a CSV export is written.
type CSVOptions struct {
	Delimiter string
	Header    bool
	Quoting   CSVQuoting
	// A spreadsheet on Windows expects CRLF.
	LineEnding CSVLineEnding
	// The text for a null. Empty by default, which reads as no value.
	NullText string
	// Whether a value a spreadsheet would run as a formula is escaped.
	SanitizeFormulas bool
}

// DefaultCSVOptions holds the options an export opens with.
func DefaultCSVOptions() CSVOptions {
	return CSVOptions{
		Delimiter: ",", Header: true, Quoting: QuoteAsNeeded, LineEnding: EndingLf,
		SanitizeFormulas: true,
	}
}

var lineEndings = map[CSVLineEnding]string{EndingLf: "\n", EndingCrlf: "\r\n"}

// formulaStarts are the characters a spreadsheet reads as the start of a formula.
var formulaStarts = []string{"=", "+", "-", "@", "\t", "\r"}

var plainNumber = regexp.MustCompile(`^[+-]?(\d+\.?\d*|\.\d+)([eE][+-]?\d+)?$`)

// sanitizeFormula makes a field text. A field that starts with `=`, `+`, `-` or `@`
// is a formula in Excel and Sheets, so a leading quote makes it text. A number is
// left as it is.
func sanitizeFormula(value string) string {
	if value == "" || plainNumber.MatchString(value) {
		return value
	}
	for _, start := range formulaStarts {
		if strings.HasPrefix(value, start) {
			return "'" + value
		}
	}
	return value
}

var csvSpecial = regexp.MustCompile(`["\n\r]`)

// escapeCSVField quotes a field that holds the delimiter, a quote or a line break.
func escapeCSVField(value string, options CSVOptions) string {
	written := value
	if options.SanitizeFormulas {
		written = sanitizeFormula(value)
	}
	needsQuotes := options.Quoting == QuoteAlways ||
		strings.Contains(written, options.Delimiter) || csvSpecial.MatchString(written)
	if !needsQuotes {
		return written
	}
	return `"` + strings.ReplaceAll(written, `"`, `""`) + `"`
}

func buildCSVHeader(columns []query.ResultColumn, options CSVOptions) string {
	written := make([]string, 0, len(columns))
	for _, column := range columns {
		written = append(written, escapeCSVField(column.Name, options))
	}
	return strings.Join(written, options.Delimiter)
}

func buildCSVRow(row []any, columns []query.ResultColumn, options CSVOptions) string {
	written := make([]string, 0, len(row))
	for index, cell := range row {
		if cell == nil {
			written = append(written, escapeCSVField(options.NullText, options))
			continue
		}
		dataType := ""
		if index < len(columns) {
			dataType = columns[index].DataType
		}
		written = append(written, escapeCSVField(core.FormatCell(cell, dataType), options))
	}
	return strings.Join(written, options.Delimiter)
}

// castToJSONValue keeps a value with a JSON type as it is. The rest are written as in
// the CSV export.
func castToJSONValue(value any, dataType string) any {
	if value == nil {
		return nil
	}
	// A column that holds a document is written as that document, not as the text of
	// one, so a reader of the file does not have to unescape it first.
	if core.IsDocumentType(dataType) {
		if embedded, isJSON := embedJSONDocument(value, dataType); isJSON {
			return embedded
		}
	}
	// JSON has no form for NaN and no form for an infinity, so those are written as the
	// text of the cell rather than refused by the encoder.
	if held, isFloat := readFloatValue(value); isFloat {
		if math.IsNaN(held) || math.IsInf(held, 0) {
			return core.FormatCell(value, dataType)
		}
	}
	switch value.(type) {
	case string, bool, float32, float64,
		int, int8, int16, int32, int64, uint, uint8, uint16, uint32, uint64:
		if _, isBytes := value.([]byte); isBytes {
			return core.FormatCell(value, dataType)
		}
		return value
	}
	return core.FormatCell(value, dataType)
}

// readFloatValue returns the value as a float, and reports nothing where it is no float.
func readFloatValue(value any) (float64, bool) {
	switch held := value.(type) {
	case float32:
		return float64(held), true
	case float64:
		return held, true
	}
	return 0, false
}

// embedJSONDocument returns the value as the JSON it holds, and reports nothing where the
// text is no document the file can carry.
func embedJSONDocument(value any, dataType string) (json.RawMessage, bool) {
	written := core.FormatCell(value, dataType)
	if !json.Valid([]byte(written)) {
		return nil, false
	}
	return json.RawMessage(written), true
}

func buildJSONRecord(row []any, columns []query.ResultColumn, keys []string) map[string]any {
	record := map[string]any{}
	for index, column := range columns {
		key := column.Name
		if index < len(keys) {
			key = keys[index]
		}
		var cell any
		if index < len(row) {
			cell = row[index]
		}
		record[key] = castToJSONValue(cell, column.DataType)
	}
	return record
}

// buildTextRecord writes every cell of the row as text, which every encoder takes.
func buildTextRecord(
	row []any, columns []query.ResultColumn, keys []string,
) map[string]string {
	record := map[string]string{}
	for index, column := range columns {
		key := column.Name
		if index < len(keys) {
			key = keys[index]
		}
		var cell any
		if index < len(row) {
			cell = row[index]
		}
		record[key] = core.FormatCell(cell, column.DataType)
	}
	return record
}

// ExportWriter writes an export a batch at a time, so a large relation is never held
// whole.
type ExportWriter interface {
	// Begin writes the first text of the file, built from the columns of the result.
	Begin(columns []query.ResultColumn) string
	WriteRows(rows [][]any, columns []query.ResultColumn) string
	// End writes the last text of the file, which JSON needs and CSV does not.
	End() string
}

type csvWriter struct{ options CSVOptions }

func (writer *csvWriter) Begin(columns []query.ResultColumn) string {
	if !writer.options.Header {
		return ""
	}
	return buildCSVHeader(columns, writer.options) + lineEndings[writer.options.LineEnding]
}

func (writer *csvWriter) WriteRows(rows [][]any, columns []query.ResultColumn) string {
	if len(rows) == 0 {
		return ""
	}
	ending := lineEndings[writer.options.LineEnding]
	written := make([]string, 0, len(rows))
	for _, row := range rows {
		written = append(written, buildCSVRow(row, columns, writer.options))
	}
	return strings.Join(written, ending) + ending
}

func (writer *csvWriter) End() string { return "" }

type jsonWriter struct{ written int }

func (writer *jsonWriter) Begin([]query.ResultColumn) string { return "[\n" }

func (writer *jsonWriter) WriteRows(rows [][]any, columns []query.ResultColumn) string {
	if len(rows) == 0 {
		return ""
	}
	keys := BuildRecordKeys(columns)
	records := make([]string, 0, len(rows))
	for _, row := range rows {
		encoded, err := json.Marshal(buildJSONRecord(row, columns, keys))
		if err != nil {
			// A row must never go missing from an export, so a record this encoder
			// refuses is written again with every cell as its text.
			encoded, _ = json.Marshal(buildTextRecord(row, columns, keys))
		}
		records = append(records, "  "+string(encoded))
	}
	separator := ",\n"
	if writer.written == 0 {
		separator = ""
	}
	writer.written += len(records)
	return separator + strings.Join(records, ",\n")
}

func (writer *jsonWriter) End() string {
	if writer.written == 0 {
		return "]\n"
	}
	return "\n]\n"
}

// CreateExportWriter returns the writer for that format.
func CreateExportWriter(format ExportFormat, csv CSVOptions) ExportWriter {
	if format == ExportCSV {
		return &csvWriter{options: csv}
	}
	return &jsonWriter{}
}

// ExportProgressRows is how many rows an export writes between two progress reports.
const ExportProgressRows = 10_000

var unsafeFilenameCharacters = regexp.MustCompile(`[^\w.-]+`)

// BuildExportFilename names the file, from the label and the moment of the export.
func BuildExportFilename(label string, format ExportFormat, stamp string) string {
	safeLabel := strings.Trim(unsafeFilenameCharacters.ReplaceAllString(label, "_"), "_")
	if safeLabel == "" {
		safeLabel = "result"
	}
	return fmt.Sprintf("%s-%s.%s", safeLabel, stamp, format)
}

// renderSQLLiteral writes a value inside a script. A missing value is written as a null.
func renderSQLLiteral(value any, dataType string, dialect *query.Dialect) string {
	if value == nil {
		return "null"
	}
	return build.RenderLiteral(value, dialect, dataType)
}

var markdownBreak = regexp.MustCompile(`\s*\n\s*`)

// BuildMarkdown writes a Markdown table. A pipe in a value is escaped, and a newline
// becomes a space.
func BuildMarkdown(columns []query.ResultColumn, rows [][]any) string {
	escapeCell := func(text string) string {
		return markdownBreak.ReplaceAllString(strings.ReplaceAll(text, "|", `\|`), " ")
	}
	header := make([]string, 0, len(columns))
	separator := make([]string, 0, len(columns))
	for _, column := range columns {
		header = append(header, escapeCell(column.Name))
		separator = append(separator, "---")
	}

	lines := []string{
		"| " + strings.Join(header, " | ") + " |",
		"| " + strings.Join(separator, " | ") + " |",
	}
	for _, row := range rows {
		cells := make([]string, 0, len(row))
		for index, value := range row {
			dataType := ""
			if index < len(columns) {
				dataType = columns[index].DataType
			}
			cells = append(cells, escapeCell(core.FormatCell(value, dataType)))
		}
		lines = append(lines, "| "+strings.Join(cells, " | ")+" |")
	}
	return strings.Join(lines, "\n")
}

// BuildInClause writes one column as `('a', 'b')` for a WHERE. Each value comes once,
// and no nulls, because IN never matches a null.
func BuildInClause(
	columns []query.ResultColumn, rows [][]any, columnIndex int, dialect *query.Dialect,
) string {
	dataType := ""
	if columnIndex >= 0 && columnIndex < len(columns) {
		dataType = columns[columnIndex].DataType
	}
	written := []string{}
	seen := map[string]bool{}

	for _, row := range rows {
		if columnIndex < 0 || columnIndex >= len(row) {
			continue
		}
		value := row[columnIndex]
		if value == nil {
			continue
		}
		literal := renderSQLLiteral(value, dataType, dialect)
		if seen[literal] {
			continue
		}
		seen[literal] = true
		written = append(written, literal)
	}
	if len(written) == 0 {
		return ""
	}
	return "(" + strings.Join(written, ", ") + ")"
}

// BuildInsertScript writes the result as INSERT statements, one per row.
func BuildInsertScript(
	columns []query.ResultColumn, rows [][]any, table query.QualifiedName, dialect *query.Dialect,
) string {
	if len(rows) == 0 {
		return ""
	}
	target := dialect.BuildQualifiedName(table)
	names := make([]string, 0, len(columns))
	for _, column := range columns {
		names = append(names, dialect.QuoteIdentifier(column.Name))
	}

	statements := make([]string, 0, len(rows))
	for _, row := range rows {
		values := make([]string, 0, len(columns))
		for index, column := range columns {
			var cell any
			if index < len(row) {
				cell = row[index]
			}
			values = append(values, renderSQLLiteral(cell, column.DataType, dialect))
		}
		statements = append(statements, fmt.Sprintf("insert into %s (%s) values (%s);",
			target, strings.Join(names, ", "), strings.Join(values, ", ")))
	}
	return strings.Join(statements, "\n") + "\n"
}
