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

// EditTarget is the table a result can be edited through, and the columns that identify one
// row.
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

// booleanTypes are the types a server lists no values for, so the two values are here.
var booleanTypes = map[string]bool{"boolean": true, "bool": true, "tinyint(1)": true}

// FindColumnChoices returns the values of a column, so the user selects the value of an enum
// or a boolean cell instead of typing it. The server lists the values of an enum. It does not
// list the two values of a boolean.
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

// FindColumnProblem returns the reason a column cannot be written, or an empty text if it
// can.
func (target EditTarget) FindColumnProblem(name string) string {
	for _, column := range target.Columns {
		if strings.EqualFold(column.Name, name) && column.IsGenerated {
			return name + " is computed by the server, so it cannot be written"
		}
	}
	return ""
}

// FindState is the search term of the statement and the replacement text. Each tab has its
// own, so a search does not follow the user to another statement.
type FindState struct {
	Term        string
	Replacement string
}

// Tab is one tab of a connection: its binding, the last run, and the changes staged on the
// result.
type Tab struct {
	ID   int
	Kind TabKind
	// The table a table tab is bound to.
	Table db.TableRef
	// The object an object tab shows.
	Object db.SchemaObject

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
	// PendingResultID is the result the changes were staged on. A run of several
	// statements reads several tables, so a change staged on one of them must never be
	// written through another one.
	PendingResultID int
	// True while the staged changes are at the server. No change can be staged then,
	// because the answer discards the staged changes and would silently discard an edit
	// made in the meantime.
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
	// True while the wheel moved the rows away from the cursor, so the cursor can be off
	// screen until the next move.
	GridRolled bool
	// The names of the columns of the cursor. A result with the same names is the same
	// result read again, for example after a sort, so it keeps the cursor.
	GridColumnKey string
	// The columns that always draw at the left, whatever the window shows.
	Frozen map[int]bool
	// The width the user set for a column with a drag of its border. A column without an
	// entry is as wide as its widest value.
	ColumnWidths map[int]int
	// True while the values of a masked column are shown.
	Unmasked bool
	// The filter over the rows already read. It hides rows and reads none.
	Screen present.ScreenFilter
	// The scroll position of the plan tree and of the detail views.
	DetailOffset int
	// The open nodes of the document tree and the position of its cursor. A folded
	// document is one row, so a result of a million rows stores the open nodes only and
	// not a state per row.
	Opened        map[string]bool
	TreeRow       int
	TreeRowOffset int
	// True while the wheel moved the rows away from the cursor, so the cursor may stand
	// off screen until it moves again.
	TreeRolled bool
	// How far the editor has scrolled, down its lines and along them.
	EditorRowOffset    int
	EditorColumnOffset int
	// True while the wheel moved the lines away from the caret, so the caret may stand off
	// screen until it moves again.
	EditorRolled bool
	// What a search of the statement looks for, and what a replace writes in its place.
	Find FindState
	// What the server said about the buffer, with the buffer it was said about. A fault
	// the scanner found comes first, so the server is asked only where the scan is clean.
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

// IsBlank is true for a query tab without a statement and without a run. A read opened from
// the tree or the history replaces such a tab.
func (tab *Tab) IsBlank() bool {
	return tab.Kind == TabQuery && strings.TrimSpace(tab.Editor.Text) == "" &&
		tab.Results.State().Kind == QueryIdle
}

// EditorVisible is true while the editor is drawn. Only a query tab has an editor.
func (tab *Tab) EditorVisible() bool {
	return tab.Kind == TabQuery
}

// Label returns the name the tab row draws. A query tab uses its first line comment, or its
// first words.
func (tab *Tab) Label() string {
	switch tab.Kind {
	case TabTable:
		return tab.Table.Name
	case TabObject:
		return tab.Object.Name
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

// tabLabelWidth is the maximum width of the name of a tab.
const tabLabelWidth = 16

// Views returns the views of this tab. The plan is not included if the server has no plan for
// the statement.
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
		// A result of plain columns has nothing to open, and a server without documents
		// has no value to write in the form that keeps its types.
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

// ActiveView returns the view that is drawn: the selected view if this tab has it, and the
// first view of the tab otherwise. A statement without rows has no data view, so the result
// of a write is drawn as statistics.
func (tab *Tab) ActiveView(session db.SessionInfo) ResultView {
	return ResolveDrawnView(tab.Views(session), tab.View)
}

// ResolveDrawnView returns the view that is drawn. A frame that already read the views of a
// tab passes them in and does not read them again.
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

// BindParameters converts the values of the `:name` placeholders into bind values of the
// statement.
func (tab *Tab) BindParameters(
	session db.SessionInfo, written string,
) (db.BoundText, error) {
	return session.Composer().BindParameters(written, tab.Parameters)
}

// InlineParameters writes the values of the `:name` placeholders into the statement text, for
// the display and for the planner. A normal run never uses it.
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
	// The plan request contains the values in the statement text. A planner without the
	// values gives a wrong estimate, and a server does not plan a statement with a
	// placeholder.
	if tab.Kind == TabTable {
		return tab.ComposeRelationRead(session).Display
	}
	// The values of the placeholders are written first, and the rewrite of the tab is
	// applied to the result.
	shown := tab.InlineParameters(session, tab.StatementUnderCaret(session))
	return strings.TrimSpace(
		tab.ComposeStatementRead(session, db.BoundText{Text: shown}).Display)
}

// StatementUnderCaret returns the selection, or the statement at the caret.
func (tab *Tab) StatementUnderCaret(session db.SessionInfo) string {
	if tab.Editor.HasSelection() {
		return strings.TrimSpace(tab.Editor.Selection())
	}
	return tab.Editor.ReadStatementAtCaret(session.Language())
}

// Rewrite returns the sort and the filter of the grid, which the engine applies to the
// read.
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

// ComposeStatementRead returns the read of a statement of the user, with the rewrite
// applied.
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

// ReadActiveResultID returns the id of the result on screen, and nothing if no statement has
// answered yet.
func (tab *Tab) ReadActiveResultID() int {
	active := tab.Results.Active()
	if active == nil {
		return 0
	}
	return active.ID
}

// StageChange stores the current staged changes, so the new change can be undone. The
// changes carry the id of the result they were staged on, and a change on another result of
// the same run is rejected: the rows of one statement written through the table of another
// one would go to the wrong table.
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

// HoldsChangesOfAnotherResult is true if changes are staged on a result that is not the
// result on screen.
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
	// Each inserted row is a map, so the rows are copied one by one. With a shared map a
	// later edit would change the snapshot the undo restores.
	for _, row := range pending.Inserts {
		held := make(map[string]any, len(row))
		maps.Copy(held, row)
		copied.Inserts = append(copied.Inserts, held)
	}
	return copied
}
