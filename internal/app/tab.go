package app

import (
	"maps"
	"slices"
	"strings"

	"github.com/turanmahmudov/masume/internal/core"
	"github.com/turanmahmudov/masume/internal/db"
	"github.com/turanmahmudov/masume/internal/present"
	"github.com/turanmahmudov/masume/internal/query"
	"github.com/turanmahmudov/masume/internal/query/build"
	"github.com/turanmahmudov/masume/internal/query/editor"
	"github.com/turanmahmudov/masume/internal/query/statement"
)

// EditTarget is the table and key columns for result edits.
type EditTarget struct {
	Table      db.TableRef
	KeyColumns []string
	// The reason the rows cannot be edited, if they cannot.
	Reason string
	// True if the result can be edited.
	Editable bool
	// The columns of the table after the read, so the cell editor can offer their values.
	Columns []db.ColumnDetail
	// The foreign keys of the table, so `g` can follow one.
	ForeignKeys []query.ForeignKey
}

// booleanTypes is the set of column types with boolean choices.
var booleanTypes = map[string]bool{"boolean": true, "bool": true, "tinyint(1)": true}

// FindColumnChoices returns catalog enum values or boolean choices for a column.
func (target EditTarget) FindColumnChoices(name string) []string {
	for _, column := range target.Columns {
		if !strings.EqualFold(column.Name, name) {
			continue
		}
		if len(column.Choices) > 0 {
			return column.Choices
		}
		if booleanTypes[strings.ToLower(strings.TrimSpace(column.DataType))] {
			return []string{"true", "false"}
		}
		return nil
	}
	return nil
}

// FindColumnProblem reports generated columns that cannot be edited.
func (target EditTarget) FindColumnProblem(name string) string {
	for _, column := range target.Columns {
		if strings.EqualFold(column.Name, name) && column.IsGenerated {
			return name + " is a generated column and cannot be edited"
		}
	}
	return ""
}

// FindState is the tab search term and replacement text.
type FindState struct {
	Term        string
	Replacement string
}

// Tab is the editor, result, view, and staged changes for one connection tab.
type Tab struct {
	ID   int
	Kind TabKind
	// The table a table tab is bound to.
	Table db.TableRef
	// The object an object tab shows.
	Object db.SchemaObject
	// The cells a notebook tab holds. Only a notebook tab has one.
	Notebook *Notebook

	Editor  *EditorBuffer
	Results *ResultStore
	// The completion suggestions of the statement. Each tab has its own.
	Completion CompletionList
	// The sort and the filter the grid applies to the read, stored outside the text.
	Sort   []core.SortState
	Filter []core.FilterStep
	// True while the tab waits for its staged changes to be applied before it closes.
	ClosingAfterApply bool
	// The values of the `:name` placeholders of the statement.
	Parameters map[string]any

	View ResultView
	// The content the pane draws for a view that is not the grid.
	ViewData PaneContent
	// True while the plan view draws the text of the server and not the tree.
	RawPlan bool

	Focus Pane
	// The staged changes of the grid. They are data until the run.
	Pending core.PendingChanges
	// PendingResultID is the result associated with the staged changes.
	PendingResultID int
	// True while applying staged changes. Further staging is disabled.
	Applying bool
	// The undo stack of the staged changes, so a change can be undone.
	undone []core.PendingChanges
	redone []core.PendingChanges
	Target EditTarget
	// The cursor of the grid and its scroll position.
	GridRow          int
	GridColumn       int
	GridRowOffset    int
	GridColumnOffset int
	// True after scrolling independently of the grid cursor.
	GridRolled bool
	// The column key for retaining the cursor across compatible results.
	GridColumnKey string
	// The columns that always draw at the left, whatever the window shows.
	Frozen map[int]bool
	// User-defined column widths. Missing entries use automatic widths.
	ColumnWidths map[int]int
	// True while the values of a masked column are shown.
	Unmasked bool
	// The filter over the rows already read. It hides rows and reads none.
	Screen present.ScreenFilter
	// The scroll position of the plan tree and of the detail views.
	DetailOffset int
	// Expanded document nodes and the tree cursor position.
	Opened        map[string]bool
	TreeRow       int
	TreeRowOffset int
	// True after scrolling independently of the tree cursor.
	TreeRolled bool
	// How far the editor has scrolled, down its lines and along them.
	EditorRowOffset    int
	EditorColumnOffset int
	// True after scrolling independently of the editor caret.
	EditorRolled bool
	// What a search of the statement looks for, and what a replace writes in its place.
	Find FindState
	// Server diagnostics and the checked buffer text.
	Served ServedDiagnostics
}

// ServedDiagnostics is the answer of the server about one buffer.
type ServedDiagnostics struct {
	SQL   string
	Found []editor.Diagnostic
}

// Pane is the pane of a tab that has the keyboard focus.
type Pane string

// The three panes of a workspace.
const (
	PaneSidebar Pane = "sidebar"
	PaneEditor  Pane = "editor"
	PaneResult  Pane = "result"
)

// NewQueryTab returns a tab bound to the text in its editor.
func NewQueryTab(id int, sql string) *Tab {
	return newTab(id, TabQuery, sql)
}

// NewTableTab returns a tab bound to one table, so it can describe that table.
func NewTableTab(id int, table db.TableRef, preview string) *Tab {
	tab := newTab(id, TabTable, preview)
	tab.Table = table
	return tab
}

// NewObjectTab returns a tab that shows the definition of one schema object.
func NewObjectTab(id int, object db.SchemaObject) *Tab {
	tab := newTab(id, TabObject, "")
	tab.Object = object
	tab.View = ViewDDL
	return tab
}

func newTab(id int, kind TabKind, sql string) *Tab {
	return &Tab{
		ID: id, Kind: kind, Editor: NewEditorBuffer(sql, len(sql)),
		Results: NewResultStore(), View: DefaultView, Focus: PaneSidebar,
		Pending: core.NewPendingChanges(), Frozen: map[int]bool{},
		Screen: present.NoScreenFilter(), Parameters: map[string]any{},
		Opened:   map[string]bool{},
		ViewData: PaneContent{Kind: DataIdle, Reason: "write a query and run it"},
	}
}

// IsBlank is true for a query tab with an empty editor and no execution state.
func (tab *Tab) IsBlank() bool {
	return tab.Kind == TabQuery && strings.TrimSpace(tab.Editor.Text) == "" &&
		tab.Results.State().Kind == QueryIdle
}

// EditorVisible is true while the pane above the result is drawn. A query tab draws the
// editor there, and a notebook tab draws its cells.
func (tab *Tab) EditorVisible() bool {
	return tab.Kind == TabQuery || tab.Kind == TabNotebook
}

// EditsText is true while the pane above the result takes typed characters.
func (tab *Tab) EditsText() bool {
	if tab.Kind == TabNotebook {
		return tab.Notebook != nil && tab.Notebook.Editing
	}
	return tab.Kind == TabQuery
}

// EditsStatements is true while the text being written is statements of the engine. The
// prose of a notebook is checked against no schema and coloured as no SQL.
func (tab *Tab) EditsStatements() bool {
	if tab.Kind != TabNotebook {
		return true
	}
	if tab.Notebook == nil {
		return false
	}
	return tab.Notebook.GetFocusedCell().RunsStatements()
}

// ListsCells is true while the cell list of a notebook holds the keyboard.
func (tab *Tab) ListsCells() bool {
	return tab.Kind == TabNotebook && tab.Notebook != nil && !tab.Notebook.Editing
}

// ReadCellOutcome returns what the last run of one cell left behind.
func (tab *Tab) ReadCellOutcome(cell *NotebookCell) CellOutcome {
	if cell == nil {
		return CellOutcome{Kind: CellIdle}
	}
	if cell.FirstResult < 0 || cell.ResultCount == 0 {
		if cell.Stale {
			return CellOutcome{Kind: CellStale}
		}
		return CellOutcome{Kind: CellIdle}
	}
	outcome := CellOutcome{Kind: CellDone}
	for at := cell.FirstResult; at < cell.FirstResult+cell.ResultCount; at++ {
		held := tab.Results.ResultAt(at)
		if held == nil {
			return CellOutcome{Kind: CellIdle}
		}
		switch held.State.Kind {
		case QueryRunning:
			return CellOutcome{Kind: CellRunning}
		case QueryFailed:
			return CellOutcome{Kind: CellFailed, Message: held.State.Message}
		case QueryIdle:
			return CellOutcome{Kind: CellIdle}
		}
		outcome.Rows += len(held.State.Result.Rows)
	}
	return outcome
}

// Label returns the table name, object name, notebook name, query name, or shortened
// editor text.
func (tab *Tab) Label() string {
	switch tab.Kind {
	case TabTable:
		return tab.Table.Name
	case TabObject:
		return tab.Object.Name
	case TabNotebook:
		return present.TruncateText(tab.NotebookName(), tabLabelWidth+2)
	}
	if named := statement.FindQueryName(tab.Editor.Text); named != "" {
		return present.TruncateText(named, tabLabelWidth+2)
	}
	written := strings.TrimSpace(core.CollapseWhitespace(tab.Editor.Text))
	if written == "" {
		return "empty"
	}
	return present.TruncateText(written, tabLabelWidth)
}

// NotebookName returns the name of the notebook of this tab.
func (tab *Tab) NotebookName() string {
	if tab.Notebook == nil {
		return "notebook"
	}
	if tab.Notebook.Title != "" {
		return tab.Notebook.Title
	}
	return "notebook"
}

// tabLabelWidth is the maximum width of the name of a tab.
const tabLabelWidth = 16

// Views returns available views, excluding unsupported plans and non-document tree views.
func (tab *Tab) Views(session db.SessionInfo) []ResultView {
	hasResultSet := true
	active := tab.Results.Active()
	if active != nil && active.State.Kind == QuerySucceeded {
		hasResultSet = len(active.State.Result.Columns) > 0
	}
	offered := ListOfferedViews(tab.Kind, hasResultSet)

	opensDocuments := tab.opensDocuments()
	kept := make([]ResultView, 0, len(offered))
	for _, view := range offered {
		switch {
		case view == ViewPlan && !tab.canExplain(session):
			continue
		// The document view requires document values.
		case view == ViewTree && !opensDocuments:
			continue
		}
		kept = append(kept, view)
	}
	return kept
}

// opensDocuments is true if the result holds a value the tree can open.
func (tab *Tab) opensDocuments() bool {
	active := tab.Results.Active()
	if active == nil || active.State.Kind != QuerySucceeded {
		return false
	}
	return present.HasDocumentColumn(active.State.Result.Columns, active.State.Result.Rows)
}

// ActiveView returns the selected view when available, or the first available view.
func (tab *Tab) ActiveView(session db.SessionInfo) ResultView {
	return ResolveDrawnView(tab.Views(session), tab.View)
}

// ResolveDrawnView selects an available view, defaulting to ViewData for an empty list.
func ResolveDrawnView(offered []ResultView, asked ResultView) ResultView {
	if slices.Contains(offered, asked) {
		return asked
	}
	if len(offered) > 0 {
		return offered[0]
	}
	return ViewData
}

// canExplain is true if the server has a plan for the read of this tab.
func (tab *Tab) canExplain(session db.SessionInfo) bool {
	if !session.Capabilities().PlansStatement {
		return false
	}
	if tab.Kind == TabTable {
		return true
	}
	if tab.Kind == TabObject {
		return false
	}
	statement := tab.StatementToExplain(session)
	if strings.TrimSpace(statement) == "" {
		return false
	}
	if session.Capabilities().PlansEveryStatement {
		return true
	}
	return session.Language().CanExplain(statement)
}

// BindParameters binds named parameters through the connection composer.
func (tab *Tab) BindParameters(
	session db.SessionInfo, written string,
) (db.BoundText, error) {
	return session.Composer().BindParameters(written, tab.Parameters)
}

// InlineParameters substitutes literals for display and plan requests, preserving the original text on failure.
func (tab *Tab) InlineParameters(session db.SessionInfo, written string) string {
	shown, err := statement.InlineQueryParameters(
		written, tab.Parameters, session.Dialect())
	if err != nil {
		return written
	}
	return shown
}

// StatementToExplain returns the statement the plan view sends to the server.
func (tab *Tab) StatementToExplain(session db.SessionInfo) string {
	// Plan requests use inline parameter values.
	if tab.Kind == TabTable {
		return tab.ComposeRelationRead(session).Display
	}
	// Apply the tab rewrite after parameter substitution.
	shown := tab.InlineParameters(session, tab.StatementToPlan(session))
	return strings.TrimSpace(
		tab.ComposeStatementRead(session, db.BoundText{Text: shown}).Display)
}

// StatementToPlan returns the active batch statement or the statement under the caret.
func (tab *Tab) StatementToPlan(session db.SessionInfo) string {
	if len(tab.Results.Results()) < 2 {
		return tab.StatementUnderCaret(session)
	}
	active := tab.Results.Active()
	if active == nil || active.Source == "" {
		return tab.StatementUnderCaret(session)
	}
	return active.Source
}

// StatementUnderCaret returns the selection, or the statement at the caret.
func (tab *Tab) StatementUnderCaret(session db.SessionInfo) string {
	if tab.Editor.HasSelection() {
		return strings.TrimSpace(tab.Editor.Selection())
	}
	return tab.Editor.ReadStatementAtCaret(session.Language())
}

// Rewrite returns the grid sort and filter.
func (tab *Tab) Rewrite() core.ReadRewrite {
	return core.ReadRewrite{Sort: tab.Sort, Filter: tab.Filter}
}

// HasRewrite is true while the grid applies a sort or a filter to the read.
func (tab *Tab) HasRewrite() bool {
	return len(tab.Sort) > 0 || len(tab.Filter) > 0
}

// RewriteSummary returns the sort and the filter in the form of the banner.
func (tab *Tab) RewriteSummary(session db.SessionInfo) string {
	inlined := build.InlineFilter(tab.Filter, session.Dialect())
	text := ""
	if inlined != nil {
		text = inlined.Text
	}
	return statement.DescribeRewrite(tab.Sort, text)
}

// ComposeRelationRead returns the read of the table a table tab is bound to.
func (tab *Tab) ComposeRelationRead(session db.SessionInfo) db.ComposedRead {
	return session.Composer().ComposeRelationRead(tab.Table, tab.Rewrite())
}

// ComposeStatementRead composes a query with the tab rewrite.
func (tab *Tab) ComposeStatementRead(session db.SessionInfo, statement db.BoundText) db.ComposedRead {
	return session.Composer().ComposeStatementRead(statement, tab.Rewrite())
}

// EffectiveSQL returns the statement the run sends, for the editor to show.
func (tab *Tab) EffectiveSQL(session db.SessionInfo) string {
	if tab.Kind == TabTable {
		return tab.ComposeRelationRead(session).Display
	}
	return tab.ComposeStatementRead(session, db.BoundText{Text: tab.Editor.Text}).Display
}

// ReadActiveResultID returns the active result ID, or zero when absent.
func (tab *Tab) ReadActiveResultID() int {
	active := tab.Results.Active()
	if active == nil {
		return 0
	}
	return active.ID
}

// StageChange snapshots staged changes and applies a new change. Staging is refused during execution or for a different result.
func (tab *Tab) StageChange(change func(*core.PendingChanges)) bool {
	if tab.Applying {
		return false
	}
	active := tab.ReadActiveResultID()
	if core.CountChanges(tab.Pending) == 0 {
		tab.PendingResultID = active
	} else if tab.PendingResultID != active {
		return false
	}
	tab.undone = append(tab.undone, copyPending(tab.Pending))
	tab.redone = nil
	pending := copyPending(tab.Pending)
	change(&pending)
	tab.Pending = pending
	return true
}

// HoldsChangesOfAnotherResult is true for staged changes on a different result.
func (tab *Tab) HoldsChangesOfAnotherResult() bool {
	return core.CountChanges(tab.Pending) > 0 &&
		tab.PendingResultID != tab.ReadActiveResultID()
}

// UndoChange undoes the last staged change.
func (tab *Tab) UndoChange() bool {
	if len(tab.undone) == 0 {
		return false
	}
	tab.redone = append(tab.redone, copyPending(tab.Pending))
	tab.Pending = tab.undone[len(tab.undone)-1]
	tab.undone = tab.undone[:len(tab.undone)-1]
	return true
}

// RedoChange restores an undone change.
func (tab *Tab) RedoChange() bool {
	if len(tab.redone) == 0 {
		return false
	}
	tab.undone = append(tab.undone, copyPending(tab.Pending))
	tab.Pending = tab.redone[len(tab.redone)-1]
	tab.redone = tab.redone[:len(tab.redone)-1]
	return true
}

// DiscardChanges deletes the staged changes.
func (tab *Tab) DiscardChanges() {
	tab.undone = nil
	tab.redone = nil
	tab.Pending = core.NewPendingChanges()
	tab.PendingResultID = 0
}

func copyPending(pending core.PendingChanges) core.PendingChanges {
	copied := core.NewPendingChanges()
	maps.Copy(copied.Edits, pending.Edits)
	for row := range pending.DeletedRows {
		copied.DeletedRows[row] = true
	}
	// Copy each inserted row map for the undo snapshot.
	for _, row := range pending.Inserts {
		held := make(map[string]any, len(row))
		maps.Copy(held, row)
		copied.Inserts = append(copied.Inserts, held)
	}
	return copied
}
