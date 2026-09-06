package ui

import (
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/turanmahmudov/masume/internal/app"
	"github.com/turanmahmudov/masume/internal/cfg"
	"github.com/turanmahmudov/masume/internal/present"
)

// The command palette lists actions, key presets, AI providers, and themes.

// The prefix of the id of a palette row that changes the AI provider, and of one that
// changes the key preset.
const (
	aiProviderPrefix = "ai-provider:"
	keyPresetPrefix  = "key-preset:"
	// configProblemsAction is available when configuration problems exist.
	configProblemsAction = "config-problems"
)

// paletteEntry is one row of the palette. Scope and Action name the action, so the row
// shows the chord bound to it.
type paletteEntry struct {
	id     string
	label  string
	detail string
	scope  cfg.KeyScope
	action ActionID
	// needs is the capability of a row that runs no action. A row with one takes the
	// capability of that action.
	needs Capability
	// detailScope and detailAction are for a row reached by the key of another action,
	// such as the one that moves between the panes.
	detailScope  cfg.KeyScope
	detailAction ActionID
}

// paneChordAction names the key that moves between the panes.
const paneChordAction = ActionFocusNextPane

// paletteEntries are the rows the palette offers, in order.
var paletteEntries = []paletteEntry{
	{id: "run-at-cursor", label: "Run the selection or the statement",
		scope: cfg.ScopeGlobal, action: ActionRunAtCursor},
	{id: "run-batch", label: "Run every statement", detail: "one result each",
		scope: cfg.ScopeGlobal, action: ActionRunBatch},
	{id: "explain", label: "Explain plan", scope: cfg.ScopeGlobal, action: ActionExplain},
	{id: "explain-analyze", label: "Explain analyze",
		scope: cfg.ScopeGlobal, action: ActionExplainAnalyze},
	{id: "cancel-query", label: "Cancel the running query",
		scope: cfg.ScopeGlobal, action: ActionCancelQuery},
	{id: "show-history", label: "Query history",
		scope: cfg.ScopeGlobal, action: ActionShowHistory},
	{id: "save-query", label: "Save this query", detail: "under a name",
		scope: cfg.ScopeGlobal, action: ActionSaveQuery},
	{id: "show-saved", label: "Saved queries",
		scope: cfg.ScopeGlobal, action: ActionShowSaved},
	{id: "show-activity", label: "Server activity",
		detail: "load, locks, and other sessions",
		scope:  cfg.ScopeGlobal, action: ActionShowActivity},
	{id: "undo-write", label: "Undo the last write",
		detail: "run the saved undo statement",
		scope:  cfg.ScopeGlobal, action: ActionUndoWrite},
	{id: "export-csv", label: "Export result as CSV",
		scope: cfg.ScopeGlobal, action: ActionExportCSV},
	{id: "export-json", label: "Export result as JSON",
		scope: cfg.ScopeGlobal, action: ActionExportJSON},
	{id: "reopen-tab", label: "Reopen the last closed tab",
		scope: cfg.ScopeGlobal, action: ActionReopenTab},
	{id: "undo-change", label: "Undo the last staged change", detail: "in the grid",
		scope: cfg.ScopeGrid, action: ActionUndoChange},
	{id: "redo-change", label: "Redo the last undone change", detail: "in the grid",
		scope: cfg.ScopeGrid, action: ActionRedoChange},
	{id: "review-changes", label: "Review staged changes", detail: "in the grid",
		scope: cfg.ScopeGrid, action: ActionReviewChanges},
	{id: "discard-changes", label: "Discard the staged changes", detail: "asks first",
		scope: cfg.ScopeGrid, action: ActionDiscardChanges},
	{id: "begin-transaction", label: "Begin transaction",
		scope: cfg.ScopeGlobal, action: ActionBeginTransaction},
	{id: "commit-transaction", label: "Commit transaction",
		scope: cfg.ScopeGlobal, action: ActionCommitTransaction},
	{id: "rollback-transaction", label: "Rollback transaction",
		scope: cfg.ScopeGlobal, action: ActionRollbackTransaction},
	{id: "toggle-autocommit", label: "Toggle autocommit",
		scope: cfg.ScopeGlobal, action: ActionToggleAutocommit},
	{id: "tab-data", label: "View: Data", detail: "result rows"},
	{id: "tab-fields", label: "View: Fields",
		detail: "the columns the server returned"},
	{id: "tab-statistics", label: "View: Statistics", detail: "affected rows and execution times"},
	{id: "tab-columns", label: "View: Columns", detail: "table columns"},
	{id: "tab-indexes", label: "View: Indexes", detail: "table indexes"},
	{id: "tab-constraints", label: "View: Constraints", detail: "table constraints"},
	{id: "tab-ddl", label: "View: DDL", detail: "the statement that defines the table"},
	{id: "tab-plan", label: "View: Plan", detail: "query plan"},
	{id: "reveal-sql", label: "Edit the query for this result",
		detail: "a table opens as a query", scope: cfg.ScopeGlobal, action: ActionRevealSQL},
	{id: "toggle-sidebar", label: "Show or hide the object tree",
		scope: cfg.ScopeGlobal, action: ActionToggleSidebar},
	{id: "toggle-result", label: "Show or hide the result",
		detail: "the editor fills the pane",
		scope:  cfg.ScopeGlobal, action: ActionToggleResult},
	{id: "focus-sidebar", label: "Focus the object tree",
		scope: cfg.ScopeGlobal, action: ActionFocusSidebar,
		detailScope: cfg.ScopeGlobal, detailAction: paneChordAction},
	{id: "focus-editor", label: "Focus the editor",
		scope: cfg.ScopeGlobal, action: ActionFocusEditor,
		detailScope: cfg.ScopeGlobal, detailAction: paneChordAction},
	{id: "focus-result", label: "Focus the result",
		scope: cfg.ScopeGlobal, action: ActionFocusResult,
		detailScope: cfg.ScopeGlobal, detailAction: paneChordAction},
	{id: "new-query-tab", label: "New query tab",
		scope: cfg.ScopeGlobal, action: ActionNewQueryTab},
	{id: "next-tab", label: "Next tab", scope: cfg.ScopeGlobal, action: ActionNextTab},
	{id: "close-tab", label: "Close this tab", detail: "asks if changes are staged",
		scope: cfg.ScopeGlobal, action: ActionCloseTab},
	{id: "name-tab", label: "Name this tab", scope: cfg.ScopeGlobal, action: ActionNameTab},
	{id: "refresh-objects", label: "Refresh the object tree",
		detail: "read the catalog again",
		scope:  cfg.ScopeGlobal, action: ActionRefreshObjects},
	{id: "copy-csv", label: "Copy the result as CSV", detail: "in the grid",
		scope: cfg.ScopeGrid, action: ActionCopyCSV},
	{id: "copy-json", label: "Copy the result as JSON", detail: "in the grid",
		scope: cfg.ScopeGrid, action: ActionCopyJSON},
	{id: "copy-markdown", label: "Copy the result as Markdown", detail: "in the grid",
		scope: cfg.ScopeGrid, action: ActionCopyMarkdown},
	{id: "copy-inserts", label: "Copy the result as INSERTs", detail: "in the grid",
		scope: cfg.ScopeGrid, action: ActionCopyInserts},
	{id: "copy-plan", label: "Copy the query plan",
		detail: "in the plan view · raw server output",
		scope:  cfg.ScopePlan, action: ActionCopyPlan},
	{id: "open-picker", label: "New connection",
		scope: cfg.ScopeGlobal, action: ActionOpenPicker},
	{id: "close-connection", label: "Close this connection", detail: "close all its tabs",
		scope: cfg.ScopeGlobal, action: ActionCloseConnection},
	{id: "next-page", label: "Fetch more rows",
		scope: cfg.ScopeGlobal, action: ActionNextPage},
	{id: "count-rows", label: "Count every row in the result", detail: "in the grid",
		scope: cfg.ScopeGrid, action: ActionCountRows},
	{id: "format-sql", label: "Format the query", detail: "one clause per line",
		scope: cfg.ScopeEditor, action: ActionFormatSQL},
	{id: "show-themes", label: "Theme",
		detail: "preview the selected theme",
		scope:  cfg.ScopeGlobal, action: ActionShowThemes},
	{id: "reload-themes", label: "Reload the theme files",
		detail: "read the theme files again"},
	{id: "show-help", label: "Help", scope: cfg.ScopeGlobal, action: ActionShowHelp},
	{id: "show-ai-chat", label: "Ask AI", detail: "ask about this database, or for a query",
		scope: cfg.ScopeGlobal, action: ActionShowAiChat},
	{id: "ai-explain-query", label: "Ask AI: explain this query",
		detail: "the query in the editor"},
	{id: "ai-optimize-query", label: "Ask AI: optimize this query",
		detail: "the query in the editor"},
	{id: "ai-fix-error", label: "Ask AI: fix the error",
		detail: "the last failed run in the editor",
		scope:  cfg.ScopeGlobal, action: ActionAiFixError},
}

// providerLabels name each provider the way a reader writes it, not the way the config
// file keys it.
var providerLabels = map[cfg.AiProviderID]string{
	cfg.ProviderAnthropic: "Anthropic",
	cfg.ProviderOpenai:    "OpenAI",
}

// readEntryDetail returns the detail of a row, which can be the chord of another action.
func (model *Model) readEntryDetail(entry paletteEntry) string {
	if entry.detailAction != "" {
		return model.registry.FormatActionChord(entry.detailScope, entry.detailAction)
	}
	return entry.detail
}

// findEntryCapability returns the capability a row needs: its own, or the one of its action.
func findEntryCapability(entry paletteEntry) Capability {
	if entry.needs != "" {
		return entry.needs
	}
	if entry.action == "" {
		return ""
	}
	return FindActionCapability(entry.scope, entry.action)
}

// buildPaletteActions returns every row the command palette offers. A row the engine cannot
// do is left out, and not shown and refused.
func (model *Model) buildPaletteActions(connection *app.Connection) []app.PaletteAction {
	capabilities := connection.Session.Capabilities()
	actions := []app.PaletteAction{}

	for _, entry := range paletteEntries {
		if !AnswersFor(capabilities, findEntryCapability(entry)) {
			continue
		}
		if !model.offersAi() && (aiPaletteRows[entry.id] || IsAiAction(entry.action)) {
			continue
		}
		chord := ""
		if entry.action != "" {
			chord = model.registry.FormatActionChord(entry.scope, entry.action)
		}
		actions = append(actions, app.PaletteAction{
			ID: entry.id, Label: entry.label,
			Detail: model.readEntryDetail(entry), Chord: chord,
		})
	}

	// One row per AI provider, with the model the config file set for it.
	for _, id := range cfg.AiProviderIDs {
		if !model.offersAi() {
			break
		}
		actions = append(actions, app.PaletteAction{
			ID: aiProviderPrefix + string(id), Label: "AI provider: " + providerLabels[id],
			Detail: model.ai.Providers[id].Model,
		})
	}
	// One row per key preset, so a new preset is offered without a second list. Nothing
	// is chosen while there is one preset.
	if presets := ListKeyPresets(); len(presets) > 1 {
		for _, preset := range presets {
			actions = append(actions, app.PaletteAction{
				ID: keyPresetPrefix + string(preset.ID), Label: "Keys: " + preset.Title,
				Detail: preset.Describe,
			})
		}
	}
	if len(model.problems) > 0 {
		actions = append(actions, app.PaletteAction{
			ID: configProblemsAction, Label: "Config problems",
			Detail: present.FormatCount(int64(len(model.problems))) +
				" · config and theme file problems",
		})
	}
	return actions
}

// paletteViews name the view each `tab-` row of the palette moves to.
var paletteViews = []app.ResultView{
	app.ViewTree,
	app.ViewData, app.ViewFields, app.ViewStatistics, app.ViewColumns,
	app.ViewIndexes, app.ViewConstraints, app.ViewDDL, app.ViewPlan,
}

// runPaletteAction returns a row of the command palette. A row that writes into the result
// pane opens it again first, so nothing is written where it cannot be read.
func (model *Model) runPaletteAction(
	connection *app.Connection, id string,
) (tea.Model, tea.Cmd) {
	connection.Overlay = app.Overlay{}
	tab := connection.Active()

	for _, view := range paletteViews {
		if id != "tab-"+string(view) {
			continue
		}
		return model.selectResultView(connection, tab, view)
	}
	if after, ok := strings.CutPrefix(id, keyPresetPrefix); ok {
		return model.switchKeyPreset(connection, after)
	}
	if after, ok := strings.CutPrefix(id, aiProviderPrefix); ok {
		return model.switchAiProvider(connection, after)
	}

	switch id {
	case "copy-plan":
		if tab.ViewData.Kind != app.DataPlan {
			return model, nil
		}
		connection.Show("plan copied")
		return model, model.keepOnClipboard(tab.ViewData.Plan.Raw)
	case "reload-themes":
		return model.reloadThemeFiles(connection)
	case "ai-explain-query":
		return model.askAi(connection, connection.Active(),
			"Explain what the query in the editor does, in plain terms.")
	case "ai-optimize-query":
		return model.askAi(connection, connection.Active(),
			"Suggest how to make the query in the editor faster or clearer, and explain why.")
	case configProblemsAction:
		connection.Overlay = app.Overlay{
			Kind: app.OverlayMessage, Title: " config problems ",
			Body: strings.Join(model.problems, "\n"),
		}
		return model, nil
	}

	action, known := FindActionID(id)
	if !known {
		connection.ShowError("unknown action: \"" + id + "\"")
		return model, nil
	}
	scope := cfg.ScopeGlobal
	if _, held := FindAction(cfg.ScopeGrid, action); held {
		if _, isGlobal := FindAction(cfg.ScopeGlobal, action); !isGlobal {
			scope = cfg.ScopeGrid
		}
	}
	if AnswersInResult(scope, action) {
		connection.ResultVisible = true
	}
	return model.runAction(connection, tab, Match{Action: action, Scope: scope})
}

// focusPane moves the keyboard to one pane, which the palette does by name.
func (model *Model) focusPane(
	connection *app.Connection, tab *app.Tab, pane app.Pane,
) (tea.Model, tea.Cmd) {
	switch pane {
	case app.PaneSidebar:
		if !connection.SidebarVisible {
			return model, nil
		}
	case app.PaneEditor:
		if !tab.EditorVisible() {
			return model, nil
		}
	case app.PaneResult:
		if !connection.ResultVisible {
			return model, nil
		}
	}
	tab.Focus = pane
	return model, nil
}

// selectResultView moves to one view by name, which the palette does.
func (model *Model) selectResultView(
	connection *app.Connection, tab *app.Tab, view app.ResultView,
) (tea.Model, tea.Cmd) {
	for at, offered := range tab.Views(connection.Session) {
		if offered == view {
			return model.selectViewAt(connection, tab, at)
		}
	}
	return model, nil
}

// switchKeyPreset applies a preset without changing the config file.
func (model *Model) switchKeyPreset(
	connection *app.Connection, written string,
) (tea.Model, tea.Cmd) {
	id, known := cfg.FindPresetID(written)
	if !known {
		return model, nil
	}
	for _, problem := range model.registry.ApplyKeySettings(
		FindKeyPreset(id), cfg.ChordChoices{}, model.offersAi()) {
		model.problems = append(model.problems, "keys: "+problem)
	}
	connection.Show("keys: " + string(id) +
		" · write preset = \"" + string(id) + "\" under [keys] to keep it")
	return model, nil
}

// switchAiProvider changes the provider the chat would send to.
func (model *Model) switchAiProvider(
	connection *app.Connection, written string,
) (tea.Model, tea.Cmd) {
	for _, id := range cfg.AiProviderIDs {
		if string(id) != written {
			continue
		}
		model.aiProvider = id
		connection.Show("ai provider set to " + written)
		return model, nil
	}
	return model, nil
}

// reloadThemeFiles reloads theme files and reapplies the configured theme.
func (model *Model) reloadThemeFiles(connection *app.Connection) (tea.Model, tea.Cmd) {
	path := cfg.ResolveConfigPath()
	documents, problems := cfg.ReadThemeDocuments(cfg.ResolveThemesPath(path))

	registry := NewThemeRegistry()
	styles := NewStyles(registry)
	found := append([]string{}, registry.ListBuiltInProblems()...)
	found = append(found, problems...)
	found = append(found, registry.RegisterDocuments(documents)...)

	name := model.settings.Theme
	if name == "" {
		name = model.styles.Theme.Name
	}
	reported, applied := styles.ApplyThemeByName(name)
	found = append(found, reported...)
	if !applied {
		connection.ShowError("unknown theme: \"" + name + "\"")
		return model, nil
	}
	found = append(found, styles.ApplyColorOverrides(model.settings.Colors)...)
	model.styles = styles
	model.problems = found

	if len(found) == 0 {
		connection.Show(present.FormatCountOf(
			int64(len(documents)), "theme file", "theme files") + " read")
		return model, nil
	}
	if len(found) == 1 {
		connection.ShowError(found[0])
		return model, nil
	}
	connection.ShowError(present.FormatCount(int64(len(found))) +
		" problems · see Config problems in the palette")
	return model, nil
}

// themeCursor returns the row of the theme picker the applied theme stands on.
func (model *Model) themeCursor() int {
	for at, choice := range model.styles.registry.ListThemeChoices() {
		if choice.Name == model.styles.Theme.Name {
			return at
		}
	}
	return 0
}
