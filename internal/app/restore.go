package app

import (
	"strconv"
	"strings"

	"github.com/turanmahmudov/masume/internal/core"
	"github.com/turanmahmudov/masume/internal/db"
	"github.com/turanmahmudov/masume/internal/hist"
)

// The open tabs of a connection, written into the history file and read back at the next
// connection. A restored table or object tab reads its data the first time it is shown, so a
// new connection asks the server for one table only.

// RestoreTabs opens the stored tabs of the profile. A profile that was never opened, or one
// whose tabs could not be read, starts with one empty query tab.
func (connection *Connection) RestoreTabs(saved hist.SavedWorkspace, buildPreview PreviewBuilder) {
	if len(saved.Tabs) == 0 {
		return
	}

	tabs := make([]*Tab, 0, len(saved.Tabs))
	unread := map[int]bool{}
	for _, held := range saved.Tabs {
		connection.nextTabID++
		tab := buildRestoredTab(connection.nextTabID, held, buildPreview)
		if tab.Kind != TabQuery {
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

// applySavedState applies the sort, the filter and the caret of a stored tab.
func applySavedState(tab *Tab, state hist.SavedTabState) {
	tab.Sort = state.Sort
	tab.Filter = state.Filter
	if state.Caret > 0 && state.Caret <= len(tab.Editor.Text) {
		tab.Editor.Caret = state.Caret
		tab.Editor.Anchor = state.Caret
	}
}

// TakeUnread returns whether this tab still has to read its data, and marks it as read. A
// restored tab reads the first time it is shown.
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
	return hist.SavedTab{Kind: "query", SQL: tab.Editor.Text, State: state}
}

// DescribeTabs returns the open tabs as one text, so a change is found without a comparison
// of every field.
func (connection *Connection) DescribeTabs() string {
	var written strings.Builder
	written.WriteString(strconv.Itoa(connection.ActiveIndex))
	for _, tab := range connection.Tabs {
		written.WriteString("\x00" + strconv.Itoa(tab.ID) + "\x00" + string(tab.Kind) + "\x00" +
			tab.Table.Schema + "." + tab.Table.Name + "\x00" +
			tab.Object.Schema + "." + tab.Object.Name + "\x00" + tab.Editor.Text)
	}
	return written.String()
}
