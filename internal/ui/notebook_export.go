package ui

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/turanmahmudov/masume/internal/app"
	"github.com/turanmahmudov/masume/internal/core"
	"github.com/turanmahmudov/masume/internal/db"
	"github.com/turanmahmudov/masume/internal/notebook"
	"github.com/turanmahmudov/masume/internal/present"
	"github.com/turanmahmudov/masume/internal/query/result"
	"github.com/turanmahmudov/masume/internal/query/statement"
)

// A report of a notebook is its prose, its statements and the rows every cell answered. It
// is the one place the rows of a notebook are written to a file, so a masked column stays
// masked in it.

// reportSuffix is the ending of the file a report is written to.
const reportSuffix = ".md"

// notebookReportWrittenMsg reports the write of one report.
type notebookReportWrittenMsg struct {
	ConnectionID int
	Path         string
	Rows         int
	Problem      string
}

// askNotebookReport asks where the report of this notebook is written.
func (model *Model) askNotebookReport(
	connection *app.Connection, tab *app.Tab,
) (tea.Model, tea.Cmd) {
	if tab.Kind != app.TabNotebook || tab.Notebook == nil {
		connection.Show("this tab holds no notebook")
		return model, nil
	}
	held := model.resolveReportPath(tab)
	connection.Overlay = app.Overlay{
		Kind: app.OverlayPrompt, Prompt: app.PromptNotebookReport,
		Title: "write a report",
		Hint:  "the rows of every cell that ran, as one Markdown file",
		Draft: app.NewEditorBuffer(held, len(held)),
	}
	return model, nil
}

// resolveReportPath returns the file a report is written to by default: the notebook file
// with a Markdown ending, or the name of the notebook in the working directory.
func (model *Model) resolveReportPath(tab *app.Tab) string {
	if path := tab.Notebook.Path; path != "" {
		return strings.TrimSuffix(path, notebook.FileSuffix) + reportSuffix
	}
	return notebook.BuildSlug(tab.NotebookName()) + reportSuffix
}

// answerNotebookReport writes the report to the path the prompt took.
func (model *Model) answerNotebookReport(
	connection *app.Connection, tab *app.Tab, written string,
) (tea.Model, tea.Cmd) {
	if written == "" || tab.Notebook == nil {
		return model, nil
	}
	blocks, rows := model.buildNotebookReport(tab)
	if rows == 0 {
		connection.Show("no cell has run, so the report would hold no rows")
		return model, nil
	}
	return model, writeNotebookReport(model.ActiveID(),
		core.ExpandHomePath(written), tab.NotebookName(), blocks, rows)
}

// writeNotebookReport writes one report away from the loop.
func writeNotebookReport(
	connectionID int, path, title string, blocks []result.ReportBlock, rows int,
) tea.Cmd {
	return func() tea.Msg {
		answered := notebookReportWrittenMsg{
			ConnectionID: connectionID, Path: path, Rows: rows,
		}
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			answered.Problem = "cannot write the report: " + db.DescribeError(err)
			return answered
		}
		written := result.BuildReport(title, blocks)
		if err := os.WriteFile(path, []byte(written), 0o600); err != nil {
			answered.Problem = "cannot write the report: " + db.DescribeError(err)
		}
		return answered
	}
}

// readNotebookReportWritten reports the write of one report.
func (model *Model) readNotebookReportWritten(
	answered notebookReportWrittenMsg,
) (tea.Model, tea.Cmd) {
	connection, _, found := model.findConnection(answered.ConnectionID)
	if !found {
		return model, nil
	}
	if answered.Problem != "" {
		connection.ShowError(answered.Problem)
		return model, nil
	}
	connection.Show(present.FormatCountOf(int64(answered.Rows), "row", "rows") +
		" written to " + core.ShortenHomePath(answered.Path))
	return model, nil
}

// buildNotebookReport returns the blocks of the report and how many rows they hold.
func (model *Model) buildNotebookReport(tab *app.Tab) ([]result.ReportBlock, int) {
	blocks := []result.ReportBlock{}
	rows := 0
	for at, cell := range tab.Notebook.Cells {
		switch cell.Kind {
		case notebook.CellText:
			blocks = append(blocks, result.ReportBlock{Prose: cell.Editor.Text})
			continue
		case notebook.CellParam:
			blocks = append(blocks, result.ReportBlock{
				Prose: "**Parameters** " + notebook.DescribeParameters(cell.Editor.Text),
			})
			continue
		case notebook.CellChart:
			blocks = append(blocks, model.buildChartReportBlock(tab, cell))
			continue
		case notebook.CellOther:
			blocks = append(blocks, result.ReportBlock{
				Fence: cell.Fence, SQL: cell.Editor.Text,
			})
			continue
		}
		held, counted := model.buildStatementReportBlocks(tab, cell, at)
		blocks = append(blocks, held...)
		rows += counted
	}
	return blocks, rows
}

// buildStatementReportBlocks returns one block per result of a statement cell.
func (model *Model) buildStatementReportBlocks(
	tab *app.Tab, cell *app.NotebookCell, at int,
) ([]result.ReportBlock, int) {
	heading := statement.FindQueryName(cell.Editor.Text)
	if heading == "" {
		heading = "cell " + strconv.Itoa(at+1)
	}
	outcome := tab.ReadCellOutcome(cell)
	if outcome.Kind != app.CellDone && outcome.Kind != app.CellFailed {
		return []result.ReportBlock{{
			Heading: heading, SQL: cell.Editor.Text,
			Note: describeReportOutcome(outcome),
		}}, 0
	}

	blocks := []result.ReportBlock{}
	rows := 0
	for index := cell.FirstResult; index < cell.FirstResult+cell.ResultCount; index++ {
		held := tab.Results.ResultAt(index)
		if held == nil {
			continue
		}
		block := result.ReportBlock{SQL: held.Source, Note: describeReportOutcome(outcome)}
		if index == cell.FirstResult {
			block.Heading = heading
		}
		if held.State.Kind == app.QueryFailed {
			block.Note = held.State.Message
			blocks = append(blocks, block)
			continue
		}
		if len(held.State.Result.Columns) == 0 {
			blocks = append(blocks, block)
			continue
		}
		// A masked column is masked in the report, as it is on the screen.
		block.Columns = held.State.Result.Columns
		block.Rows = maskReportRows(held.State.Result, readMaskedColumns(held))
		block.Note = ""
		rows += len(block.Rows)
		blocks = append(blocks, block)
	}
	return blocks, rows
}

// describeReportOutcome returns the line a block carries in place of rows.
func describeReportOutcome(outcome app.CellOutcome) string {
	switch outcome.Kind {
	case app.CellFailed:
		return outcome.Message
	case app.CellStale:
		return "the rows of this cell were replaced by a later run"
	case app.CellRunning:
		return "this cell was still running"
	case app.CellDone:
		return ""
	}
	return "this cell did not run"
}

// buildChartReportBlock returns the block of a chart cell: its bars as a block of text.
func (model *Model) buildChartReportBlock(
	tab *app.Tab, cell *app.NotebookCell,
) result.ReportBlock {
	spec := notebook.ReadChart(notebook.Cell{Attrs: cell.Attrs})
	block := result.ReportBlock{
		Heading: notebook.NameChart(spec), Fence: "text",
	}
	rows, problem := readCellChartRows(tab, spec)
	if problem != "" {
		block.Note = problem
		return block
	}
	rows = capChartRows(rows, spec.Top)
	if spec.Shape == notebook.ChartLine {
		values := make([]float64, 0, len(rows))
		for _, row := range rows {
			values = append(values, row.Value)
		}
		block.SQL = present.BuildSparkline(values)
		return block
	}
	block.SQL = strings.Join(
		present.BuildChartBars(rows, reportChartLabelWidth, reportChartBarWidth), "\n")
	return block
}

// The widths of one bar of a chart in a report.
const (
	reportChartLabelWidth = 20
	reportChartBarWidth   = 40
)

// readMaskedColumns returns the columns of a result the grid hides.
func readMaskedColumns(held *app.StatementResult) map[int]bool {
	names := make([]string, 0, len(held.State.Result.Columns))
	for _, column := range held.State.Result.Columns {
		names = append(names, column.Name)
	}
	return present.FindMaskedColumns(names, present.DefaultMasking())
}

// maskReportRows returns the rows with every masked column hidden, as the grid hides it.
func maskReportRows(held db.QueryResult, masked map[int]bool) [][]any {
	if len(masked) == 0 {
		return held.Rows
	}
	rows := make([][]any, 0, len(held.Rows))
	for _, row := range held.Rows {
		kept := make([]any, 0, len(row))
		for at, value := range row {
			if masked[at] {
				kept = append(kept, present.MaskedDisplay)
				continue
			}
			kept = append(kept, value)
		}
		rows = append(rows, kept)
	}
	return rows
}
