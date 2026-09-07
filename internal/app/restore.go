package app

import (
	"strconv"
	"strings"

	"github.com/turanmahmudov/masume/internal/core"
	"github.com/turanmahmudov/masume/internal/db"
	"github.com/turanmahmudov/masume/internal/hist"
	"github.com/turanmahmudov/masume/internal/notebook"
)

// Workspace persistence restores tab state. Table and object tabs load data on first display.

// RestoreTabs restores saved tabs or preserves the current tabs when the saved list is empty.
func (connection *Connection) RestoreTabs(saved hist.SavedWorkspace, buildPreview PreviewBuilder) {
	if len(saved.Tabs) == 0 {
		return
	}

	tabs := make([]*Tab, 0, len(saved.Tabs))
	unread := map[int]bool{}
	for _, held := range saved.Tabs {
		connection.nextTabID++
		tab := buildRestoredTab(connection.nextTabID, held, buildPreview)
		if tab.Kind != TabQuery && tab.Kind != TabNotebook {
			unread[tab.ID] = true
		}
		tabs = append(tabs, tab)
	}

	connection.Tabs = tabs
	connection.Unread = unread
	connection.ActiveIndex = core.ClampIndex(saved.ActiveIndex, len(tabs))
}

// PreviewBuilder returns the read of a table, which a restored table tab starts with.
type PreviewBuilder func(table db.TableRef) string

// buildRestoredTab returns the tab of one stored tab.
func buildRestoredTab(id int, saved hist.SavedTab, buildPreview PreviewBuilder) *Tab {
	switch saved.Kind {
	case "object":
		tab := NewObjectTab(id, db.SchemaObject{
			Schema: saved.Schema, Name: saved.Name,
			Kind: db.SchemaObjectKind(saved.ObjectKind),
			// Only the tree shows a detail next to a name.
			Identity: saved.Identity,
		})
		applySavedState(tab, saved.State)
		return tab
	case "notebook":
		// The text of the notebook is stored, so a notebook that was never saved comes
		// back as well, and no cell is run to restore it.
		tab := NewNotebookTab(id, notebook.Parse(saved.SQL), saved.Identity,
			notebook.Origin(saved.ObjectKind))
		applySavedState(tab, saved.State)
		applySavedCells(tab, saved.State)
		return tab
	case "table":
		table := db.TableRef{
			Schema: saved.Schema, Name: saved.Name,
			Kind: db.RelationKind(saved.TableKind),
		}
		tab := NewTableTab(id, table, buildPreview(table))
		applySavedState(tab, saved.State)
		return tab
	}
	tab := NewQueryTab(id, saved.SQL)
	applySavedState(tab, saved.State)
	return tab
}

// applySavedCells puts the list of a restored notebook back on the cell it stood on, with
// the cells that were folded away still folded.
func applySavedCells(tab *Tab, state hist.SavedTabState) {
	folded := map[string]bool{}
	for _, id := range state.Folded {
		folded[id] = true
	}
	for _, cell := range tab.Notebook.Cells {
		cell.Folded = folded[cell.ID]
	}
	tab.Notebook.FocusCell(state.Cell)
	tab.SettleFocusedCell()
	// The sort, the filter and the caret of the stored tab belong to the cell it stood on.
	tab.Sort, tab.Filter = state.Sort, state.Filter
	if state.Caret > 0 && state.Caret <= len(tab.Editor.Text) {
		tab.Editor.Caret, tab.Editor.Anchor = state.Caret, state.Caret
	}
	tab.KeepFocusedCell()
}

// applySavedState applies the sort, the filter and the caret of a stored tab.
func applySavedState(tab *Tab, state hist.SavedTabState) {
	tab.Sort = state.Sort
	tab.Filter = state.Filter
	if state.Caret > 0 && state.Caret <= len(tab.Editor.Text) {
		tab.Editor.Caret = state.Caret
		tab.Editor.Anchor = state.Caret
	}
}

// TakeUnread clears and returns the pending first-read flag for a tab.
func (connection *Connection) TakeUnread(tab *Tab) bool {
	if tab == nil || !connection.Unread[tab.ID] {
		return false
	}
	delete(connection.Unread, tab.ID)
	return true
}

// BuildWorkspaceSnapshot returns the tabs in the form the history file stores.
func (connection *Connection) BuildWorkspaceSnapshot() hist.SavedWorkspace {
	tabs := make([]hist.SavedTab, 0, len(connection.Tabs))
	for _, tab := range connection.Tabs {
		tabs = append(tabs, buildSavedTab(tab))
	}
	connection.workspaceChange++
	return hist.SavedWorkspace{
		Tabs: tabs, ActiveIndex: connection.ActiveIndex,
		Change: connection.workspaceChange,
	}
}

// buildSavedTab returns one tab in the form the history file stores.
func buildSavedTab(tab *Tab) hist.SavedTab {
	state := hist.SavedTabState{
		Caret: tab.Editor.Caret, Sort: tab.Sort, Filter: tab.Filter,
	}
	switch tab.Kind {
	case TabTable:
		return hist.SavedTab{
			Kind: "table", Schema: tab.Table.Schema, Name: tab.Table.Name,
			TableKind: string(tab.Table.Kind), State: state,
		}
	case TabObject:
		return hist.SavedTab{
			Kind: "object", Schema: tab.Object.Schema, Name: tab.Object.Name,
			ObjectKind: string(tab.Object.Kind), Identity: tab.Object.Identity, State: state,
		}
	}
	if tab.Kind == TabNotebook && tab.Notebook != nil {
		state.Cell = tab.Notebook.Focused
		state.Folded = tab.Notebook.ListFoldedCells()
		return hist.SavedTab{
			Kind: "notebook", SQL: notebook.Write(tab.Notebook.BuildDocument()),
			Identity: tab.Notebook.Path, ObjectKind: string(tab.Notebook.Origin),
			State: state,
		}
	}
	return hist.SavedTab{Kind: "query", SQL: tab.Editor.Text, State: state}
}

// DescribeTabs returns a text signature of the active index, tab identities, and editor contents.
func (connection *Connection) DescribeTabs() string {
	var written strings.Builder
	written.WriteString(strconv.Itoa(connection.ActiveIndex))
	for _, tab := range connection.Tabs {
		written.WriteString("\x00" + strconv.Itoa(tab.ID) + "\x00" + string(tab.Kind) + "\x00" +
			tab.Table.Schema + "." + tab.Table.Name + "\x00" +
			tab.Object.Schema + "." + tab.Object.Name + "\x00" + tab.Editor.Text)
		if tab.Notebook == nil {
			continue
		}
		written.WriteString("\x00" + tab.Notebook.Path + "\x00" +
			strconv.Itoa(tab.Notebook.Focused))
		for _, cell := range tab.Notebook.Cells {
			written.WriteString("\x00" + cell.ID + "\x00" + string(cell.Kind) +
				"\x00" + strconv.FormatBool(cell.Folded) + "\x00" + cell.Editor.Text)
		}
	}
	return written.String()
}
