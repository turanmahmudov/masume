package ui

import (
	"context"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/turanmahmudov/masume/internal/app"
	"github.com/turanmahmudov/masume/internal/cfg"
	"github.com/turanmahmudov/masume/internal/core"
	"github.com/turanmahmudov/masume/internal/db"
	"github.com/turanmahmudov/masume/internal/present"
	"github.com/turanmahmudov/masume/internal/writeplan"
)

// The card that measures a write before it runs, and the undo it leaves behind.

// planReadTimeout is how long the reads of one plan have in all. A user waits on them with
// the write unanswered, so they get less than a statement of their own.
const planReadTimeout = 10 * time.Second

// measuringPlanTitle is the title of the card while the reads run. An answer that lands
// after the card was closed or replaced is dropped, so the title is compared.
const measuringPlanTitle = " write plan "

// writePlanBuiltMsg returns the plan of one write.
type writePlanBuiltMsg struct {
	ConnectionID int
	TabID        int
	// The statement as the user wrote it, which the run that follows the card sends.
	Written string
	// The same statement with the value of every `:name` mark written in, as measured.
	SQL      string
	Plan     writeplan.Plan
	Measured bool
}

func buildWritePlan(
	connectionID, tabID int, written string,
	session writeplan.Source, request writeplan.Request,
) tea.Cmd {
	return func() tea.Msg {
		ctx, stop := context.WithTimeout(context.Background(), planReadTimeout)
		defer stop()

		plan, measured := writeplan.Build(ctx, session, request)
		return writePlanBuiltMsg{
			ConnectionID: connectionID, TabID: tabID, Written: written,
			SQL: request.SQL, Plan: plan, Measured: measured,
		}
	}
}

// askWithWritePlan opens the card that says the write is being measured, and starts the
// reads. The measured statement carries its values, because the count reads its predicate.
func (model *Model) askWithWritePlan(
	connection *app.Connection, tab *app.Tab, written string, read db.ComposedRead,
) (tea.Model, tea.Cmd) {
	profile := connection.Profile()
	connection.Overlay = app.Overlay{
		Kind: app.OverlayMessage, Title: measuringPlanTitle,
		Body: "measuring what this write does…\n\n" + read.Display,
	}
	return model, buildWritePlan(model.ActiveID(), tab.ID, written, connection.Session,
		writeplan.Request{
			SQL: read.Display, Tables: connection.Catalog.Tables, Mode: profile.WritePlan,
			UndoRows: profile.UndoRows,
			InTransaction: connection.Session.ReadTransactionState() ==
				db.TransactionOpen,
		})
}

// readWritePlanAnswer draws the plan over the line that said it was being measured. A write
// this client could not read as one relation falls back to the plain question.
func (model *Model) readWritePlanAnswer(answered writePlanBuiltMsg) (tea.Model, tea.Cmd) {
	connection, tab, found := model.findConnectionTab(answered.ConnectionID, answered.TabID)
	// A user who closed the card while the reads ran is not shown it again.
	if !found || connection.Overlay.Kind != app.OverlayMessage ||
		connection.Overlay.Title != measuringPlanTitle {
		return model, nil
	}

	statements := []string{answered.Written}
	reads, built := model.composeStatementReads(connection, tab, statements)
	if !built {
		connection.Overlay = app.Overlay{}
		return model, nil
	}
	if !answered.Measured {
		return model.askPlainWriteQuestion(connection, tab, statements, reads)
	}

	plan := answered.Plan
	connection.Overlay = app.Overlay{
		Kind: app.OverlayWritePlan, Plan: plan,
		Title: " write plan · " + connection.Profile().Name + " ",
		Answers: app.OverlayAnswers{Answer: func(confirmed bool) app.AnswerCommand {
			if !confirmed {
				return nil
			}
			return carryAnswer(model.startRun(
				connection, tab, statements, reads, plan.Undo))
		}},
	}
	return model, nil
}

// composeStatementReads binds the statement again for the run that follows the card. The
// values of a `:name` mark were asked for before the plan, so nothing is asked twice.
func (model *Model) composeStatementReads(
	connection *app.Connection, tab *app.Tab, statements []string,
) ([]db.ComposedRead, bool) {
	reads := make([]db.ComposedRead, 0, len(statements))
	for _, written := range statements {
		bound, err := tab.BindParameters(connection.Session, written)
		if err != nil {
			connection.ShowError(db.DescribeError(err))
			return nil, false
		}
		reads = append(reads, tab.ComposeStatementRead(connection.Session, bound))
	}
	return reads, true
}

// describeWriteOutcome is what the bar says once a planned write has run.
func (model *Model) describeWriteOutcome(undo writeplan.Undo) string {
	if !undo.IsHeld() {
		return "no undo was kept: " + undo.Reason
	}
	return model.registry.FormatActionChords(cfg.ScopeGlobal, ActionUndoWrite) +
		" undoes this write, " + present.FormatCountOf(int64(undo.Rows), "row", "rows")
}

// describeMissingUndo says why there is nothing to undo.
func describeMissingUndo(connection *app.Connection) string {
	if connection.Profile().WritePlan != cfg.PlanUndo {
		return `this connection does not keep an undo. Set write_plan = "undo" on the ` +
			"profile to read the rows of a write before it changes them"
	}
	if !connection.Session.Capabilities().PlansWrites {
		return "this server does not measure a write, so no undo is kept for one"
	}
	return "nothing has been written on this connection yet"
}

// undoLastWrite asks whether the last write is undone.
func (model *Model) undoLastWrite(connection *app.Connection) (tea.Model, tea.Cmd) {
	held := connection.Undo
	if held == nil {
		connection.Show(describeMissingUndo(connection))
		return model, nil
	}
	if !held.Undo.IsHeld() {
		connection.Show("the last write kept no undo: " + held.Undo.Reason)
		return model, nil
	}
	if connection.Profile().AccessMode == cfg.AccessReadOnly {
		connection.ShowError("this connection is read-only, so nothing was written")
		return model, nil
	}

	id, session := model.ActiveID(), connection.Session
	connection.Overlay = app.Overlay{
		Kind: app.OverlayConfirm, Title: " undo the write ",
		Body: describeUndoQuestion(*held),
		Answers: app.OverlayAnswers{Answer: func(confirmed bool) app.AnswerCommand {
			if !confirmed {
				return nil
			}
			return carryAnswer(applyUndo(id, session, held.Undo))
		}},
	}
	return model, nil
}

func describeUndoQuestion(held app.HeldUndo) string {
	written := "Undo this write? " +
		present.FormatCountOf(int64(held.Undo.Rows), "row", "rows") + " of " +
		held.Undo.Table.Name + " go back to the values they held " +
		core.FormatLargestUnit(time.Since(held.RanAt)) + " ago.\n\n" + held.SQL
	if len(held.Undo.Display) > 0 {
		written += "\n\n" + held.Undo.Display[0]
	}
	if len(held.Undo.Display) > 1 {
		written += "\n… and " + present.FormatCountOf(
			int64(len(held.Undo.Display)-1), "statement", "statements") + " more"
	}
	return written
}

// undoWrittenMsg returns an attempt at undoing a write.
type undoWrittenMsg struct {
	ConnectionID int
	Table        db.TableRef
	Rows         int
	Problem      string
}

// applyUndo runs the undo, all of it or none.
func applyUndo(
	connectionID int, session db.TransactionKeeper, undo writeplan.Undo,
) tea.Cmd {
	return func() tea.Msg {
		answered := undoWrittenMsg{
			ConnectionID: connectionID, Table: undo.Table, Rows: undo.Rows,
		}
		if err := session.ApplyChanges(context.Background(), undo.Changes); err != nil {
			answered.Problem = db.DescribeError(err)
		}
		return answered
	}
}

// readUndoAnswer reports what the undo did, and reads the relation again where the tab on
// show is the one that changed.
func (model *Model) readUndoAnswer(answered undoWrittenMsg) (tea.Model, tea.Cmd) {
	connection, _, found := model.findConnection(answered.ConnectionID)
	if !found {
		return model, nil
	}
	if answered.Problem != "" {
		connection.ShowError(answered.Problem)
		return model, nil
	}

	connection.Undo = nil
	connection.Overlay = app.Overlay{}
	connection.Show("undone, " +
		present.FormatCountOf(int64(answered.Rows), "row", "rows"))

	tab := connection.Active()
	if tab == nil || tab.Kind != app.TabTable || tab.Table != answered.Table {
		return model, nil
	}
	return model.runTabRead(connection, tab)
}
