package ui

import (
	"context"
	"strconv"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/turanmahmudov/masume/internal/app"
	"github.com/turanmahmudov/masume/internal/core"
	"github.com/turanmahmudov/masume/internal/db"
	"github.com/turanmahmudov/masume/internal/query/statement"
)

// A notebook run is the batch runner with a longer plan. The cells are turned into
// statements, the parameter cells give the values, and everything after that is the path a
// batch of one buffer takes.

// runNotebook sends the cells of one scope.
func (model *Model) runNotebook(
	connection *app.Connection, tab *app.Tab, scope app.RunScope,
) (tea.Model, tea.Cmd) {
	book := tab.Notebook
	if book == nil {
		return model, nil
	}
	if model.refuseSecondRun(connection, tab) {
		return model, nil
	}
	tab.View = resolveRowView(tab.View)

	plan := book.BuildRunPlan(scope, connection.Session.Language().SplitStatements)
	if plan.IsEmpty() {
		connection.Show("no statement to run in " + describeRunScope(scope))
		return model, nil
	}
	if problem := model.applyNotebookParameters(connection, tab); problem != "" {
		connection.ShowError(problem)
		return model, nil
	}
	// A cell that names another cell carries the statement of that cell, and none of its
	// rows: the named statement runs again inside this one.
	if problem := model.refuseReferences(connection, book); problem != "" {
		connection.ShowError(problem)
		return model, nil
	}
	expanded, err := book.ExpandRunPlan(plan)
	if err != nil {
		connection.ShowError(err.Error())
		return model, nil
	}
	plan = expanded
	model.reportEngineMismatch(connection, book)

	book.Stopped = false
	book.ApplyRunPlan(plan)
	if model.opensNotebookTransaction(connection, book) {
		// The transaction opens before the first cell is sent. A begin beside the run
		// would reach the server in either order, and the cells would run outside it.
		return model, model.beginThen(connection,
			model.sendNotebookPlan(connection, tab, plan))
	}
	return model, model.sendNotebookPlan(connection, tab, plan)
}

// sendNotebookPlan composes the statements of a plan and hands them to the batch runner.
func (model *Model) sendNotebookPlan(
	connection *app.Connection, tab *app.Tab, plan app.RunPlan,
) tea.Cmd {
	// A statement whose `:name` has no value in a parameter cell still opens the form the
	// editor opens, so a notebook without a parameter cell runs as well.
	if !holdsEveryValue(tab, plan.Statements) {
		_, command := model.execute(connection, tab, plan.Statements)
		return command
	}
	reads := make([]db.ComposedRead, 0, len(plan.Statements))
	for at, written := range plan.Statements {
		bound, err := tab.BindParameters(connection.Session, written)
		if err != nil {
			connection.ShowError(db.DescribeError(err))
			return nil
		}
		// Every statement carries the sort and the filter of its own cell, and none of
		// another cell.
		reads = append(reads, connection.Session.Composer().ComposeStatementRead(
			bound, tab.Notebook.Cells[plan.CellOf[at]].BuildRewrite()))
	}
	_, command := model.executeBound(connection, tab, plan.Statements, reads)
	return command
}

// describeRunScope returns the cells one scope covers.
func describeRunScope(scope app.RunScope) string {
	switch scope {
	case app.RunCell:
		return "this cell"
	case app.RunFromCell:
		return "this cell and the ones below it"
	case app.RunMarkedCells:
		return "the marked cells"
	}
	return "this notebook"
}

// holdsEveryValue is true while every `:name` of the statements already has a value.
func holdsEveryValue(tab *app.Tab, statements []string) bool {
	for _, written := range statements {
		for _, name := range statement.FindQueryParameters(written) {
			held, found := tab.Parameters[strings.ToLower(name)]
			if !found || held == nil || held == "" {
				return false
			}
		}
	}
	return true
}

// applyNotebookParameters reads the parameter cells into the values of the tab, and returns
// the reason they cannot be read.
func (model *Model) applyNotebookParameters(
	connection *app.Connection, tab *app.Tab,
) string {
	values, problems := tab.Notebook.ReadParameterValues()
	if len(problems) > 0 {
		return "the parameter cell cannot be read: " + problems[0]
	}
	for name, value := range values {
		tab.Parameters[name] = value
	}
	return ""
}

// refuseReferences returns why a reference cannot run on this server. An engine without a
// subquery cannot hold the statement of another cell inside one of its own.
func (model *Model) refuseReferences(
	connection *app.Connection, book *app.Notebook,
) string {
	if !book.HoldsReferences() {
		return ""
	}
	if core.ResolveEngineInfo(connection.Profile().Engine).Family == core.FamilyMongo {
		return "a cell of this engine cannot name another cell; " +
			"write the values into the statement instead"
	}
	return ""
}

// reportEngineMismatch says so where the notebook was written for another engine.
func (model *Model) reportEngineMismatch(connection *app.Connection, book *app.Notebook) {
	if book.Engine == "" {
		return
	}
	held := string(connection.Profile().Engine)
	if strings.EqualFold(book.Engine, held) {
		return
	}
	connection.Show("this notebook was written for " + book.Engine +
		"; this connection is " + held)
}

// opensNotebookTransaction is true where the policy asks for one transaction and this run
// is the one that opens it.
func (model *Model) opensNotebookTransaction(
	connection *app.Connection, book *app.Notebook,
) bool {
	if !book.Run.RunsInOneTransaction() || book.HoldsTransaction {
		return false
	}
	if !connection.Session.Capabilities().HasTransactions {
		connection.Show("no transactions on this server; every cell commits itself")
		return false
	}
	// A transaction the reader opened stays theirs to close.
	if connection.Session.ReadTransactionState() != db.TransactionNone {
		return false
	}
	book.HoldsTransaction = true
	return true
}

// beginThen opens a transaction and then runs what follows it, in that order and on one
// goroutine.
func (model *Model) beginThen(connection *app.Connection, next tea.Cmd) tea.Cmd {
	session := connection.Session
	id := model.ActiveID()
	return func() tea.Msg {
		if err := session.BeginTransaction(context.Background()); err != nil {
			return transactionRanMsg{
				ConnectionID: id, Action: ActionBeginTransaction,
				Problem: db.DescribeError(err),
			}
		}
		if next == nil {
			return nil
		}
		return next()
	}
}

// rollBackThen rolls a failed transaction back and then runs what follows it. A server that
// holds a failed transaction refuses every statement until it is rolled back, so a run that
// continues after a failure rolls back first.
func (model *Model) rollBackThen(connection *app.Connection, next tea.Cmd) tea.Cmd {
	session := connection.Session
	id := model.ActiveID()
	return func() tea.Msg {
		if err := session.RollbackTransaction(context.Background()); err != nil {
			return transactionRanMsg{
				ConnectionID: id, Action: ActionRollbackTransaction,
				Problem: db.DescribeError(err),
			}
		}
		if next == nil {
			return nil
		}
		return next()
	}
}

// resumesAfterFailure returns the command that runs the cells after a failed one, with a
// rollback before it where the transaction cannot take another statement.
func (model *Model) resumesAfterFailure(
	connection *app.Connection, tab *app.Tab, next tea.Cmd,
) tea.Cmd {
	if connection.Session.ReadTransactionState() != db.TransactionFailed {
		return next
	}
	if tab.Notebook != nil {
		tab.Notebook.HoldsTransaction = false
	}
	connection.Show("the transaction failed and was rolled back; the run goes on")
	return model.rollBackThen(connection, next)
}

// closeNotebookRun commits the one transaction of a notebook once the last cell answered.
func (model *Model) closeNotebookRun(
	connection *app.Connection, tab *app.Tab,
) tea.Cmd {
	book := tab.Notebook
	if book == nil || !book.HoldsTransaction {
		return nil
	}
	book.HoldsTransaction = false
	session := connection.Session
	id := model.ActiveID()
	return func() tea.Msg {
		err := session.CommitTransaction(context.Background())
		return transactionRanMsg{
			ConnectionID: id, Action: ActionCommitTransaction,
			Problem: db.DescribeError(err),
		}
	}
}

// reportFailedCell names the cell a failure stands in, and says the transaction is open.
func (model *Model) reportFailedCell(
	connection *app.Connection, tab *app.Tab, index int,
) {
	book := tab.Notebook
	if book == nil {
		return
	}
	at := book.FindCellOfResult(index)
	if at < 0 {
		return
	}
	tab.KeepFocusedCell()
	book.FocusCell(at)
	tab.SettleFocusedCell()
	written := "cell " + strconv.Itoa(at+1) + " failed"
	if book.HoldsTransaction {
		written += "; the transaction is still open"
	}
	connection.ShowError(written)
}

// continuesAfterFailure is true while the policy of the notebook runs the cells after a
// failed one.
func continuesAfterFailure(tab *app.Tab) bool {
	return tab.Kind == app.TabNotebook && tab.Notebook != nil &&
		!tab.Notebook.Run.StopsOnError() && !tab.Notebook.Stopped
}
