package headless

import (
	"fmt"
	"io"
	"strings"

	"github.com/turanmahmudov/masume/internal/core"
	"github.com/turanmahmudov/masume/internal/present"
	"github.com/turanmahmudov/masume/internal/query"
	"github.com/turanmahmudov/masume/internal/query/result"
)

// Where the rows of a result go. The rows are handed over as they arrive and never held
// twice.

// rowSink takes the rows of one result and writes them in one format.
type rowSink interface {
	// TakeRows takes one batch. The columns are the same for every batch of a result.
	TakeRows(rows [][]any, columns []query.ResultColumn) error
	// Finish writes what is left, and the shape of a result that held no rows at all.
	Finish(columns []query.ResultColumn) error
}

// createRowSink returns the sink of that format.
func createRowSink(format Format, out io.Writer) rowSink {
	switch format {
	case FormatCSV, FormatJSON:
		return &streamingSink{
			out:    out,
			writer: result.CreateExportWriter(resolveExportFormat(format), result.DefaultCSVOptions()),
		}
	case FormatMarkdown:
		return &heldSink{out: out, write: result.BuildMarkdown}
	default:
		return &heldSink{out: out, write: buildPlainTable}
	}
}

// resolveExportFormat returns the export writer of the format.
func resolveExportFormat(format Format) result.ExportFormat {
	if format == FormatCSV {
		return result.ExportCSV
	}
	return result.ExportJSON
}

// streamingSink writes each batch as it arrives. CSV and JSON are written this way.
type streamingSink struct {
	out    io.Writer
	writer result.ExportWriter
	began  bool
}

func (sink *streamingSink) begin(columns []query.ResultColumn) error {
	if sink.began {
		return nil
	}
	sink.began = true
	_, err := io.WriteString(sink.out, sink.writer.Begin(columns))
	return err
}

func (sink *streamingSink) TakeRows(rows [][]any, columns []query.ResultColumn) error {
	if err := sink.begin(columns); err != nil {
		return err
	}
	_, err := io.WriteString(sink.out, sink.writer.WriteRows(rows, columns))
	return err
}

// Finish writes the end of the document. A result of no rows still writes the shape of one:
// a CSV of its header alone, and an empty JSON array.
func (sink *streamingSink) Finish(columns []query.ResultColumn) error {
	if err := sink.begin(columns); err != nil {
		return err
	}
	_, err := io.WriteString(sink.out, sink.writer.End())
	return err
}

// heldSink holds every row until the result is whole, because its format measures the widest
// cell of a column before it writes the first one.
type heldSink struct {
	out   io.Writer
	write func(columns []query.ResultColumn, rows [][]any) string
	rows  [][]any
}

func (sink *heldSink) TakeRows(rows [][]any, _ []query.ResultColumn) error {
	sink.rows = append(sink.rows, rows...)
	return nil
}

func (sink *heldSink) Finish(columns []query.ResultColumn) error {
	_, err := fmt.Fprintln(sink.out, strings.TrimRight(sink.write(columns, sink.rows), "\n"))
	return err
}

// buildPlainTable writes the rows as a table of spaces, with each column as wide as its
// widest cell.
func buildPlainTable(columns []query.ResultColumn, rows [][]any) string {
	widths := make([]int, len(columns))
	for at, column := range columns {
		widths[at] = present.MeasureText(column.Name)
	}

	written := make([][]string, 0, len(rows))
	for _, row := range rows {
		cells := make([]string, 0, len(columns))
		for at := range columns {
			cell := ""
			if at < len(row) {
				cell = present.SafeText(core.FormatCell(row[at], columns[at].DataType))
			}
			widths[at] = max(widths[at], present.MeasureText(cell))
			cells = append(cells, cell)
		}
		written = append(written, cells)
	}

	heads := make([]string, 0, len(columns))
	rules := make([]string, 0, len(columns))
	for at, column := range columns {
		heads = append(heads, present.PadText(column.Name, widths[at]))
		rules = append(rules, strings.Repeat("-", widths[at]))
	}
	lines := []string{
		strings.TrimRight(strings.Join(heads, "  "), " "), strings.Join(rules, "  "),
	}
	for _, cells := range written {
		padded := make([]string, 0, len(cells))
		for at, cell := range cells {
			padded = append(padded, present.PadText(cell, widths[at]))
		}
		lines = append(lines, strings.TrimRight(strings.Join(padded, "  "), " "))
	}
	return strings.Join(lines, "\n")
}
