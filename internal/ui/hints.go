package ui

import (
	"slices"
	"strconv"
	"strings"

	"github.com/turanmahmudov/masume/internal/app"
	"github.com/turanmahmudov/masume/internal/cfg"
	"github.com/turanmahmudov/masume/internal/core"
	"github.com/turanmahmudov/masume/internal/notebook"
	"github.com/turanmahmudov/masume/internal/present"
)

// Hint is one key and its label in the status bar.
type Hint struct {
	Key   string
	Label string
	// True for a key that is always there. The bar draws it a step back.
	Standing bool
	// True for a primary key. The main mode shows it as well.
	Main bool
	// The action the key runs, so a press on the hint runs it as the key does. A hint of a
	// pair holds both, and the half of the key that was pressed decides which one.
	Scope  cfg.KeyScope
	Action ActionID
	Second ActionID
}

// HintContext is what the hint bar reads to decide what to show.
type HintContext struct {
	Pane         app.Pane
	Capabilities core.Capabilities
	TabKind      app.TabKind
	// True while the cell list of a notebook holds the keyboard.
	ListsCells bool
	// The kind of the focused cell of a notebook.
	CellKind notebook.CellKind
	View     app.ResultView
	// The views this tab offers.
	Views     []app.ResultView
	HasResult bool
	// How many connections are open.
	Connections  int
	HasSelection bool
	// False while the tree is hidden.
	SidebarVisible bool
	Rewritten      bool
	FilterSteps    int
	Staged         int
	CanFetchMore   bool
	CanCountRows   bool
	Running        bool
	// True while the system schemas are hidden, so the key that shows them says so.
	SystemSchemasHidden bool
	QueryFailed         bool
	TreeRow             *present.TreeRow
}

// resolveKeyHints returns the key hints mode. An unset mode is the full mode.
func (model *Model) resolveKeyHints() cfg.KeyHintsMode {
	if model.settings.KeyHints == "" {
		return cfg.KeyHintsFull
	}
	return model.settings.KeyHints
}

// showsKeyHints is false in the off mode, which hides every key hint.
func (model *Model) showsKeyHints() bool {
	return model.resolveKeyHints() != cfg.KeyHintsOff
}

// showsEveryKeyHint is true in the full mode, which shows every key hint.
func (model *Model) showsEveryKeyHint() bool {
	return model.resolveKeyHints() == cfg.KeyHintsFull
}

// buildHint returns the hint for an action. It returns nothing if the action has no key, or
// the server cannot do it.
func (model *Model) buildHint(
	capabilities core.Capabilities, scope cfg.KeyScope, id ActionID, label string,
) (Hint, bool) {
	if !AnswersFor(capabilities, FindActionCapability(scope, id)) {
		return Hint{}, false
	}
	return model.buildScreenHint(scope, id, label)
}

func (model *Model) buildScreenHint(
	scope cfg.KeyScope, id ActionID, label string,
) (Hint, bool) {
	chords := model.registry.FindActionChords(scope, id)
	if len(chords) == 0 {
		return Hint{}, false
	}
	return Hint{
		Key: FormatChord(chords[0]), Label: label, Scope: scope, Action: id,
		Main: IsMainHint(scope, id),
	}, true
}

// buildPairHint returns one hint for a pair of actions, such as fold and unfold.
func (model *Model) buildPairHint(
	scope cfg.KeyScope, previous, next ActionID, label, separator string,
) (Hint, bool) {
	key := model.registry.FormatChordPair(scope, previous, next, separator)
	if key == "" {
		return Hint{}, false
	}
	return Hint{
		Key: key, Label: label, Scope: scope, Action: previous, Second: next,
		Main: IsMainHint(scope, previous),
	}, true
}

// hintList gathers the keys of one bar. A key the server, the row or the state does not offer
// is left out, so the bar never shows a gap where a hint would be.
type hintList struct {
	hints []Hint
}

// add keeps one hint where the builder found one.
func (list *hintList) add(hint Hint, found bool) {
	if found {
		list.hints = append(list.hints, hint)
	}
}

// addAll keeps every hint another bar built.
func (list *hintList) addAll(hints []Hint) {
	list.hints = append(list.hints, hints...)
}

// build returns the hints kept, in the order they were added.
func (list *hintList) build() []Hint {
	if list.hints == nil {
		return []Hint{}
	}
	return list.hints
}

// copySelection and quit are the two keys the bar closes with. The renderer quits on this
// key itself, so no action is bound to it.
var (
	copySelection = Hint{Key: "^C", Label: "copy selection", Standing: true}
	quitHint      = Hint{Key: "^C", Label: "quit", Standing: true}
)

// addCopyOrQuit puts a copy at the head of the bar, and a quit at its end.
func addCopyOrQuit(keys []Hint, hasSelection bool) []Hint {
	if hasSelection {
		return append([]Hint{copySelection}, keys...)
	}
	return append(keys, quitHint)
}

// BuildPickerHints returns the keys of the profile picker. The list keys come from the
// registry, so a preset moves the hint too.
func (model *Model) BuildPickerHints(hasSelection bool) []Hint {
	keys := hintList{}
	keys.add(model.buildPairHint(
		cfg.ScopeList, ActionCursorUp, ActionCursorDown, "select", ""))
	keys.add(model.buildScreenHint(cfg.ScopeList, ActionChooseRow, "connect"))
	return addCopyOrQuit(keys.build(), hasSelection)
}

// BuildConnectingHints returns the keys while the client waits for a server. Escape is
// written out because the root model reads it off the event, not the registry.
func (model *Model) BuildConnectingHints(hasSelection bool) []Hint {
	keys := hintList{}
	keys.add(model.buildScreenHint(cfg.ScopeDialog, ActionClose, "cancel"))
	return addCopyOrQuit(keys.build(), hasSelection)
}

// BuildCardScreenHints returns the keys of a screen whose card names its own keys. The bar
// shows only the copy or quit key.
func (model *Model) BuildCardScreenHints(hasSelection bool) []Hint {
	return addCopyOrQuit(nil, hasSelection)
}

func standWith(hint Hint, found bool) (Hint, bool) {
	if !found {
		return Hint{}, false
	}
	hint.Standing = true
	return hint, true
}

// buildAnswerHint returns the primary key of a field or a list: the key it is answered or
// left with. The registry has no such key, and no press runs it.
func buildAnswerHint(chord, label string) Hint {
	return Hint{Key: chord, Label: label, Main: true}
}

// buildAsideHint returns a secondary key of a field or a list, such as a move or a scroll.
// Only the full mode shows it.
func buildAsideHint(chord, label string) Hint {
	return Hint{Key: chord, Label: label}
}

// keepKeylessHints returns the hints with no key. The off mode shows these alone.
func keepKeylessHints(hints []Hint) []Hint {
	kept := make([]Hint, 0, len(hints))
	for _, hint := range hints {
		if hint.Key == "" {
			kept = append(kept, hint)
		}
	}
	return kept
}

// keepMainHints returns the hints of the main mode: the standing keys and the primary keys.
func keepMainHints(hints []Hint) []Hint {
	kept := make([]Hint, 0, len(hints))
	for _, hint := range hints {
		if hint.Standing || hint.Main {
			kept = append(kept, hint)
		}
	}
	return kept
}

// buildOpenHint returns what Enter does on the row under the cursor.
func (model *Model) buildOpenHint(
	capabilities core.Capabilities, row present.TreeRow,
) (Hint, bool) {
	open := func(label string) (Hint, bool) {
		return model.buildHint(capabilities, cfg.ScopeTree, ActionOpenNode, label)
	}
	switch row.Node.Kind {
	case present.NodeTable, present.NodeObject:
		return open("open")
	case present.NodeColumn:
		return open("insert the column name")
	case present.NodeSchema:
		// A favourite schema is not the row its tables hang from, so opening it reveals
		// that row.
		if !row.Expandable {
			return open("reveal")
		}
		if row.Expanded {
			return open("fold")
		}
		return open("unfold")
	}
	if !row.Expandable {
		return Hint{}, false
	}
	if row.Expanded {
		return open("fold")
	}
	return open("unfold")
}

func describeFold(node present.TreeNode) string {
	switch node.Kind {
	case present.NodeTable:
		return "fold the columns"
	case present.NodeSchema:
		return "fold the tables"
	}
	return "fold the contents"
}

// buildTreeHints returns the keys the row under the cursor returns. Each kind of row returns
// a different set.
func (model *Model) buildTreeHints(
	capabilities core.Capabilities, row *present.TreeRow, systemSchemasHidden bool,
) []Hint {
	keys := hintList{}

	if row != nil {
		keys.add(model.buildOpenHint(capabilities, *row))
		if row.Node.Kind == present.NodeTable {
			keys.add(model.buildHint(
				capabilities, cfg.ScopeTree, ActionOpenInNewTab, "open in a new tab"))
		}
		if row.Node.Kind == present.NodeTable || row.Node.Kind == present.NodeColumn {
			keys.add(model.buildHint(
				capabilities, cfg.ScopeTree, ActionDescribeTable, "describe"))
		}
		if row.Expandable {
			keys.add(model.buildPairHint(cfg.ScopeTree, ActionFoldRow, ActionUnfoldRow,
				describeFold(row.Node), ""))
		}
		if _, marks := present.FindFavouriteOf(row.Node); marks {
			label := "favourite"
			if row.Marked {
				label = "unfavourite"
			}
			keys.add(model.buildHint(capabilities, cfg.ScopeTree, ActionToggleFavourite, label))
		}
		if len(app.BuildObjectActions(row.Node, capabilities)) > 0 {
			keys.add(model.buildHint(
				capabilities, cfg.ScopeTree, ActionObjectMenu, "menu"))
		}
	}

	keys.add(model.buildHint(capabilities, cfg.ScopeTree, ActionFilterTree, "filter"))
	systemSchemas := "hide system schemas"
	if systemSchemasHidden {
		systemSchemas = "show system schemas"
	}
	keys.add(model.buildHint(
		capabilities, cfg.ScopeTree, ActionToggleSystemSchemas, systemSchemas))
	keys.add(model.buildHint(capabilities, cfg.ScopeGlobal, ActionRefreshObjects, "refresh"))
	keys.add(model.buildHint(
		capabilities, cfg.ScopeGlobal, ActionToggleSidebar, "hide the explorer"))
	return keys.build()
}

func (model *Model) buildEditorHints(capabilities core.Capabilities) []Hint {
	keys := hintList{}
	keys.add(model.buildHint(
		capabilities, cfg.ScopeGlobal, ActionRunAtCursor, "run the statement"))
	keys.add(model.buildHint(capabilities, cfg.ScopeGlobal, ActionRunBatch, "run all"))
	keys.add(model.buildHint(capabilities, cfg.ScopeGlobal, ActionExplain, "explain"))
	// One key opens the field that finds, and the field itself offers to replace, so the
	// bar names both.
	keys.add(model.buildHint(
		capabilities, cfg.ScopeEditor, ActionFindInStatement, "find or replace"))
	// The key that reaches the model stands on the border of the editor, where the one key
	// the editor offers the model always stands, so the bar does not name it a second time.
	keys.add(model.buildHint(
		capabilities, cfg.ScopeGlobal, ActionToggleResult, "full height"))
	// The editor returns these keys itself, so the registry cannot move them.
	keys.add(model.buildScreenHint(cfg.ScopeDialog, ActionAcceptCompletion, "complete"))
	// A caret key with Shift takes the selection along. The registry binds the two as one
	// action, and the hint marks the pair the caret moves with.
	if pair := model.registry.FormatChordPair(
		cfg.ScopeEditor, ActionCaretLeft, ActionCaretRight, ""); pair != "" {
		keys.addAll([]Hint{buildAsideHint("⇧ "+pair, "select")})
	}
	keys.add(model.buildScreenHint(cfg.ScopeEditor, ActionSelectAll, "select all"))
	return keys.build()
}

// findViewToLeaveFor returns the view a digit key goes to: the data view if there is one,
// else the first other view.
func findViewToLeaveFor(view app.ResultView, views []app.ResultView) (app.ResultView, bool) {
	if view != app.ViewData {
		if slices.Contains(views, app.ViewData) {
			return app.ViewData, true
		}
	}
	for _, offered := range views {
		if offered != view {
			return offered, true
		}
	}
	return "", false
}

func (model *Model) buildViewHints(
	view app.ResultView, views []app.ResultView, capabilities core.Capabilities,
) []Hint {
	scroll, _ := model.buildPairHint(
		cfg.ScopeList, ActionCursorUp, ActionCursorDown, "scroll", "")
	switch view {
	case app.ViewTree:
		keys := hintList{}
		keys.add(model.buildHint(
			capabilities, cfg.ScopeDocument, ActionOpenNode, "open or fold"))
		keys.add(model.buildPairHint(
			cfg.ScopeDocument, ActionFoldRow, ActionUnfoldRow, "fold or open", ""))
		keys.add(model.buildPairHint(
			cfg.ScopeDocument, ActionCursorUp, ActionCursorDown, "move", ""))
		keys.add(model.buildHint(
			capabilities, cfg.ScopeDocument, ActionCopyValue, "copy value"))
		keys.add(model.buildHint(
			capabilities, cfg.ScopeDocument, ActionCopyPath, "copy field"))
		keys.add(model.buildHint(
			capabilities, cfg.ScopeDocument, ActionSearchColumns, "search"))
		keys.add(model.buildHint(
			capabilities, cfg.ScopeDocument, ActionCountRows, "count"))
		return keys.build()
	}
	if view == app.ViewPlan {
		keys := hintList{}
		keys.add(model.buildHint(
			capabilities, cfg.ScopePlan, ActionToggleRawPlan, "raw or tree"))
		keys.add(model.buildHint(capabilities, cfg.ScopeGlobal, ActionExplain, "explain"))
		keys.add(model.buildHint(
			capabilities, cfg.ScopeGlobal, ActionExplainAnalyze, "analyze"))
		keys.add(model.buildHint(capabilities, cfg.ScopePlan, ActionCopyPlan, "copy"))
		keys.add(model.buildHint(capabilities, cfg.ScopePlan, ActionAiCheckPlan, "ask ai"))
		keys.addAll([]Hint{scroll})
		return keys.build()
	}

	target, found := findViewToLeaveFor(view, views)
	if !found {
		return []Hint{scroll}
	}
	position := 0
	for at, offered := range views {
		if offered == target {
			position = at
			break
		}
	}
	label := "the " + string(target)
	if target == app.ViewData {
		label = "back to the rows"
	}
	return []Hint{buildAnswerHint(strconv.Itoa(position+1), label), scroll}
}

// BuildHints returns the keys the bar names, which depend on the cursor position and what
// the result can do.
func (model *Model) BuildHints(context HintContext) []Hint {
	// With nothing selected the key quits instead of copying.
	selection := []Hint{}
	if context.HasSelection {
		selection = []Hint{copySelection}
	}
	capabilities := context.Capabilities

	connection := hintList{}
	if context.Connections > 1 {
		connection.add(standWith(model.buildPairHint(cfg.ScopeGlobal,
			ActionPreviousConnection, ActionNextConnection, "connection", " ")))
	}

	// A hidden tree can be reached by no other key, so the bar names it first.
	missingSidebar := hintList{}
	if !context.SidebarVisible {
		missingSidebar.add(standWith(model.buildHint(
			capabilities, cfg.ScopeGlobal, ActionToggleSidebar, "explorer")))
	}

	// Every bar reads the same way: the copy first, the connection keys last.
	closeBar := func(hints []Hint) []Hint {
		written := hintList{}
		written.addAll(selection)
		written.addAll(missingSidebar.build())
		written.addAll(hints)
		written.addAll(connection.build())
		return written.build()
	}

	// A running query owns the connection, so cancelling is the only useful key.
	if context.Running {
		keys := hintList{}
		keys.add(model.buildHint(
			capabilities, cfg.ScopeGlobal, ActionCancelQuery, "cancel"))
		return closeBar(keys.build())
	}

	if context.Pane == app.PaneSidebar {
		return closeBar(model.buildTreeHints(
			capabilities, context.TreeRow, context.SystemSchemasHidden))
	}
	if context.Pane == app.PaneEditor {
		if context.ListsCells {
			return closeBar(model.buildNotebookHints(context))
		}
		if context.TabKind == app.TabNotebook {
			return closeBar(model.buildCellEditorHints(context))
		}
		return closeBar(model.buildEditorHints(capabilities))
	}

	// After a failure: run again once it is fixed. The key that asks the model why it
	// failed stands on the border of the editor, so the bar leaves it to the editor.
	if context.QueryFailed {
		keys := hintList{}
		keys.add(model.buildHint(
			capabilities, cfg.ScopeGlobal, ActionRunAtCursor, "run"))
		return closeBar(keys.build())
	}

	// Sorting and filtering act on rows, so a view without rows offers other keys.
	if context.View != app.ViewData {
		return closeBar(model.buildViewHints(context.View, context.Views, capabilities))
	}

	if !context.HasResult {
		keys := hintList{}
		keys.add(model.buildHint(capabilities, cfg.ScopeGlobal, ActionRunAtCursor, "run"))
		return closeBar(keys.build())
	}

	// The menu holds the row and cell keys, so the bar keeps only the rest.
	keys := hintList{}
	keys.add(model.buildHint(capabilities, cfg.ScopeGrid, ActionOpenMenu, "menu"))
	keys.add(model.buildHint(capabilities, cfg.ScopeGrid, ActionSearchColumns, "search"))
	keys.add(model.buildHint(capabilities, cfg.ScopeGrid, ActionFilterWhere, "where"))

	if context.FilterSteps > 1 {
		keys.add(model.buildHint(
			capabilities, cfg.ScopeGrid, ActionPopFilter, "drop the last filter"))
	}
	if context.Rewritten {
		keys.add(model.buildHint(capabilities, cfg.ScopeGrid, ActionClearRewrites, "clear"))
		keys.add(model.buildHint(
			capabilities, cfg.ScopeGlobal, ActionRevealSQL, "edit as a query"))
	} else if context.TabKind == app.TabTable {
		keys.add(model.buildHint(
			capabilities, cfg.ScopeGlobal, ActionRevealSQL, "edit as a query"))
	}
	keys.add(model.buildHint(capabilities, cfg.ScopeGrid, ActionGoToColumn, "go to column"))
	keys.add(model.buildHint(capabilities, cfg.ScopeGrid, ActionFreezeColumns, "freeze"))
	if context.CanFetchMore {
		keys.add(model.buildHint(
			capabilities, cfg.ScopeGlobal, ActionNextPage, "more rows"))
	}
	if context.CanCountRows {
		keys.add(model.buildHint(
			capabilities, cfg.ScopeGrid, ActionCountRows, "count rows"))
	}
	if context.Staged > 0 {
		keys.add(model.buildHint(capabilities, cfg.ScopeGrid, ActionReviewChanges,
			"review "+strconv.Itoa(context.Staged)))
	}

	return closeBar(keys.build())
}

// hintSeparator stands between two hints of the bar.
const hintSeparator = " · "

// MeasureHints returns how many cells the hints take.
func MeasureHints(hints []Hint) int {
	if len(hints) == 0 {
		return 0
	}
	written := make([]string, 0, len(hints))
	for _, hint := range hints {
		written = append(written, hint.Key+" "+hint.Label)
	}
	return present.MeasureText(strings.Join(written, hintSeparator))
}

// cutMark shows that the bar dropped keys that did not fit.
var cutMark = Hint{Key: "…"}

// FitHints drops hints from the end until they fit beside the status message.
func FitHints(hints []Hint, available int) []Hint {
	if available <= 0 {
		return nil
	}
	if MeasureHints(hints) <= available {
		return hints
	}

	room := available - MeasureHints([]Hint{cutMark}) - present.MeasureText(hintSeparator)
	for count := len(hints) - 1; count > 0; count-- {
		if MeasureHints(hints[:count]) <= room {
			return append(append([]Hint{}, hints[:count]...), cutMark)
		}
	}
	return nil
}
