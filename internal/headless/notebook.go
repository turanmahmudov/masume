package headless

import (
	"context"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/turanmahmudov/masume/internal/cfg"
	"github.com/turanmahmudov/masume/internal/db"
	"github.com/turanmahmudov/masume/internal/db/engines"
	"github.com/turanmahmudov/masume/internal/notebook"
	"github.com/turanmahmudov/masume/internal/present"
	"github.com/turanmahmudov/masume/internal/query"
	"github.com/turanmahmudov/masume/internal/query/result"
	"github.com/turanmahmudov/masume/internal/query/statement"
)

// A notebook run without a screen has no write confirmation, no write plan and no undo, so
// a write cell needs AllowWrites.

// The widths of one bar of a chart in a report.
const (
	reportLabelWidth = 20
	reportBarWidth   = 40
)

// NotebookOptions is a headless notebook request.
type NotebookOptions struct {
	Options
	Notebook notebook.Notebook
	// The cells to run, by id. An empty list runs every cell.
	Only []string
	// True while a write cell may run.
	AllowWrites bool
}

// cellAnswer is what one cell of the run answered.
type cellAnswer struct {
	Columns []query.ResultColumn
	Rows    [][]any
	// The report of a statement without a result set.
	Change string
}

// RunNotebook runs the cells of a notebook and returns the exit code of the run.
func RunNotebook(
	ctx context.Context, adapters engines.Adapters, options NotebookOptions,
) int {
	if cfg.NeedsPasswordPrompt(options.Profile) && options.Password == "" {
		options.report("%s requires a password; headless mode cannot prompt for passwords",
			options.Profile.Name)
		return CodeConnection
	}
	// A plan runs no cell, so a write cell needs no confirmation.
	if !options.Explain {
		if code := refuseNotebookWrites(options); code != CodeOK {
			return code
		}
	}
	if code := refuseSeveralJSONResults(options); code != CodeOK {
		return code
	}

	session, preConnect, code := openSession(ctx, adapters, options.Options)
	if code != CodeOK {
		return code
	}
	defer func() {
		_ = session.Close()
		preConnect.Stop()
	}()

	values := resolveNotebookValues(options)
	// The values of the parameter cells are the values of the run, so a plan of a
	// statement binds them as the run does.
	options.Params = values
	if code := expandNotebookReferences(&options); code != CodeOK {
		return code
	}
	report := newNotebookReport(options)
	answers := map[string]cellAnswer{}
	held := CodeOK

	transaction := options.Notebook.Run.RunsInOneTransaction() &&
		session.Capabilities().HasTransactions
	if transaction {
		if err := session.BeginTransaction(ctx); err != nil {
			options.report("%s", db.DescribeError(err))
			return CodeConnection
		}
	}

	for at, cell := range options.Notebook.Cells {
		if !runsThisCell(options, cell) {
			continue
		}
		code := runNotebookCell(
			ctx, session, options, cell, at, values, answers, report)
		if code == CodeOK {
			continue
		}
		held = code
		if options.Notebook.Run.StopsOnError() {
			options.report("cells after cell %d were not run", at+1)
			break
		}
	}

	if transaction {
		held = closeNotebookTransaction(ctx, session, options, held)
	}
	return held
}

// expandNotebookReferences writes the statement of a named cell in place of every
// `{{cell:id}}` of the notebook.
func expandNotebookReferences(options *NotebookOptions) int {
	sources := map[string]string{}
	for _, cell := range options.Notebook.Cells {
		if cell.RunsStatements() {
			sources[cell.ID] = cell.Source
		}
	}
	for at, cell := range options.Notebook.Cells {
		if !runsThisCell(*options, cell) {
			continue
		}
		if !cell.RunsStatements() || !notebook.HoldsReference(cell.Source) {
			continue
		}
		written, err := notebook.ExpandReferences(cell.Source, sources)
		if err != nil {
			options.report("%s", err)
			return CodeStatement
		}
		options.Notebook.Cells[at].Source = written
	}
	return CodeOK
}

// refuseSeveralJSONResults refuses a run that would write more than one JSON document,
// because the documents back to back are no JSON a reader can parse.
func refuseSeveralJSONResults(options NotebookOptions) int {
	if options.Format != FormatJSON || options.Explain {
		return CodeOK
	}
	cells := 0
	for _, cell := range options.Notebook.Cells {
		if cell.RunsStatements() && runsThisCell(options, cell) {
			cells++
		}
	}
	if cells <= 1 {
		return CodeOK
	}
	options.report("json output supports one statement cell per run; this run covers %d. "+
		"Use markdown or csv, or name one cell with --only", cells)
	return CodeStatement
}

// refuseNotebookWrites refuses a write cell where nothing can confirm it.
func refuseNotebookWrites(options NotebookOptions) int {
	writes := 0
	for _, cell := range options.Notebook.Cells {
		if cell.AsksConfirmation() && runsThisCell(options, cell) {
			writes++
		}
	}
	if writes == 0 || options.AllowWrites {
		return CodeOK
	}
	options.report("%s a confirmation, and a run without a screen cannot ask; "+
		"pass --allow-writes to run them",
		present.FormatCountOf(int64(writes), "write cell needs", "write cells need"))
	return CodeStatement
}

// runsThisCell is true for a cell the request covers.
func runsThisCell(options NotebookOptions, cell notebook.Cell) bool {
	if len(options.Only) == 0 {
		return true
	}
	return slices.Contains(options.Only, cell.ID)
}

// resolveNotebookValues returns the values of the parameter cells, with the values of the
// command line over them.
func resolveNotebookValues(options NotebookOptions) map[string]any {
	values := map[string]any{}
	for _, cell := range options.Notebook.Cells {
		if cell.Kind != notebook.CellParam {
			continue
		}
		held, _ := notebook.ReadParameters(cell.Source)
		for name, value := range held {
			values[name] = value
		}
	}
	for name, value := range options.Params {
		values[name] = value
	}
	return values
}

// closeNotebookTransaction commits a run that finished, and rolls back one that failed.
func closeNotebookTransaction(
	ctx context.Context, session db.Session, options NotebookOptions, held int,
) int {
	if held != CodeOK {
		if err := session.RollbackTransaction(ctx); err != nil {
			options.report("%s", db.DescribeError(err))
		}
		options.report("the transaction was rolled back")
		return held
	}
	if err := session.CommitTransaction(ctx); err != nil {
		options.report("%s", db.DescribeError(err))
		return CodeStatement
	}
	return CodeOK
}

// runNotebookCell runs one cell and writes what it answered.
func runNotebookCell(
	ctx context.Context, session db.Session, options NotebookOptions,
	cell notebook.Cell, at int, values map[string]any,
	answers map[string]cellAnswer, report *notebookReport,
) int {
	switch cell.Kind {
	case notebook.CellText:
		report.writeProse(cell)
		return CodeOK
	case notebook.CellParam:
		report.writeParameters(values)
		return CodeOK
	case notebook.CellChart:
		report.writeChart(cell, answers)
		return CodeOK
	case notebook.CellOther:
		return CodeOK
	}

	spoken := session.Language()
	statements := spoken.SplitStatements(cell.Source)
	if len(statements) == 0 {
		return CodeOK
	}
	if options.Format == FormatJSON && !options.Explain && len(statements) > 1 {
		options.report("json output supports one statement per cell; cell %d holds %d. "+
			"Use markdown or csv, or split the cell", at+1, len(statements))
		return CodeStatement
	}
	// A plan of every statement runs no write and returns no row, so it is written in
	// place of the run.
	if options.Explain {
		report.openCell(cell, at)
		for _, sql := range statements {
			if code := writePlan(ctx, session, spoken, options.Options, sql); code != CodeOK {
				return code
			}
		}
		return CodeOK
	}

	report.openCell(cell, at)
	for _, sql := range statements {
		started := time.Now()
		answered, code := readOneCellStatement(ctx, session, options, values, sql)
		if code != CodeOK {
			report.reportCell(at, cell, "fail", 0, time.Since(started))
			return code
		}
		answers[cell.ID] = answered
		if err := report.writeAnswer(sql, answered); err != nil {
			options.report("cannot write the result: %v", err)
			return CodeStatement
		}
		report.reportCell(at, cell, "ok", len(answered.Rows), time.Since(started))
	}
	return CodeOK
}

// readOneCellStatement binds and runs one statement of a cell.
func readOneCellStatement(
	ctx context.Context, session db.Session, options NotebookOptions,
	values map[string]any, sql string,
) (cellAnswer, int) {
	writes := session.Language().ResolveWriteRisk(sql) != statement.RiskNone
	if writes && options.Profile.AccessMode == cfg.AccessReadOnly {
		options.report("%s is read-only, so the statement was not sent", options.Profile.Name)
		return cellAnswer{}, CodeRefused
	}
	if writes && !options.AllowWrites {
		options.report("this cell writes, and a run without a screen cannot ask; " +
			"pass --allow-writes to run it")
		return cellAnswer{}, CodeStatement
	}

	bound, err := session.Composer().BindParameters(sql, values)
	if err != nil {
		options.report("%s", err)
		return cellAnswer{}, CodeStatement
	}
	// A read with its own limit streams every row within that limit, as `masume run` does.
	if !writes && options.RowLimit == 0 && session.Language().HoldsRowLimit(sql) {
		return streamCellRead(ctx, session, options, bound)
	}

	answered, err := session.RunQuery(
		ctx, bound.Text, resolveRowLimit(options.Options), bound.Params)
	if err != nil {
		options.report("%s", db.DescribeError(err))
		return cellAnswer{}, CodeStatement
	}
	if len(answered.Columns) == 0 && !answered.HoldsResultSet {
		return cellAnswer{Change: describeChange(answered)}, CodeOK
	}
	held := cellAnswer{Columns: answered.Columns, Rows: answered.Rows}
	if !answered.Truncated {
		return held, CodeOK
	}
	return held, reportCellTruncation(options, len(answered.Rows), writes)
}

// reportCellTruncation names the rows a cell did not return.
func reportCellTruncation(options NotebookOptions, written int, writes bool) int {
	if options.RowLimit > 0 {
		options.report("returned the first %d rows; the result exceeds the requested limit",
			written)
		return CodeOK
	}
	if writes {
		options.report("returned only the first %d rows; write results are incomplete. "+
			"The write was not repeated. Do not automatically retry the write", written)
		return CodeStatement
	}
	options.report("returned the first %d rows; the result exceeds the page size. "+
		"Add a statement limit or --limit to read more", written)
	return CodeOK
}

// streamCellRead reads every row of a cell, a batch at a time.
func streamCellRead(
	ctx context.Context, session db.Session, options NotebookOptions, bound db.BoundText,
) (cellAnswer, int) {
	held := cellAnswer{}
	_, err := session.StreamQuery(ctx, bound.Text, bound.Params,
		resolveBatchSize(options.Options),
		func(rows [][]any, columns []query.ResultColumn) error {
			held.Columns = columns
			held.Rows = append(held.Rows, rows...)
			return nil
		})
	if err != nil {
		options.report("%s", db.DescribeError(err))
		return cellAnswer{}, CodeStatement
	}
	return held, CodeOK
}

// notebookReport writes the answers of a run: a Markdown report, or one result per cell in
// the format the request asked for.
type notebookReport struct {
	options NotebookOptions
	// True while the whole notebook is written as one Markdown document.
	markdown  bool
	wroteHead bool
}

// newNotebookReport opens the report of one run.
func newNotebookReport(options NotebookOptions) *notebookReport {
	report := &notebookReport{
		options: options, markdown: options.Format == FormatMarkdown,
	}
	// A notebook that opens with prose carries its own heading.
	cells := options.Notebook.Cells
	report.wroteHead = len(cells) > 0 && cells[0].Kind == notebook.CellText
	return report
}

// writeHead writes the title of the notebook, one time.
func (report *notebookReport) writeHead() {
	if report.wroteHead || !report.markdown {
		return
	}
	report.wroteHead = true
	title := report.options.Notebook.Title
	if title == "" {
		title = "notebook"
	}
	report.write("# " + title + "\n")
}

// writeProse writes a text cell into a Markdown report.
func (report *notebookReport) writeProse(cell notebook.Cell) {
	if !report.markdown {
		return
	}
	report.writeHead()
	report.write(strings.Trim(cell.Source, "\n") + "\n")
}

// writeParameters writes the values the run bound.
func (report *notebookReport) writeParameters(values map[string]any) {
	if !report.markdown {
		return
	}
	report.writeHead()
	report.write("**Parameters**  " + notebook.DescribeParameters(
		notebook.WriteParameters(values)) + "\n")
}

// writeChart draws the result of the source cell as bars.
func (report *notebookReport) writeChart(cell notebook.Cell, answers map[string]cellAnswer) {
	if !report.markdown {
		return
	}
	report.writeHead()
	spec := notebook.ReadChart(cell)
	held, found := answers[spec.Source]
	if !found {
		report.write("_no result for the source cell " + spec.Source + "_\n")
		return
	}
	rows, problem := present.BuildChartRows(
		held.Columns, held.Rows, spec.Label, spec.Value, spec.SortsByValue)
	if problem != "" {
		report.write("_" + problem + "_")
		return
	}
	if spec.Top > 0 && spec.Top < len(rows) {
		rows = rows[:spec.Top]
	}
	if spec.Shape == notebook.ChartLine {
		report.write("```\n" + present.BuildSparkline(collectChartValues(rows)) + "\n```\n")
		return
	}
	report.write("```\n" + strings.Join(
		present.BuildChartBars(rows, reportLabelWidth, reportBarWidth), "\n") + "\n```\n")
}

// collectChartValues returns the values of the rows of a chart.
func collectChartValues(rows []present.ChartRow) []float64 {
	values := make([]float64, 0, len(rows))
	for _, row := range rows {
		values = append(values, row.Value)
	}
	return values
}

// openCell writes the heading of one statement cell.
func (report *notebookReport) openCell(cell notebook.Cell, at int) {
	if !report.markdown {
		return
	}
	report.writeHead()
	name := statement.FindQueryName(cell.Source)
	if name == "" {
		name = "cell " + strconv.Itoa(at+1)
	}
	report.write("## " + name + "\n")
}

// writeAnswer writes the rows of one statement.
func (report *notebookReport) writeAnswer(sql string, answered cellAnswer) error {
	if report.markdown {
		report.write(strings.TrimRight(result.BuildReport("", []result.ReportBlock{{
			SQL: strings.TrimSpace(sql), Columns: answered.Columns,
			Rows: answered.Rows, Note: answered.Change,
		}}), "\n"))
		return nil
	}
	if answered.Change != "" {
		report.options.report("%s", answered.Change)
		return nil
	}
	sink := createRowSink(report.options.Format, report.options.Out)
	if len(answered.Rows) > 0 {
		if err := sink.TakeRows(answered.Rows, answered.Columns); err != nil {
			return err
		}
	}
	return sink.Finish(answered.Columns)
}

// reportCell writes one line about a cell to the error stream.
func (report *notebookReport) reportCell(
	at int, cell notebook.Cell, state string, rows int, took time.Duration,
) {
	name := statement.FindQueryName(cell.Source)
	if name == "" {
		name = cell.ID
	}
	_, _ = fmt.Fprintf(report.options.Err, "%-5s cell %-3d %-28s %6s rows %8s\n",
		state, at+1, present.TruncateText(name, 28), strconv.Itoa(rows),
		present.FormatDuration(took))
}

// write writes one block of the Markdown report, and the blank line after it.
func (report *notebookReport) write(text string) {
	_, _ = fmt.Fprintf(report.options.Out, "%s\n\n", strings.TrimRight(text, "\n"))
}
