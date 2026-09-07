package app

import (
	"strconv"
	"strings"

	"github.com/turanmahmudov/masume/internal/core"
	"github.com/turanmahmudov/masume/internal/hist"
	"github.com/turanmahmudov/masume/internal/notebook"
	"github.com/turanmahmudov/masume/internal/present"
	"github.com/turanmahmudov/masume/internal/query"
	"github.com/turanmahmudov/masume/internal/query/statement"
)

// A notebook tab holds an ordered list of cells. Each cell carries its own editor buffer,
// and the results of a run stay in the result store of the tab, one per statement.

// NotebookCell is one cell of an open notebook.
type NotebookCell struct {
	ID   string
	Kind notebook.CellKind
	// The whole fence line of a cell of an unknown kind.
	Fence  string
	Attrs  []notebook.Attribute
	Editor *EditorBuffer
	// What this cell keeps of the view of its own result: the view, the cursors, and the
	// sort and the filter that are part of the statement the run sends.
	State CellViewState
	// True while the source and the result of the cell are folded away.
	Folded bool
	// True for a cell the reader marked for a partial run.
	Marked bool
	// The first result of this cell in the store of the tab, and how many it has.
	// A cell that did not run in the last run has none.
	FirstResult int
	ResultCount int
	// True for a cell that answered before, and whose rows a later run of another cell
	// replaced.
	Stale bool
}

// FindAttr returns the value of that pair of the fence, and false where the cell has none.
func (cell *NotebookCell) FindAttr(name string) (string, bool) {
	for _, attr := range cell.Attrs {
		if attr.Name == name {
			return attr.Value, true
		}
	}
	return "", false
}

// AsksConfirmation is true for a cell whose fence asks for one more question before a write.
func (cell *NotebookCell) AsksConfirmation() bool {
	value, held := cell.FindAttr("write")
	return held && value == "confirm"
}

// RunsStatements is true for a cell the run sends to the server.
func (cell *NotebookCell) RunsStatements() bool {
	return cell.Kind == notebook.CellSQL
}

// Title returns the name of the cell: the first comment line of a statement, the first
// line of prose, the values of a parameter cell, or the columns of a chart.
func (cell *NotebookCell) BuildTitle() string {
	switch cell.Kind {
	case notebook.CellParam:
		return notebook.DescribeParameters(cell.Editor.Text)
	case notebook.CellChart:
		return notebook.NameChart(notebook.ReadChart(notebook.Cell{Attrs: cell.Attrs}))
	case notebook.CellText:
		return present.SafeText(readFirstLine(cell.Editor.Text))
	}
	if named := statement.FindQueryName(cell.Editor.Text); named != "" {
		return present.SafeText(named)
	}
	written := strings.TrimSpace(core.CollapseWhitespace(cell.Editor.Text))
	if written == "" {
		return "empty"
	}
	return present.SafeText(written)
}

// readFirstLine returns the first line that holds something.
func readFirstLine(text string) string {
	for _, line := range strings.Split(text, "\n") {
		if held := strings.TrimSpace(strings.TrimLeft(line, "#> ")); held != "" {
			return held
		}
	}
	return "empty"
}

// CellOutcomeKind is how a cell ended in the last run.
type CellOutcomeKind string

// The four states of a cell.
const (
	CellIdle    CellOutcomeKind = "idle"
	CellRunning CellOutcomeKind = "running"
	CellDone    CellOutcomeKind = "done"
	CellFailed  CellOutcomeKind = "failed"
	// CellStale is a cell that answered before, and whose rows a later run replaced.
	CellStale CellOutcomeKind = "stale"
)

// CellOutcome is what the last run of one cell left behind.
type CellOutcome struct {
	Kind CellOutcomeKind
	Rows int
	// The message of a failed cell.
	Message string
}

// Notebook is the open state of one notebook: its cells, its metadata and its run policy.
type Notebook struct {
	// The file of the notebook, or empty for one that was never saved.
	Path   string
	Origin notebook.Origin
	Title  string
	// The profiles the notebook is offered on.
	Profiles []string
	Engine   string
	Run      notebook.RunPolicy
	// The front matter as it was read, so a key this build does not know survives a save.
	FrontMatter string

	Cells []*NotebookCell
	// The cell the list stands on.
	Focused int
	// True while the caret is inside the focused cell.
	Editing bool
	// How far the list has scrolled.
	Offset int
	// True after the wheel moved the list away from the focused cell.
	Rolled bool
	// True after an edit the file does not hold yet.
	Dirty bool
	// True while the run of this notebook holds a transaction of its own.
	HoldsTransaction bool
	// True after the reader stopped the run. A stopped run runs no further cell, whatever
	// the error policy says.
	Stopped bool

	undone [][]*NotebookCell
	redone [][]*NotebookCell
}

// NewNotebookTab returns a tab that holds one notebook.
func NewNotebookTab(id int, book notebook.Notebook, path string, origin notebook.Origin) *Tab {
	tab := newTab(id, TabNotebook, "")
	tab.Notebook = buildNotebook(book, path, origin)
	tab.Editor = tab.Notebook.GetFocusedCell().Editor
	return tab
}

// buildNotebook returns the open state of a notebook that was read from a file.
func buildNotebook(book notebook.Notebook, path string, origin notebook.Origin) *Notebook {
	held := &Notebook{
		Path: path, Origin: origin, Title: book.Title, Profiles: book.Profiles,
		Engine: book.Engine, Run: book.Run, FrontMatter: book.FrontMatter,
	}
	for _, cell := range book.Cells {
		held.Cells = append(held.Cells, &NotebookCell{
			ID: cell.ID, Kind: cell.Kind, Fence: cell.Fence, Attrs: cell.Attrs,
			// The caret opens at the end of the cell, as it does for a restored tab.
			Editor: NewEditorBuffer(cell.Source, len(cell.Source)), FirstResult: -1,
		})
	}
	if len(held.Cells) == 0 {
		held.Cells = []*NotebookCell{held.buildCell(notebook.CellSQL)}
	}
	if held.Title == "" && path != "" {
		held.Title = notebook.ReadName(path)
	}
	return held
}

// NewNotebook returns an empty notebook with one statement cell.
func NewNotebook() notebook.Notebook {
	return notebook.Notebook{Run: notebook.DefaultPolicy()}
}

// buildCell returns a new empty cell with an id no other cell has.
func (book *Notebook) buildCell(kind notebook.CellKind) *NotebookCell {
	return &NotebookCell{
		ID: book.resolveFreeID("cell"), Kind: kind,
		Editor: NewEditorBuffer("", 0), FirstResult: -1,
	}
}

// resolveFreeID returns the name, or the name with a number where another cell holds it.
func (book *Notebook) resolveFreeID(wanted string) string {
	taken := map[string]bool{}
	for _, cell := range book.Cells {
		taken[cell.ID] = true
	}
	id := wanted
	for count := 2; taken[id]; count++ {
		id = wanted + "-" + strconv.Itoa(count)
	}
	return id
}

// FocusedCell returns the cell the list stands on.
func (book *Notebook) GetFocusedCell() *NotebookCell {
	if len(book.Cells) == 0 {
		book.Cells = []*NotebookCell{book.buildCell(notebook.CellSQL)}
	}
	book.Focused = core.ClampIndex(book.Focused, len(book.Cells))
	return book.Cells[book.Focused]
}

// CountCells returns how many cells the notebook holds.
func (book *Notebook) CountCells() int {
	return len(book.Cells)
}

// FindCellIndex returns where the cell of that id stands, and -1 where the notebook has none.
func (book *Notebook) FindCellIndex(id string) int {
	for at, cell := range book.Cells {
		if cell.ID == id {
			return at
		}
	}
	return -1
}

// Rewrite returns the sort and the filter of one cell.
func (cell *NotebookCell) BuildRewrite() core.ReadRewrite {
	return core.ReadRewrite{Sort: cell.State.Sort, Filter: cell.State.Filter}
}

// FindCellOfResult returns the cell one result of the store belongs to, and -1 where no
// cell holds it.
func (book *Notebook) FindCellOfResult(index int) int {
	for at, cell := range book.Cells {
		if cell.FirstResult < 0 {
			continue
		}
		if index >= cell.FirstResult && index < cell.FirstResult+cell.ResultCount {
			return at
		}
	}
	return -1
}

// MarkedCells returns the cells the reader marked, in the order they stand.
func (book *Notebook) ListMarkedCells() []int {
	marked := []int{}
	for at, cell := range book.Cells {
		if cell.Marked {
			marked = append(marked, at)
		}
	}
	return marked
}

// ClearMarks takes the mark off every cell.
func (book *Notebook) ClearMarks() {
	for _, cell := range book.Cells {
		cell.Marked = false
	}
}

// BuildDocument returns the notebook in the form the file holds.
func (book *Notebook) BuildDocument() notebook.Notebook {
	document := notebook.Notebook{
		Title: book.Title, Profiles: book.Profiles, Engine: book.Engine,
		Run: book.Run, FrontMatter: book.FrontMatter,
	}
	for _, cell := range book.Cells {
		document.Cells = append(document.Cells, notebook.Cell{
			ID: cell.ID, Kind: cell.Kind, Source: cell.Editor.Text,
			Fence: cell.Fence, Attrs: cell.Attrs,
		})
	}
	return document
}

// ReadParameterValues returns the values every parameter cell holds, and their problems.
func (book *Notebook) ReadParameterValues() (map[string]any, []string) {
	values := map[string]any{}
	problems := []string{}
	for _, cell := range book.Cells {
		if cell.Kind != notebook.CellParam {
			continue
		}
		held, found := notebook.ReadParameters(cell.Editor.Text)
		for name, value := range held {
			values[name] = value
		}
		problems = append(problems, found...)
	}
	return values, problems
}

// BuildChatNotebook returns the conversation as a notebook: the prose of every turn as
// text cells, and every statement the model wrote as a statement cell.
func BuildChatNotebook(messages []ChatMessage) notebook.Notebook {
	book := notebook.Notebook{Run: notebook.DefaultPolicy()}
	for _, message := range messages {
		if message.Role == hist.ChatRoleUser {
			book.Cells = append(book.Cells, notebook.Cell{
				Kind: notebook.CellText, Source: "**Question** " + oneLine(message.Content),
			})
			if book.Title == "" {
				book.Title = present.TruncateText(oneLine(message.Content), chatTitleWidth)
			}
			continue
		}
		for _, segment := range query.SplitMessageSegments(message.Content) {
			written := strings.TrimSpace(segment.Content)
			if written == "" {
				continue
			}
			kind := notebook.CellText
			if segment.Kind == query.SegmentSQL {
				kind = notebook.CellSQL
			}
			book.Cells = append(book.Cells, notebook.Cell{Kind: kind, Source: written})
		}
	}
	if book.Title == "" {
		book.Title = "chat notebook"
	}
	return notebook.Parse(notebook.Write(book))
}

// chatTitleWidth is how much of the first question becomes the title.
const chatTitleWidth = 48

// oneLine returns the text as one line.
func oneLine(text string) string {
	return strings.TrimSpace(core.CollapseWhitespace(text))
}

// BuildReplyNotebook returns one reply of the model as a notebook: its prose as text cells,
// every statement it wrote as a statement cell, and a parameter cell for the `:name` marks
// those statements hold.
func BuildReplyNotebook(reply, title string) notebook.Notebook {
	book := notebook.Notebook{Title: title, Run: notebook.DefaultPolicy()}
	names := []string{}
	seen := map[string]bool{}
	for _, segment := range query.SplitMessageSegments(reply) {
		written := strings.TrimSpace(segment.Content)
		if written == "" {
			continue
		}
		if segment.Kind != query.SegmentSQL {
			book.Cells = append(book.Cells,
				notebook.Cell{Kind: notebook.CellText, Source: written})
			continue
		}
		book.Cells = append(book.Cells,
			notebook.Cell{Kind: notebook.CellSQL, Source: written})
		for _, name := range statement.FindQueryParameters(written) {
			held := strings.ToLower(name)
			if !seen[held] {
				seen[held] = true
				names = append(names, held)
			}
		}
	}
	// A statement that binds a `:name` needs a value, so the notebook opens with a cell
	// that holds one line per mark for the reader to fill in.
	if len(names) > 0 {
		lines := make([]string, 0, len(names))
		for _, name := range names {
			lines = append(lines, name+" = ''")
		}
		book.Cells = append([]notebook.Cell{{
			Kind: notebook.CellParam, Source: strings.Join(lines, "\n"),
		}}, book.Cells...)
	}
	return notebook.Parse(notebook.Write(book))
}

// CellViewState is everything one cell keeps about the view of its own result: which view
// is drawn, where the cursors stand, and what the grid holds over it. The tab holds the
// state of the focused cell, and the cells hold the rest, so a sort of one cell reaches no
// other cell.
type CellViewState struct {
	View     ResultView
	ViewData PaneContent
	RawPlan  bool

	Sort   []core.SortState
	Filter []core.FilterStep

	GridRow          int
	GridColumn       int
	GridRowOffset    int
	GridColumnOffset int
	GridRolled       bool
	GridColumnKey    string
	Frozen           map[int]bool
	ColumnWidths     map[int]int
	Unmasked         bool
	Screen           present.ScreenFilter

	DetailOffset  int
	Opened        map[string]bool
	TreeRow       int
	TreeRowOffset int
	TreeRolled    bool

	EditorRowOffset    int
	EditorColumnOffset int
	EditorRolled       bool

	Find FindState

	Pending         core.PendingChanges
	PendingResultID int
	// The undo stacks of the staged changes of this cell.
	undone []core.PendingChanges
	redone []core.PendingChanges
	// True once this state was read off a tab. A cell that was never focused takes the
	// state of a new one.
	filled bool
}

// NewCellViewState returns the view state of a cell that was never focused.
func NewCellViewState() CellViewState {
	return CellViewState{
		View: DefaultView, Frozen: map[int]bool{}, ColumnWidths: map[int]int{},
		Opened: map[string]bool{}, Screen: present.NoScreenFilter(),
		Pending:  core.NewPendingChanges(),
		ViewData: PaneContent{Kind: DataIdle, Reason: idleCellReason},
	}
}

// idleCellReason is what the result pane of a cell that never ran says.
const idleCellReason = "run this cell to read its rows"

// ReadCellView returns the view state the tab holds now.
func ReadCellView(tab *Tab) CellViewState {
	return CellViewState{
		View: tab.View, ViewData: tab.ViewData, RawPlan: tab.RawPlan,
		Sort: tab.Sort, Filter: tab.Filter,
		GridRow: tab.GridRow, GridColumn: tab.GridColumn,
		GridRowOffset: tab.GridRowOffset, GridColumnOffset: tab.GridColumnOffset,
		GridRolled: tab.GridRolled, GridColumnKey: tab.GridColumnKey,
		Frozen: tab.Frozen, ColumnWidths: tab.ColumnWidths,
		Unmasked: tab.Unmasked, Screen: tab.Screen,
		DetailOffset: tab.DetailOffset, Opened: tab.Opened,
		TreeRow: tab.TreeRow, TreeRowOffset: tab.TreeRowOffset,
		TreeRolled:         tab.TreeRolled,
		EditorRowOffset:    tab.EditorRowOffset,
		EditorColumnOffset: tab.EditorColumnOffset, EditorRolled: tab.EditorRolled,
		Find:    tab.Find,
		Pending: tab.Pending, PendingResultID: tab.PendingResultID,
		undone: tab.undone, redone: tab.redone,
		filled: true,
	}
}

// ApplyCellView writes one view state onto the tab. A cell that was never focused takes the
// state of a new one, so its maps are never nil.
func ApplyCellView(tab *Tab, held CellViewState) {
	if !held.filled {
		held = NewCellViewState()
	}
	tab.View, tab.ViewData, tab.RawPlan = held.View, held.ViewData, held.RawPlan
	tab.Sort, tab.Filter = held.Sort, held.Filter
	tab.GridRow, tab.GridColumn = held.GridRow, held.GridColumn
	tab.GridRowOffset, tab.GridColumnOffset = held.GridRowOffset, held.GridColumnOffset
	tab.GridRolled, tab.GridColumnKey = held.GridRolled, held.GridColumnKey
	tab.Frozen, tab.ColumnWidths = held.Frozen, held.ColumnWidths
	tab.Unmasked, tab.Screen = held.Unmasked, held.Screen
	tab.DetailOffset, tab.Opened = held.DetailOffset, held.Opened
	tab.TreeRow, tab.TreeRowOffset = held.TreeRow, held.TreeRowOffset
	tab.TreeRolled = held.TreeRolled
	tab.EditorRowOffset, tab.EditorColumnOffset = held.EditorRowOffset, held.EditorColumnOffset
	tab.EditorRolled = held.EditorRolled
	tab.Find = held.Find
	tab.Pending, tab.PendingResultID = held.Pending, held.PendingResultID
	tab.undone, tab.redone = held.undone, held.redone
	if tab.Frozen == nil {
		tab.Frozen = map[int]bool{}
	}
	if tab.ColumnWidths == nil {
		tab.ColumnWidths = map[int]int{}
	}
	if tab.Opened == nil {
		tab.Opened = map[string]bool{}
	}
}

// SettleFocusedCell moves the tab onto the cell the notebook stands on: its buffer, and the
// view of its own result.
func (tab *Tab) SettleFocusedCell() {
	if tab.Notebook == nil {
		return
	}
	cell := tab.Notebook.GetFocusedCell()
	ApplyCellView(tab, cell.State)
	tab.Editor = cell.Editor
}

// KeepFocusedCell writes the view the tab holds now into the cell it belongs to.
func (tab *Tab) KeepFocusedCell() {
	if tab.Notebook == nil {
		return
	}
	tab.Notebook.GetFocusedCell().State = ReadCellView(tab)
}
