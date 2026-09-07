package app

import (
	"time"

	"github.com/turanmahmudov/masume/internal/cfg"
	"github.com/turanmahmudov/masume/internal/core"
	"github.com/turanmahmudov/masume/internal/db"
	"github.com/turanmahmudov/masume/internal/hist"
	"github.com/turanmahmudov/masume/internal/load"
	"github.com/turanmahmudov/masume/internal/notebook"
	"github.com/turanmahmudov/masume/internal/present"
	"github.com/turanmahmudov/masume/internal/query/result"
	"github.com/turanmahmudov/masume/internal/writeplan"
)

// OverlayKind is the workspace overlay category.
type OverlayKind string

// The overlays the client draws.
const (
	OverlayNone        OverlayKind = "none"
	OverlayHelp        OverlayKind = "help"
	OverlayHistory     OverlayKind = "history"
	OverlayRowDetail   OverlayKind = "row-detail"
	OverlayCell        OverlayKind = "cell"
	OverlayCellEdit    OverlayKind = "cell-edit"
	OverlayChanges     OverlayKind = "changes"
	OverlayValueFilter OverlayKind = "value-filter"
	OverlayParameters  OverlayKind = "parameters"
	OverlayPalette     OverlayKind = "palette"
	OverlayObjectMenu  OverlayKind = "object-menu"
	OverlayCopyMenu    OverlayKind = "copy-menu"
	OverlayThemePicker OverlayKind = "theme-picker"
	OverlayActionMenu  OverlayKind = "action-menu"
	OverlayDiagram     OverlayKind = "diagram"
	OverlaySaved       OverlayKind = "saved"
	// OverlayNotebooks lists the notebooks of the project and of the user.
	OverlayNotebooks OverlayKind = "notebooks"
	// OverlayChart is the form of one chart cell.
	OverlayChart     OverlayKind = "chart"
	OverlayActivity  OverlayKind = "activity"
	OverlayMessage   OverlayKind = "message"
	OverlayAiChat    OverlayKind = "ai-chat"
	OverlayAiChats   OverlayKind = "ai-chats"
	OverlayConfirm   OverlayKind = "confirm"
	OverlayWritePlan OverlayKind = "write-plan"
	OverlayChoice    OverlayKind = "choice"
	OverlayExport    OverlayKind = "export"
	OverlayImport    OverlayKind = "import"
	OverlayPrompt    OverlayKind = "prompt"
)

// WholeRow is the row index a cell editor uses when it holds a whole new row.
const WholeRow = -1

// MenuAction is one row of a menu: its name, what it does, and whether it destroys data.
type MenuAction struct {
	ID    string
	Label string
	// The optional action target icon.
	Icon cfg.IconKind
	// The key this row is bound to, so the menu offers what a key also reaches.
	Chord       string
	Detail      string
	Destructive bool
}

// Choice is one answer to a question with more than two answers.
type Choice struct {
	MenuAction
	// The letter that picks it, shown at the start of its line.
	Key string
}

// PaletteAction is one entry of the command palette.
type PaletteAction struct {
	ID     string
	Label  string
	Detail string
	// Its key, drawn in the same column as the keys of a menu.
	Chord string
}

// PromptKind is the one-line input category.
type PromptKind string

// The prompts the workspace opens.
const (
	PromptNone       PromptKind = ""
	PromptTabName    PromptKind = "tab-name"
	PromptWhere      PromptKind = "where"
	PromptSearch     PromptKind = "search"
	PromptGoToColumn PromptKind = "go-to-column"
	PromptSaveName   PromptKind = "save-name"
	PromptFind       PromptKind = "find"
	// PromptCellName names the focused cell of a notebook.
	PromptCellName PromptKind = "cell-name"
	// PromptNotebookName is the name a notebook is saved under.
	PromptNotebookName PromptKind = "notebook-name"
	// PromptAiNotebook is what a notebook the model builds is to cover.
	PromptAiNotebook PromptKind = "ai-notebook"
	// PromptNotebookReport is the file the report of a notebook is written to.
	PromptNotebookReport PromptKind = "notebook-report"
	// PromptNotebookRename is the new name of a notebook file.
	PromptNotebookRename PromptKind = "notebook-rename"
	PromptReplace        PromptKind = "replace"
)

// ListState is the shared selection, scroll, and filter state for overlay lists.
type ListState struct {
	// The cursor of the list an overlay draws, and how far it has scrolled.
	Cursor int
	Offset int
	// True after scrolling independently of the cursor.
	Rolled bool
	// The term the field at the top of a list holds.
	Term string
}

// RowWindow is the rows already read, which the row viewer steps through.
type RowWindow struct {
	Columns []db.ResultColumn
	Rows    [][]any
	Index   int
}

// CellTarget is the one cell the viewer shows and the editor writes.
type CellTarget struct {
	Column      db.ResultColumn
	Value       any
	RowIndex    int
	ColumnIndex int
	// Allowed values for selection instead of text input.
	Choices []string
}

// ChartRequest is the chart the form of a chart cell is building.
type ChartRequest struct {
	// The cell the form writes to.
	Cell string
	// The cell the chart reads, and the two columns it draws.
	Source string
	Label  string
	Value  string
	Shape  string
	// True while the rows are sorted by the value, largest first.
	SortsByValue bool
	// Top is how many rows the chart draws. Zero draws every row.
	Top int
}

// ExportRequest is the export the form is writing.
type ExportRequest struct {
	Path     string
	Format   result.ExportFormat
	CSV      result.CSVOptions
	RowCount int
	// True for the complete query result instead of loaded rows only.
	WholeRead bool
}

// ImportStage is where an import stands.
type ImportStage string

const (
	// ImportPick is the stage that picks the file out of a directory.
	ImportPick ImportStage = "pick"
	// ImportFile is the stage that asks for the file and how it is read.
	ImportFile ImportStage = "file"
	// ImportMapping is the stage that maps the columns, after the file was read.
	ImportMapping ImportStage = "mapping"
	// ImportReview is the stage that shows what the import would do, and its SQL.
	ImportReview ImportStage = "review"
)

// ImportRequest is the import the form is building.
type ImportRequest struct {
	Stage       ImportStage
	Plan        load.Plan
	TargetNames []string
	// True after an explicit format choice disables filename-based detection.
	FormatChosen    bool
	DelimiterChosen bool
	Report          load.CheckReport
	Statements      []string
	Running         bool
	Written         int
}

// AnswerCommand is a deferred command returned to the UI loop.
type AnswerCommand func() any

// OverlayAnswers is the overlay response callbacks. Each callback returns a deferred command for the UI loop.
type OverlayAnswers struct {
	// What the answer of a question runs.
	Answer func(bool) AnswerCommand
	// What the answer of a menu or a choice runs.
	ID func(string) AnswerCommand
	// What the answer of a value filter runs.
	Kept func(map[string]bool) AnswerCommand
	// What the answer of a parameter form runs.
	Values func(map[string]any) AnswerCommand
}

// Overlay is the active workspace dialog state. Kind is the dialog category; unused fields remain empty.
type Overlay struct {
	Kind  OverlayKind
	Title string
	Body  string

	List ListState

	Entries []hist.HistoryEntry
	Saved   []SavedRow
	// The notebooks the card lists.
	Notebooks []notebook.Entry
	Actions   []MenuAction
	Palette   []PaletteAction
	Choices   []Choice
	Sessions  []db.Activity
	Changes   []db.Change
	Lines     []string

	// The last reading of the server, and the state of the card the reader set.
	Server ServerReading
	View   DashboardView

	Window RowWindow
	Cell   CellTarget
	// What the form of a chart cell holds.
	Chart ChartRequest
	// What the write of the card would do.
	Plan    writeplan.Plan
	Export  ExportRequest
	Import  ImportRequest
	Answers OverlayAnswers

	// The content height fixed when the dialog opens.
	ContentRows int
	// What a key of the card reported, drawn before the keys.
	Notice string
	// The text a field of the overlay holds.
	Draft *EditorBuffer

	// The values of one column, and the ones kept on screen.
	Values []present.ValueCount
	Kept   map[string]bool

	// The `:name` marks of the statement, and the values the user filled in.
	Names []string

	// The active form field index.
	Field int

	// The action menu key scope, with global fallback for unbound actions.
	Scope cfg.KeyScope

	// The prompt a one-line field is asking for.
	Prompt PromptKind
	// The input hint below the prompt.
	Hint string
}

// The panels the dashboard folds.
const (
	PanelBlocking = "blocking"
	PanelSlow     = "slow"
)

// DashboardPanels is the list of collapsible dashboard panels.
var DashboardPanels = []string{PanelBlocking, PanelSlow}

// ServerReading is what one read of the server answered.
type ServerReading struct {
	Load  db.ServerLoad
	Locks []db.LockWait
	Slow  []db.StatementStat
	// Available parts of the server response.
	HasLoad  bool
	HasLocks bool
	HasSlow  bool
	ReadAt   time.Time
}

// DashboardView is the retained panel state and previous sample for rate calculations.
type DashboardView struct {
	Folded map[string]bool
	// True while a server request is pending.
	Reading bool
	// The reading before the one on screen, which a rate is measured against.
	Previous    db.ServerLoad
	PreviousAt  time.Time
	HasPrevious bool
}

// ResolveCounterRate returns the increase per second, or false for nonpositive duration or a decreasing counter.
func ResolveCounterRate(before, after int64, span time.Duration) (float64, bool) {
	if span <= 0 || after < before {
		return 0, false
	}
	return float64(after-before) / span.Seconds(), true
}

// IsPanelFolded is true where the reader folded that panel away.
func (view DashboardView) IsPanelFolded(panel string) bool {
	return view.Folded[panel]
}

// FoldPanel folds a panel away, or opens it again.
func (view *DashboardView) FoldPanel(panel string, folded bool) {
	if view.Folded == nil {
		view.Folded = map[string]bool{}
	}
	view.Folded[panel] = folded
}

// IsOpen is true while an overlay owns the keyboard.
func (overlay Overlay) IsOpen() bool {
	return overlay.Kind != OverlayNone && overlay.Kind != ""
}

// Object menu action IDs.
const (
	ObjectGenerateSelect = "gen-select"
	ObjectGenerateInsert = "gen-insert"
	ObjectAddColumn      = "add-column"
	ObjectCreateIndex    = "create-index"
	ObjectRenameTable    = "rename-table"
	ObjectCreateTable    = "create-table"
	ObjectCreateView     = "create-view"
	ObjectErDiagram      = "er-diagram"
	ObjectImportFile     = "import-file"
	ObjectImportNewTable = "import-new-table"
	ObjectTruncate       = "truncate"
	ObjectDropRelation   = "drop-relation"
	ObjectDropSchema     = "drop-schema"
	ObjectDropObject     = "drop-object"
)

// The entry offered on a table and on a view, which are read the same way.
var generateSelect = MenuAction{
	ID: ObjectGenerateSelect, Label: "Generate SELECT", Detail: "into the editor",
	Icon: cfg.IconQuery,
}

// Table actions include target icons and destructive action markers.
var tableActions = []MenuAction{
	{
		ID: ObjectErDiagram, Label: "ER diagram", Detail: "related tables",
		Icon: cfg.IconForeignKey,
	},
	generateSelect,
	{
		ID: ObjectGenerateInsert, Label: "Generate INSERT", Detail: "into the editor",
		Icon: cfg.IconQuery,
	},
	{
		ID: ObjectImportFile, Label: "Import a file…", Detail: "a CSV or a JSON file",
		Icon: cfg.IconTable,
	},
	{ID: ObjectAddColumn, Label: "Add column…", Detail: "ALTER TABLE into the editor", Icon: cfg.IconColumn},
	{ID: ObjectCreateIndex, Label: "Create index…", Detail: "CREATE INDEX into the editor", Icon: cfg.IconIndex},
	{ID: ObjectRenameTable, Label: "Rename table…", Detail: "ALTER TABLE into the editor", Icon: cfg.IconTable},
	{
		ID: ObjectTruncate, Label: "Truncate table", Detail: "TRUNCATE into the editor",
		Icon: cfg.IconNote, Destructive: true,
	},
	{
		ID: ObjectDropRelation, Label: "Drop table", Detail: "DROP TABLE into the editor",
		Icon: cfg.IconNote, Destructive: true,
	},
}

var viewActions = []MenuAction{
	generateSelect,
	{
		ID: ObjectDropRelation, Label: "Drop view", Detail: "DROP VIEW into the editor",
		Icon: cfg.IconNote, Destructive: true,
	},
}

var objectActions = []MenuAction{
	{
		ID: ObjectDropObject, Label: "Drop", Detail: "DROP into the editor",
		Icon: cfg.IconNote, Destructive: true,
	},
}

var schemaActions = []MenuAction{
	{
		ID: ObjectImportNewTable, Label: "Import a file…", Detail: "into a new table",
		Icon: cfg.IconTable,
	},
	{
		ID: ObjectCreateTable, Label: "Create table…", Detail: "into the editor",
		Icon: cfg.IconTable,
	},
	{
		ID: ObjectCreateView, Label: "Create view…", Detail: "into the editor",
		Icon: cfg.IconView,
	},
	{
		ID: ObjectDropSchema, Label: "Drop schema", Detail: "DROP SCHEMA into the editor",
		Icon: cfg.IconNote, Destructive: true,
	},
}

// objectActionNeeds is the server capability check for each restricted action.
var objectActionNeeds = map[string]func(core.Capabilities) bool{
	ObjectTruncate: func(capabilities core.Capabilities) bool { return capabilities.TruncatesTable },
}

// BuildObjectActions returns actions for a tree node when the server supports DDL generation.
func BuildObjectActions(node present.TreeNode, capabilities core.Capabilities) []MenuAction {
	if !capabilities.WritesDDL {
		return nil
	}

	offered := func(actions []MenuAction) []MenuAction {
		kept := make([]MenuAction, 0, len(actions))
		for _, action := range actions {
			needs, held := objectActionNeeds[action.ID]
			if held && !needs(capabilities) {
				continue
			}
			kept = append(kept, action)
		}
		return kept
	}

	switch node.Kind {
	case present.NodeSchema:
		return offered(schemaActions)
	case present.NodeObject:
		return offered(objectActions)
	case present.NodeTable:
		if node.Table.Kind == db.RelationTable {
			return offered(tableActions)
		}
		return offered(viewActions)
	}
	return nil
}

// BuildObjectTitle returns the object menu title.
func BuildObjectTitle(node present.TreeNode) string {
	switch node.Kind {
	case present.NodeSchema:
		return "schema " + node.Schema
	case present.NodeTable:
		return string(node.Table.Kind) + " " + node.Table.Name
	case present.NodeObject:
		return string(node.Object.Kind) + " " + node.Object.Name
	}
	return ""
}
