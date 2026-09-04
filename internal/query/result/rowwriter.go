package result

import (
	"io"

	"github.com/turanmahmudov/masume/internal/query"
)

// One stream of rows. The client and a run without a screen both write a result through this
// one interface, so neither carries the begin, rows and end of a format itself.

// RowWriter writes the rows of one result to a stream.
type RowWriter interface {
	// WriteRows writes one batch. The columns are the same for every batch of a result.
	WriteRows(rows [][]any, columns []query.ResultColumn) error
	// Close writes the end of the file. A result of no rows still writes a file that reads
	// back: an empty JSON array, and a CSV of its header alone.
	Close(columns []query.ResultColumn) error
}

// CreateRowWriter returns the writer of that format.
func CreateRowWriter(format ExportFormat, csv CSVOptions, out io.Writer) RowWriter {
	return &textRowWriter{out: out, writer: CreateExportWriter(format, csv)}
}

// textRowWriter writes a format that is built as text.
type textRowWriter struct {
	out    io.Writer
	writer ExportWriter
	began  bool
}

func (writer *textRowWriter) begin(columns []query.ResultColumn) error {
	if writer.began {
		return nil
	}
	writer.began = true
	_, err := io.WriteString(writer.out, writer.writer.Begin(columns))
	return err
}

func (writer *textRowWriter) WriteRows(rows [][]any, columns []query.ResultColumn) error {
	if err := writer.begin(columns); err != nil {
		return err
	}
	_, err := io.WriteString(writer.out, writer.writer.WriteRows(rows, columns))
	return err
}

func (writer *textRowWriter) Close(columns []query.ResultColumn) error {
	if err := writer.begin(columns); err != nil {
		return err
	}
	_, err := io.WriteString(writer.out, writer.writer.End())
	return err
}
