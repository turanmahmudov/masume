package ui

import (
	"strconv"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/turanmahmudov/masume/internal/app"
	"github.com/turanmahmudov/masume/internal/cfg"
	"github.com/turanmahmudov/masume/internal/core"
	"github.com/turanmahmudov/masume/internal/db"
	"github.com/turanmahmudov/masume/internal/present"
	"github.com/turanmahmudov/masume/internal/query"
	"github.com/turanmahmudov/masume/internal/query/language"
	"github.com/turanmahmudov/masume/internal/query/statement"
)

// runStatementAtCursor runs the selection, or the statement at the caret. A selection wins
// over the statement the caret stands in.
// resolveRowView returns the view a run lands on. A read answers rows, so a view that draws
// something else about the relation steps aside for them. A view that draws the rows
// themselves is the one the reader is working in, and a sort or a filter run from inside it
// must not throw them back to another one.
func resolveRowView(held app.ResultView) app.ResultView {
	if DrawsResultRows(held) {
		return held
	}
	return app.ViewData
}

func (model *Model) runStatementAtCursor(
	connection *app.Connection, tab *app.Tab,
) (tea.Model, tea.Cmd) {
	if model.refuseSecondRun(connection, tab) {
		return model, nil
	}
	tab.View = resolveRowView(tab.View)
	if !tab.EditorVisible() {
		return model.runTabRead(connection, tab)
	}

	selected := strings.TrimSpace(tab.Editor.Selection())
	if selected == "" {
		statement := tab.Editor.ReadStatementAtCaret(connection.Session.Language())
		return model.execute(connection, tab, []string{statement})
	}

	parts := connection.Session.Language().SplitStatements(selected)
	if len(parts) == 0 {
		parts = []string{selected}
	}
	return model.execute(connection, tab, parts)
}

// runWholeBuffer runs every statement in the buffer, one result each.
func (model *Model) runWholeBuffer(
	connection *app.Connection, tab *app.Tab,
) (tea.Model, tea.Cmd) {
	if model.refuseSecondRun(connection, tab) {
		return model, nil
	}
	tab.View = resolveRowView(tab.View)
	if !tab.EditorVisible() {
		return model.runTabRead(connection, tab)
	}
	return model.execute(connection, tab, connection.Session.Language().SplitStatements(
		tab.Editor.Text))
}

// refuseSecondRun reports whether this tab is already running, and says so. A run started
// beside the one going replaces only what the client keeps: the statements of the run
// before it are already with the server, and an INSERT asked for twice is written twice.
func (model *Model) refuseSecondRun(connection *app.Connection, tab *app.Tab) bool {
	if !tab.Results.IsRunning() {
		return false
	}
	connection.Show("this tab is still running; stop it first, or open another tab")
	return true
}

// runTabRead reads the relation a table tab is bound to. An object tab has nothing to run.
func (model *Model) runTabRead(
	connection *app.Connection, tab *app.Tab,
) (tea.Model, tea.Cmd) {
	if tab.Kind == app.TabObject {
		return model, readObjectDDL(
			model.ActiveID(), tab.ID, connection.Session, tab.Object)
	}
	if tab.Kind != app.TabTable {
		return model.execute(connection, tab, connection.Session.Language().SplitStatements(
			tab.Editor.Text))
	}

	tab.View = resolveRowView(tab.View)
	read := tab.ComposeRelationRead(connection.Session)
	model.replaceResults(connection, tab,
		[]string{read.Display}, connection.Profile().PageSize)

	reads := []db.ComposedRead{read}
	runID := model.startBatch(connection, tab, reads, connection.Profile().PageSize)
	return model, runStatements(model.ActiveID(), tab.ID, runID, 0, connection.Session,
		reads, connection.Profile().PageSize, model.log, connection.Profile().Name)
}

// execute runs the statements of the user. It asks for the values of every `:name` mark
// first, and then for a yes where the profile says to.
func (model *Model) execute(
	connection *app.Connection, tab *app.Tab, statements []string,
) (tea.Model, tea.Cmd) {
	kept := make([]string, 0, len(statements))
	for _, statement := range statements {
		if strings.TrimSpace(statement) != "" {
			kept = append(kept, statement)
		}
	}
	if len(kept) == 0 {
		connection.Show("there is nothing to run")
		return model, nil
	}

	return model.buildRuns(connection, tab, kept, 0, nil,
		func(reads []db.ComposedRead) (tea.Model, tea.Cmd) {
			return model.executeBound(connection, tab, kept, reads)
		})
}

// buildRuns asks for the values of the `:name` marks of each statement in turn, one card per
// statement that holds a mark, and composes the read of that statement straight after, because
// the values of one statement are not the values of the next. A cancel closes the card and
// nothing runs.
func (model *Model) buildRuns(
	connection *app.Connection, tab *app.Tab, statements []string, at int,
	reads []db.ComposedRead, then func([]db.ComposedRead) (tea.Model, tea.Cmd),
) (tea.Model, tea.Cmd) {
	compose := func(written string) (db.ComposedRead, error) {
		bound, err := tab.BindParameters(connection.Session, written)
		if err != nil {
			return db.ComposedRead{}, err
		}
		return tab.ComposeStatementRead(connection.Session, bound), nil
	}
	for ; at < len(statements); at++ {
		names := statement.FindQueryParameters(statements[at])
		if len(names) == 0 {
			read, err := compose(statements[at])
			if err != nil {
				connection.ShowError(db.DescribeError(err))
				return model, nil
			}
			reads = append(reads, read)
			continue
		}
		kept := statement.ResolveParameterValues(names, tab.Parameters)
		written := statement.BuildParameterForm(names, kept)
		asked, after := statements[at], at+1
		held := reads
		connection.Overlay = app.Overlay{
			Kind: app.OverlayParameters, Names: names,
			Draft:       app.NewEditorBuffer(written, 0),
			ContentRows: strings.Count(written, "\n") + 1,
			Answers: app.OverlayAnswers{Values: func(given map[string]any) app.AnswerCommand {
				tab.Parameters = statement.ResolveParameterValues(names, given)
				read, err := compose(asked)
				if err != nil {
					connection.ShowError(db.DescribeError(err))
					return nil
				}
				_, command := model.buildRuns(connection, tab, statements, after,
					append(held, read), then)
				return carryAnswer(command)
			}},
		}
		return model, nil
	}
	return then(reads)
}

// executeBound runs the statements once every `:name` mark has a value.
func (model *Model) executeBound(
	connection *app.Connection, tab *app.Tab, kept []string, reads []db.ComposedRead,
) (tea.Model, tea.Cmd) {
	spoken := connection.Session.Language()
	// A read-only connection refuses a write before it reaches the server.
	risk := language.ResolveBatchRisk(kept, spoken)
	if connection.Profile().AccessMode == cfg.AccessReadOnly && risk != statement.RiskNone {
		connection.ShowError("this connection is read-only, so the statement was not sent")
		return model, nil
	}

	if db.NeedsConfirmation(connection.Profile().ConfirmWrites, risk) {
		question := statement.BuildConfirmation(connection.Profile().Name,
			string(connection.Profile().Environment), risk, kept)
		connection.Overlay = app.Overlay{
			Kind: app.OverlayConfirm, Title: " " + question.Title + " ", Body: question.Body,
			Answers: app.OverlayAnswers{Answer: func(confirmed bool) app.AnswerCommand {
				if !confirmed {
					return nil
				}
				return carryAnswer(model.startRun(connection, tab, kept, reads))
			}},
		}
		return model, nil
	}

	return model, model.startRun(connection, tab, kept, reads)
}

// replaceResults opens the entries of a run and takes off what belonged to the result they
// replace: the screen filter, and the staged work.
//
// A staged change names a row by its place in the result. A run puts other rows in those
// places, and a sort puts the same rows in other places, so a change that outlived either
// would be written to a row the reader never chose. Every path that replaces a result comes
// through here, so none of them can keep one.
func (model *Model) replaceResults(
	connection *app.Connection, tab *app.Tab, statements []string, pageSize int,
) {
	// Silence would read as the client losing what the reader typed.
	if dropped := core.CountChanges(tab.Pending); dropped > 0 {
		connection.Show(present.DescribeDroppedChanges(dropped))
	}
	tab.Results.Start(statements, pageSize)
	tab.Screen = present.NoScreenFilter()
	tab.DiscardChanges()
}

// startRun opens one entry per statement and asks the server for each of them in order.
func (model *Model) startRun(
	connection *app.Connection, tab *app.Tab, statements []string, reads []db.ComposedRead,
) tea.Cmd {
	pageSize := connection.Profile().PageSize
	model.replaceResults(connection, tab, statements, pageSize)

	// The answer is what the user asked for, so the keyboard goes to the pane that holds it.
	// The list of names belongs to the editor, so it closes with the move.
	connection.ResultVisible = true
	tab.Focus = app.PaneResult
	tab.Completion.Close()

	runID := model.startBatch(connection, tab, reads, pageSize)
	return runStatements(model.ActiveID(), tab.ID, runID, 0, connection.Session, reads,
		pageSize, model.log, connection.Profile().Name)
}

// startBatch opens a run of that tab and returns the number it is stamped with. The number
// tells the answers of this run from the answers of the run it replaced.
func (model *Model) startBatch(
	connection *app.Connection, tab *app.Tab, reads []db.ComposedRead, rowLimit int,
) int {
	return model.runs.start(
		batchKey{connectionID: model.ActiveID(), tabID: tab.ID},
		&runBatch{reads: reads, rowLimit: rowLimit, profileName: connection.Profile().Name},
	)
}

// findBatch returns the run of that tab, and whether the answer belongs to it. An answer of
// a run that was replaced belongs to nothing.
func (model *Model) findBatch(connectionID, tabID, runID int) (*runBatch, bool) {
	return model.runs.find(batchKey{connectionID: connectionID, tabID: tabID}, runID)
}

// stopBatch ends the run of a tab, whether it finished or a statement failed.
func (model *Model) stopBatch(connectionID, tabID int) {
	model.runs.stop(batchKey{connectionID: connectionID, tabID: tabID})
}

// findTab returns the tab of that id on a connection.
func findTab(connection *app.Connection, tabID int) (*app.Tab, bool) {
	for _, tab := range connection.Tabs {
		if tab.ID == tabID {
			return tab, true
		}
	}
	return nil, false
}

// findConnectionTab returns the connection and the tab a message names. A message can land
// after either of them was closed, so both are looked for.
func (model *Model) findConnectionTab(
	connectionID, tabID int,
) (*app.Connection, *app.Tab, bool) {
	connection, _, found := model.findConnection(connectionID)
	if !found {
		return nil, nil, false
	}
	tab, held := findTab(connection, tabID)
	if !held {
		return nil, nil, false
	}
	return connection, tab, true
}

// readQueryAnswer keeps what one statement of a run answered.
func (model *Model) readQueryAnswer(answered queryRanMsg) (tea.Model, tea.Cmd) {
	connection, _, found := model.findConnection(answered.ConnectionID)
	if !found {
		return model, nil
	}
	tab, held := findTab(connection, answered.TabID)
	if !held {
		// The tab was closed while the statement ran, so its run ends here and the
		// statements after this one are never asked for.
		model.stopBatch(answered.ConnectionID, answered.TabID)
		return model, nil
	}
	// An answer of a run the tab no longer holds is dropped. Its entry belongs to the run
	// that replaced it, and stopping that run would leave it waiting for nothing.
	if _, belongs := model.findBatch(answered.ConnectionID, answered.TabID, answered.RunID); !belongs {
		return model, nil
	}

	if answered.Problem != "" {
		tab.Results.Fail(answered.Index, answered.Problem)
		// The statements after this one were written for a state the server no longer
		// holds, so they are never asked for.
		tab.Results.SkipRest(answered.Index+1, "not run: an earlier statement failed")
		model.stopBatch(answered.ConnectionID, answered.TabID)
		return model, nil
	}
	if answered.Last {
		model.stopBatch(answered.ConnectionID, answered.TabID)
	}

	tab.Results.Succeed(answered.Index, answered.Read, answered.Result)
	model.placeResultCursor(connection, tab, answered.Result.Columns)
	tab.Target = model.resolveEditTarget(connection, tab)
	// A row is written through the columns of its relation, and a relation opened without
	// its fold being opened has none read yet, so they are asked for now.
	readColumns := model.readTargetColumns(connection, tab)

	// A first page that does not fill the pane is topped up, so a tall pane is not left
	// half empty until the reader scrolls.
	topUp := model.approachDrawnResultEnd(connection, tab)

	next := model.askNextStatement(connection, answered)

	// A statement that changed the objects of the server makes the tree stale.
	source := ""
	if active := tab.Results.Active(); active != nil {
		source = active.Source
	}
	if source != "" && connection.Session.Language().ChangesCatalog(source) {
		connection.Catalog.Loading = true
		return model, tea.Batch(readColumns, topUp, next,
			readCatalog(answered.ConnectionID, connection.Session, quietCatalogRead))
	}
	return model, tea.Batch(readColumns, topUp, next)
}

// askNextStatement asks the server for the statement after the one that just answered, and
// returns nothing where the run is over.
func (model *Model) askNextStatement(
	connection *app.Connection, answered queryRanMsg,
) tea.Cmd {
	batch, held := model.findBatch(answered.ConnectionID, answered.TabID, answered.RunID)
	if !held {
		return nil
	}
	return runStatements(answered.ConnectionID, answered.TabID, answered.RunID,
		answered.Index+1, connection.Session, batch.reads, batch.rowLimit,
		model.log, batch.profileName)
}

// readTargetColumns asks the server for the columns a result would be written through, and
// returns nothing where they are already read or already asked for.
func (model *Model) readTargetColumns(
	connection *app.Connection, tab *app.Tab,
) tea.Cmd {
	table := tab.Target.Table
	if table.Name == "" || table.Kind != db.RelationTable {
		return nil
	}
	tableID := present.BuildTableID(table)
	if _, asked := connection.Catalog.Details[tableID]; asked {
		return nil
	}
	connection.Catalog.Details[tableID] = present.TableDetailState{Kind: present.DetailLoading}
	return readTableDetail(model.ActiveID(), connection.Session, table)
}

// placeResultCursor puts the cursor of every view of the result where the result that landed
// asks for. A result whose columns are named as the ones before it is the same result read
// again, such as one the user sorted, so it keeps the cursor and only holds it inside the
// rows. A result of other columns starts at the first cell.
//
// The tree names an opened document by its place in the result, so a result read again puts
// the reader back on a different document. Its opened documents are dropped with the cursor.
func (model *Model) placeResultCursor(
	connection *app.Connection, tab *app.Tab, columns []query.ResultColumn,
) {
	names := make([]string, 0, len(columns))
	for _, column := range columns {
		names = append(names, column.Name)
	}
	key := strings.Join(names, "|")
	if key == tab.GridColumnKey {
		tab.GridRow = clamp(tab.GridRow, len(model.buildGridShape(connection, tab).Text))
		tab.TreeRow = clamp(tab.TreeRow, model.buildDocumentTree(connection, tab).CountRows())
		return
	}
	tab.GridColumnKey = key
	tab.GridRow, tab.GridColumn = 0, 0
	tab.GridRowOffset, tab.GridColumnOffset = 0, 0
	tab.TreeRow, tab.TreeRowOffset, tab.TreeRolled = 0, 0, false
	tab.Opened = map[string]bool{}
}

// resolveEditTarget returns the relation a result can be edited as. It follows every run,
// rather than being whatever the tree opened last.
func (model *Model) resolveEditTarget(
	connection *app.Connection, tab *app.Tab,
) app.EditTarget {
	if tab.Kind == app.TabTable {
		return model.buildEditTarget(connection, tab.Table)
	}

	active := tab.Results.Active()
	if active == nil || active.State.Kind != app.QuerySucceeded {
		return app.EditTarget{Reason: "nothing has run yet"}
	}
	source, single := connection.Session.Composer().FindStatementSource(active.Source)
	if !single {
		return app.EditTarget{
			Reason: "the rows are not the rows of one relation, so no row can be identified",
		}
	}

	table, known := model.findTableByName(connection, source)
	if !known {
		return app.EditTarget{Reason: "the catalog does not hold " + source.Name}
	}
	return model.buildEditTarget(connection, table)
}

// findTableByName matches the name a statement wrote against the tables the connection knows.
func (model *Model) findTableByName(
	connection *app.Connection, source statement.SelectSource,
) (db.TableRef, bool) {
	return db.FindTableByName(
		connection.Catalog.Tables, source, connection.Session.Describe().DefaultSchema)
}

// buildEditTarget returns what a result of that relation can be edited as.
func (model *Model) buildEditTarget(
	connection *app.Connection, table db.TableRef,
) app.EditTarget {
	target := app.EditTarget{Table: table}
	if table.Kind != db.RelationTable {
		target.Reason = "a " + string(table.Kind) + " is read, not written"
		return target
	}

	state, read := connection.Catalog.Details[present.BuildTableID(table)]
	switch {
	case read && state.Kind == present.DetailFailed:
		// A read that failed is not a read that is running, and saying so would leave the
		// user waiting for an answer that already came.
		target.Reason = "cannot read the columns of " + table.Name + ": " + state.Message
		return target
	case !read || state.Kind != present.DetailReady:
		target.Reason = "reading the columns of " + table.Name + "…"
		return target
	}

	target.Columns = state.Detail.Columns
	target.ForeignKeys = state.Detail.ForeignKeys
	for _, column := range state.Detail.Columns {
		if column.IsPrimaryKey {
			target.KeyColumns = append(target.KeyColumns, column.Name)
		}
	}
	if len(target.KeyColumns) == 0 {
		target.Reason = table.Name + " has no primary key, so no row can be identified"
		return target
	}
	target.Editable = true
	return target
}

// fetchMore reads the rows after the ones already drawn, for a reader who asked for them. A
// reader who asks and gets nothing is told why.
func (model *Model) fetchMore(
	connection *app.Connection, tab *app.Tab,
) (tea.Model, tea.Cmd) {
	if tab.Results.Active() == nil || !tab.Results.CanFetchMore() {
		connection.Show("every row of this result is already read")
		return model, nil
	}
	return model, model.readMoreRows(connection, tab)
}

// readMoreRows returns the command that reads the page after the rows already drawn, and
// nothing where a read of it is already on its way: the end of the rows can be reached again
// while one runs, and both reads would start at the same offset.
func (model *Model) readMoreRows(connection *app.Connection, tab *app.Tab) tea.Cmd {
	active := tab.Results.Active()
	if active == nil || active.FetchingMore {
		return nil
	}
	active.FetchingMore = true
	return readNextPage(model.ActiveID(), tab.ID, tab.Results.ActiveIndex(), active.ID,
		connection.Session, active.Read,
		db.ReadWindow{Limit: active.PageSize, Offset: len(active.State.Result.Rows)})
}

// readPageAnswer adds the next page to the result already drawn.
func (model *Model) readPageAnswer(answered pageReadMsg) (tea.Model, tea.Cmd) {
	connection, tab, found := model.findConnectionTab(answered.ConnectionID, answered.TabID)
	if !found {
		return model, nil
	}
	// A tab that ran again holds another result at the same position, so a page of the
	// run before it belongs to nothing and is dropped.
	paged := tab.Results.ResultAt(answered.Index)
	if paged == nil || paged.ID != answered.ResultID {
		return model, nil
	}
	if answered.Problem != "" {
		// The rows already read stay, so a failed page is only reported.
		paged.FetchingMore = false
		connection.ShowError(answered.Problem)
		return model, nil
	}
	tab.Results.AppendRows(answered.Index, answered.Result)
	// A page that still leaves the foot of the grid near the end is followed by the next
	// one, so a pane taller than a page fills itself. A page for a statement that is not
	// on screen, and one that brought no rows, lead nowhere: the second would be asked
	// for again at the same offset.
	if answered.Index != tab.Results.ActiveIndex() || len(answered.Result.Rows) == 0 {
		return model, nil
	}
	return model, model.approachDrawnResultEnd(connection, tab)
}

// countRows counts the whole result, once.
func (model *Model) countRows(
	connection *app.Connection, tab *app.Tab,
) (tea.Model, tea.Cmd) {
	active := tab.Results.Active()
	if active == nil || !tab.Results.CanCountRows() {
		connection.Show("this result cannot be counted without running it again")
		return model, nil
	}
	if active.HasTotalRows || active.Counting {
		return model, nil
	}
	// The count of a whole relation takes as long as the server needs to read it, so the
	// size says one is on its way.
	active.Counting = true
	return model, countRows(model.ActiveID(), tab.ID, tab.Results.ActiveIndex(), active.ID,
		connection.Session, active.Read)
}

// readCountAnswer keeps the count of the whole result.
func (model *Model) readCountAnswer(answered countedMsg) (tea.Model, tea.Cmd) {
	connection, tab, found := model.findConnectionTab(answered.ConnectionID, answered.TabID)
	if !found {
		return model, nil
	}
	// A tab that ran again holds another result at the same position, so a count of the
	// run before it belongs to nothing and is dropped.
	counted := tab.Results.ResultAt(answered.Index)
	if counted == nil || counted.ID != answered.ResultID {
		return model, nil
	}
	counted.Counting = false
	if answered.Problem != "" {
		connection.ShowError(answered.Problem)
		return model, nil
	}
	counted.TotalRows = answered.Total
	counted.HasTotalRows = answered.HasTotal
	return model, nil
}

// The two plans a server draws: one it works out, and one it works out by running the
// statement and measuring it.
const (
	explainOnly    = false
	explainAnalyze = true
)

// explain asks the server how it would run the statement, and draws the plan in place of the
// grid.
func (model *Model) explain(
	connection *app.Connection, tab *app.Tab, analyze bool,
) (tea.Model, tea.Cmd) {
	// The planner is given the values written in, so the marks are asked for first.
	asked := tab.StatementUnderCaret(connection.Session)
	if len(statement.FindQueryParameters(asked)) > 0 {
		return model.buildRuns(connection, tab, []string{asked}, 0, nil,
			func([]db.ComposedRead) (tea.Model, tea.Cmd) {
				return model.explainBound(connection, tab, analyze)
			})
	}
	return model.explainBound(connection, tab, analyze)
}

// explainBound asks the server for the plan once every `:name` mark has a value.
func (model *Model) explainBound(
	connection *app.Connection, tab *app.Tab, analyze bool,
) (tea.Model, tea.Cmd) {
	written := tab.StatementToExplain(connection.Session)
	if strings.TrimSpace(written) == "" {
		connection.Show("write a statement first")
		return model, nil
	}
	if !connection.Session.Capabilities().PlansEveryStatement &&
		!connection.Session.Language().CanExplain(written) {
		connection.Show("the server has no plan for this statement")
		return model, nil
	}

	// A measured plan runs the statement it measures. A statement that writes is only
	// ever estimated, whatever the key asked for: this view plans a statement, and a plan
	// is not how a write is run.
	if analyze && connection.Session.Language().ResolveWriteRisk(written) != statement.RiskNone {
		analyze = false
		connection.Show("this statement writes, so the plan is estimated and nothing ran")
	}

	tab.View = app.ViewPlan
	tab.ViewData = app.PaneContent{Kind: app.DataLoading, StartedAt: time.Now()}
	if active := tab.Results.Active(); active != nil {
		active.Plan = app.PlanState{Kind: app.PlanLoading}
	}
	return model, readPlan(model.ActiveID(), tab.ID, tab.ReadActiveResultID(),
		connection.Session, written, analyze)
}

// readPlanAnswer draws the plan the server sent.
func (model *Model) readPlanAnswer(answered planReadMsg) (tea.Model, tea.Cmd) {
	_, tab, found := model.findConnectionTab(answered.ConnectionID, answered.TabID)
	if !found {
		return model, nil
	}
	// A plan of a run that was replaced belongs to nothing, so it is dropped rather than
	// drawn over the result that took its place.
	active := tab.Results.Active()
	if tab.ReadActiveResultID() != answered.ResultID {
		return model, nil
	}
	if answered.Problem != "" {
		if active != nil {
			active.Plan = app.PlanState{
				Kind: app.PlanFailed, Message: answered.Problem,
			}
		}
		tab.ViewData = app.PaneContent{Kind: app.DataFailed, Message: answered.Problem}
		return model, nil
	}
	if active != nil {
		active.Plan = app.PlanState{Kind: app.PlanReady, Plan: answered.Plan}
	}
	tab.DetailOffset = 0
	tab.ViewData = app.PaneContent{Kind: app.DataPlan, Plan: answered.Plan}
	return model, nil
}

// showKeptPlan draws the plan the statement already has, and reports whether it had one. A
// view that describes a result writes into the one slot the pane draws from, so the plan is
// put back into it every time the plan is shown again.
func (model *Model) showKeptPlan(tab *app.Tab) bool {
	active := tab.Results.Active()
	if active == nil {
		return false
	}
	switch active.Plan.Kind {
	case app.PlanReady:
		tab.ViewData = app.PaneContent{Kind: app.DataPlan, Plan: active.Plan.Plan}
	case app.PlanFailed:
		tab.ViewData = app.PaneContent{
			Kind: app.DataFailed, Message: active.Plan.Message,
		}
	case app.PlanLoading:
		tab.ViewData = app.PaneContent{Kind: app.DataLoading, StartedAt: time.Now()}
	default:
		return false
	}
	return true
}

// readRelationViewAnswer draws what a view that describes a relation answered.
func (model *Model) readRelationViewAnswer(answered relationViewMsg) (tea.Model, tea.Cmd) {
	_, tab, found := model.findConnectionTab(answered.ConnectionID, answered.TabID)
	if !found {
		return model, nil
	}
	// A view the user left is not drawn over the one they moved to.
	if tab.View != answered.View {
		return model, nil
	}
	tab.DetailOffset = 0
	tab.ViewData = answered.Content
	return model, nil
}

// showSelectedResult draws the result now chosen. The edit target is built again, because
// a target belongs to the result on screen: a run of several statements reads several
// relations, and a row written through the target of another one lands in the wrong place.
func (model *Model) showSelectedResult(
	connection *app.Connection, tab *app.Tab,
) (tea.Model, tea.Cmd) {
	tab.Target = model.resolveEditTarget(connection, tab)
	return model.loadShownView(connection, tab)
}

// selectViewAt moves the pane to the view at that position of the strip.
func (model *Model) selectViewAt(
	connection *app.Connection, tab *app.Tab, index int,
) (tea.Model, tea.Cmd) {
	views := tab.Views(connection.Session)
	if index < 0 || index >= len(views) {
		return model, nil
	}
	tab.View = views[index]
	return model.loadShownView(connection, tab)
}

// stepView moves to the view before or after the one drawn.
func (model *Model) stepView(
	connection *app.Connection, tab *app.Tab, step int,
) (tea.Model, tea.Cmd) {
	views := tab.Views(connection.Session)
	if len(views) == 0 {
		return model, nil
	}
	at := 0
	drawn := tab.ActiveView(connection.Session)
	for index, view := range views {
		if view == drawn {
			at = index
			break
		}
	}
	tab.View = views[wrap(at+step, len(views))]
	return model.loadShownView(connection, tab)
}

// loadShownView asks for what the view now drawn needs.
func (model *Model) loadShownView(
	connection *app.Connection, tab *app.Tab,
) (tea.Model, tea.Cmd) {
	id := model.ActiveID()
	tab.DetailOffset = 0

	switch tab.ActiveView(connection.Session) {
	case app.ViewData:
		return model, nil

	case app.ViewPlan:
		// A plan already read is shown again, however many other views were looked at
		// in between, and an analyzed plan is not asked for again as an estimate.
		if model.showKeptPlan(tab) {
			return model, nil
		}
		return model.explain(connection, tab, explainOnly)

	// The fields of a result and the statistics of a statement are read from the result
	// itself, so the frame that draws them builds them.
	case app.ViewFields, app.ViewStatistics:
		return model, nil

	case app.ViewDDL:
		if tab.Kind == app.TabObject {
			tab.ViewData = app.PaneContent{Kind: app.DataLoading, StartedAt: time.Now()}
			return model, readObjectDDL(id, tab.ID, connection.Session, tab.Object)
		}
	}

	if tab.Kind != app.TabTable {
		tab.ViewData = app.PaneContent{
			Kind: app.DataIdle, Reason: "this tab describes the statement it ran",
		}
		return model, nil
	}
	tab.ViewData = app.PaneContent{Kind: app.DataLoading, StartedAt: time.Now()}
	return model, readRelationView(
		id, tab.ID, connection.Session, tab.Table, tab.ActiveView(connection.Session))
}

// buildStatistics writes what a statement with no result set did. The row count comes first,
// for an UPDATE or a DELETE.
func buildStatistics(tab *app.Tab) []app.Statistic {
	active := tab.Results.Active()
	if active == nil {
		return nil
	}
	succeeded := active.State.Kind == app.QuerySucceeded
	result := active.State.Result

	lines := []app.Statistic{}
	if succeeded && result.HasAffected {
		lines = append(lines, app.Statistic{
			Label: "updated rows", Value: strconv.FormatInt(result.Affected, 10),
			Leading: true,
		})
	}
	command := "unknown"
	if succeeded && result.Command != "" {
		command = result.Command
	}
	lines = append(lines, app.Statistic{Label: "command", Value: command})
	if succeeded {
		lines = append(lines, app.Statistic{
			Label: "execute time", Value: present.FormatDuration(result.Elapsed),
		})
	}
	if !active.StartedAt.IsZero() {
		lines = append(lines, app.Statistic{
			Label: "start time", Value: core.FormatClockTime(active.StartedAt),
		})
	}
	if !active.FinishedAt.IsZero() {
		lines = append(lines, app.Statistic{
			Label: "finish time", Value: core.FormatClockTime(active.FinishedAt),
		})
	}
	lines = append(lines, app.Statistic{
		Label: "query", Value: strings.TrimSpace(core.CollapseWhitespace(active.Source)),
	})
	return lines
}

// readChangesAnswer reports what applying the staged work did.
func (model *Model) readChangesAnswer(answered changesAppliedMsg) (tea.Model, tea.Cmd) {
	connection, tab, found := model.findConnectionTab(answered.ConnectionID, answered.TabID)
	if !found {
		return model, nil
	}
	tab.Applying = false
	if answered.Problem != "" {
		// A write the server refused leaves the work staged, so the tab stays open.
		tab.ClosingAfterApply = false
		connection.ShowError(answered.Problem)
		return model, nil
	}

	tab.DiscardChanges()
	connection.Overlay = app.Overlay{}
	connection.Show(present.FormatCountOf(
		int64(answered.Applied), "change", "changes") + " applied")
	if tab.ClosingAfterApply {
		tab.ClosingAfterApply = false
		connection.CloseTab(connection.IndexOfTab(tab.ID))
		return model, nil
	}
	return model.runTabRead(connection, tab)
}

// readTransactionAnswer reports what a transaction step did.
func (model *Model) readTransactionAnswer(answered transactionRanMsg) (tea.Model, tea.Cmd) {
	connection, _, found := model.findConnection(answered.ConnectionID)
	if !found {
		return model, nil
	}
	if answered.Problem != "" {
		connection.ShowError(answered.Problem)
		return model, nil
	}
	switch answered.Action {
	case ActionBeginTransaction:
		connection.Show("a transaction is open")
	case ActionCommitTransaction:
		connection.Show("committed")
	default:
		connection.Show("rolled back")
	}
	return model, nil
}

// readCancelAnswer reports what stopping the running statement did.
func (model *Model) readCancelAnswer(answered cancelledMsg) (tea.Model, tea.Cmd) {
	connection, _, found := model.findConnection(answered.ConnectionID)
	if !found {
		return model, nil
	}
	if answered.Problem != "" {
		connection.Overlay = app.Overlay{
			Kind: app.OverlayMessage, Title: " cancel failed ", Body: answered.Problem,
		}
		return model, nil
	}
	if answered.Stopped {
		connection.Show("cancel signal sent")
		return model, nil
	}
	connection.Show("nothing to cancel")
	return model, nil
}

// applyStagedChanges builds the statements the staged work asks for, and runs them.
func (model *Model) applyStagedChanges(
	connection *app.Connection, tab *app.Tab,
) (tea.Model, tea.Cmd) {
	// A read-only connection refuses a staged write before it reaches the server, the same
	// way a statement of the editor is refused.
	if connection.Profile().AccessMode == cfg.AccessReadOnly {
		connection.ShowError("this connection is read-only, so nothing was written")
		return model, nil
	}
	changes, err := model.buildChanges(connection, tab)
	if err != nil {
		connection.ShowError(db.DescribeError(err))
		return model, nil
	}
	if len(changes) == 0 {
		connection.Show("nothing is staged")
		return model, nil
	}
	// A server that cannot apply the set as one leaves the changes before a failure
	// written, so the question is put before anything is sent.
	if len(changes) > 1 && !connection.Session.Capabilities().AppliesChangesTogether {
		return model.askToApplyOneAtATime(connection, tab, changes)
	}
	tab.Applying = true
	return model, applyChanges(model.ActiveID(), tab.ID, connection.Session, changes)
}

// askToApplyOneAtATime asks before writing a set the server cannot apply as one. A
// standalone MongoDB holds no transaction, so a failure part way through leaves the
// changes before it written and the ones after it unwritten.
func (model *Model) askToApplyOneAtATime(
	connection *app.Connection, tab *app.Tab, changes []db.Change,
) (tea.Model, tea.Cmd) {
	id, tabID, session := model.ActiveID(), tab.ID, connection.Session
	connection.Overlay = app.Overlay{
		Kind:  app.OverlayConfirm,
		Title: " apply one at a time ",
		Body: "This server holds no transaction, so the " + strconv.Itoa(len(changes)) +
			" changes are applied one after the other. A failure part way through " +
			"leaves the ones before it written. Apply them?",
		Answers: app.OverlayAnswers{Answer: func(confirmed bool) app.AnswerCommand {
			if !confirmed {
				return nil
			}
			tab.Applying = true
			return carryAnswer(applyChanges(id, tabID, session, changes))
		}},
	}
	return model, nil
}

// stagedElsewhereMessage is what the bar says where the staged work belongs to another
// statement of the same run.
const stagedElsewhereMessage = "work is staged against another statement of this run; " +
	"show that one again to apply it, or discard the work"

// applyingMessage is what the bar says where the staged work is already with the server.
const applyingMessage = "the staged work is being written; wait for it to answer"

// describeStageRefusal returns why a change could not be staged.
func describeStageRefusal(tab *app.Tab) string {
	if tab.Applying {
		return applyingMessage
	}
	return stagedElsewhereMessage
}

// buildChanges turns the staged work of the grid into statements.
func (model *Model) buildChanges(
	connection *app.Connection, tab *app.Tab,
) ([]db.Change, error) {
	active := tab.Results.Active()
	if active == nil || active.State.Kind != app.QuerySucceeded {
		return nil, core.NewEditError("there is no result to write against")
	}
	// The rows below come from the result on screen, and the target names the relation
	// the staged work was staged against. They have to be the same one.
	if tab.Applying {
		return nil, core.NewEditError(applyingMessage)
	}
	if tab.HoldsChangesOfAnotherResult() {
		return nil, core.NewEditError(stagedElsewhereMessage)
	}
	if !tab.Target.Editable {
		return nil, core.NewEditError(tab.Target.Reason)
	}
	return connection.Session.Composer().BuildChanges(db.ChangeTarget{
		Table: tab.Target.Table, Columns: active.State.Result.Columns,
		Rows: active.State.Result.Rows, KeyColumns: tab.Target.KeyColumns,
	}, tab.Pending)
}

// readWhenShown reads what a restored tab describes, the first time it is shown. A
// connect draws every tab and reads only the one on screen.
func (model *Model) readWhenShown(connection *app.Connection) (tea.Model, tea.Cmd) {
	tab := connection.Active()
	if !connection.TakeUnread(tab) {
		return model, nil
	}
	return model.runTabRead(connection, tab)
}

// saveWorkspace writes the tabs of the connection into the history file, so the next
// connect opens what was left. The tabs are read here and stored away from the loop, so a
// key press never waits for the file.
func (model *Model) saveWorkspace(connection *app.Connection) tea.Cmd {
	if connection == nil || model.log == nil {
		return nil
	}
	log, name := model.log, connection.Profile().Name
	snapshot := connection.BuildWorkspaceSnapshot()
	return writeHistory(model.ActiveID(), "the tabs were not stored", func() error {
		return log.SaveWorkspace(name, snapshot)
	})
}
