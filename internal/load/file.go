// Package load reads a data file into a table: it samples the file, infers a type for each
// column, maps the columns onto a table, checks every row, and builds the statements that
// write them. Nothing here reaches a server.
package load

import (
	"bufio"
	"encoding/csv"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	"github.com/turanmahmudov/masume/internal/core"
)

// FileFormat is the shape of a file an import reads.
type FileFormat string

const (
	FileCSV  FileFormat = "csv"
	FileJSON FileFormat = "json"
)

// FileFormats lists the formats, the one a file without a known extension is read as first.
var FileFormats = []FileFormat{FileCSV, FileJSON}

// fileExtensions give the format of each extension a data file uses.
var fileExtensions = map[string]FileFormat{
	".csv": FileCSV, ".tsv": FileCSV, ".txt": FileCSV,
	".json": FileJSON, ".jsonl": FileJSON, ".ndjson": FileJSON,
}

// ListFileExtensions returns the extensions of a file an import reads.
func ListFileExtensions() []string {
	held := make([]string, 0, len(fileExtensions))
	for extension := range fileExtensions {
		held = append(held, extension)
	}
	slices.Sort(held)
	return held
}

// tabDelimited are the extensions of a file whose fields are separated by a tab.
var tabDelimited = map[string]bool{".tsv": true}

// byteOrderMark is the mark a writer can put at the head of a UTF-8 file.
const byteOrderMark = "\ufeff"

// ErrStopWalk stops a walk of a file from the row it is handed.
var ErrStopWalk = errors.New("stop reading the file")

// FileError is a file an import cannot read.
type FileError struct{ Reason string }

func (err FileError) Error() string { return err.Reason }

func failFile(format string, parts ...any) error {
	return FileError{Reason: fmt.Sprintf(format, parts...)}
}

// ReadOptions holds how one file is read.
type ReadOptions struct {
	Format FileFormat
	// One character, which separates the fields of a CSV.
	Delimiter string
	HasHeader bool
	// The text a CSV writes a value that is not there with, for example `\N`. A field
	// that is empty is read as a value that is not there whatever this holds.
	NullText string
}

// DefaultReadOptions returns how a file is read where nothing was chosen.
func DefaultReadOptions() ReadOptions {
	return ReadOptions{Format: FileCSV, Delimiter: ",", HasHeader: true, NullText: `\N`}
}

// FindFileFormat returns the format of the file, read from its extension. A name with no
// known extension is read as a CSV, which is what a data file mostly is.
func FindFileFormat(path string) FileFormat {
	if format, known := fileExtensions[strings.ToLower(filepath.Ext(path))]; known {
		return format
	}
	return FileCSV
}

// FindFormatNamed parses the text as a format.
func FindFormatNamed(written string) (FileFormat, bool) {
	return core.FindAllowed(FileFormats, strings.ToLower(strings.TrimSpace(written)))
}

// BuildReadOptions returns how that file is read: its format, and a tab for a file whose
// extension marks its fields as separated by one.
func BuildReadOptions(path string) ReadOptions {
	options := DefaultReadOptions()
	options.Format = FindFileFormat(path)
	if tabDelimited[strings.ToLower(filepath.Ext(path))] {
		options.Delimiter = "\t"
	}
	return options
}

// Row is one row of a file, with the line of the file it was read from.
type Row struct {
	// Line is the line the row starts on, counted from one.
	Line   int
	Values []any
}

// WalkFile reads the file: the names of its columns first, and then every row. The names
// are handed over again, longer than before, wherever a row adds a column the ones before
// it did not hold, which a file of documents can do. A row is as long as the names were
// when it was read, so a row from before a column appeared holds no value for it.
//
// It returns nothing where the whole file was read, and the error the caller answered with
// where the caller stopped it.
func WalkFile(
	path string, options ReadOptions,
	onNames func(names []string) error, onRow func(row Row) error,
) error {
	file, err := os.Open(core.ExpandHomePath(path))
	if err != nil {
		return failFile("%s cannot be read: %v", path, err)
	}
	defer func() { _ = file.Close() }()

	if options.Format == FileJSON {
		return walkJSON(file, onNames, onRow)
	}
	return walkCSV(file, options, onNames, onRow)
}

// readDelimiter returns the one character that separates the fields of a CSV.
func readDelimiter(written string) (rune, error) {
	held := []rune(written)
	if len(held) != 1 {
		return 0, failFile("the delimiter has to be one character")
	}
	return held[0], nil
}

// buildColumnNames returns the names of the columns of a CSV: the header row, with a name
// for every field the header leaves blank, and a name for every repeat. A file with no
// header row is named by position.
func buildColumnNames(header []string, hasHeader bool) []string {
	names := make([]string, 0, len(header))
	taken := map[string]bool{}
	for at, written := range header {
		name := strings.TrimSpace(strings.TrimPrefix(written, byteOrderMark))
		if !hasHeader || name == "" {
			name = "column_" + strconv.Itoa(at+1)
		}
		unique := name
		for suffix := 2; taken[strings.ToLower(unique)]; suffix++ {
			unique = name + "_" + strconv.Itoa(suffix)
		}
		taken[strings.ToLower(unique)] = true
		names = append(names, unique)
	}
	return names
}

// readCSVCell returns one field as the value it holds. An empty field, and a field that
// holds the null text, are a value that is not there.
func readCSVCell(field string, nullText string) any {
	if field == "" || (nullText != "" && field == nullText) {
		return nil
	}
	return field
}

func walkCSV(
	file io.Reader, options ReadOptions,
	onNames func([]string) error, onRow func(Row) error,
) error {
	delimiter, err := readDelimiter(options.Delimiter)
	if err != nil {
		return err
	}

	reader := csv.NewReader(file)
	reader.Comma = delimiter
	// A row of the wrong length is reported by this package, not by the reader.
	reader.FieldsPerRecord = -1
	reader.ReuseRecord = false

	// The reader panics for the line of a record it does not hold.
	readLine := func(record []string) int {
		if len(record) == 0 {
			return 0
		}
		line, _ := reader.FieldPos(0)
		return line
	}

	first, err := reader.Read()
	if errors.Is(err, io.EOF) {
		return failFile("the file holds no row")
	}
	if err != nil {
		return describeCSVFault(err)
	}

	names := buildColumnNames(first, options.HasHeader)
	if err := onNames(names); err != nil {
		return err
	}

	if !options.HasHeader {
		if err := onRow(buildCSVRow(readLine(first), first, names, options)); err != nil {
			return err
		}
	}

	for {
		record, readErr := reader.Read()
		if errors.Is(readErr, io.EOF) {
			return nil
		}
		if readErr != nil {
			return describeCSVFault(readErr)
		}
		if err := onRow(
			buildCSVRow(readLine(record), record, names, options)); err != nil {
			return err
		}
	}
}

// describeCSVFault returns why a line of a CSV cannot be read.
func describeCSVFault(err error) error {
	if fault, is := errors.AsType[*csv.ParseError](err); is {
		return failFile("line %d cannot be read: %v", fault.Line, fault.Err)
	}
	return failFile("the file cannot be read: %v", err)
}

// buildCSVRow returns one record as a row. A record shorter than the header is filled with
// values that are not there, and a longer one keeps its extra fields.
func buildCSVRow(line int, record, names []string, options ReadOptions) Row {
	values := make([]any, 0, max(len(record), len(names)))
	for _, field := range record {
		values = append(values, readCSVCell(field, options.NullText))
	}
	for len(values) < len(names) {
		values = append(values, nil)
	}
	return Row{Line: line, Values: values}
}

// walkJSON reads an array of objects, or one object per line. The names of the columns are
// the names of the first object, in the order it writes them, and every name a later object
// adds.
func walkJSON(file io.Reader, onNames func([]string) error, onRow func(Row) error) error {
	counter := &lineCounter{reader: cutByteOrderMark(file)}
	decoder := json.NewDecoder(counter)
	decoder.UseNumber()
	readLine := func() int { return counter.readLineAt(decoder.InputOffset()) }

	opening, err := decoder.Token()
	if errors.Is(err, io.EOF) {
		return failFile("the file holds no document")
	}
	if err != nil {
		return failFile("the file is no JSON: %v", err)
	}
	delimiter, isDelimiter := opening.(json.Delim)
	if !isDelimiter || (delimiter != '[' && delimiter != '{') {
		return failFile("the file holds no array of documents and no document")
	}
	inArray := delimiter == '['

	names := []string{}
	held := map[string]bool{}
	reported := false

	readOne := func(line int, first json.Token) error {
		fields, order, readErr := readJSONObject(decoder, first)
		if readErr != nil {
			return readErr
		}
		grew := false
		for _, name := range order {
			if !held[name] {
				held[name] = true
				names = append(names, name)
				grew = true
			}
		}
		if grew || !reported {
			reported = true
			if err := onNames(names); err != nil {
				return err
			}
		}
		values := make([]any, 0, len(names))
		for _, name := range names {
			values = append(values, fields[name])
		}
		return onRow(Row{Line: line, Values: values})
	}

	if !inArray {
		if err := readOne(readLine(), opening); err != nil {
			return err
		}
	}
	for {
		token, tokenErr := decoder.Token()
		if errors.Is(tokenErr, io.EOF) {
			break
		}
		if tokenErr != nil {
			return failFile("the file is no JSON: %v", tokenErr)
		}
		if delimiter, isDelimiter := token.(json.Delim); isDelimiter && delimiter == ']' {
			if decoder.More() {
				return failFile("the file holds more than the array of documents")
			}
			break
		}
		if err := readOne(readLine(), token); err != nil {
			return err
		}
	}

	if !reported {
		return failFile("the file holds no document")
	}
	return nil
}

// readJSONObject reads one object into its fields and the order it writes them in. The
// opening brace is already read.
func readJSONObject(
	decoder *json.Decoder, opening json.Token,
) (map[string]any, []string, error) {
	brace, isDelimiter := opening.(json.Delim)
	if !isDelimiter || brace != '{' {
		return nil, nil, failFile("the file holds a value that is no document")
	}

	fields := map[string]any{}
	order := []string{}
	for {
		token, err := decoder.Token()
		if err != nil {
			return nil, nil, failFile("the file is no JSON: %v", err)
		}
		if end, isEnd := token.(json.Delim); isEnd && end == '}' {
			return fields, order, nil
		}
		name, isName := token.(string)
		if !isName {
			return nil, nil, failFile("the file holds a document with no field name")
		}

		value, valueErr := readJSONValue(decoder)
		if valueErr != nil {
			return nil, nil, valueErr
		}
		if _, repeated := fields[name]; !repeated {
			order = append(order, name)
		}
		if raw, isRaw := value.(json.RawMessage); isRaw {
			value = string(raw)
		}
		fields[name] = value
	}
}

// readJSONValue reads one value of a document. An object and an array come back as
// json.RawMessage holding the text they were written as.
func readJSONValue(decoder *json.Decoder) (any, error) {
	token, err := decoder.Token()
	if err != nil {
		return nil, failFile("the file is no JSON: %v", err)
	}
	delimiter, isDelimiter := token.(json.Delim)
	if !isDelimiter {
		return token, nil
	}

	closing := json.Delim('}')
	written := &strings.Builder{}
	if delimiter == '[' {
		closing = ']'
	}
	written.WriteString(string(delimiter))
	if err := writeJSONRest(decoder, written, closing); err != nil {
		return nil, err
	}
	return json.RawMessage(written.String()), nil
}

// writeJSONInto writes one value of a nested document.
func writeJSONInto(written *strings.Builder, value any) {
	if raw, isRaw := value.(json.RawMessage); isRaw {
		written.Write(raw)
		return
	}
	written.WriteString(core.WriteJSONValue(value))
}

// writeJSONRest writes the rest of an object or an array as text.
func writeJSONRest(decoder *json.Decoder, written *strings.Builder, closing json.Delim) error {
	first := true
	for {
		if !decoder.More() {
			token, err := decoder.Token()
			if err != nil {
				return failFile("the file is no JSON: %v", err)
			}
			if end, isEnd := token.(json.Delim); isEnd && end == closing {
				written.WriteString(string(closing))
				return nil
			}
			return failFile("the file is no JSON")
		}
		if !first {
			written.WriteString(",")
		}
		first = false

		if closing == '}' {
			name, err := decoder.Token()
			if err != nil {
				return failFile("the file is no JSON: %v", err)
			}
			text, isText := name.(string)
			if !isText {
				return failFile("the file holds a document with no field name")
			}
			written.WriteString(core.WriteJSONText(text) + ":")
		}
		value, err := readJSONValue(decoder)
		if err != nil {
			return err
		}
		writeJSONInto(written, value)
	}
}

// cutByteOrderMark returns the file without the mark a writer can put at its head.
func cutByteOrderMark(file io.Reader) io.Reader {
	buffered := bufio.NewReader(file)
	head, err := buffered.Peek(len(byteOrderMark))
	if err == nil && string(head) == byteOrderMark {
		_, _ = buffered.Discard(len(byteOrderMark))
	}
	return buffered
}

// lineCounter counts the lines a reader hands over.
type lineCounter struct {
	reader io.Reader
	breaks []int64
	read   int64
}

func (counter *lineCounter) Read(into []byte) (int, error) {
	count, err := counter.reader.Read(into)
	for at := 0; at < count; at++ {
		if into[at] == '\n' {
			counter.breaks = append(counter.breaks, counter.read+int64(at))
		}
	}
	counter.read += int64(count)
	return count, err
}

// readLineAt returns the line the byte at an offset belongs to, counted from one.
func (counter *lineCounter) readLineAt(offset int64) int {
	before, _ := slices.BinarySearch(counter.breaks, offset)
	return before + 1
}
