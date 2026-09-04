package ui

import (
	"context"
	"slices"
	"strconv"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	uv "github.com/charmbracelet/ultraviolet"

	"github.com/turanmahmudov/masume/internal/app"
	"github.com/turanmahmudov/masume/internal/cfg"
	"github.com/turanmahmudov/masume/internal/core"
	"github.com/turanmahmudov/masume/internal/db"
	"github.com/turanmahmudov/masume/internal/present"
	"github.com/turanmahmudov/masume/internal/query/result"
	"github.com/turanmahmudov/masume/internal/query/statement"
)

// readWorkspaceKey returns what one press does in the workspace. An overlay owns the keyboard
// while it is open, and a prompt owns it while it holds the caret.
func (model *Model) readWorkspaceKey(key tea.Key) (next tea.Model, command tea.Cmd) {
	connection := model.Active()
	if connection == nil {
		model.screen = ScreenPickingProfile
		return model, nil
	}
	// Every press that changes the tabs writes them into the history file, so a
	// connect opens what was left. The write joins whatever the press itself asked for.
	before := connection.DescribeTabs()
	defer func() {
		if connection.DescribeTabs() != before {
			command = tea.Batch(command, model.saveWorkspace(connection))
		}
	}()

	if connection.Overlay.IsOpen() {
		return model.readOverlayKey(connection, key)
	}
	if connection.Tree.Filtering {
		return model.readTreeFilterKey(connection, key)
	}

	tab := connection.Active()
	// Escape belongs to no action. It closes the list over the statement first, and with
	// nothing open it lets a selection go.
	if key.Code == tea.KeyEscape {
		model.keymap.Reset()
		if tab.Completion.IsListing() {
			tab.Completion.Dismiss()
			return model, nil
		}
		tab.Editor.ClearSelection()
		model.selection = screenSelection{}
		return model, nil
	}

	// The editor returns a key without a modifier while it holds the caret, so a letter
	// types rather than running an action.
	typesInEditor := tab.Focus == app.PaneEditor && tab.EditorVisible()

	// The editor is a scope of its own, so its keys are bound, checked for conflicts and
	// written into the help like the keys of every other pane. The workspace is read first,
	// so a chord the user gives the workspace still wins.
	scopes := []cfg.KeyScope{cfg.ScopeGlobal}
	if typesInEditor {
		scopes = append(scopes, cfg.ScopeEditor)
	} else {
		switch tab.Focus {
		case app.PaneSidebar:
			scopes = append(scopes, cfg.ScopeTree)
		case app.PaneResult:
			switch tab.ActiveView(connection.Session) {
			case app.ViewPlan:
				scopes = append(scopes, cfg.ScopePlan)
			case app.ViewTree:
				scopes = append(scopes, cfg.ScopeDocument)
			default:
				scopes = append(scopes, cfg.ScopeGrid)
			}
		}
	}

	// A key with no modifier that types a character belongs to the editor while the
	// caret is in it, so a full stop writes a full stop and does not step a statement.
	if typesInEditor && typesCharacter(key) {
		return model.readEditorKey(connection, tab, key)
	}
	// The completion list owns the keys that work it, so Tab takes a candidate rather
	// than stepping to the next pane.
	if typesInEditor && ownsCompletionKey(tab, key) {
		return model.readEditorKey(connection, tab, key)
	}
	// A view of the result that is not the grid holds no cursor: it scrolls with the keys
	// of a list, which is what its hint offers.
	// The tree holds a cursor of its own, so it takes the keys that move one before the
	// rows of a list are scrolled under it.
	if tab.Focus == app.PaneResult && !typesInEditor &&
		tab.ActiveView(connection.Session) == app.ViewTree {
		if match, matched := model.keymap.Match(key, cfg.ScopeDocument); matched {
			return model.runDocumentTreeAction(connection, tab, match)
		}
	}
	if tab.Focus == app.PaneResult && !typesInEditor &&
		tab.ActiveView(connection.Session) != app.ViewData {
		if match, matched := model.keymap.Match(key, cfg.ScopeList); matched &&
			scrollDetailView(tab, match) {
			return model, nil
		}
	}

	match, matched := model.keymap.Match(key, scopes...)
	if matched {
		next, command := model.runAction(connection, tab, match)
		// A restored tab reads what it describes the first time it is shown.
		if _, read := model.readWhenShown(connection); read != nil {
			return next, tea.Batch(command, read)
		}
		return next, command
	}
	if match.Waiting {
		return model, nil
	}

	// A key the registry does not bind reaches the pane that holds the caret.
	if typesInEditor {
		return model.readEditorKey(connection, tab, key)
	}
	return model, nil
}

// ownsCompletionKey is true for a key the completion list returns while it stands over
// the statement. A key carrying Shift grows the selection of the statement and one carrying
// Control or Alt runs an action, so the list leaves those to the editor.
func ownsCompletionKey(tab *app.Tab, key tea.Key) bool {
	if tab == nil || !tab.Completion.IsListing() || holdsModifier(key) {
		return false
	}
	switch key.Code {
	case tea.KeyEscape, tea.KeyTab, tea.KeyUp, tea.KeyDown,
		tea.KeyLeft, tea.KeyRight, tea.KeyHome, tea.KeyEnd:
		return true
	}
	return false
}

// typesCharacter is true for a press that writes one character: no modifier but Shift,
// and a key that is a character rather than a named one.
func typesCharacter(key tea.Key) bool {
	if key.Mod.Contains(uv.ModCtrl) || key.Mod.Contains(uv.ModAlt) ||
		key.Mod.Contains(uv.ModMeta) || key.Mod.Contains(uv.ModSuper) ||
		key.Mod.Contains(uv.ModHyper) {
		return false
	}
	return key.Text != "" && len([]rune(key.Text)) == 1
}

// holdsModifier is true for a press that carries a modifier which changes what the key means.
// A terminal reports the locks it has on as modifiers too, and Caps Lock or Num Lock changes
// nothing about an arrow, so a key is not read as modified for those.
func holdsModifier(key tea.Key) bool {
	return key.Mod.Contains(uv.ModShift) || key.Mod.Contains(uv.ModCtrl) ||
		key.Mod.Contains(uv.ModAlt) || key.Mod.Contains(uv.ModMeta) ||
		key.Mod.Contains(uv.ModSuper) || key.Mod.Contains(uv.ModHyper)
}

// runAction returns what one action does. The action was already matched in a scope, so this
// only carries it out.
func (model *Model) runAction(
	connection *app.Connection, tab *app.Tab, match Match,
) (tea.Model, tea.Cmd) {
	// The keys this server cannot do stay unbound, instead of reporting a refusal.
	if !AnswersFor(
		connection.Session.Capabilities(),
		FindActionCapability(match.Scope, match.Action),
	) {
		return model, nil
	}

	switch match.Scope {
	case cfg.ScopeTree:
		return model.runTreeAction(connection, match)
	case cfg.ScopeGrid:
		return model.runGridAction(connection, tab, match)
	case cfg.ScopePlan:
		return model.runPlanAction(connection, tab, match)
	case cfg.ScopeDocument:
		return model.runDocumentTreeAction(connection, tab, match)
	case cfg.ScopeEditor:
		return model.runEditorAction(connection, tab, match)
	}
	return model.runGlobalAction(connection, tab, match)
}

// runGlobalAction returns the actions the workspace handles wherever the focus is.
func (model *Model) runGlobalAction(
	connection *app.Connection, tab *app.Tab, match Match,
) (tea.Model, tea.Cmd) {
	id := model.ActiveID()
	// A hidden result is shown again before something writes into it.
	if AnswersInResult(match.Scope, match.Action) {
		connection.ResultVisible = true
	}

	switch match.Action {
	case ActionShowHelp:
		connection.Overlay = app.Overlay{
			Kind: app.OverlayHelp, Draft: app.NewEditorBuffer("", 0),
		}
	case ActionShowPalette:
		connection.Overlay = app.Overlay{
			Kind: app.OverlayPalette, Palette: model.buildPaletteActions(connection),
			Draft: app.NewEditorBuffer("", 0),
		}
	case ActionShowHistory:
		return model, readHistory(id, model.log, connection.Profile().Name, historyLimit)
	case ActionShowSaved:
		return model, readSaved(id, model.log, connection.Profile().Name)

	case ActionFocusNextPane:
		model.stepPane(connection, tab, 1)
	case ActionFocusPreviousPane:
		model.stepPane(connection, tab, -1)

	case ActionToggleSidebar:
		connection.SidebarVisible = !connection.SidebarVisible
		if !connection.SidebarVisible && tab.Focus == app.PaneSidebar {
			// Focus leaves the tree with it.
			tab.Focus = app.PaneEditor
			if !tab.EditorVisible() {
				tab.Focus = app.PaneResult
			}
		}
	case ActionToggleResult:
		connection.ResultVisible = !connection.ResultVisible
		if !connection.ResultVisible && tab.Focus == app.PaneResult {
			// Focus leaves the result with it. A table tab has no editor to take it, and
			// a hidden sidebar cannot take it either.
			switch {
			case tab.EditorVisible():
				tab.Focus = app.PaneEditor
			case connection.SidebarVisible:
				tab.Focus = app.PaneSidebar
			default:
				// Nothing is left to focus, so the result stays on screen.
				connection.ResultVisible = true
			}
		}

	case ActionNewQueryTab:
		// A new query tab opens with the caret in the editor.
		connection.OpenQueryTab("").Focus = app.PaneEditor
	case ActionCloseTab:
		return model.requestCloseTab(connection)
	case ActionReopenTab:
		if !connection.ReopenTab() {
			connection.Show("no tab was closed lately")
		}
	case ActionPreviousTab:
		connection.StepTab(-1)
	case ActionNextTab:
		connection.StepTab(1)
	case ActionActivateTab:
		if match.Digit > 0 {
			connection.ActivateTab(match.Digit - 1)
		}
	case ActionNameTab:
		return model.startNaming(connection, tab)

	case ActionPreviousConnection:
		model.connections.step(-1)
	case ActionNextConnection:
		model.connections.step(1)
	case ActionOpenPicker:
		model.screen = ScreenPickingProfile
		model.picker.problem = ""
	case ActionCloseConnection:
		return model.requestCloseConnection(connection)

	case ActionRunAtCursor:
		return model.runStatementAtCursor(connection, tab)
	case ActionRunBatch:
		return model.runWholeBuffer(connection, tab)
	case ActionExplain:
		return model.explain(connection, tab, explainOnly)
	case ActionExplainAnalyze:
		return model.explain(connection, tab, explainAnalyze)
	case ActionNextPage:
		return model.fetchMore(connection, tab)
	case ActionRefreshObjects:
		connection.Catalog.Loading = true
		return model, readCatalog(id, connection.Session, announceCatalogRead)

	case ActionRevealSQL:
		return model.revealSQL(connection, tab)
	case ActionSelectView:
		if match.Digit > 0 {
			return model.selectViewAt(connection, tab, match.Digit-1)
		}
	case ActionPreviousView:
		return model.stepView(connection, tab, -1)
	case ActionNextView:
		return model.stepView(connection, tab, 1)
	case ActionPreviousStatement:
		tab.Results.SelectNextResult(-1)
		return model.showSelectedResult(connection, tab)
	case ActionNextStatement:
		tab.Results.SelectNextResult(1)
		return model.showSelectedResult(connection, tab)

	case ActionExportCSV:
		return model.openExport(connection, tab, result.ExportCSV)
	case ActionExportJSON:
		return model.openExport(connection, tab, result.ExportJSON)

	case ActionBeginTransaction, ActionCommitTransaction, ActionRollbackTransaction:
		return model.runTransaction(connection, match.Action)
	case ActionToggleAutocommit:
		connection.Autocommit = !connection.Autocommit
		if connection.Autocommit {
			connection.Show("autocommit is on")
		} else {
			connection.Show("autocommit is off: a write needs a commit")
		}

	case ActionCancelQuery:
		return model.cancelQuery(connection)

	case ActionSaveQuery:
		if !tab.EditorVisible() {
			return model, nil
		}
		connection.Overlay = app.Overlay{
			Kind: app.OverlayPrompt, Prompt: app.PromptSaveName, Title: "save as",
			Draft: app.NewEditorBuffer("", 0),
		}
	case ActionShowActivity:
		return model, readActivity(id, connection.Session, readAsked)
	case ActionUndoWrite:
		return model.undoLastWrite(connection)
	case ActionShowThemes:
		connection.Overlay = app.Overlay{
			// The theme the picker opened on, so a walk that is cancelled goes back
			// to it and the row it is on carries the mark.
			Kind: app.OverlayThemePicker, List: app.ListState{Cursor: model.themeCursor()},
			Body: model.styles.Theme.Name, Draft: app.NewEditorBuffer("", 0),
		}
	case ActionFocusSidebar:
		return model.focusPane(connection, tab, app.PaneSidebar)
	case ActionFocusEditor:
		return model.focusPane(connection, tab, app.PaneEditor)
	case ActionFocusResult:
		return model.focusPane(connection, tab, app.PaneResult)

	case ActionShowAiChat, ActionSendToAi, ActionAiFixError:
		return model.runAiAction(connection, tab, match)
	}
	return model, nil
}

// stepPane moves the caret to the next pane that is drawn.
func (model *Model) stepPane(connection *app.Connection, tab *app.Tab, step int) {
	order := []app.Pane{}
	if connection.SidebarVisible {
		order = append(order, app.PaneSidebar)
	}
	if tab.EditorVisible() {
		order = append(order, app.PaneEditor)
	}
	if connection.ResultVisible {
		order = append(order, app.PaneResult)
	}
	if len(order) == 0 {
		return
	}

	at := 0
	for index, pane := range order {
		if pane == tab.Focus {
			at = index
			break
		}
	}
	tab.Focus = order[wrap(at+step, len(order))]
}

// startNaming opens the prompt that names a query tab. A table tab is named by its relation
// and cannot be renamed.
func (model *Model) startNaming(
	connection *app.Connection, tab *app.Tab,
) (tea.Model, tea.Cmd) {
	switch tab.Kind {
	case app.TabTable:
		connection.Show("a table tab is named by its table")
		return model, nil
	case app.TabObject:
		connection.Show("this tab is named by the " + string(tab.Object.Kind) + " it shows")
		return model, nil
	}

	named := statement.FindQueryName(tab.Editor.Text)
	connection.Overlay = app.Overlay{
		Kind: app.OverlayPrompt, Prompt: app.PromptTabName, Title: "name",
		Hint:  "written as a comment on the first line of the query",
		Draft: app.NewEditorBuffer(named, len(named)),
	}
	return model, nil
}

// buildLastTabNotice writes what stays and what closes the connection instead, because the
// last tab of a connection is never closed.
func (model *Model) buildLastTabNotice() string {
	closing := model.registry.FormatActionChordCompact(
		cfg.ScopeGlobal, ActionCloseConnection)
	if closing == "" {
		return "the last tab stays open"
	}
	return "the last tab stays · " + closing + " closes the connection"
}

// detailLinesFloor is a row past the end of any view, so a key that asks for the last row
// lands on it. The draw holds the offset to the rows it has.
const detailLinesFloor = 1 << 20

// scrollDetailView moves the rows a view of the result shows, for a view that is not the
// grid. The draw holds the offset to the rows there are, so a step past the end lands on the
// last of them. It reports whether the action belonged to the view.
func scrollDetailView(tab *app.Tab, match Match) bool {
	switch match.Action {
	case ActionCursorUp:
		tab.DetailOffset--
	case ActionCursorDown:
		tab.DetailOffset++
	case ActionCursorPageUp:
		tab.DetailOffset -= listPage
	case ActionCursorPageDown:
		tab.DetailOffset += listPage
	case ActionCursorFirstRow:
		tab.DetailOffset = 0
	case ActionCursorLastRow:
		tab.DetailOffset = detailLinesFloor
	default:
		return false
	}
	if tab.DetailOffset < 0 {
		tab.DetailOffset = 0
	}
	return true
}

// requestCloseTab asks before a tab with staged work is closed.
func (model *Model) requestCloseTab(connection *app.Connection) (tea.Model, tea.Cmd) {
	if len(connection.Tabs) <= 1 {
		connection.Show(model.buildLastTabNotice())
		return model, nil
	}
	tab := connection.Active()
	staged := core.CountChanges(tab.Pending)
	if staged == 0 {
		connection.CloseTab(connection.ActiveIndex)
		return model, nil
	}

	// Closing a tab drops its staged work, so the question names the count and offers to
	// write the work first.
	one := "them"
	if staged == 1 {
		one = "it"
	}
	index := connection.ActiveIndex
	connection.Overlay = app.Overlay{
		Kind:  app.OverlayChoice,
		Title: " close tab ",
		Body:  "This tab holds " + present.DescribeStagedChanges(staged) + ".",
		Choices: []app.Choice{
			{Key: "r",
				ID: "apply", Label: "run and close",
				Detail: "applies " + one + ", then closes the tab"},
			{Key: "d",
				ID: "discard", Label: "discard and close",
				Detail: "throws " + one + " away", Destructive: true},
			{Key: "c",
				ID: "cancel", Label: "keep the tab", Detail: "changes nothing"},
		},
		Answers: app.OverlayAnswers{ID: func(chosen string) app.AnswerCommand {
			return carryAnswer(model.closeTabAnswer(connection, tab, index, chosen))
		}},
	}
	return model, nil
}

// closeTabAnswer carries out the answer to the question a tab with staged work asks before it
// closes, and returns the work the answer started.
func (model *Model) closeTabAnswer(
	connection *app.Connection, tab *app.Tab, index int, chosen string,
) tea.Cmd {
	switch chosen {
	case "discard":
		tab.DiscardChanges()
		connection.CloseTab(index)
	case "apply":
		// A write the server refused leaves the work staged, so the tab stays open.
		tab.ClosingAfterApply = true
		_, command := model.applyStagedChanges(connection, tab)
		return command
	}
	return nil
}

// requestCloseConnection closes the connection with every tab on it, and asks first when
// there is more than one.
func (model *Model) requestCloseConnection(connection *app.Connection) (tea.Model, tea.Cmd) {
	staged := 0
	for _, tab := range connection.Tabs {
		staged += core.CountChanges(tab.Pending)
	}
	if len(connection.Tabs) <= 1 && staged == 0 {
		return model.closeConnection()
	}

	body := strconv.Itoa(len(connection.Tabs)) + " tabs are open on " +
		connection.Profile().Name + "."
	if len(connection.Tabs) == 1 {
		body = "One tab is open on " + connection.Profile().Name + "."
	}
	// Closing drops the staged work of every tab, so the question names it.
	if staged > 0 {
		body += " They hold " + present.DescribeStagedChanges(staged) + "."
		if len(connection.Tabs) == 1 {
			body = strings.Replace(body, " They hold ", " It holds ", 1)
		}
	}
	connection.Overlay = app.Overlay{
		Kind:  app.OverlayConfirm,
		Title: " close connection ",
		Body:  body + " Close the connection and all of them?",
		Answers: app.OverlayAnswers{Answer: func(confirmed bool) app.AnswerCommand {
			if !confirmed {
				return nil
			}
			// Through the same path as an unasked close, so the session and the command
			// that opened the port are ended and not left behind.
			_, command := model.closeConnection()
			return carryAnswer(command)
		}},
	}
	return model, nil
}

// closeConnection ends the connection on screen and moves to the one beside it.
func (model *Model) closeConnection() (tea.Model, tea.Cmd) {
	connection := model.Active()
	if connection == nil {
		return model, nil
	}
	session, preConnect := connection.Session, connection.PreConnect
	model.closeActiveConnection()
	return model, closeSession(session, preConnect)
}

// closeActiveConnection takes the connection out of the list, and opens the picker where
// none is left.
func (model *Model) closeActiveConnection() {
	closed, id, held := model.connections.closeActive()
	if !held {
		return
	}
	model.runs.stopConnection(id)
	// A chat still asking the model holds this session and writes into a channel nobody
	// reads any more. Its goroutine would fill that channel and then wait for ever.
	closed.Chat.Stopped()
	if model.connections.count() == 0 {
		model.screen = ScreenPickingProfile
	}
}

// runTransaction opens, commits or rolls back the transaction of the user.
func (model *Model) runTransaction(
	connection *app.Connection, action ActionID,
) (tea.Model, tea.Cmd) {
	session := connection.Session
	id := model.ActiveID()
	return model, func() tea.Msg {
		ctx := context.Background()
		var err error
		switch action {
		case ActionBeginTransaction:
			err = session.BeginTransaction(ctx)
		case ActionCommitTransaction:
			err = session.CommitTransaction(ctx)
		default:
			err = session.RollbackTransaction(ctx)
		}
		return transactionRanMsg{
			ConnectionID: id, Action: action, Problem: db.DescribeError(err),
		}
	}
}

// cancelQuery asks the server to stop the statement that holds the connection, and stops an
// export that streams at the same time.
func (model *Model) cancelQuery(connection *app.Connection) (tea.Model, tea.Cmd) {
	connection.StopExport()
	session := connection.Session
	id := model.ActiveID()
	return model, func() tea.Msg {
		stopped, err := session.CancelRunningQuery(context.Background())
		return cancelledMsg{
			ConnectionID: id, Stopped: stopped, Problem: db.DescribeError(err),
		}
	}
}

// revealSQL writes the sort and the filter into the buffer and shows the editor, so a rewrite
// the grid made becomes text that can be copied, run and taken further.
func (model *Model) revealSQL(
	connection *app.Connection, tab *app.Tab,
) (tea.Model, tea.Cmd) {
	// A tab with no buffer of its own opens its read as a query tab.
	if !tab.EditorVisible() {
		opened := connection.OpenQueryTab(tab.EffectiveSQL(connection.Session))
		opened.Focus = app.PaneEditor
		connection.Show("this read is a query now")
		return model, nil
	}

	rewritten := tab.HasRewrite()
	tab.Editor.SetText(tab.ComposeStatementRead(
		connection.Session, db.BoundText{Text: tab.Editor.Text}).Display)
	tab.Sort, tab.Filter = nil, nil
	tab.Focus = app.PaneEditor
	if rewritten {
		connection.Show("the rewrite is in your query now")
		return model, nil
	}
	connection.Show("editing the query of this tab")
	return model, nil
}

// readEditorKey returns a press the registry does not bind while the editor holds the caret:
// the keys of the completion list, and the character to write. The caret comes back into
// view after every press, and the status bar is told what stands selected.
func (model *Model) readEditorKey(
	connection *app.Connection, tab *app.Tab, key tea.Key,
) (tea.Model, tea.Cmd) {
	next, command := model.runEditorKey(connection, tab, key)
	tab.EditorRolled = false
	return next, command
}

// runEditorKey returns what one press writes into the statement.
func (model *Model) runEditorKey(
	connection *app.Connection, tab *app.Tab, key tea.Key,
) (tea.Model, tea.Cmd) {
	list := &tab.Completion

	// The list takes a key before the editor does, so it never stands over a word the
	// caret left. Enter never takes a candidate: the editor is for writing, and Tab is
	// the only key that takes one.
	if list.IsListing() {
		switch key.Code {
		case tea.KeyEscape:
			list.Dismiss()
			return model, nil
		case tea.KeyTab:
			model.acceptCompletion(connection, tab)
			return model, nil
		case tea.KeyUp, tea.KeyDown:
			step := -1
			if key.Code == tea.KeyDown {
				step = 1
			}
			list.Step(step)
			return model, nil
		case tea.KeyLeft, tea.KeyRight, tea.KeyHome, tea.KeyEnd:
			list.Close()
		}
	}
	if key.Mod.Contains(uv.ModCtrl) || key.Mod.Contains(uv.ModAlt) {
		return model, nil
	}
	if key.Text != "" {
		tab.Editor.Insert(key.Text)
		return model, model.reportEdit(connection, tab)
	}
	return model, nil
}

// runEditorAction returns an action of the statement being written. The caret comes back into
// view after every one, and the status bar is told what stands selected.
func (model *Model) runEditorAction(
	connection *app.Connection, tab *app.Tab, match Match,
) (tea.Model, tea.Cmd) {
	// A tab with no statement of its own has nothing for these to work on. The keys never
	// reach here without one, but the palette offers the format to any tab.
	if !tab.EditorVisible() {
		return model, nil
	}
	next, command := model.stepEditorAction(connection, tab, match)
	tab.EditorRolled = false
	return next, command
}

// stepEditorAction carries out one action of the editor. A move is bound to a chord and to
// its shifted twin, and the shifted one takes the selection along.
func (model *Model) stepEditorAction(
	connection *app.Connection, tab *app.Tab, match Match,
) (tea.Model, tea.Cmd) {
	buffer := tab.Editor
	selecting := match.Chord.Shift

	switch match.Action {
	case ActionCaretLeft:
		buffer.MoveCaret(-1, selecting)
	case ActionCaretRight:
		buffer.MoveCaret(1, selecting)
	case ActionCaretUp:
		buffer.MoveLine(-1, selecting)
	case ActionCaretDown:
		buffer.MoveLine(1, selecting)
	case ActionCaretWordLeft:
		buffer.MoveWord(-1, selecting)
	case ActionCaretWordRight:
		buffer.MoveWord(1, selecting)
	case ActionCaretLineStart:
		// Home reaches the first word of the line, and the first cell of it on a second
		// press.
		buffer.MoveToLineStart(selecting)
	case ActionCaretLineEnd:
		buffer.MoveToLineEnd(selecting)
	case ActionCaretTextStart:
		buffer.MoveToStart(selecting)
	case ActionCaretTextEnd:
		buffer.MoveToEnd(selecting)
	case ActionCaretPageUp:
		buffer.MovePage(-1, model.resolveEditorPageRows(), selecting)
	case ActionCaretPageDown:
		buffer.MovePage(1, model.resolveEditorPageRows(), selecting)
	case ActionSelectAll:
		buffer.SelectAll()

	case ActionOpenLine:
		// The new line opens under the one the caret is on, with the same indent, and one
		// step more where the line above left a bracket open.
		if tab.Completion.IsListing() {
			tab.Completion.Close()
		}
		if buffer.HasSelection() {
			buffer.Insert("\n")
			return model, model.reportEdit(connection, tab)
		}
		written, caret := OpenLineWithIndent(buffer.Text, buffer.Caret)
		buffer.SetTextWithCaret(written, caret)
		return model, model.reportEdit(connection, tab)
	case ActionDeleteBack:
		buffer.DeleteBackward()
		return model, model.reportEdit(connection, tab)
	case ActionDeleteForward:
		buffer.DeleteForward()
		return model, model.reportEdit(connection, tab)
	case ActionDeleteWordBack:
		buffer.DeleteWordBackward()
		return model, model.reportEdit(connection, tab)
	case ActionDeleteWordForward:
		buffer.DeleteWordForward()
		return model, model.reportEdit(connection, tab)

	case ActionUndoEdit:
		return model, model.undoEdit(connection, tab)
	case ActionRedoEdit:
		return model, model.redoEdit(connection, tab)
	case ActionPasteText:
		return model, model.pasteIntoEditor(connection, tab)
	case ActionFormatSQL:
		// One press writes the statement out again, one clause per line.
		buffer.SetText(connection.Session.Language().FormatStatement(buffer.Text))
		return model, model.reportEdit(connection, tab)

	case ActionCommentLines:
		mark := connection.Session.Language().LineComment()
		if mark == "" {
			connection.Show("this server has no comment mark")
			return model, nil
		}
		if !buffer.CommentLines(mark) {
			return model, nil
		}
		return model, model.reportEdit(connection, tab)
	case ActionIndentLines:
		if !buffer.IndentLines(indentWidth) {
			return model, nil
		}
		return model, model.reportEdit(connection, tab)
	case ActionOutdentLines:
		if !buffer.OutdentLines(indentWidth) {
			return model, nil
		}
		return model, model.reportEdit(connection, tab)

	case ActionFindInStatement:
		return model.startFinding(connection, tab, app.PromptFind)
	case ActionReplaceInStatement:
		return model.startFinding(connection, tab, app.PromptReplace)
	case ActionNextMatch:
		return model.stepMatch(connection, tab, 1)
	case ActionPreviousMatch:
		return model.stepMatch(connection, tab, -1)
	case ActionNextProblem:
		return model.stepProblem(connection, tab)
	}
	return model, nil
}

// startFinding opens the field that searches the statement, or the one that says what to
// write in place of what it found.
func (model *Model) startFinding(
	connection *app.Connection, tab *app.Tab, kind app.PromptKind,
) (tea.Model, tea.Cmd) {
	// Replace writes over the matches of the term the reader last looked for, so without one
	// there is nothing to replace. Saying so here saves them typing a replacement first.
	if kind == app.PromptReplace && tab.Find.Term == "" {
		connection.Show("look for something first, with the key that finds in the statement")
		return model, nil
	}

	written, title := tab.Find.Term, "find"
	switch {
	case kind == app.PromptReplace:
		// The title names the term, so the field says what it is about to overwrite.
		written = tab.Find.Replacement
		title = "replace " + tab.Find.Term + " with"
	case !tab.Editor.HasSelection():
	case !strings.Contains(tab.Editor.Selection(), "\n"):
		// A selection of one line is what the reader is looking at, so the field opens on it.
		written = tab.Editor.Selection()
	}
	connection.Overlay = app.Overlay{
		Kind: app.OverlayPrompt, Prompt: kind, Title: title,
		Draft: app.NewEditorBuffer(written, len(written)),
	}
	return model, nil
}

// turnFindIntoReplace takes the term the find field holds and opens the replace field on it.
// The term is kept as though it had been submitted, so the same matches are replaced.
func (model *Model) turnFindIntoReplace(
	connection *app.Connection, tab *app.Tab, overlay app.Overlay,
) (tea.Model, tea.Cmd) {
	term := overlay.Draft.Text
	if term == "" {
		connection.Show("type what to look for first")
		return model, nil
	}
	tab.Find.Term = term
	connection.Overlay = app.Overlay{}
	return model.startFinding(connection, tab, app.PromptReplace)
}

// stepMatch moves the caret to the match before or after it, and takes that match, so the
// next press steps on from there. The search wraps at the ends of the statement.
func (model *Model) stepMatch(
	connection *app.Connection, tab *app.Tab, step int,
) (tea.Model, tea.Cmd) {
	term := tab.Find.Term
	if term == "" {
		connection.Show("there is nothing to look for yet")
		return model, nil
	}
	found := tab.Editor.FindMatches(term)
	if len(found) == 0 {
		connection.Show("no match for " + term)
		return model, nil
	}

	at := resolveNextMatch(found, resolveSearchStart(tab.Editor), step)
	tab.Editor.SelectRange(found[at], found[at]+len(term))
	tab.EditorRolled = false
	connection.Show(strconv.Itoa(at+1) + " of " +
		present.FormatCountOf(int64(len(found)), "match", "matches"))
	return model, nil
}

// resolveSearchStart returns the offset a step of the search counts from: the start of the
// match that stands taken, or the caret where none does.
func resolveSearchStart(buffer *app.EditorBuffer) int {
	if buffer.HasSelection() {
		start, _ := buffer.SelectionRange()
		return start
	}
	return buffer.Caret
}

// resolveNextMatch returns which match a step reaches, wrapping at either end.
func resolveNextMatch(found []int, from, step int) int {
	if step < 0 {
		for at, f := range slices.Backward(found) {
			if f < from {
				return at
			}
		}
		return len(found) - 1
	}
	for at, start := range found {
		if start > from {
			return at
		}
	}
	return 0
}

// resolveFindTerm returns what the editor marks as found: the text of the field while the
// search is open, so a match shows while it is still being typed, and the text the tab keeps
// once the field is answered.
func resolveFindTerm(connection *app.Connection, tab *app.Tab) string {
	if prompt, asking := findPromptBar(connection, app.PromptFind); asking {
		return prompt.Draft.Text
	}
	return tab.Find.Term
}

// stepProblem moves the caret to the fault after the one it stands on, wrapping at the end.
// The fault row names this key, so it has to reach the fault it names.
func (model *Model) stepProblem(
	connection *app.Connection, tab *app.Tab,
) (tea.Model, tea.Cmd) {
	faults := model.findDiagnostics(connection, tab)
	if len(faults) == 0 {
		connection.Show("the statement reports no problem")
		return model, nil
	}
	at := 0
	for index, fault := range faults {
		if fault.Start > tab.Editor.Caret {
			at = index
			break
		}
	}
	// The caret is put where the fault begins rather than over the whole of it, because a
	// fault the server reports can run to the end of the statement.
	tab.Editor.PlaceCaret(faults[at].Start, false)
	tab.EditorRolled = false
	connection.Show(strconv.Itoa(at+1) + " of " +
		present.FormatCountOf(int64(len(faults)), "problem", "problems"))
	return model, nil
}

// resolveEditorPageRows returns how many lines one press of Page Up or Page Down moves the
// caret, which is the lines the pane shows.
func (model *Model) resolveEditorPageRows() int {
	if model.layout.editorTextRows > 1 {
		return model.layout.editorTextRows - 1
	}
	return 1
}

// undoEdit takes back the last edit of the buffer.
func (model *Model) undoEdit(connection *app.Connection, tab *app.Tab) tea.Cmd {
	if !tab.Editor.Undo() {
		connection.Show("there is nothing more to undo")
		return nil
	}
	return model.reportEdit(connection, tab)
}

// redoEdit writes the edit that was taken back again.
func (model *Model) redoEdit(connection *app.Connection, tab *app.Tab) tea.Cmd {
	if !tab.Editor.Redo() {
		connection.Show("there is nothing more to redo")
		return nil
	}
	return model.reportEdit(connection, tab)
}

// pasteIntoEditor writes what this client last copied at the caret. The terminal owns the
// system clipboard, and a paste it makes arrives as a paste of its own.
func (model *Model) pasteIntoEditor(connection *app.Connection, tab *app.Tab) tea.Cmd {
	if model.clipboard == "" {
		connection.Show("nothing was copied here yet, so use the paste key of your terminal")
		return nil
	}
	tab.Editor.Insert(model.clipboard)
	return model.reportEdit(connection, tab)
}

// reportEdit returns a change of the buffer: the word under the caret is new, so the
// list is built again for it.
func (model *Model) reportEdit(connection *app.Connection, tab *app.Tab) tea.Cmd {
	tab.Completion.Dismissed = false
	model.refreshCompletion(connection, tab)
	// An answer about another buffer would mark a line the user already corrected.
	if tab.Served.SQL != tab.Editor.Text {
		tab.Served = app.ServedDiagnostics{}
	}
	commands := []tea.Cmd{model.readNamedTableDetails(connection, tab)}
	if strings.TrimSpace(tab.Editor.Text) != "" &&
		len(model.findLocalDiagnostics(connection, tab)) == 0 {
		commands = append(commands, scheduleStatementCheck(
			model.ActiveID(), tab.ID, tab.Editor.Text))
	}
	return tea.Batch(commands...)
}

// readCheckDue sends the buffer to the server once the typing has stopped, and only where
// the scan found nothing.
func (model *Model) readCheckDue(due checkDueMsg) (tea.Model, tea.Cmd) {
	connection, tab, found := model.findConnectionTab(due.ConnectionID, due.TabID)
	if !found || tab.Editor.Text != due.SQL ||
		len(model.findLocalDiagnostics(connection, tab)) > 0 {
		return model, nil
	}
	return model, checkStatements(due.ConnectionID, due.TabID, connection.Session, due.SQL)
}

// readChecked keeps what the server said about the buffer it was asked about.
func (model *Model) readChecked(answered checkedMsg) (tea.Model, tea.Cmd) {
	_, tab, found := model.findConnectionTab(answered.ConnectionID, answered.TabID)
	if !found || tab.Editor.Text != answered.SQL {
		return model, nil
	}
	tab.Served = app.ServedDiagnostics{SQL: answered.SQL, Found: answered.Found}
	return model, nil
}

// readNamedTableDetails asks for the columns of every relation the statement names whose
// columns are not known yet. The read starts as soon as a relation is named, so a column
// after a dot can be completed without opening the tree first. A read that failed is not
// asked for again.
func (model *Model) readNamedTableDetails(
	connection *app.Connection, tab *app.Tab,
) tea.Cmd {
	if connection.Catalog.Details == nil {
		connection.Catalog.Details = map[string]present.TableDetailState{}
	}
	id := model.ActiveID()
	commands := []tea.Cmd{}
	for _, reference := range statement.FindTableReferences(
		tab.Editor.Text, connection.Session.Dialect().Syntax) {
		table, found := model.findTableByName(connection, reference.SelectSource)
		if !found {
			continue
		}
		tableID := present.BuildTableID(table)
		if _, read := connection.Catalog.Details[tableID]; read {
			continue
		}
		connection.Catalog.Details[tableID] = present.TableDetailState{
			Kind: present.DetailLoading,
		}
		commands = append(commands, readTableDetail(id, connection.Session, table))
	}
	if len(commands) == 0 {
		return nil
	}
	return tea.Batch(commands...)
}

// transactionRanMsg returns a transaction step.
type transactionRanMsg struct {
	ConnectionID int
	Action       ActionID
	Problem      string
}

// cancelledMsg returns an attempt at stopping the running statement.
type cancelledMsg struct {
	ConnectionID int
	Stopped      bool
	Problem      string
}

// runTreeAction returns the keys of the object tree.
func (model *Model) runTreeAction(
	connection *app.Connection, match Match,
) (tea.Model, tea.Cmd) {
	rows := model.treeRows(connection)
	count := len(rows)

	// A move of the cursor brings it back into view, whatever the wheel rolled to.
	switch match.Action {
	case ActionCursorUp, ActionCursorDown, ActionCursorPageUp, ActionCursorPageDown,
		ActionCursorFirstRow, ActionCursorLastRow:
		connection.Tree.Rolled = false
	}

	switch match.Action {
	case ActionCursorUp:
		connection.Tree.Cursor = wrap(connection.Tree.Cursor-1, count)
	case ActionCursorDown:
		connection.Tree.Cursor = wrap(connection.Tree.Cursor+1, count)
	case ActionCursorPageUp:
		connection.Tree.Cursor = clamp(connection.Tree.Cursor-listPage, count)
	case ActionCursorPageDown:
		connection.Tree.Cursor = clamp(connection.Tree.Cursor+listPage, count)
	case ActionCursorFirstRow:
		connection.Tree.Cursor = 0
	case ActionCursorLastRow:
		connection.Tree.Cursor = clamp(count-1, count)
	case ActionToggleSystemSchemas:
		connection.Tree.HideSystemSchemas = !connection.Tree.HideSystemSchemas
	case ActionFilterTree:
		row, found := model.treeRowAt(connection, rows)
		connection.Tree.Filtering = true
		connection.Tree.FilterScope = ""
		if found {
			connection.Tree.FilterScope = resolveFilterScope(row)
		}
		return model, nil
	}

	row, found := model.treeRowAt(connection, rows)
	if !found {
		return model, nil
	}

	switch match.Action {
	case ActionFoldRow:
		if row.Expandable && row.Expanded {
			delete(connection.Tree.Expanded, row.ID)
		}
	case ActionUnfoldRow:
		if row.Expandable && !row.Expanded {
			return model.toggleFold(connection, row)
		}
	case ActionOpenNode:
		return model.openTreeNode(connection, row)
	case ActionOpenInNewTab:
		if row.Node.Kind == present.NodeTable {
			preview := connection.Session.Composer().ComposeRelationRead(
				row.Node.Table, core.ReadRewrite{}).Display
			tab := connection.OpenTableInNewTab(row.Node.Table, preview)
			return model.runTabRead(connection, tab)
		}
	case ActionDescribeTable:
		return model.describeTable(connection, row)
	case ActionToggleFavourite:
		favourite, marks := present.FindFavouriteOf(row.Node)
		if !marks {
			return model, nil
		}
		connection.Marks.ToggleFavourite(favourite)
		return model, keepMarks(
			model.ActiveID(), model.log, connection.Profile().Name, favourite)
	case ActionObjectMenu:
		actions := app.BuildObjectActions(row.Node, connection.Session.Capabilities())
		if len(actions) == 0 {
			return model, nil
		}
		connection.Overlay = app.Overlay{
			Kind: app.OverlayObjectMenu, Title: app.BuildObjectTitle(row.Node),
			Draft:   app.NewEditorBuffer("", 0),
			Actions: actions,
		}
	}
	return model, nil
}

// resolveFilterScope returns what a filter opened on this row searches. On a schema row it
// reads every schema; inside a schema it reads that schema alone.
func resolveFilterScope(row present.TreeRow) string {
	switch row.Node.Kind {
	case present.NodeTable, present.NodeColumn:
		return core.BuildSchemaID(row.Node.Table.Schema)
	case present.NodeCategory:
		return core.BuildSchemaID(row.Node.Schema)
	case present.NodeObject:
		return core.BuildSchemaID(row.Node.Object.Schema)
	}
	return ""
}

// toggleFold opens a fold, and asks for what it needs to draw.
func (model *Model) toggleFold(
	connection *app.Connection, row present.TreeRow,
) (tea.Model, tea.Cmd) {
	if connection.Tree.Expanded[row.ID] {
		delete(connection.Tree.Expanded, row.ID)
		return model, nil
	}
	connection.Tree.Expanded[row.ID] = true

	id := model.ActiveID()
	switch row.Node.Kind {
	case present.NodeTable:
		// Asking again retries a read that failed.
		return model, readTableDetail(id, connection.Session, row.Node.Table)
	case present.NodeSchema:
		connection.Marks.VisitSchema(row.Node.Schema, time.Now())
		return model, keepVisit(
			model.ActiveID(), model.log, connection.Profile().Name, row.Node.Schema)
	}
	return model, nil
}

// openTreeNode returns what Enter does on the row under the cursor.
func (model *Model) openTreeNode(
	connection *app.Connection, row present.TreeRow,
) (tea.Model, tea.Cmd) {
	switch row.Node.Kind {
	case present.NodeTable:
		preview := connection.Session.Composer().ComposeRelationRead(
			row.Node.Table, core.ReadRewrite{}).Display
		tab := connection.OpenTable(row.Node.Table, preview)
		return model.runTabRead(connection, tab)
	case present.NodeObject:
		tab := connection.OpenObject(row.Node.Object)
		return model, readObjectDDL(
			model.ActiveID(), tab.ID, connection.Session, row.Node.Object)
	case present.NodeColumn:
		// A column inserts its name into the editor. A tab that shows a relation has
		// none, so one is opened for it.
		tab := connection.Active()
		if !tab.EditorVisible() {
			tab = connection.OpenQueryTab("")
		}
		tab.Focus = app.PaneEditor
		tab.Editor.Insert(row.Node.Table.Name + "." + row.Node.Column.Name)
		return model, nil
	}
	if row.Expandable {
		return model.toggleFold(connection, row)
	}
	if row.Node.Kind == present.NodeSchema {
		// A favourite schema is not the row its tables hang from, so opening it reveals
		// that row and puts the cursor on it.
		id := core.BuildSchemaID(row.Node.Schema)
		connection.Tree.Expanded[id] = true
		connection.Marks.VisitSchema(row.Node.Schema, time.Now())
		model.focusTreeRow(connection, id)
		return model, keepVisit(
			model.ActiveID(), model.log, connection.Profile().Name, row.Node.Schema)
	}
	return model, nil
}

// focusTreeRow puts the cursor of the tree on the row of that id, and brings it into view. A
// row a fold just opened is drawn in the next frame, and the rows are built here, so the
// cursor lands on it at once.
func (model *Model) focusTreeRow(connection *app.Connection, id string) {
	for at, row := range model.treeRows(connection) {
		if row.ID == id {
			connection.Tree.Cursor, connection.Tree.Rolled = at, false
			return
		}
	}
}

// describeTable opens the relation under the cursor on its columns view.
func (model *Model) describeTable(
	connection *app.Connection, row present.TreeRow,
) (tea.Model, tea.Cmd) {
	table := row.Node.Table
	if row.Node.Kind != present.NodeTable && row.Node.Kind != present.NodeColumn {
		return model, nil
	}
	preview := connection.Session.Composer().ComposeRelationRead(
		table, core.ReadRewrite{}).Display
	tab := connection.OpenTable(table, preview)
	tab.View = app.ViewColumns
	tab.Focus = app.PaneResult
	return model, readRelationView(
		model.ActiveID(), tab.ID, connection.Session, table, app.ViewColumns)
}

// readTreeFilterKey returns a press while the filter of the tree holds the keyboard.
func (model *Model) readTreeFilterKey(
	connection *app.Connection, key tea.Key,
) (tea.Model, tea.Cmd) {
	switch key.Code {
	case tea.KeyEscape:
		connection.Tree.Filter = ""
		connection.Tree.FilterScope = ""
		connection.Tree.Filtering = false
		return model, nil
	case tea.KeyEnter:
		connection.Tree.Filtering = false
		return model, nil
	case tea.KeyBackspace:
		if connection.Tree.Filter != "" {
			runes := []rune(connection.Tree.Filter)
			connection.Tree.Filter = string(runes[:len(runes)-1])
		}
		connection.Tree.Cursor = 0
		return model, nil
	}
	if key.Text != "" && !key.Mod.Contains(uv.ModCtrl) && !key.Mod.Contains(uv.ModAlt) {
		connection.Tree.Filter += key.Text
		connection.Tree.Cursor = 0
	}
	return model, nil
}

// treeRows returns the rows of the object tree of this connection.
func (model *Model) treeRows(connection *app.Connection) []present.TreeRow {
	return connection.BuildTree(time.Now()).Rows
}

// treeRowAt returns the row the cursor of the tree stands on.
func (model *Model) treeRowAt(
	connection *app.Connection, rows []present.TreeRow,
) (present.TreeRow, bool) {
	at := clamp(connection.Tree.Cursor, len(rows))
	if len(rows) == 0 {
		return present.TreeRow{}, false
	}
	return rows[at], true
}

// requestDiscardChanges throws the staged work away without closing the tab, and asks first.
func (model *Model) requestDiscardChanges(
	connection *app.Connection, tab *app.Tab,
) (tea.Model, tea.Cmd) {
	staged := core.CountChanges(tab.Pending)
	if staged == 0 {
		connection.Show("nothing is staged")
		return model, nil
	}
	connection.Overlay = app.Overlay{
		Kind:  app.OverlayConfirm,
		Title: " discard changes ",
		Body:  "Throw away " + present.DescribeStagedChanges(staged) + "?",
		Answers: app.OverlayAnswers{Answer: func(confirmed bool) app.AnswerCommand {
			if !confirmed {
				return nil
			}
			tab.DiscardChanges()
			connection.Show("discarded " + present.FormatStagedChanges(staged))
			return nil
		}},
	}
	return model, nil
}

// readPaste writes what the terminal pasted into the field that holds the caret: the
// statement of the editor, or the field of the connection form.
func (model *Model) readPaste(written string) (tea.Model, tea.Cmd) {
	if written == "" {
		return model, nil
	}
	if model.screen == ScreenEditingConnection {
		return model.pasteIntoForm(written)
	}
	connection := model.Active()
	if model.screen != ScreenWorking || connection == nil || connection.Overlay.IsOpen() {
		return model, nil
	}
	tab := connection.Active()
	if tab == nil || tab.Focus != app.PaneEditor || !tab.EditorVisible() {
		return model, nil
	}
	// A terminal ends the lines of a paste as its own source did, and the buffer holds one
	// break of its own.
	written = strings.ReplaceAll(strings.ReplaceAll(written, "\r\n", "\n"), "\r", "\n")
	tab.Editor.Insert(written)
	tab.EditorRolled = false
	return model, model.reportEdit(connection, tab)
}
