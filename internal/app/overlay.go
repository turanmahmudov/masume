package app

import (
	"github.com/turanmahmudov/masume/internal/cfg"
	"github.com/turanmahmudov/masume/internal/core"
	"github.com/turanmahmudov/masume/internal/db"
	"github.com/turanmahmudov/masume/internal/hist"
	"github.com/turanmahmudov/masume/internal/present"
	"github.com/turanmahmudov/masume/internal/query/result"
)

// OverlayKind names what is drawn over the workspace.
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
	OverlayActivity    OverlayKind = "activity"
	OverlayMessage     OverlayKind = "message"
	OverlayAiChat      OverlayKind = "ai-chat"
	OverlayAiChats     OverlayKind = "ai-chats"
	OverlayConfirm     OverlayKind = "confirm"
	OverlayChoice      OverlayKind = "choice"
	OverlayExport      OverlayKind = "export"
	OverlayPrompt      OverlayKind = "prompt"
)

// WholeRow is the row index a cell editor uses when it holds a whole new row.
const WholeRow = -1

// MenuAction is one row of a menu: its name, what it does, and whether it destroys data.
type MenuAction struct {
	ID    string
	Label string
	// The glyph of what the entry acts on, or nothing for an entry that acts on the thing
	// the menu was opened on.
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

// PromptKind names what a one-line prompt is asking for.
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
	PromptReplace    PromptKind = "replace"
)

// ListState is where a list overlay stands. Every kind that draws rows of its own uses it,
// so the keys that walk a list are written once.
type ListState struct {
	// The cursor of the list an overlay draws, and how far it has scrolled.
	Cursor int
	Offset int
	// True while the wheel moved the rows away from the cursor, so the cursor may stand
	// off screen until it moves again.
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
	// The values the column takes. A cell with a list is picked, not typed, so the
	// editor draws the list and holds no field.
	Choices []string
}

// ExportRequest is the export the form is writing.
type ExportRequest struct {
	Path     string
	Format   result.ExportFormat
	CSV      result.CSVOptions
	RowCount int
	// True where the export writes every row the read returns, not only the rows read
	// so far.
	WholeRead bool
}

// AnswerCommand is the work an answer starts, handed back to the caller that ran the answer.
// It is the shape of a command of the draw loop, named here so this package stays free of it.
type AnswerCommand func() any

// OverlayAnswers is what the answer of an overlay runs. Each kind sets the one it asks with,
// and the caller sets it when it opens the overlay. An answer hands its work back rather than
// starting it, because only the draw loop may start one.
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

// Overlay is the card on top of the workspace, or none where nothing is drawn over it. One
// struct holds the state of every kind, so a field a kind does not use stands empty rather
// than being read from somewhere else. Kind says which one it is, and each kind reads the
// fields its own shape needs.
type Overlay struct {
	Kind  OverlayKind
	Title string
	Body  string

	List ListState

	Entries  []hist.HistoryEntry
	Saved    []hist.SavedQuery
	Actions  []MenuAction
	Palette  []PaletteAction
	Choices  []Choice
	Sessions []db.Activity
	Changes  []db.Change
	Lines    []string

	Window  RowWindow
	Cell    CellTarget
	Export  ExportRequest
	Answers OverlayAnswers

	// The rows the card gives its content, read when it opens, so the card does not
	// grow while the user types.
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

	// Which field of a form holds the caret.
	Field int

	// The scope the entries of a menu of actions are run in. An entry the scope does not
	// bind is run in the global scope.
	Scope cfg.KeyScope

	// The prompt a one-line field is asking for.
	Prompt PromptKind
	// The hint under the prompt, which says what the field is for.
	Hint string
}

// IsOpen is true while an overlay owns the keyboard.
func (overlay Overlay) IsOpen() bool {
	return overlay.Kind != OverlayNone && overlay.Kind != ""
}

// The ids of the object menu, so a typo is a build error rather than a row that runs nothing.
const (
	ObjectGenerateSelect = "gen-select"
	ObjectGenerateInsert = "gen-insert"
	ObjectAddColumn      = "add-column"
	ObjectCreateIndex    = "create-index"
	ObjectRenameTable    = "rename-table"
	ObjectCreateTable    = "create-table"
	ObjectCreateView     = "create-view"
	ObjectErDiagram      = "er-diagram"
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

// Each entry carries the glyph of what it acts on, and an entry that removes something
// carries the mark of a warning, so a menu is read by its marks as well as by its words.
var tableActions = []MenuAction{
	{
		ID: ObjectErDiagram, Label: "ER diagram", Detail: "tables it relates to",
		Icon: cfg.IconForeignKey,
	},
	generateSelect,
	{
		ID: ObjectGenerateInsert, Label: "Generate INSERT", Detail: "into the editor",
		Icon: cfg.IconQuery,
	},
	{ID: ObjectAddColumn, Label: "Add column…", Detail: "alter table", Icon: cfg.IconColumn},
	{ID: ObjectCreateIndex, Label: "Create index…", Detail: "create index", Icon: cfg.IconIndex},
	{ID: ObjectRenameTable, Label: "Rename table…", Detail: "alter table", Icon: cfg.IconTable},
	{
		ID: ObjectTruncate, Label: "Truncate table", Detail: "removes every row",
		Icon: cfg.IconNote, Destructive: true,
	},
	{
		ID: ObjectDropRelation, Label: "Drop table", Detail: "removes the table",
		Icon: cfg.IconNote, Destructive: true,
	},
}

var viewActions = []MenuAction{
	generateSelect,
	{
		ID: ObjectDropRelation, Label: "Drop view", Detail: "removes the view",
		Icon: cfg.IconNote, Destructive: true,
	},
}

var objectActions = []MenuAction{
	{
		ID: ObjectDropObject, Label: "Drop", Detail: "removes the object",
		Icon: cfg.IconNote, Destructive: true,
	},
}

var schemaActions = []MenuAction{
	{
		ID: ObjectCreateTable, Label: "Create table…", Detail: "into the editor",
		Icon: cfg.IconTable,
	},
	{
		ID: ObjectCreateView, Label: "Create view…", Detail: "into the editor",
		Icon: cfg.IconView,
	},
	{
		ID: ObjectDropSchema, Label: "Drop schema", Detail: "removes the schema",
		Icon: cfg.IconNote, Destructive: true,
	},
}

// objectActionNeeds says which capability an entry of the object menu asks of the server.
// An action the server has no statement for is not offered.
var objectActionNeeds = map[string]func(core.Capabilities) bool{
	ObjectTruncate: func(capabilities core.Capabilities) bool { return capabilities.TruncatesTable },
}

// BuildObjectActions returns what the object menu offers on this node. Every entry ends in a
// statement, written into the editor or run, so a server that takes commands instead of SQL
// has none of them.
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

// BuildObjectTitle names what the object menu was opened on.
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
