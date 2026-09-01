package ui

import (
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/turanmahmudov/masume/internal/app"
	"github.com/turanmahmudov/masume/internal/cfg"
	"github.com/turanmahmudov/masume/internal/present"
)

// The command palette: the rows it offers, what each row runs, and the switches of a preset,
// a provider and a theme that only it reaches.

// The prefix of the id of a palette row that changes the AI provider, and of one that
// changes the key preset.
const (
	aiProviderPrefix = "ai-provider:"
	keyPresetPrefix  = "key-preset:"
	// configProblemsAction shows the faults of the config. It is offered only if there
	// are any, so the row itself is the message.
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
	{id: "save-query", label: "Save this query", detail: "names the editor buffer",
		scope: cfg.ScopeGlobal, action: ActionSaveQuery},
	{id: "show-saved", label: "Saved queries",
		scope: cfg.ScopeGlobal, action: ActionShowSaved},
	{id: "activity", label: "Server activity", detail: "what other sessions are running",
		scope: cfg.ScopeGlobal, action: ActionShowActivity},
	{id: "export-csv", label: "Export result as CSV",
		scope: cfg.ScopeGlobal, action: ActionExportCSV},
	{id: "export-json", label: "Export result as JSON",
		scope: cfg.ScopeGlobal, action: ActionExportJSON},
	{id: "reopen-tab", label: "Reopen the tab closed last",
		scope: cfg.ScopeGlobal, action: ActionReopenTab},
	{id: "undo-change", label: "Undo the last staged change", detail: "in the grid",
		scope: cfg.ScopeGrid, action: ActionUndoChange},
	{id: "redo-change", label: "Redo the change taken back", detail: "in the grid",
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
	{id: "tab-data", label: "View: Data", detail: "the first entry"},
	{id: "tab-fields", label: "View: Fields",
		detail: "a query tab · what the server answered with"},
	{id: "tab-statistics", label: "View: Statistics", detail: "what a write did · rows, timing"},
	{id: "tab-columns", label: "View: Columns", detail: "a table tab · how the table is defined"},
	{id: "tab-indexes", label: "View: Indexes", detail: "a table tab only"},
	{id: "tab-constraints", label: "View: Constraints", detail: "a table tab only"},
	{id: "tab-ddl", label: "View: DDL", detail: "a table tab only"},
	{id: "tab-plan", label: "View: Plan", detail: "the last entry"},
	{id: "reveal-sql", label: "Edit the query behind this result",
		detail: "a table opens as a query", scope: cfg.ScopeGlobal, action: ActionRevealSQL},
	{id: "toggle-sidebar", label: "Show or hide the object tree",
		scope: cfg.ScopeGlobal, action: ActionToggleSidebar},
	{id: "toggle-result", label: "Show or hide the result",
		detail: "the editor takes the height",
		scope:  cfg.ScopeGlobal, action: ActionToggleResult},
	{id: "focus-sidebar", label: "Focus the table list",
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
	{id: "close-tab", label: "Close this tab", detail: "asks if work is staged",
		scope: cfg.ScopeGlobal, action: ActionCloseTab},
	{id: "name-tab", label: "Name this tab", scope: cfg.ScopeGlobal, action: ActionNameTab},
	{id: "refresh-objects", label: "Refresh the object tree",
		detail: "reloads tables and roles",
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
		detail: "in the plan · as the server sent it",
		scope:  cfg.ScopePlan, action: ActionCopyPlan},
	{id: "open-picker", label: "New connection",
		scope: cfg.ScopeGlobal, action: ActionOpenPicker},
	{id: "close-connection", label: "Close this connection", detail: "every tab of it",
		scope: cfg.ScopeGlobal, action: ActionCloseConnection},
	{id: "next-page", label: "Fetch more rows",
		scope: cfg.ScopeGlobal, action: ActionNextPage},
	{id: "count-rows", label: "Count every row in the result", detail: "in the grid",
		scope: cfg.ScopeGrid, action: ActionCountRows},
	{id: "format-sql", label: "Format the query", detail: "reflows the editor buffer",
		scope: cfg.ScopeEditor, action: ActionFormatSQL},
	{id: "show-themes", label: "Theme",
		detail: "each one shown while the cursor stands on it",
		scope:  cfg.ScopeGlobal, action: ActionShowThemes},
	{id: "reload-themes", label: "Reload the theme files",
		detail: "for a theme being written, so the app is not restarted"},
	{id: "show-help", label: "Help", scope: cfg.ScopeGlobal, action: ActionShowHelp},
	{id: "show-ai-chat", label: "Ask AI", detail: "chat, and write queries for you",
		scope: cfg.ScopeGlobal, action: ActionShowAiChat},
	{id: "ai-explain-query", label: "Ask AI: explain this query",
		detail: "the one in the editor"},
	{id: "ai-optimize-query", label: "Ask AI: optimize this query",
		detail: "the one in the editor"},
	{id: "ai-fix-error", label: "Ask AI: fix the error",
		detail: "the editor's last failed run",
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
				" · what the config file and the theme files got wrong",
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
		connection.ShowError("\"" + id + "\" is offered by the palette and nothing runs it")
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

// switchKeyPreset applies another preset. The choice is not written to the config file,
// because that would drop its comments, so the app reports what to write.
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

// reloadThemeFiles reads the theme files again, so a theme can be edited without a restart.
// The theme named in the config file is applied again, so an edit to the theme on screen is
// seen at once.
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
		connection.ShowError("theme \"" + name + "\" is not one there is")
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
		" problems · the palette lists them")
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
