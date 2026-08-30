package ui

import (
	tea "charm.land/bubbletea/v2"

	"github.com/turanmahmudov/masume/internal/app"
	"github.com/turanmahmudov/masume/internal/cfg"
	"github.com/turanmahmudov/masume/internal/core"
)

// menuEntry is one row a menu of actions offers: the action it runs, what it is called, and
// what it does. An entry the server cannot answer, or that has nothing to act on, is left out.
type menuEntry struct {
	action ActionID
	label  string
	detail string
	icon   cfg.IconKind
	offer  bool
}

// buildActionMenu returns the rows of a menu of actions, with the chord each one is bound to
// in this scope, so the menu offers what a key also reaches.
func (model *Model) buildActionMenu(
	capabilities core.Capabilities, scope cfg.KeyScope, offered []menuEntry,
) []app.MenuAction {
	actions := make([]app.MenuAction, 0, len(offered))
	for _, entry := range offered {
		if !entry.offer || !AnswersFor(capabilities, FindActionCapability(scope, entry.action)) {
			continue
		}
		chord := model.registry.FormatActionChords(scope, entry.action)
		if chord == "" {
			chord = model.registry.FormatActionChords(cfg.ScopeGlobal, entry.action)
		}
		actions = append(actions, app.MenuAction{
			ID: string(entry.action), Label: entry.label, Detail: entry.detail,
			Icon: entry.icon, Chord: chord,
		})
	}
	return actions
}

// openActionMenu draws a menu of actions over the workspace, and does nothing where the menu
// would be empty.
func (model *Model) openActionMenu(
	connection *app.Connection, title string, scope cfg.KeyScope, actions []app.MenuAction,
) (tea.Model, tea.Cmd) {
	if len(actions) == 0 {
		return model, nil
	}
	connection.Overlay = app.Overlay{
		Kind: app.OverlayActionMenu, Title: title, Scope: scope,
		Draft: app.NewEditorBuffer("", 0), Actions: actions,
	}
	return model, nil
}

// openTabMenu returns the right button on a tab: what a reader does to the tab itself.
func (model *Model) openTabMenu(connection *app.Connection) (tea.Model, tea.Cmd) {
	tab := connection.Active()
	return model.openActionMenu(connection, " "+tab.Label()+" ", cfg.ScopeGlobal,
		model.buildActionMenu(connection.Session.Capabilities(), cfg.ScopeGlobal, []menuEntry{
			{ActionNewQueryTab, "New query tab", "beside this one", cfg.IconQuery, true},
			{
				ActionNameTab, "Rename tab", "written as a comment on the first line",
				cfg.IconNote, tab.Kind == app.TabQuery,
			},
			{
				ActionSaveQuery, "Save this query", "under a name",
				cfg.IconFavourites, tab.Kind == app.TabQuery,
			},
			{
				ActionReopenTab, "Reopen the tab closed last", "",
				cfg.IconRecent, connection.HasClosedTab(),
			},
			{
				ActionCloseTab, "Close tab", "", cfg.IconNote,
				len(connection.Tabs) > 1,
			},
		}))
}

// openConnectionMenu returns the right button on a row of the connection list.
func (model *Model) openConnectionMenu(connection *app.Connection) (tea.Model, tea.Cmd) {
	return model.openActionMenu(connection, " "+connection.Profile().Name+" ",
		cfg.ScopeGlobal,
		model.buildActionMenu(connection.Session.Capabilities(), cfg.ScopeGlobal, []menuEntry{
			{ActionNewQueryTab, "New query tab", "on this connection", cfg.IconQuery, true},
			{ActionRefreshObjects, "Refresh the objects", "read the catalog again", cfg.IconRecent, true},
			{ActionShowActivity, "Server activity", "what the other connections are doing", cfg.IconRole, true},
			{
				ActionToggleAutocommit, "Autocommit", "commit each statement on its own",
				cfg.IconTrigger, true,
			},
			{ActionOpenPicker, "Connections…", "open another one", cfg.IconFolder, true},
			{ActionCloseConnection, "Close connection", "", cfg.IconNote, true},
		}))
}

// openEditorMenu returns the right button on the statement.
func (model *Model) openEditorMenu(
	connection *app.Connection, tab *app.Tab,
) (tea.Model, tea.Cmd) {
	written := tab.Editor.Text != ""
	return model.openActionMenu(connection, " statement ", cfg.ScopeEditor,
		model.buildActionMenu(connection.Session.Capabilities(), cfg.ScopeEditor, []menuEntry{
			{ActionRunAtCursor, "Run", "the selection, or the statement at the caret", cfg.IconQuery, written},
			{ActionRunBatch, "Run every statement", "one result each", cfg.IconQuery, written},
			{ActionExplain, "Explain", "how the server will read it", cfg.IconPlan, written},
			{ActionFormatSQL, "Format", "lay the statement out again", cfg.IconNote, written},
			{ActionCommentLines, "Comment lines", "the lines the selection covers", cfg.IconNote, written},
			{ActionSelectAll, "Select all", "", cfg.IconColumn, written},
			{ActionPasteText, "Paste", "what this client last copied", cfg.IconQuery, true},
			{ActionFindInStatement, "Find…", "", cfg.IconRecent, written},
			{ActionSaveQuery, "Save this query", "under a name", cfg.IconFavourites, written},
		}))
}

// openColumnMenu returns the right button on the name of a column.
func (model *Model) openColumnMenu(
	connection *app.Connection, tab *app.Tab, shape GridShape,
) (tea.Model, tea.Cmd) {
	name := " the column "
	if tab.GridColumn < len(shape.Columns) {
		name = " " + shape.Columns[tab.GridColumn].Name + " "
	}
	sorts := connection.Session.Capabilities().SortsRead
	return model.openActionMenu(connection, name, cfg.ScopeGrid,
		model.buildActionMenu(connection.Session.Capabilities(), cfg.ScopeGrid, []menuEntry{
			{ActionSortColumn, "Sort by column", "order the read by it", cfg.IconIndex, sorts},
			{ActionAddSortColumn, "Add column to sort", "order by it as well", cfg.IconIndex, sorts},
			{ActionFilterByValues, "Filter by values", "choose which values stay", cfg.IconColumn, len(shape.Text) > 0},
			{ActionFreezeColumns, "Freeze up to this column", "keep it on screen while the rest scroll", cfg.IconPrimaryKey, true},
			{ActionGoToColumn, "Go to column…", "by name", cfg.IconRecent, true},
			{ActionSearchColumns, "Search the columns", "", cfg.IconRecent, true},
			{ActionToggleMasking, "Show masked values", "", cfg.IconNote, true},
		}))
}
