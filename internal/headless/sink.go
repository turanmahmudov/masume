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

// Result writers for streaming and buffered output formats.

// rowSink takes the rows of one result and writes them in one format.
type rowSink interface {
	// TakeRows takes one batch. The columns are the same for every batch of a result.
	TakeRows(rows [][]any, columns []query.ResultColumn) error
	// Finish completes the output, including empty results.
	Finish(columns []query.ResultColumn) error
}

// createRowSink returns the sink of that format.
func createRowSink(format Format, out io.Writer) rowSink {
	switch format {
	case FormatCSV, FormatJSON:
		return &streamingSink{
			writer: result.CreateRowWriter(
				resolveExportFormat(format), result.DefaultCSVOptions(), out),
		}
	case FormatMarkdown:
		return &heldSink{out: out, write: result.BuildMarkdown}
	default:
		return &heldSink{out: out, write: buildPlainTable}
	}
}

// resolveExportFormat returns the export format of a run format.
func resolveExportFormat(format Format) result.ExportFormat {
	if format == FormatCSV {
		return result.ExportCSV
	}
	return result.ExportJSON
}

// streamingSink writes each batch as it arrives. CSV and JSON are written this way.
type streamingSink struct{ writer result.RowWriter }

func (sink *streamingSink) TakeRows(rows [][]any, columns []query.ResultColumn) error {
	return sink.writer.WriteRows(rows, columns)
}

// Finish completes the document. Empty results produce a CSV header or an empty JSON array.
func (sink *streamingSink) Finish(columns []query.ResultColumn) error {
	return sink.writer.Close(columns)
}

// heldSink buffers all rows for formats with aligned columns.
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

// buildPlainTable writes a space-aligned table using the widest cell in each column.
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
