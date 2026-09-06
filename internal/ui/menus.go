package ui

import (
	tea "charm.land/bubbletea/v2"

	"github.com/turanmahmudov/masume/internal/app"
	"github.com/turanmahmudov/masume/internal/cfg"
	"github.com/turanmahmudov/masume/internal/core"
)

// menuEntry is an action with a label, detail, and availability flag.
type menuEntry struct {
	action ActionID
	label  string
	detail string
	icon   cfg.IconKind
	offer  bool
}

// buildActionMenu returns available actions with their key bindings.
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

// openTabMenu opens the tab context menu.
func (model *Model) openTabMenu(connection *app.Connection) (tea.Model, tea.Cmd) {
	tab := connection.Active()
	return model.openActionMenu(connection, " "+tab.Label()+" ", cfg.ScopeGlobal,
		model.buildActionMenu(connection.Session.Capabilities(), cfg.ScopeGlobal, []menuEntry{
			{ActionNewQueryTab, "New query tab", "beside this tab", cfg.IconQuery, true},
			{
				ActionNameTab, "Rename tab", "name this tab",
				cfg.IconNote, tab.Kind == app.TabQuery,
			},
			{
				ActionSaveQuery, "Save this query", "under a name",
				cfg.IconFavourites, tab.Kind == app.TabQuery,
			},
			{
				ActionReopenTab, "Reopen the last closed tab", "",
				cfg.IconRecent, connection.HasClosedTab(),
			},
			{
				ActionCloseTab, "Close tab", "", cfg.IconNote,
				len(connection.Tabs) > 1,
			},
		}))
}

// openConnectionMenu opens the connection context menu.
func (model *Model) openConnectionMenu(connection *app.Connection) (tea.Model, tea.Cmd) {
	return model.openActionMenu(connection, " "+connection.Profile().Name+" ",
		cfg.ScopeGlobal,
		model.buildActionMenu(connection.Session.Capabilities(), cfg.ScopeGlobal, []menuEntry{
			{ActionNewQueryTab, "New query tab", "on this connection", cfg.IconQuery, true},
			{ActionRefreshObjects, "Refresh the object tree", "read the catalog again", cfg.IconRecent, true},
			{ActionShowActivity, "Server activity", "load, locks, and other sessions", cfg.IconRole, true},
			{
				ActionToggleAutocommit, "Autocommit", "commit each statement on its own",
				cfg.IconTrigger, true,
			},
			{ActionOpenPicker, "Connections…", "open another connection", cfg.IconFolder, true},
			{ActionCloseConnection, "Close connection", "", cfg.IconNote, true},
		}))
}

// openEditorMenu opens the editor context menu.
func (model *Model) openEditorMenu(
	connection *app.Connection, tab *app.Tab,
) (tea.Model, tea.Cmd) {
	written := tab.Editor.Text != ""
	return model.openActionMenu(connection, " statement ", cfg.ScopeEditor,
		model.buildActionMenu(connection.Session.Capabilities(), cfg.ScopeEditor, []menuEntry{
			{ActionRunAtCursor, "Run", "the selection, or the statement at the caret", cfg.IconQuery, written},
			{ActionRunBatch, "Run every statement", "one result each", cfg.IconQuery, written},
			{ActionExplain, "Explain", "estimated query plan", cfg.IconPlan, written},
			{ActionFormatSQL, "Format", "one clause per line", cfg.IconNote, written},
			{ActionCommentLines, "Comment lines", "comment or uncomment selected lines", cfg.IconNote, written},
			{ActionSelectAll, "Select all", "", cfg.IconColumn, written},
			{ActionPasteText, "Paste", "text last copied in this client", cfg.IconQuery, true},
			{ActionFindInStatement, "Find…", "", cfg.IconRecent, written},
			{ActionSaveQuery, "Save this query", "under a name", cfg.IconFavourites, written},
		}))
}

// openColumnMenu opens the column context menu.
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
			{ActionSortColumn, "Sort by column", "sort rows by this column", cfg.IconIndex, sorts},
			{ActionAddSortColumn, "Add column to sort", "add another sort column", cfg.IconIndex, sorts},
			{ActionFilterByValues, "Filter by values", "choose values to keep", cfg.IconColumn, len(shape.Text) > 0},
			{ActionFreezeColumns, "Freeze column", "freeze or unfreeze this column", cfg.IconPrimaryKey, true},
			{ActionGoToColumn, "Go to column…", "by name", cfg.IconRecent, true},
			{ActionSearchColumns, "Search rows", "search loaded rows", cfg.IconRecent, true},
			{ActionToggleMasking, "Show or hide masked values", "", cfg.IconNote, true},
		}))
}
