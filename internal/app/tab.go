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

// EditTarget is the relation a result can be edited as, and the columns that name one row.
type EditTarget struct {
	Table      db.TableRef
	KeyColumns []string
	// The reason the rows cannot be edited, where they cannot.
	Reason string
	// True where the result can be edited.
	Editable bool
	// The columns of the relation, once read, so a cell editor can offer their values.
	Columns []db.ColumnDetail
	// The foreign keys of the relation, so `g` can follow one.
	ForeignKeys []query.ForeignKey
}

// booleanTypes are the types a server lists no members for, so the two of them stand here.
var booleanTypes = map[string]bool{"boolean": true, "bool": true, "tinyint(1)": true}

// FindColumnChoices returns the values a column takes, so a cell of an enum or of a boolean
// is picked rather than typed. The server lists the members of an enum. It does not list the
// two of a boolean.
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

// FindColumnProblem returns why a column cannot be written, or an empty text where it can.
func (target EditTarget) FindColumnProblem(name string) string {
	for _, column := range target.Columns {
		if strings.EqualFold(column.Name, name) && column.IsGenerated {
			return name + " is computed by the server, so it cannot be written"
		}
	}
	return ""
}

// FindState is what a search of the statement looks for, and what a replace writes in its
// place. Each tab keeps its own, so a search does not follow the reader to another statement.
type FindState struct {
	Term        string
	Replacement string
}

// Tab is one tab of a connection: what it is bound to, what it ran, and what is staged
// against the result.
type Tab struct {
	ID   int
	Kind TabKind
	// The relation a table tab is bound to.
	Table db.TableRef
	// The object an object tab shows.
	Object db.SchemaObject

	Editor  *EditorBuffer
	Results *ResultStore
	// The suggestions over the statement, which each tab keeps its own of.
	Completion CompletionList
	// The sort and the filter the grid laid over the read, kept apart from the text.
	Sort   []core.SortState
	Filter []core.FilterStep
	// True while the tab is waiting for its staged work to be applied before it closes.
	ClosingAfterApply bool
	// The values bound to the `:name` marks of the statement.
	Parameters map[string]any

	View ResultView
	// What the pane draws for a view that is not the grid.
	ViewData PaneContent
	// True while the plan view draws the text the server sent rather than the tree.
	RawPlan bool

	Focus Pane
	// The staged work of the grid, which is data until the moment it runs.
	Pending core.PendingChanges
	// PendingResultID names the result the staged work was staged against. A run of
	// several statements reads several relations, so work staged on one of them must
	// never be written back through another.
	PendingResultID int
	// True while the staged work is with the server. Nothing may be staged then, because
	// the answer throws the work away and would take an edit made in the meantime with
	// it, without a word.
	Applying bool
	// The undo stack of the staged work, so a change can be taken back.
	undone []core.PendingChanges
	redone []core.PendingChanges
	Target EditTarget
	// The cursor of the grid, and how far it has scrolled.
	GridRow          int
	GridColumn       int
	GridRowOffset    int
	GridColumnOffset int
	// True while the wheel moved the rows away from the cursor, so the cursor may stand
	// off screen until it moves again.
	GridRolled bool
	// The names of the columns the cursor belongs to. A result with the same names is the
	// same result read again, such as one the user sorted, so it keeps the cursor.
	GridColumnKey string
	// The columns that always draw at the left, whatever the window shows.
	Frozen map[int]bool
	// The width the reader set for a column by dragging its border. A column that is not
	// named here is as wide as the widest value it holds.
	ColumnWidths map[int]int
	// True while the values of a column whose name suggests a secret are shown.
	Unmasked bool
	// The filter over the rows already read, which hides rows and reads none.
	Screen present.ScreenFilter
	// How far the tree of the plan and the detail views have scrolled.
	DetailOffset int
	// The nodes of the document tree the reader opened, and where the cursor stands in it.
	// A folded document is one row, so a result of a million rows keeps only what was
	// opened rather than a state per row.
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

// Pane names which of the three panes of a tab holds the keyboard.
type Pane string

// The three panes a workspace holds.
const (
	PaneSidebar Pane = "sidebar"
	PaneEditor  Pane = "editor"
	PaneResult  Pane = "result"
)

// NewQueryTab opens a tab bound to the text in its editor.
func NewQueryTab(id int, sql string) *Tab {
	return newTab(id, TabQuery, sql)
}

// NewTableTab opens a tab bound to one relation, so it can describe it.
func NewTableTab(id int, table db.TableRef, preview string) *Tab {
	tab := newTab(id, TabTable, preview)
	tab.Table = table
	return tab
}

// NewObjectTab opens a tab that shows the definition of one schema object.
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

// IsBlank is true for a query tab with no statement and no run behind it, which is the tab
// a read opened from the tree or the history takes the place of.
func (tab *Tab) IsBlank() bool {
	return tab.Kind == TabQuery && strings.TrimSpace(tab.Editor.Text) == "" &&
		tab.Results.State().Kind == QueryIdle
}

// EditorVisible is true while the editor is drawn. Only a query tab has one.
func (tab *Tab) EditorVisible() bool {
	return tab.Kind == TabQuery
}

// Label returns the name the tab row draws. A query tab is named by the line comment it
// opens with, or by its first words.
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

// tabLabelWidth is how wide the name of a tab may be drawn.
const tabLabelWidth = 16

// Views returns the views this tab offers, with the plan left out where the server has none
// for the statement.
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
		// A result of plain columns has nothing to open, and a server that keeps no
		// document has nothing to write in the form that carries its types.
		case view == ViewTree && !opensDocuments:
			continue
		}
		kept = append(kept, view)
	}
	return kept
}

// opensDocuments is true where the result holds a value a tree opens.
func (tab *Tab) opensDocuments() bool {
	active := tab.Results.Active()
	if active == nil || active.State.Kind != QuerySucceeded {
		return false
	}
	return present.HasDocumentColumn(active.State.Result.Columns, active.State.Result.Rows)
}

// ActiveView returns the view drawn: the one asked for where this tab offers it, and the
// first one it offers otherwise. A statement that answered no rows offers no data view, so
// what a write did is drawn as its statistics.
func (tab *Tab) ActiveView(session db.SessionInfo) ResultView {
	return ResolveDrawnView(tab.Views(session), tab.View)
}

// ResolveDrawnView returns which of the views offered is drawn. A frame that already read
// the views of a tab hands them over rather than asking for them again.
func ResolveDrawnView(offered []ResultView, asked ResultView) ResultView {
	if slices.Contains(offered, asked) {
		return asked
	}
	if len(offered) > 0 {
		return offered[0]
	}
	return ViewData
}

// canExplain is true where the server has a plan for what this tab reads.
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

// BindParameters writes the values of the `:name` marks into the statement as bind values.
func (tab *Tab) BindParameters(
	session db.SessionInfo, written string,
) (db.BoundText, error) {
	return session.Composer().BindParameters(written, tab.Parameters)
}

// InlineParameters writes the values of the `:name` marks into the statement itself, for the
// display and for the planner, never for an ordinary run.
func (tab *Tab) InlineParameters(session db.SessionInfo, written string) string {
	shown, err := statement.InlineQueryParameters(
		written, tab.Parameters, session.Dialect())
	if err != nil {
		return written
	}
	return shown
}

// StatementToExplain returns the statement the plan view asks the server about.
func (tab *Tab) StatementToExplain(session db.SessionInfo) string {
	// The plan is asked for with the values written into the statement. A planner that
	// cannot see a value estimates wrongly, and a server plans no statement that still
	// carries a placeholder.
	if tab.Kind == TabTable {
		return tab.ComposeRelationRead(session).Display
	}
	// The values of the marks go in first, and the rewrite of the tab is laid around
	// what that gives.
	shown := tab.InlineParameters(session, tab.StatementUnderCaret(session))
	return strings.TrimSpace(
		tab.ComposeStatementRead(session, db.BoundText{Text: shown}).Display)
}

// StatementUnderCaret returns the selection, or the statement the caret stands in.
func (tab *Tab) StatementUnderCaret(session db.SessionInfo) string {
	if tab.Editor.HasSelection() {
		return strings.TrimSpace(tab.Editor.Selection())
	}
	return tab.Editor.ReadStatementAtCaret(session.Language())
}

// Rewrite returns the sort and the filter the grid laid on, which the engine reads through.
func (tab *Tab) Rewrite() core.ReadRewrite {
	return core.ReadRewrite{Sort: tab.Sort, Filter: tab.Filter}
}

// HasRewrite is true while the grid laid a sort or a filter over the read.
func (tab *Tab) HasRewrite() bool {
	return len(tab.Sort) > 0 || len(tab.Filter) > 0
}

// RewriteSummary writes the sort and the filter as the banner shows them.
func (tab *Tab) RewriteSummary(session db.SessionInfo) string {
	inlined := build.InlineFilter(tab.Filter, session.Dialect())
	text := ""
	if inlined != nil {
		text = inlined.Text
	}
	return statement.DescribeRewrite(tab.Sort, text)
}

// ComposeRelationRead returns the read of the relation a table tab is bound to.
func (tab *Tab) ComposeRelationRead(session db.SessionInfo) db.ComposedRead {
	return session.Composer().ComposeRelationRead(tab.Table, tab.Rewrite())
}

// ComposeStatementRead returns the read of a statement of the user, with the rewrite laid
// over it.
func (tab *Tab) ComposeStatementRead(session db.SessionInfo, statement db.BoundText) db.ComposedRead {
	return session.Composer().ComposeStatementRead(statement, tab.Rewrite())
}

// EffectiveSQL writes what the run will send, for the editor to reveal.
func (tab *Tab) EffectiveSQL(session db.SessionInfo) string {
	if tab.Kind == TabTable {
		return tab.ComposeRelationRead(session).Display
	}
	return tab.ComposeStatementRead(session, db.BoundText{Text: tab.Editor.Text}).Display
}

// ReadActiveResultID returns the id of the result on screen, and nothing where none of
// the statements has answered yet.
func (tab *Tab) ReadActiveResultID() int {
	active := tab.Results.Active()
	if active == nil {
		return 0
	}
	return active.ID
}

// StageChange keeps the staged work so far, so the change can be taken back. The work is
// stamped with the result it was staged against, and a change against another result of
// the same run is refused: the rows of one statement written through the relation of
// another would land in the wrong relation.
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

// HoldsChangesOfAnotherResult is true where work is staged against a result that is not
// the one on screen.
func (tab *Tab) HoldsChangesOfAnotherResult() bool {
	return core.CountChanges(tab.Pending) > 0 &&
		tab.PendingResultID != tab.ReadActiveResultID()
}

// UndoChange takes the last staged change back.
func (tab *Tab) UndoChange() bool {
	if len(tab.undone) == 0 {
		return false
	}
	tab.redone = append(tab.redone, copyPending(tab.Pending))
	tab.Pending = tab.undone[len(tab.undone)-1]
	tab.undone = tab.undone[:len(tab.undone)-1]
	return true
}

// RedoChange puts a change back on.
func (tab *Tab) RedoChange() bool {
	if len(tab.redone) == 0 {
		return false
	}
	tab.undone = append(tab.undone, copyPending(tab.Pending))
	tab.Pending = tab.redone[len(tab.redone)-1]
	tab.redone = tab.redone[:len(tab.redone)-1]
	return true
}

// DiscardChanges throws the staged work away.
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
	// Each inserted row is a map, so the rows are copied one by one. A shared map would
	// let a later edit write into the snapshot an undo restores.
	for _, row := range pending.Inserts {
		held := make(map[string]any, len(row))
		maps.Copy(held, row)
		copied.Inserts = append(copied.Inserts, held)
	}
	return copied
}
