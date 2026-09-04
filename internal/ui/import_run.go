package ui

import (
	"context"
	"strconv"

	tea "charm.land/bubbletea/v2"

	"github.com/turanmahmudov/masume/internal/app"
	"github.com/turanmahmudov/masume/internal/db"
	"github.com/turanmahmudov/masume/internal/load"
	"github.com/turanmahmudov/masume/internal/query"
)

// The three steps of an import: the file is read, what it would do is checked, and only
// then are the rows written. Each one runs off the draw loop.

// importReadMsg carries what a read of the file found, and the columns of the table the
// rows go into, or why either could not be read.
type importReadMsg struct {
	ConnectionID int
	Sample       load.Sample
	Target       []load.TargetColumn
	Problem      string
}

// importCheckedMsg carries what the dry run found, and the SQL the review shows.
type importCheckedMsg struct {
	ConnectionID int
	Report       load.CheckReport
	Statements   []string
	Problem      string
}

// importRanMsg carries how many rows the import wrote, or why it stopped.
type importRanMsg struct {
	ConnectionID int
	Written      int
	Problem      string
}

// The two tables an import writes into, named so a call names the one it opens.
const (
	intoNewTable         = true
	intoTableThatIsThere = false
)

// openImport asks for the file and the mapping rather than writing at once. The table the
// menu was opened on is the one the rows go into.
func (model *Model) openImport(
	connection *app.Connection, table db.TableRef, creating bool,
) (tea.Model, tea.Cmd) {
	if !connection.Session.Capabilities().WritesDDL {
		connection.Show("this server has no import")
		return model, nil
	}

	connection.Overlay = app.Overlay{
		Kind: app.OverlayImport, Title: " import ",
		Import: app.ImportRequest{
			Stage: app.ImportPick,
			Plan: load.Plan{
				Options: load.DefaultReadOptions(), Table: table.Qualified(),
				CreatesTable: creating,
			},
		},
		Draft: app.NewEditorBuffer("", 0),
	}
	// The card opens on the picker, and the path can still be typed on the row it fills in.
	return model, model.openFilePicker(model.ActiveID())
}

// buildImportTarget returns the columns of the table the rows go into, as the import reads
// them.
func buildImportTarget(detail db.TableDetail) []load.TargetColumn {
	target := make([]load.TargetColumn, 0, len(detail.Columns))
	for _, column := range detail.Columns {
		target = append(target, load.TargetColumn{
			Name: column.Name, DataType: column.DataType,
			// A column with a default is left out of a row and still refuses an
			// empty value written into it.
			Optional:  column.Nullable || column.HasDefault || column.IsGenerated,
			TakesNull: column.Nullable,
			Generated: column.IsGenerated,
		})
	}
	return target
}

// ListTargetNames returns the names a mapping steps through. A column the server fills
// itself is left out.
func ListTargetNames(target []load.TargetColumn) []string {
	names := make([]string, 0, len(target))
	for _, column := range target {
		if column.Generated {
			continue
		}
		names = append(names, column.Name)
	}
	return names
}

// stepImport takes the import to its next stage: the file is read, then what it would do is
// checked, and only then are the rows written.
func (model *Model) stepImport(
	connection *app.Connection, overlay *app.Overlay,
) (tea.Model, tea.Cmd) {
	held := &overlay.Import
	if held.Running {
		return model, nil
	}
	if held.Stage == app.ImportPick {
		return model, nil
	}
	// The row the picker fills in opens it again, at whichever stage the form stands.
	if readFieldKey(*overlay) == "path" {
		held.Stage = app.ImportPick
		overlay.Notice = ""
		return model, model.openFilePicker(model.ActiveID())
	}
	if overlay.Draft != nil && held.Stage != app.ImportReview {
		ReadImportField(overlay, overlay.Draft.Text)
	}
	if held.Stage == app.ImportFile {
		ApplyImportPath(overlay)
	}
	if problem := FindImportProblem(
		*overlay, connection.Session.Dialect()); problem != "" {
		overlay.Notice = problem
		return model, nil
	}

	overlay.Notice = ""
	held.Running = true
	id := model.ActiveID()
	switch held.Stage {
	case app.ImportFile:
		return model, readImportFile(id, connection.Session, held.Plan)
	case app.ImportMapping:
		return model, checkImportFile(id, held.Plan, connection.Session.Dialect())
	}
	return model, runImport(connection, id, held.Plan, connection.Session.Dialect())
}

// leaveImportReview takes the review back to the form.
func leaveImportReview(overlay *app.Overlay) bool {
	if overlay.Kind != app.OverlayImport || overlay.Import.Stage != app.ImportReview {
		return false
	}
	overlay.Import.Stage = app.ImportMapping
	overlay.Import.Statements = nil
	overlay.Notice = ""
	return true
}

// readImportFile reads the head of the file and the columns of the table. The table is read
// from the server, because the tree reads one only once it is unfolded.
func readImportFile(
	connectionID int, session db.Session, plan load.Plan,
) tea.Cmd {
	return func() tea.Msg {
		sample, err := load.ReadSample(plan.Path, plan.Options)
		if err != nil {
			return importReadMsg{ConnectionID: connectionID, Problem: err.Error()}
		}
		answered := importReadMsg{ConnectionID: connectionID, Sample: sample}
		if plan.CreatesTable {
			return answered
		}

		ctx, stop := context.WithTimeout(context.Background(), readTimeout)
		defer stop()
		detail, err := session.DescribeTable(ctx, db.TableRef{
			Schema: plan.Table.Schema, Name: plan.Table.Name,
		})
		if err != nil {
			return importReadMsg{
				ConnectionID: connectionID, Problem: db.DescribeError(err),
			}
		}
		answered.Target = buildImportTarget(detail)
		return answered
	}
}

// checkImportFile reads the whole file and reports the rows the import cannot write.
func checkImportFile(connectionID int, plan load.Plan, dialect *query.Dialect) tea.Cmd {
	return func() tea.Msg {
		report, err := plan.CheckFile()
		if err != nil {
			return importCheckedMsg{ConnectionID: connectionID, Problem: err.Error()}
		}
		statements, err := load.DescribeStatements(plan, dialect)
		if err != nil {
			return importCheckedMsg{ConnectionID: connectionID, Problem: err.Error()}
		}
		return importCheckedMsg{
			ConnectionID: connectionID, Report: report, Statements: statements,
		}
	}
}

// runImport writes the rows of the file, in batches, inside one transaction, and leaves out
// the rows the check refused.
func runImport(
	connection *app.Connection, connectionID int, plan load.Plan, dialect *query.Dialect,
) tea.Cmd {
	session := connection.Session
	// Closing the card ends the import, and the server rolls the transaction back.
	ctx, stop := context.WithCancel(context.Background())
	connection.BeginImport(stop)

	return func() tea.Msg {
		defer stop()
		fail := func(problem string) tea.Msg {
			return importRanMsg{ConnectionID: connectionID, Problem: problem}
		}

		if err := session.BeginTransaction(ctx); err != nil {
			return fail(db.DescribeError(err))
		}
		written, err := writeImportRows(ctx, session, plan, dialect)
		if err != nil {
			// The rollback runs whatever stopped the import, so it takes its own context.
			if rollbackErr := session.RollbackTransaction(
				context.WithoutCancel(ctx)); rollbackErr != nil {
				return fail(db.DescribeError(err) + ", and the rollback failed: " +
					db.DescribeError(rollbackErr))
			}
			return fail(db.DescribeError(err))
		}
		if err := session.CommitTransaction(ctx); err != nil {
			return fail(db.DescribeError(err))
		}
		return importRanMsg{ConnectionID: connectionID, Written: written}
	}
}

// writeImportRows makes the table where the import makes one, and then writes every row the
// file holds that the check did not refuse.
func writeImportRows(
	ctx context.Context, session db.Session, plan load.Plan, dialect *query.Dialect,
) (int, error) {
	if plan.CreatesTable {
		if _, err := session.RunQuery(
			ctx, load.BuildCreateTable(plan, dialect), 1, nil); err != nil {
			return 0, err
		}
	}

	written := 0
	batchRows := load.ResolveBatchRows(len(plan.ListMappedColumns()), dialect)
	batch := make([]load.Row, 0, batchRows)
	flush := func() error {
		if len(batch) == 0 {
			return nil
		}
		values, err := load.BuildRows(plan, batch)
		if err != nil {
			return err
		}
		statement, err := load.BuildInsert(plan, values, dialect)
		if err != nil {
			return err
		}
		if _, err := session.RunQuery(ctx, statement.SQL, 1, statement.Params); err != nil {
			return err
		}
		written += len(batch)
		batch = batch[:0]
		return nil
	}

	// A file of documents can name a column after the sample was read.
	named := len(plan.Sample.Columns)
	err := load.WalkFile(plan.Path, plan.Options,
		func(read []string) error {
			named = max(named, len(read))
			return nil
		},
		func(row load.Row) error {
			if !plan.HoldsWritableRow(row, named) {
				return nil
			}
			batch = append(batch, row)
			if len(batch) < batchRows {
				return nil
			}
			return flush()
		})
	if err != nil {
		return written, err
	}
	return written, flush()
}

// readImportAnswer keeps what a read of the file found, so the form can map its columns.
func (model *Model) readImportAnswer(answered importReadMsg) (tea.Model, tea.Cmd) {
	connection, _, found := model.findConnection(answered.ConnectionID)
	if !found || connection.Overlay.Kind != app.OverlayImport {
		return model, nil
	}
	overlay := &connection.Overlay
	overlay.Import.Running = false

	if answered.Problem != "" {
		overlay.Notice = answered.Problem
		return model, nil
	}
	overlay.Notice = ""
	held := &overlay.Import
	held.TargetNames = ListTargetNames(answered.Target)
	held.Plan = load.BuildPlan(held.Plan.Path, held.Plan.Options, answered.Sample,
		held.Plan.Table, answered.Target)
	held.Stage = app.ImportMapping
	StepImportField(overlay, 0)
	return model, nil
}

// readImportCheck keeps what the dry run found and shows the review.
func (model *Model) readImportCheck(answered importCheckedMsg) (tea.Model, tea.Cmd) {
	connection, _, found := model.findConnection(answered.ConnectionID)
	if !found || connection.Overlay.Kind != app.OverlayImport {
		return model, nil
	}
	overlay := &connection.Overlay
	overlay.Import.Running = false

	if answered.Problem != "" {
		overlay.Notice = answered.Problem
		return model, nil
	}
	overlay.Notice = ""
	overlay.Import.Report = answered.Report
	overlay.Import.Statements = answered.Statements
	overlay.Import.Stage = app.ImportReview
	return model, nil
}

// readImportRun reports what the import wrote, and reads the object tree again where it
// made a table.
func (model *Model) readImportRun(answered importRanMsg) (tea.Model, tea.Cmd) {
	connection, id, found := model.findConnection(answered.ConnectionID)
	if !found {
		return model, nil
	}
	if connection.Overlay.Kind == app.OverlayImport {
		connection.Overlay.Import.Running = false
	}

	if answered.Problem != "" {
		if connection.Overlay.Kind == app.OverlayImport {
			connection.Overlay.Notice = answered.Problem
			return model, nil
		}
		connection.ShowError(answered.Problem)
		return model, nil
	}

	creating := connection.Overlay.Kind == app.OverlayImport &&
		connection.Overlay.Import.Plan.CreatesTable
	connection.Overlay = app.Overlay{}
	connection.Show(strconv.Itoa(answered.Written) + " rows were written")
	if creating {
		return model, readCatalog(id, connection.Session, quietCatalogRead)
	}
	return model, nil
}
