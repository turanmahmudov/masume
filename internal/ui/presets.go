package ui

import (
	"github.com/turanmahmudov/masume/internal/cfg"
)

// A preset is a whole set of keys, not a change over another one, so every key is readable
// in one place. The chords are written as in the config file, so one parser reads a preset
// and a user file.

// KeyPreset is one preset: a whole set of chords under a name.
type KeyPreset struct {
	ID    cfg.PresetID
	Title string
	// One line, for the palette row and the help.
	Describe string
	// The chords of the preset, keyed by action.
	Chords map[string][]string
}

// defaultChords are the keys this app ships.
var defaultChords = map[string][]string{
	"global:run-batch":           {"alt+r"},
	"global:new-query-tab":       {"alt+n"},
	"global:toggle-sidebar":      {"alt+s"},
	"global:toggle-result":       {"alt+d"},
	"global:reveal-sql":          {"alt+e"},
	"global:name-tab":            {"alt+t"},
	"global:close-tab":           {"alt+w"},
	"global:reopen-tab":          {"alt+shift+w"},
	"global:previous-tab":        {"[", "alt+up"},
	"global:next-tab":            {"]", "alt+down"},
	"global:previous-connection": {"{", "alt+left"},
	"global:next-connection":     {"}", "alt+right"},

	"global:run-at-cursor":        {"ctrl+r"},
	"global:explain":              {"ctrl+e"},
	"global:explain-analyze":      {"ctrl+y"},
	"global:show-history":         {"ctrl+t"},
	"global:show-saved":           {"ctrl+q"},
	"global:cancel-query":         {"ctrl+x"},
	"global:show-palette":         {"ctrl+k"},
	"global:show-ai-chat":         {"ctrl+i"},
	"global:ai-fix-error":         {"ctrl+h"},
	"global:send-to-ai":           {"alt+i"},
	"global:next-page":            {"ctrl+f"},
	"global:export-csv":           {"ctrl+s"},
	"global:export-json":          {"ctrl+g"},
	"global:begin-transaction":    {"ctrl+b"},
	"global:commit-transaction":   {"ctrl+l"},
	"global:rollback-transaction": {"ctrl+u"},
	"global:toggle-autocommit":    {"ctrl+o"},
	"global:undo-write":           {"alt+u"},
	"global:open-picker":          {"ctrl+n"},
	"global:close-connection":     {"ctrl+w"},

	// The statement being written is saved under a name.
	"global:save-query": {"ctrl+p"},

	// Two prefixes carry the commands that are asked for rarely, so the single keys stay
	// free: `alt+p` moves the focus to a pane by name, and `alt+o` opens a card.
	"global:focus-sidebar":         {"alt+p s"},
	"global:focus-editor":          {"alt+p e"},
	"global:focus-result":          {"alt+p r"},
	"global:show-themes":           {"alt+o t"},
	"global:show-notebooks":        {"alt+o n"},
	"global:notebook-run-policy":   {"alt+o p"},
	"global:write-notebook-report": {"alt+o r"},

	// A terminal that drops the Shift of Alt+Shift+N sends Alt+N, which opens a query
	// tab, so the first chord carries no Shift.
	"global:new-notebook-tab": {"alt+b", "alt+shift+n"},
	"global:show-activity":    {"alt+o a"},

	"global:show-help":          {"?"},
	"global:previous-statement": {";"},
	"global:next-statement":     {"'"},
	"global:refresh-objects":    {"f5"},

	// Tab moves between the panes.
	"global:focus-next-pane":     {"tab"},
	"global:focus-previous-pane": {"shift+tab"},

	// Alt and a number choose a tab. A number alone chooses a view. Each one is a single
	// binding over nine keys.
	"global:activate-tab":  {"alt+digit"},
	"global:select-view":   {"digit"},
	"global:previous-view": {","},
	"global:next-view":     {"."},

	"grid:cursor-up":        {"up"},
	"grid:cursor-down":      {"down"},
	"grid:cursor-page-up":   {"pageup"},
	"grid:cursor-page-down": {"pagedown"},
	"grid:cursor-first-row": {"home"},
	"grid:cursor-last-row":  {"end"},
	"grid:cursor-left":      {"left"},
	"grid:cursor-right":     {"right"},
	"grid:sort-column":      {"s"},
	"grid:add-sort-column":  {"S"},
	"grid:open-row":         {"return"},
	"grid:view-cell":        {"v"},
	"grid:edit-cell":        {"e"},
	"grid:toggle-delete":    {"d"},
	"grid:duplicate-row":    {"D"},
	"grid:review-changes":   {"p"},
	"grid:undo-change":      {"ctrl+z"},
	// Only a kitty-protocol terminal reports Ctrl+Shift+Z apart from Ctrl+Z, so the shifted
	// letter is bound too, or a plain terminal has no redo key.
	"grid:redo-change":        {"ctrl+shift+z", "Z"},
	"grid:count-rows":         {"t"},
	"grid:follow-foreign-key": {"g"},
	"grid:insert-row":         {"n"},
	"grid:copy-menu":          {"y"},
	// The copy menu offers these four as well. Each one keeps a chord of its own, under
	// one prefix, so a copy that is made often needs no menu.
	"grid:copy-csv":         {"C c"},
	"grid:copy-json":        {"C j"},
	"grid:copy-markdown":    {"C m"},
	"grid:copy-inserts":     {"C i"},
	"grid:discard-changes":  {"X"},
	"grid:open-menu":        {"m"},
	"grid:filter-by-cell":   {"f"},
	"grid:filter-by-values": {"F"},
	"grid:exclude-cell":     {"x"},
	"grid:clear-rewrites":   {"c"},
	"grid:pop-filter":       {"u"},
	"grid:freeze-columns":   {"z"},
	"grid:toggle-masking":   {"M"},
	"grid:go-to-column":     {"a"},
	"grid:search-columns":   {"/"},
	"grid:filter-where":     {"w"},

	// The statement being written. Every move is bound twice: the plain chord moves the
	// caret, and the shifted one takes the selection with it.
	"editor:caret-left":       {"left", "shift+left"},
	"editor:caret-right":      {"right", "shift+right"},
	"editor:caret-up":         {"up", "shift+up"},
	"editor:caret-down":       {"down", "shift+down"},
	"editor:caret-word-left":  {"ctrl+left", "ctrl+shift+left"},
	"editor:caret-word-right": {"ctrl+right", "ctrl+shift+right"},
	"editor:caret-line-start": {"home", "shift+home"},
	"editor:caret-line-end":   {"end", "shift+end"},
	"editor:caret-text-start": {"ctrl+home", "ctrl+shift+home"},
	"editor:caret-text-end":   {"ctrl+end", "ctrl+shift+end"},
	"editor:caret-page-up":    {"pageup", "shift+pageup"},
	"editor:caret-page-down":  {"pagedown", "shift+pagedown"},

	"editor:select-all":          {"ctrl+a"},
	"editor:delete-back":         {"backspace"},
	"editor:delete-forward":      {"delete"},
	"editor:delete-word-back":    {"ctrl+backspace", "alt+backspace"},
	"editor:delete-word-forward": {"ctrl+delete", "alt+delete"},
	"editor:open-line":           {"return"},
	"editor:undo-edit":           {"ctrl+z"},
	// Only a kitty-protocol terminal reports Ctrl+Shift+Z apart from Ctrl+Z, so the redo
	// binds a second chord, as the redo of the grid does.
	"editor:redo-edit":  {"ctrl+shift+z", "alt+z"},
	"editor:paste-text": {"ctrl+v"},
	"editor:format-sql": {"ctrl+d"},
	// The commands of the editor that no other pane has sit on Alt, because Ctrl is full.
	"editor:comment-lines":     {"alt+c"},
	"editor:indent-lines":      {"alt+]"},
	"editor:outdent-lines":     {"alt+["},
	"editor:find-in-statement": {"alt+f"},
	"editor:next-match":        {"f3"},
	"editor:previous-match":    {"shift+f3"},
	// The row that reports a fault names this key, so it has to reach the fault it names.
	"editor:next-problem": {"f8"},
	"editor:leave-cell":   {"escape"},

	// The cell list of a notebook. Single letters are free there, because the list takes
	// no typed text.
	"notebook:cursor-up":           {"up", "k"},
	"notebook:cursor-down":         {"down", "j"},
	"notebook:cursor-first-row":    {"home"},
	"notebook:cursor-last-row":     {"end"},
	"notebook:edit-cell-source":    {"return"},
	"notebook:run-cell":            {"r"},
	"notebook:run-from-cell":       {"R"},
	"notebook:run-marked-cells":    {"m"},
	"notebook:add-cell-below":      {"b"},
	"notebook:add-cell-above":      {"a"},
	"notebook:set-cell-kind":       {"c"},
	"notebook:delete-cell":         {"d d"},
	"notebook:undo-cell-change":    {"u"},
	"notebook:redo-cell-change":    {"Z"},
	"notebook:move-cell-up":        {"K"},
	"notebook:move-cell-down":      {"J"},
	"notebook:copy-cell":           {"y"},
	"notebook:cut-cell":            {"x"},
	"notebook:paste-cell":          {"p"},
	"notebook:toggle-cell-output":  {"o"},
	"notebook:toggle-every-output": {"O"},
	"notebook:mark-cell":           {"space"},
	"notebook:name-cell":           {"t"},

	"document:clear-rewrites":   {"c"},
	"document:copy-path":        {"shift+y"},
	"document:copy-value":       {"y"},
	"document:count-rows":       {"t"},
	"document:cursor-down":      {"down"},
	"document:cursor-first-row": {"home"},
	"document:cursor-last-row":  {"end"},
	"document:cursor-page-down": {"pagedown"},
	"document:cursor-page-up":   {"pageup"},
	"document:cursor-up":        {"up"},
	"document:fold-row":         {"left"},
	"document:open-node":        {"return"},
	"document:pop-filter":       {"u"},
	"document:search-columns":   {"/"},
	"document:unfold-row":       {"right"},

	"plan:toggle-raw-plan": {"r"},
	"plan:copy-plan":       {"y"},
	"plan:ai-check-plan":   {"i"},

	"tree:cursor-up":             {"up"},
	"tree:cursor-down":           {"down"},
	"tree:cursor-page-up":        {"pageup"},
	"tree:cursor-page-down":      {"pagedown"},
	"tree:cursor-first-row":      {"home"},
	"tree:cursor-last-row":       {"end"},
	"tree:fold-row":              {"left"},
	"tree:unfold-row":            {"right"},
	"tree:open-node":             {"return"},
	"tree:open-in-new-tab":       {"o"},
	"tree:describe-table":        {"i"},
	"tree:object-menu":           {"m"},
	"tree:filter-tree":           {"/"},
	"tree:toggle-favourite":      {"f"},
	"tree:toggle-system-schemas": {"h"},

	// The list keys, defined here like every other key, and not read off the event.
	"list:cursor-up":        {"up"},
	"list:cursor-down":      {"down"},
	"list:cursor-page-up":   {"pageup"},
	"list:cursor-page-down": {"pagedown"},
	"list:cursor-first-row": {"home"},
	"list:cursor-last-row":  {"end"},
	"list:choose-row":       {"return"},

	"dialog:close":                {"escape"},
	"dialog:answer-yes":           {"y"},
	"dialog:answer-no":            {"n"},
	"dialog:new-connection":       {"n"},
	"dialog:edit-connection":      {"e"},
	"dialog:delete-connection":    {"d"},
	"dialog:save-form":            {"ctrl+s"},
	"dialog:test-connection":      {"ctrl+t"},
	"dialog:save-cell":            {"ctrl+s"},
	"dialog:prettify-json":        {"ctrl+f"},
	"dialog:replace-in-statement": {"ctrl+r"},
	"dialog:set-null":             {"ctrl+l"},
	"dialog:set-empty":            {"ctrl+e"},
	"dialog:set-default":          {"ctrl+d"},
	"dialog:run-with-values":      {"ctrl+r"},
	"dialog:write-export":         {"ctrl+s"},
	"dialog:copy-value":           {"ctrl+a", "y"},
	"dialog:open-in-new-tab":      {"alt+return"},
	"dialog:list-secondary":       {"ctrl+d"},
	"dialog:stop-session":         {"x"},
	// The keys that fold a schema in the tree, so a panel of a card folds the same way.
	"dialog:fold-row":        {"left"},
	"dialog:unfold-row":      {"right"},
	"dialog:toggle-value":    {"space"},
	"dialog:keep-all-values": {"a"},
	"dialog:keep-only-value": {"o"},
	"dialog:apply-changes":   {"ctrl+y"},
	"dialog:discard-changes": {"x"},
	"dialog:insert-ai-sql":   {"ctrl+j"},
	// The same key that cancels a running query in the workspace. The workspace takes no
	// key while a dialog is open, so the panel binds its own.
	"dialog:stop-ai-reply": {"ctrl+x"},
	// `ctrl+l` clears the screen in a shell, and it leaves `ctrl+n` for the keys below.
	"dialog:new-ai-chat":   {"ctrl+l"},
	"dialog:show-ai-chats": {"ctrl+o"},
	// The conversation becomes a notebook, with one cell per statement the model wrote.
	"dialog:chat-to-notebook": {"ctrl+g"},
	// The panel has a field, so these keys must work while the user types. One control byte
	// each, which reaches the app through tmux and ssh. Not a modified arrow, because
	// `alt+up` switches tab and depends on the terminal.
	"dialog:scroll-back":    {"pageup"},
	"dialog:scroll-forward": {"pagedown"},
	"dialog:previous-turn":  {"ctrl+p"},
	"dialog:next-turn":      {"ctrl+n"},
	// The panel sends the question on the plain key and writes a line with a modifier.
	"dialog:send-question": {"return"},
	"dialog:write-newline": {"shift+return", "alt+return"},

	// A form steps through its rows and through the values of the row under the cursor.
	// One card owns each of these keys. The same arrow then serves a form, a file picker
	// and a card of one row.
	"dialog:previous-field":  {"up"},
	"dialog:next-field":      {"down", "tab"},
	"dialog:previous-value":  {"left"},
	"dialog:next-value":      {"right"},
	"dialog:apply-step":      {"return"},
	"dialog:step-back":       {"escape"},
	"dialog:previous-row":    {"left"},
	"dialog:next-row":        {"right"},
	"dialog:scroll-left":     {"left"},
	"dialog:scroll-right":    {"right"},
	"dialog:open-directory":  {"right"},
	"dialog:leave-directory": {"left"},
	"dialog:use-keyring":     {"tab"},
	// The list of completions owns the keyboard while it is open. Its key is bound here,
	// and it may share the chord the workspace uses to step through the panes.
	"dialog:accept-completion": {"tab"},
}

// DefaultPreset is the set of keys this app ships.
var DefaultPreset = KeyPreset{
	ID: cfg.PresetDefault, Title: "default",
	Describe: "the keys this app ships with", Chords: defaultChords,
}

var keyPresets = []KeyPreset{DefaultPreset}

// ListKeyPresets returns every preset the app offers.
func ListKeyPresets() []KeyPreset {
	return keyPresets
}

// FindKeyPreset returns the preset of that name, or the default one.
func FindKeyPreset(id cfg.PresetID) KeyPreset {
	for _, preset := range keyPresets {
		if preset.ID == id {
			return preset
		}
	}
	return DefaultPreset
}
