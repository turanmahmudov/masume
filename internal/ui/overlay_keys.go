package ui

import (
	"context"
	"strconv"
	"strings"

	tea "charm.land/bubbletea/v2"
	uv "github.com/charmbracelet/ultraviolet"

	"github.com/turanmahmudov/masume/internal/app"
	"github.com/turanmahmudov/masume/internal/cfg"
	"github.com/turanmahmudov/masume/internal/core"
	"github.com/turanmahmudov/masume/internal/db"
	"github.com/turanmahmudov/masume/internal/hist"
	"github.com/turanmahmudov/masume/internal/present"
	"github.com/turanmahmudov/masume/internal/query/build"
	"github.com/turanmahmudov/masume/internal/query/statement"
)

// readOverlayKey returns what one press does while an overlay owns the keyboard.
func (model *Model) readOverlayKey(
	connection *app.Connection, key tea.Key,
) (tea.Model, tea.Cmd) {
	overlay := &connection.Overlay
	tab := connection.Active()

	// The picker of an import reads every key of its own stage, so its own list keys
	// work as they do everywhere else it is used.
	if overlay.Kind == app.OverlayImport && overlay.Import.Stage == app.ImportPick &&
		key.Code != tea.KeyEscape {
		if held, command, taken := model.readPickerMessage(tea.KeyPressMsg(key)); taken {
			return held, command
		}
	}

	// Escape belongs to no action. It closes what is open, or steps back where the card
	// holds a stage to step back to.
	if key.Code == tea.KeyEscape {
		if leaveImportReview(overlay) {
			return model, nil
		}
		model.cancelOverlay(connection, overlay)
		return model, nil
	}

	// The card of one row steps to the row beside it, and the registry binds the sideways
	// arrows in the grid only, so the card takes them itself.
	if overlay.Kind == app.OverlayRowDetail &&
		(key.Code == tea.KeyLeft || key.Code == tea.KeyRight) {
		step := -1
		if key.Code == tea.KeyRight {
			step = 1
		}
		overlay.Window.Index = wrap(overlay.Window.Index+step, len(overlay.Window.Rows))
		// A row of its own starts at its first column.
		overlay.List.Offset = 0
		return model, nil
	}

	// A diagram scrolls sideways as well, and the registry binds the sideways arrows in
	// the grid only, so the card takes them itself.
	if overlay.Kind == app.OverlayDiagram &&
		(key.Code == tea.KeyLeft || key.Code == tea.KeyRight) {
		step := ActionCursorLeft
		if key.Code == tea.KeyRight {
			step = ActionCursorRight
		}
		scrollDiagram(overlay, Match{Action: step})
		return model, nil
	}

	// A card with several returns takes its own letters, before any binding: the letters
	// belong to the answers and not to the registry.
	if overlay.Kind == app.OverlayChoice {
		if chosen, found := findChoiceKey(overlay.Choices, key); found {
			answer := overlay.Answers.ID
			connection.Overlay = app.Overlay{}
			return model, model.runIDAnswer(answer, chosen)
		}
	}

	// The dialog scope binds one chord to more than one action, so the card on show is
	// asked first which of them it takes. A card that is a field takes no key of a list,
	// because the arrows belong to the field.
	scopes := []cfg.KeyScope{cfg.ScopeDialog}
	if takesListKeys(*overlay) {
		scopes = append(scopes, cfg.ScopeList)
	}
	match, matched := model.keymap.MatchFirst(key,
		FindDialogActions(string(overlay.Kind)), scopes...)
	if matched {
		if handled, held, command := model.runOverlayAction(
			connection, tab, overlay, match); handled {
			model.previewTheme(connection)
			return held, command
		}
	}

	// A field of the overlay takes what the registry did not bind.
	if overlay.Draft != nil {
		held, command := model.readOverlayField(connection, tab, overlay, key)
		model.previewTheme(connection)
		return held, command
	}
	return model, nil
}

// takesListKeys is true for a card whose rows the keys of a list move. A card that is a
// field takes those keys into the field instead: only a card that draws a list binds them
// at all.
func takesListKeys(overlay app.Overlay) bool {
	switch overlay.Kind {
	case app.OverlayCellEdit:
		// A cell picked from a list is a list; a cell written into is a field.
		return len(overlay.Cell.Choices) > 0
	case app.OverlayParameters, app.OverlayExport, app.OverlayImport, app.OverlayPrompt,
		app.OverlayChoice, app.OverlayMessage, app.OverlayConfirm, app.OverlayAiChat:
		return false
	}
	return true
}

// cancelOverlay closes what is open without an answer, and puts back what a preview changed.
func (model *Model) cancelOverlay(connection *app.Connection, overlay *app.Overlay) {
	// An import that writes now is ended with the card that started it.
	if overlay.Kind == app.OverlayImport {
		connection.StopImport()
	}
	if overlay.Kind == app.OverlayThemePicker && overlay.Body != "" &&
		overlay.Body != model.styles.Theme.Name {
		model.styles.ApplyThemeByName(overlay.Body)
	}
	model.answerNothing(overlay)
	connection.Overlay = app.Overlay{}
}

// previewTheme applies the theme the cursor of the picker stands on, so the app itself is
// the preview. A cancel puts the theme it opened on back.
func (model *Model) previewTheme(connection *app.Connection) {
	overlay := &connection.Overlay
	if overlay.Kind != app.OverlayThemePicker {
		return
	}
	choices := model.filterThemes(*overlay)
	if overlay.List.Cursor < 0 || overlay.List.Cursor >= len(choices) {
		return
	}
	if choices[overlay.List.Cursor].Name == model.styles.Theme.Name {
		return
	}
	model.styles.ApplyThemeByName(choices[overlay.List.Cursor].Name)
}

// answerNothing tells a question its caller that it was cancelled, so nothing is left waiting.
func (model *Model) answerNothing(overlay *app.Overlay) {
	if overlay.Answers.Answer != nil {
		model.runAnswer(overlay.Answers.Answer, false)
	}
	if overlay.Answers.ID != nil {
		model.runIDAnswer(overlay.Answers.ID, "")
	}
}

// answerParameters reads the values the card holds and hands them to what asked for them.
// The form is read as JSON, so a null stays a null and a number stays a number.
func (model *Model) answerParameters(
	connection *app.Connection, overlay *app.Overlay,
) tea.Cmd {
	values, err := statement.ReadParameterForm(overlay.Draft.Text)
	if err != nil {
		overlay.Notice = db.DescribeError(err)
		return nil
	}
	answer := overlay.Answers.Values
	connection.Overlay = app.Overlay{}
	if answer == nil {
		return nil
	}
	return runAnswerCommand(answer(values))
}

// findChoiceKey returns the id of the answer this press picks, and whether it picks one.
func findChoiceKey(choices []app.Choice, key tea.Key) (string, bool) {
	if key.Mod.Contains(uv.ModCtrl) || key.Mod.Contains(uv.ModAlt) {
		return "", false
	}
	for _, choice := range choices {
		if choice.Key != "" && key.Text == choice.Key {
			return choice.ID, true
		}
	}
	return "", false
}

// runIDAnswer calls what the answer of a card of several returns runs, and hands back the
// command it started.
func (model *Model) runIDAnswer(answer func(string) app.AnswerCommand, chosen string) tea.Cmd {
	if answer == nil {
		return nil
	}
	return runAnswerCommand(answer(chosen))
}

// runAnswer calls what the answer of a card runs, and hands back the command it started.
func (model *Model) runAnswer(answer func(bool) app.AnswerCommand, confirmed bool) tea.Cmd {
	if answer == nil {
		return nil
	}
	return runAnswerCommand(answer(confirmed))
}

// runAnswerCommand gives the draw loop the work an answer handed back.
func runAnswerCommand(command app.AnswerCommand) tea.Cmd {
	if command == nil {
		return nil
	}
	return func() tea.Msg { return command() }
}

// carryAnswer hands a command back through an answer, which names it without the draw loop.
func carryAnswer(command tea.Cmd) app.AnswerCommand {
	if command == nil {
		return nil
	}
	return func() any { return command() }
}

// overlayRowCount returns how many rows the list of an overlay holds.
func (model *Model) overlayRowCount(connection *app.Connection, overlay app.Overlay) int {
	switch overlay.Kind {
	case app.OverlayHistory:
		return len(model.filterHistory(overlay))
	case app.OverlaySaved:
		return len(model.filterSaved(overlay))
	case app.OverlayPalette:
		return len(model.filterPalette(overlay))
	case app.OverlayObjectMenu, app.OverlayCopyMenu, app.OverlayActionMenu:
		return len(model.filterMenu(overlay))
	case app.OverlayChoice:
		return len(overlay.Choices)
	case app.OverlayValueFilter:
		return len(overlay.Values)
	case app.OverlayActivity:
		return len(overlay.Sessions)
	case app.OverlayThemePicker:
		return len(model.filterThemes(overlay))
	case app.OverlayHelp:
		return model.countHelpRows(overlay)
	case app.OverlayAiChats:
		return len(model.filterConversations(overlay, connection.Chat))
	}
	return 0
}

// countHelpRows returns how many rows the help draws, headings and gaps included, which is
// how far it can scroll.
func (model *Model) countHelpRows(overlay app.Overlay) int {
	if term := model.readOverlayTerm(overlay); term != "" {
		return len(model.findHelpRows(term))
	}
	rows := 0
	for _, section := range model.listHelpSections() {
		rows += 2 + len(section.Entries)
	}
	return rows
}

// scrollDiagram moves a diagram by rows and by columns. It reports whether the action
// belonged to the diagram.
func scrollDiagram(overlay *app.Overlay, match Match) bool {
	widest := 0
	for _, line := range overlay.Lines {
		if measured := len([]rune(line)); measured > widest {
			widest = measured
		}
	}
	switch match.Action {
	case ActionCursorUp:
		overlay.List.Cursor = clamp(overlay.List.Cursor-1, len(overlay.Lines))
	case ActionCursorDown:
		overlay.List.Cursor = clamp(overlay.List.Cursor+1, len(overlay.Lines))
	case ActionCursorPageUp, ActionScrollBack:
		overlay.List.Cursor = clamp(overlay.List.Cursor-helpPageRows, len(overlay.Lines))
	case ActionCursorPageDown, ActionScrollForward:
		overlay.List.Cursor = clamp(overlay.List.Cursor+helpPageRows, len(overlay.Lines))
	case ActionCursorFirstRow:
		overlay.List.Cursor, overlay.List.Offset = 0, 0
	case ActionCursorLastRow:
		overlay.List.Cursor = clamp(len(overlay.Lines)-1, len(overlay.Lines))
	case ActionCursorLeft:
		overlay.List.Offset = clamp(overlay.List.Offset-diagramScrollStep, widest)
	case ActionCursorRight:
		overlay.List.Offset = clamp(overlay.List.Offset+diagramScrollStep, widest)
	default:
		return false
	}
	return true
}

// diagramScrollStep is how many columns one press moves a diagram.
const diagramScrollStep = 8

// helpPageRows is how many rows a page key moves the help.
const helpPageRows = 10

// scrollHelp moves the help by rows rather than by a cursor, because it highlights no row.
// It reports whether the action belonged to the help.
func (model *Model) scrollHelp(overlay *app.Overlay, match Match, count int) bool {
	switch match.Action {
	case ActionCursorUp:
		overlay.List.Cursor = clamp(overlay.List.Cursor-1, count)
	case ActionCursorDown:
		overlay.List.Cursor = clamp(overlay.List.Cursor+1, count)
	case ActionCursorPageUp, ActionScrollBack:
		overlay.List.Cursor = clamp(overlay.List.Cursor-helpPageRows, count)
	case ActionCursorPageDown, ActionScrollForward:
		overlay.List.Cursor = clamp(overlay.List.Cursor+helpPageRows, count)
	case ActionCursorFirstRow:
		overlay.List.Cursor = 0
	case ActionCursorLastRow:
		overlay.List.Cursor = clamp(count-1, count)
	default:
		return false
	}
	return true
}

// scrollCardLines moves the lines a card shows, for a card that scrolls without a cursor. The
// rows come from the frame drawn before, because only the draw knows how many rows the value
// takes at that width. It reports whether the action belonged to the card.
func (model *Model) scrollCardLines(overlay *app.Overlay, match Match) bool {
	switch match.Action {
	case ActionCursorUp:
		overlay.List.Offset--
	case ActionCursorDown:
		overlay.List.Offset++
	case ActionCursorPageUp, ActionScrollBack:
		overlay.List.Offset -= listPage
	case ActionCursorPageDown, ActionScrollForward:
		overlay.List.Offset += listPage
	case ActionCursorFirstRow:
		overlay.List.Offset = 0
	case ActionCursorLastRow:
		overlay.List.Offset = model.layout.cardLines
	default:
		return false
	}
	overlay.List.Offset = clampOffset(
		overlay.List.Offset, model.layout.cardBody, model.layout.cardLines)
	return true
}

// runOverlayAction returns an action while an overlay is open. It reports whether the action
// belonged to this overlay.
func (model *Model) runOverlayAction(
	connection *app.Connection, tab *app.Tab, overlay *app.Overlay, match Match,
) (bool, tea.Model, tea.Cmd) {
	count := model.overlayRowCount(connection, *overlay)

	// The help scrolls rather than moving a cursor, so it takes the list keys first.
	if overlay.Kind == app.OverlayHelp && model.scrollHelp(overlay, match, count) {
		return true, model, nil
	}
	// The viewer of a cell and the row it was read from hold no cursor either: what is
	// taller than the card scrolls inside it.
	if (overlay.Kind == app.OverlayCell || overlay.Kind == app.OverlayRowDetail) &&
		model.scrollCardLines(overlay, match) {
		return true, model, nil
	}
	// A diagram scrolls both ways, because a box is as wide as it is.
	if overlay.Kind == app.OverlayDiagram && scrollDiagram(overlay, match) {
		return true, model, nil
	}

	// The find field turns into the replace field, carrying the term with it, so finding
	// and replacing takes one key and the second half is offered where it is needed.
	if overlay.Prompt == app.PromptFind && match.Action == ActionReplaceInStatement {
		held, command := model.turnFindIntoReplace(connection, tab, *overlay)
		return true, held, command
	}

	// A cell that is picked returns the keys of a list itself: its cursor stops at the
	// first row and at the last one, and Enter saves what it stands on.
	if overlay.Kind == app.OverlayCellEdit && len(overlay.Cell.Choices) > 0 {
		if handled, held, command := model.runCellPickAction(
			connection, tab, overlay, match); handled {
			return handled, held, command
		}
	}

	// The panel of the chat returns the keys of a list itself, because its rows are a
	// conversation and not a list.
	if overlay.Kind == app.OverlayAiChat {
		return model.runChatAction(connection, tab, match)
	}

	// A move of the cursor brings it back into view, whatever the wheel rolled to.
	switch match.Action {
	case ActionCursorUp, ActionCursorDown, ActionCursorPageUp, ActionCursorPageDown,
		ActionCursorFirstRow, ActionCursorLastRow, ActionScrollBack, ActionScrollForward:
		overlay.List.Rolled = false
	}

	switch match.Action {
	case ActionClose:
		if leaveImportReview(overlay) {
			return true, model, nil
		}
		model.cancelOverlay(connection, overlay)
		return true, model, nil

	case ActionCursorUp:
		overlay.List.Cursor = wrap(overlay.List.Cursor-1, count)
		return true, model, nil
	case ActionCursorDown:
		overlay.List.Cursor = wrap(overlay.List.Cursor+1, count)
		return true, model, nil
	case ActionCursorPageUp, ActionScrollBack:
		overlay.List.Cursor = clamp(overlay.List.Cursor-listPage, count)
		overlay.List.Offset = clamp(overlay.List.Offset-listPage, count)
		return true, model, nil
	case ActionCursorPageDown, ActionScrollForward:
		overlay.List.Cursor = clamp(overlay.List.Cursor+listPage, count)
		overlay.List.Offset = clamp(overlay.List.Offset+listPage, count)
		return true, model, nil
	case ActionCursorFirstRow:
		overlay.List.Cursor, overlay.List.Offset = 0, 0
		return true, model, nil
	case ActionCursorLastRow:
		overlay.List.Cursor = clamp(count-1, count)
		return true, model, nil

	case ActionChooseRow:
		if overlay.Kind == app.OverlayAiChats {
			held, command := model.chooseConversation(connection)
			return true, held, command
		}
		held, command := model.chooseOverlayRow(connection, tab, overlay, chooseInSameTab)
		return true, held, command
	case ActionListSecondary:
		if overlay.Kind == app.OverlayAiChats {
			held, command := model.removeChosenConversation(connection)
			return true, held, command
		}
	case ActionOpenInNewTab:
		held, command := model.chooseOverlayRow(connection, tab, overlay, chooseInNewTab)
		return true, held, command
	}

	switch overlay.Kind {
	case app.OverlayConfirm, app.OverlayWritePlan:
		switch match.Action {
		case ActionAnswerYes:
			answer := overlay.Answers.Answer
			connection.Overlay = app.Overlay{}
			return true, model, model.runAnswer(answer, true)
		case ActionAnswerNo:
			model.answerNothing(overlay)
			connection.Overlay = app.Overlay{}
			return true, model, nil
		}

	case app.OverlayChanges:
		switch match.Action {
		case ActionApplyChanges:
			connection.Overlay = app.Overlay{}
			held, command := model.applyStagedChanges(connection, tab)
			return true, held, command
		case ActionDiscardChanges:
			tab.DiscardChanges()
			connection.Overlay = app.Overlay{}
			connection.Show("the staged changes were discarded")
			return true, model, nil
		}

	case app.OverlayCell:
		// The viewer only reads, so the key that selects all means "copy this value".
		// The card stays open and says what it did.
		if match.Action == ActionCopyValue {
			written := present.FormatForViewer(overlay.Cell.Value, overlay.Cell.Column.DataType)
			overlay.Notice = "copied to clipboard"
			return true, model, model.keepOnClipboard(written)
		}

	case app.OverlayCellEdit:
		switch match.Action {
		case ActionSaveCell:
			held, command := model.saveCell(connection, tab, *overlay)
			return true, held, command
		case ActionPrettifyJSON:
			if len(overlay.Cell.Choices) > 0 ||
				!present.IsJSONType(overlay.Cell.Column.DataType) {
				return true, model, nil
			}
			written, isJSON := present.PrettifyJSON(overlay.Draft.Text)
			if !isJSON {
				overlay.Notice = "not valid JSON"
				return true, model, nil
			}
			overlay.Draft.SetText(written)
			overlay.Notice = "prettified"
			return true, model, nil
		case ActionSetNull:
			held, command := model.stageCellValue(
				connection, tab, *overlay, core.CellValue{Kind: core.CellNull})
			return true, held, command
		case ActionSetEmpty:
			held, command := model.stageCellValue(
				connection, tab, *overlay, core.CellValue{Kind: core.CellEmpty})
			return true, held, command
		case ActionSetDefault:
			held, command := model.stageCellValue(
				connection, tab, *overlay, core.CellValue{Kind: core.CellDefault})
			return true, held, command
		}

	case app.OverlayParameters:
		switch match.Action {
		case ActionRunWithValues:
			return true, model, model.answerParameters(connection, overlay)
		case ActionPrettifyJSON:
			written, isJSON := present.PrettifyJSON(overlay.Draft.Text)
			if !isJSON {
				overlay.Notice = "not valid JSON"
				return true, model, nil
			}
			overlay.Draft.SetText(written)
			overlay.Notice = "prettified"
			return true, model, nil
		}

	case app.OverlayValueFilter:
		switch match.Action {
		case ActionToggleValue:
			if overlay.List.Cursor < len(overlay.Values) {
				value := overlay.Values[overlay.List.Cursor].Value
				if overlay.Kept[value] {
					delete(overlay.Kept, value)
				} else {
					overlay.Kept[value] = true
				}
			}
			return true, model, nil
		case ActionKeepAllValues:
			for _, value := range overlay.Values {
				overlay.Kept[value.Value] = true
			}
			return true, model, nil
		case ActionKeepOnlyValue:
			if overlay.List.Cursor < len(overlay.Values) {
				overlay.Kept = map[string]bool{overlay.Values[overlay.List.Cursor].Value: true}
			}
			return true, model, nil
		}

	case app.OverlayExport:
		if match.Action == ActionWriteExport {
			held, command := model.writeExport(connection, tab, *overlay)
			return true, held, command
		}

	case app.OverlaySaved:
		if match.Action == ActionListSecondary {
			held, command := model.deleteSavedQuery(connection, overlay)
			return true, held, command
		}

	case app.OverlayActivity:
		switch match.Action {
		case ActionFoldRow, ActionUnfoldRow:
			// Every panel folds together, because the cursor stands in the session list
			// and not on a panel, so there is no one panel the key could mean.
			for _, panel := range app.DashboardPanels {
				overlay.View.FoldPanel(panel, match.Action == ActionFoldRow)
			}
			return true, model, nil
		}
		if overlay.List.Cursor >= len(overlay.Sessions) {
			return false, model, nil
		}
		session := overlay.Sessions[overlay.List.Cursor]
		switch match.Action {
		case ActionStopSession:
			held, command := model.askStopBackend(connection, session, stopStatement)
			return true, held, command
		case ActionListSecondary:
			held, command := model.askStopBackend(connection, session, endSession)
			return true, held, command
		}
	}
	return false, model, nil
}

// The two places a row of a list opens in.
const (
	chooseInSameTab = false
	chooseInNewTab  = true
)

// chooseOverlayRow returns what the row under the cursor of a list opens.
func (model *Model) chooseOverlayRow(
	connection *app.Connection, tab *app.Tab, overlay *app.Overlay, inNewTab bool,
) (tea.Model, tea.Cmd) {
	switch overlay.Kind {
	case app.OverlayImport:
		return model.stepImport(connection, overlay)

	case app.OverlayHistory:
		entries := model.filterHistory(*overlay)
		if overlay.List.Cursor >= len(entries) {
			return model, nil
		}
		statement := entries[overlay.List.Cursor].SQL
		connection.Overlay = app.Overlay{}
		return model.loadSQL(connection, tab, statement, inNewTab)

	case app.OverlaySaved:
		queries := model.filterSaved(*overlay)
		if overlay.List.Cursor >= len(queries) {
			return model, nil
		}
		statement := queries[overlay.List.Cursor].SQL
		connection.Overlay = app.Overlay{}
		return model.loadSQL(connection, tab, statement, inNewTab)

	case app.OverlayPalette:
		actions := model.filterPalette(*overlay)
		if overlay.List.Cursor >= len(actions) {
			return model, nil
		}
		return model.runPaletteAction(connection, actions[overlay.List.Cursor].ID)

	case app.OverlayObjectMenu:
		actions := model.filterMenu(*overlay)
		if overlay.List.Cursor >= len(actions) {
			return model, nil
		}
		chosen := actions[overlay.List.Cursor]
		connection.Overlay = app.Overlay{}
		return model.runObjectAction(connection, tab, chosen)

	case app.OverlayCopyMenu:
		actions := model.filterMenu(*overlay)
		if overlay.List.Cursor >= len(actions) {
			return model, nil
		}
		chosen := actions[overlay.List.Cursor].ID
		connection.Overlay = app.Overlay{}
		return model.runCopy(connection, tab, chosen)

	case app.OverlayActionMenu:
		actions := model.filterMenu(*overlay)
		if overlay.List.Cursor >= len(actions) {
			return model, nil
		}
		chosen := actions[overlay.List.Cursor].ID
		connection.Overlay = app.Overlay{}
		action, known := FindActionID(chosen)
		if !known {
			return model, nil
		}
		scope := overlay.Scope
		if scope == "" {
			scope = cfg.ScopeGrid
		}
		if _, held := FindAction(scope, action); !held {
			scope = cfg.ScopeGlobal
		}
		return model.runAction(connection, tab, Match{Action: action, Scope: scope})

	case app.OverlayValueFilter:
		kept := overlay.Kept
		columnIndex := overlay.Cell.ColumnIndex
		available := len(overlay.Values)
		connection.Overlay = app.Overlay{}
		tab.Screen = present.ApplyValueFilter(tab.Screen, columnIndex, kept, available)
		tab.GridRow = 0
		return model, nil

	case app.OverlayThemePicker:
		choices := model.filterThemes(*overlay)
		if overlay.List.Cursor >= len(choices) {
			return model, nil
		}
		name := choices[overlay.List.Cursor].Name
		connection.Overlay = app.Overlay{}
		return model.keepTheme(connection, name)

	case app.OverlayActivity:
		if overlay.List.Cursor >= len(overlay.Sessions) {
			return model, nil
		}
		statement := overlay.Sessions[overlay.List.Cursor].Query
		if strings.TrimSpace(statement) == "" {
			connection.Show("that session is running no statement")
			return model, nil
		}
		connection.Overlay = app.Overlay{}
		return model.loadSQL(connection, tab, statement, inNewTab)

	case app.OverlayPrompt:
		return model.answerPrompt(connection, tab, *overlay)

	case app.OverlayConfirm:
		answer := overlay.Answers.Answer
		connection.Overlay = app.Overlay{}
		return model, model.runAnswer(answer, true)

	case app.OverlayCellEdit:
		return model.saveCell(connection, tab, *overlay)
	}
	return model, nil
}

// loadSQL writes a statement into this tab, or into a new one.
func (model *Model) loadSQL(
	connection *app.Connection, tab *app.Tab, statement string, inNewTab bool,
) (tea.Model, tea.Cmd) {
	if inNewTab || tab.Kind != app.TabQuery {
		opened := connection.OpenQueryTab(statement)
		opened.Focus = app.PaneEditor
		return model, nil
	}
	tab.Editor.SetText(statement)
	tab.Focus = app.PaneEditor
	return model, nil
}

// The two ways the server stops a session. Cancelling ends the statement and leaves the
// connection open; ending the session closes the connection with it.
const (
	stopStatement = false
	endSession    = true
)

type stoppedBackendMsg struct {
	ConnectionID int
	PID          int64
	Ended        bool
	Stopped      bool
	Problem      string
}

func stopBackend(id int, session db.ServerAdmin, pid int64, ends bool) tea.Cmd {
	return func() tea.Msg {
		ctx, stop := context.WithTimeout(context.Background(), readTimeout)
		defer stop()

		stopped, err := session.CancelBackend(ctx, pid, ends)
		return stoppedBackendMsg{
			ConnectionID: id, PID: pid, Ended: ends, Stopped: stopped,
			Problem: db.DescribeError(err),
		}
	}
}

func (model *Model) readStopBackendAnswer(answered stoppedBackendMsg) (tea.Model, tea.Cmd) {
	connection, _, found := model.findConnection(answered.ConnectionID)
	if !found {
		return model, nil
	}
	named := strconv.FormatInt(answered.PID, 10)
	done := "the statement of session " + named
	if answered.Ended {
		done = "session " + named
	}
	switch {
	case answered.Problem != "":
		connection.ShowError(done + " was not stopped: " + answered.Problem)
	case answered.Stopped:
		connection.Show(done + " was stopped")
	default:
		connection.Show("the server holds no session " + named + " any more")
	}
	return model, nil
}

// keepTheme applies the theme and writes it under `[ui] theme`.
func (model *Model) keepTheme(
	connection *app.Connection, name string,
) (tea.Model, tea.Cmd) {
	problems, applied := model.styles.ApplyThemeByName(name)
	if !applied {
		connection.ShowError("there is no theme called " + name)
		return model, nil
	}
	model.problems = append(model.problems, problems...)

	if err := cfg.SaveTheme(name, cfg.ResolveConfigPath()); err != nil {
		connection.ShowError(err.Error())
		return model, nil
	}
	connection.Show("the theme is now " + name)
	return model, nil
}

// askStopBackend asks before another session of the server is stopped. Ending the session
// closes its connection as well, so each way names what it does.
func (model *Model) askStopBackend(
	connection *app.Connection, session db.Activity, ends bool,
) (tea.Model, tea.Cmd) {
	pid := session.PID
	held := connection.Session
	id := model.ActiveID()

	named := strconv.FormatInt(pid, 10)
	title, question, said := " stop the statement ",
		"Stop the statement of session "+named+"?",
		"asked the server to stop the statement of session "+named
	if ends {
		title, question, said = " end the session ",
			"End session "+named+"? Its statement stops and its connection closes.",
			"asked the server to end session "+named
	}

	connection.Overlay = app.Overlay{
		Kind: app.OverlayConfirm, Title: title,
		Body: question + "\n\n" + present.TruncateText(session.Query, 60),
		Answers: app.OverlayAnswers{Answer: func(confirmed bool) app.AnswerCommand {
			if !confirmed {
				return nil
			}
			connection.Show(said)
			return carryAnswer(stopBackend(id, held, pid, ends))
		}},
	}
	return model, nil
}

// deleteSavedQuery removes the statement under the cursor from the file.
func (model *Model) deleteSavedQuery(
	connection *app.Connection, overlay *app.Overlay,
) (tea.Model, tea.Cmd) {
	queries := model.filterSaved(*overlay)
	if overlay.List.Cursor >= len(queries) {
		return model, nil
	}
	name := queries[overlay.List.Cursor].Name
	return model, dropSavedQuery(
		model.ActiveID(), model.log, connection.Profile().Name, name)
}

// answerPrompt returns the one-line field an action opened.
func (model *Model) answerPrompt(
	connection *app.Connection, tab *app.Tab, overlay app.Overlay,
) (tea.Model, tea.Cmd) {
	written := strings.TrimSpace(overlay.Draft.Text)
	// A search is answered with the text as it was typed: a blank at either end of it is
	// part of what the reader is looking for.
	if overlay.Prompt == app.PromptFind || overlay.Prompt == app.PromptReplace {
		written = overlay.Draft.Text
	}
	connection.Overlay = app.Overlay{}

	switch overlay.Prompt {
	case app.PromptTabName:
		tab.Editor.SetText(statement.ApplyQueryName(tab.Editor.Text, written))
		return model, nil

	case app.PromptWhere:
		kept := []core.FilterStep{}
		for _, step := range tab.Filter {
			if step.Kind != core.FilterRaw {
				kept = append(kept, step)
			}
		}
		if step, holds := core.BuildRawFilter(written); holds {
			kept = append(kept, step)
		}
		tab.Filter = kept
		return model.runTabRead(connection, tab)

	case app.PromptSearch:
		tab.Screen = present.ApplySearchTerm(tab.Screen, written)
		tab.GridRow = 0
		return model, nil

	case app.PromptGoToColumn:
		return model.goToColumn(connection, tab, written)

	case app.PromptFind:
		tab.Find.Term = written
		if written == "" {
			return model, nil
		}
		return model.stepMatch(connection, tab, 1)

	case app.PromptReplace:
		if tab.Find.Term == "" {
			connection.Show("look for something first")
			return model, nil
		}
		tab.Find.Replacement = written
		count := tab.Editor.ReplaceMatches(tab.Find.Term, written)
		if count == 0 {
			connection.Show("no match for " + tab.Find.Term)
			return model, nil
		}
		connection.Show(present.FormatCountOf(int64(count), "match", "matches") + " replaced")
		return model, model.reportEdit(connection, tab)

	case app.PromptSaveName:
		if written == "" {
			return model, nil
		}
		connection.Show("saved as " + written)
		return model, keepSavedQuery(
			model.ActiveID(), model.log, connection.Profile().Name, written, tab.Editor.Text)
	}
	return model, nil
}

// saveCell stages the value the cell editor holds.
func (model *Model) saveCell(
	connection *app.Connection, tab *app.Tab, overlay app.Overlay,
) (tea.Model, tea.Cmd) {
	// A whole new row is written as JSON, so the form is read as one. Every name the form
	// holds is a column of the row, an empty text included: a column the row is to leave
	// to the server is taken out of the form instead.
	if overlay.Cell.RowIndex == app.WholeRow {
		values, err := statement.ReadRowForm(overlay.Draft.Text)
		if err != nil {
			connection.ShowError(db.DescribeError(err))
			return model, nil
		}
		if !tab.StageChange(func(pending *core.PendingChanges) {
			pending.Inserts = append(pending.Inserts, values)
		}) {
			connection.ShowError(describeStageRefusal(tab))
			return model, nil
		}
		connection.Overlay = app.Overlay{}
		connection.Show("a new row is staged")
		return model, nil
	}

	// A cell picked from a list saves the value the cursor stands on.
	if len(overlay.Cell.Choices) > 0 {
		if overlay.List.Cursor < 0 || overlay.List.Cursor >= len(overlay.Cell.Choices) {
			return model, nil
		}
		return model.stageCellValue(connection, tab, overlay, core.CellValue{
			Kind: core.CellText, Text: overlay.Cell.Choices[overlay.List.Cursor]})
	}

	// The card writes JSON over several lines, and the column stores it on one.
	written := overlay.Draft.Text
	if present.IsJSONType(overlay.Cell.Column.DataType) {
		if compact, isJSON := present.CompactJSON(written); isJSON {
			written = compact
		}
	}
	return model.stageCellValue(connection, tab, overlay,
		core.CellValue{Kind: core.CellText, Text: written})
}

// runCellPickAction moves the cursor of a cell that is picked, or saves the value it stands
// on. It reports whether the action belonged to the list.
func (model *Model) runCellPickAction(
	connection *app.Connection, tab *app.Tab, overlay *app.Overlay, match Match,
) (bool, tea.Model, tea.Cmd) {
	count := len(overlay.Cell.Choices)
	switch match.Action {
	case ActionCursorUp:
		overlay.List.Cursor = clamp(overlay.List.Cursor-1, count)
	case ActionCursorDown:
		overlay.List.Cursor = clamp(overlay.List.Cursor+1, count)
	case ActionChooseRow:
		held, command := model.saveCell(connection, tab, *overlay)
		return true, held, command
	default:
		return false, model, nil
	}
	return true, model, nil
}

// stageCellValue stages one chosen value for the cell the editor was opened on.
func (model *Model) stageCellValue(
	connection *app.Connection, tab *app.Tab, overlay app.Overlay, value core.CellValue,
) (tea.Model, tea.Cmd) {
	rowIndex, columnIndex := overlay.Cell.RowIndex, overlay.Cell.ColumnIndex
	if rowIndex == app.WholeRow {
		connection.ShowError("a new row needs a JSON object, not a single value")
		return model, nil
	}
	if !tab.StageChange(func(pending *core.PendingChanges) {
		pending.Edits[core.BuildEditKey(rowIndex, columnIndex)] = core.CellEdit{
			RowIndex: rowIndex, ColumnIndex: columnIndex, Value: value,
		}
	}) {
		connection.ShowError(describeStageRefusal(tab))
		return model, nil
	}
	connection.Overlay = app.Overlay{}
	connection.Show(present.FormatStagedChanges(core.CountChanges(tab.Pending)))
	return model, nil
}

// runObjectAction returns a row of the object menu. Every entry ends in a statement, written
// into the editor or asked about first.
func (model *Model) runObjectAction(
	connection *app.Connection, tab *app.Tab, chosen app.MenuAction,
) (tea.Model, tea.Cmd) {
	rows := model.treeRows(connection)
	row, found := model.treeRowAt(connection, rows)
	if !found {
		return model, nil
	}
	dialect := connection.Session.Dialect()
	statement := ""

	switch chosen.ID {
	case app.ObjectGenerateSelect:
		statement = build.GenerateSelect(row.Node.Table.Qualified(), dialect)
	case app.ObjectGenerateInsert:
		statement = model.buildInsertTemplate(connection, row.Node.Table)
	case app.ObjectAddColumn:
		statement = build.GenerateAddColumn(row.Node.Table.Qualified(), dialect)
	case app.ObjectCreateIndex:
		statement = build.GenerateCreateIndex(row.Node.Table.Qualified(), dialect)
	case app.ObjectRenameTable:
		statement = build.GenerateRenameTable(row.Node.Table.Qualified(), dialect)
	case app.ObjectCreateTable:
		statement = build.GenerateCreateTable(row.Node.Schema, dialect)
	case app.ObjectCreateView:
		statement = build.GenerateCreateView(row.Node.Schema, dialect)
	case app.ObjectErDiagram:
		return model.showDiagram(connection, row.Node.Table)
	case app.ObjectImportFile:
		return model.openImport(connection, row.Node.Table, intoTableThatIsThere)
	case app.ObjectImportNewTable:
		return model.openImport(connection, db.TableRef{
			Schema: row.Node.Schema, Name: "",
		}, intoNewTable)

	case app.ObjectTruncate:
		statement = build.GenerateTruncate(row.Node.Table.Qualified(), dialect)
	case app.ObjectDropRelation:
		statement = build.GenerateDrop(
			row.Node.Table.Qualified(), string(row.Node.Table.Kind), dialect)
	case app.ObjectDropSchema:
		statement = build.GenerateDropSchema(row.Node.Schema, dialect)
	case app.ObjectDropObject:
		object := row.Node.Object
		statement = build.GenerateDropObject(build.TemplateObject{
			Schema: object.Schema, Name: object.Name, Kind: string(object.Kind),
			Detail: object.Detail, Identity: object.Identity,
		}, dialect)
	}

	if statement == "" {
		return model, nil
	}
	// A statement that removes something is written into the editor as well, so the user
	// reads it before it runs.
	opened := connection.OpenQueryTab(statement)
	opened.Focus = app.PaneEditor
	if chosen.Destructive {
		connection.Show("the statement is in the editor: read it, then run it")
	}
	return model, nil
}

// buildInsertTemplate writes an INSERT for the relation, from the columns the catalog
// already read. A relation that has not been read names one column, so the user has a
// statement to change rather than an empty one.
func (model *Model) buildInsertTemplate(
	connection *app.Connection, table db.TableRef,
) string {
	dialect := connection.Session.Dialect()
	state, read := connection.Catalog.Details[present.BuildTableID(table)]
	if !read || state.Kind != present.DetailReady {
		return build.GenerateInsert(table.Qualified(),
			[]build.TemplateColumn{{Name: "column_name"}}, dialect)
	}

	columns := make([]build.TemplateColumn, 0, len(state.Detail.Columns))
	for _, column := range state.Detail.Columns {
		if column.IsGenerated {
			continue
		}
		columns = append(columns, build.TemplateColumn{
			Name: column.Name, HasDefault: column.HasDefault,
		})
	}
	return build.GenerateInsert(table.Qualified(), columns, dialect)
}

// showDiagram draws the relation and the relations a foreign key joins to it. Every read
// happens off the frame, so the card opens with a line that says it is being read.
func (model *Model) showDiagram(
	connection *app.Connection, table db.TableRef,
) (tea.Model, tea.Cmd) {
	connection.Overlay = app.Overlay{
		Kind: app.OverlayMessage, Title: " diagram ", Body: "reading the catalog…",
	}
	return model, readDiagram(
		model.ActiveID(), connection.Session, table, connection.Catalog.Tables)
}

// readDiagramAnswer draws the diagram the reads answered, over the line that said it was
// being read.
func (model *Model) readDiagramAnswer(answered diagramMsg) (tea.Model, tea.Cmd) {
	connection, _, found := model.findConnection(answered.ConnectionID)
	// A user who closed the card while the reads ran is not shown it again.
	if !found || connection.Overlay.Kind != app.OverlayMessage ||
		connection.Overlay.Title != " diagram " {
		return model, nil
	}
	if answered.Problem != "" {
		connection.Overlay = app.Overlay{
			Kind: app.OverlayMessage, Title: " diagram failed ", Body: answered.Problem,
		}
		return model, nil
	}
	lines := answered.Lines
	if len(lines) == 0 {
		lines = []string{"nothing joins to this relation"}
	}
	connection.Overlay = app.Overlay{
		Kind: app.OverlayDiagram, Title: " diagram · " + answered.Title + " ", Lines: lines,
	}
	return model, nil
}

// readOverlayField returns a press the registry did not bind, which the field of the overlay
// takes.
func (model *Model) readOverlayField(
	connection *app.Connection, tab *app.Tab, overlay *app.Overlay, key tea.Key,
) (tea.Model, tea.Cmd) {
	buffer := overlay.Draft
	// A card whose field holds more than one line writes a line on Enter. The chat sends on
	// Enter instead, because a question is asked far more often than it is written over
	// several lines, and a modifier writes the line there.
	multiline := overlay.Kind == app.OverlayCellEdit || overlay.Kind == app.OverlayAiChat ||
		overlay.Kind == app.OverlayParameters
	if overlay.Kind == app.OverlayAiChat && key.Code == tea.KeyEnter {
		if key.Mod.Contains(uv.ModAlt) || key.Mod.Contains(uv.ModShift) {
			buffer.Insert("\n")
			return model, nil
		}
		return model.submitChatQuestion(connection, tab)
	}

	// The import form returns the arrows the same way the export form does, and its
	// review takes none of them.
	if overlay.Kind == app.OverlayImport && overlay.Import.Stage != app.ImportReview {
		switch key.Code {
		case tea.KeyUp:
			StepImportField(overlay, -1)
			return model, nil
		case tea.KeyDown, tea.KeyTab:
			StepImportField(overlay, 1)
			return model, nil
		case tea.KeyLeft, tea.KeyRight:
			if len(BuildImportFields(*overlay)[overlay.Field].Choices) > 0 {
				step := 1
				if key.Code == tea.KeyLeft {
					step = -1
				}
				StepImportChoice(overlay, step)
				return model, nil
			}
		}
	}

	// A form returns the arrows itself: one moves through the rows, the other steps
	// through the values of the row under the cursor.
	if overlay.Kind == app.OverlayExport {
		switch key.Code {
		case tea.KeyUp:
			StepExportField(overlay, -1)
			return model, nil
		case tea.KeyDown, tea.KeyTab:
			StepExportField(overlay, 1)
			return model, nil
		case tea.KeyLeft:
			if len(BuildExportFields(*overlay)[overlay.Field].Choices) > 0 {
				StepExportChoice(overlay, -1)
				return model, nil
			}
		case tea.KeyRight:
			if len(BuildExportFields(*overlay)[overlay.Field].Choices) > 0 {
				StepExportChoice(overlay, 1)
				return model, nil
			}
		}
	}

	switch key.Code {
	case tea.KeyEnter:
		if multiline {
			buffer.Insert("\n")
			return model, nil
		}
		return model.chooseOverlayRow(connection, tab, overlay, chooseInSameTab)
	case tea.KeyBackspace:
		buffer.DeleteBackward()
		model.resetOverlayCursor(connection, overlay)
		if overlay.Kind == app.OverlayExport {
			ReadExportField(overlay, buffer.Text)
		}
		if overlay.Kind == app.OverlayImport {
			ReadImportField(overlay, buffer.Text)
		}
		return model, nil
	case tea.KeyDelete:
		buffer.DeleteForward()
		if overlay.Kind == app.OverlayExport {
			ReadExportField(overlay, buffer.Text)
		}
		if overlay.Kind == app.OverlayImport {
			ReadImportField(overlay, buffer.Text)
		}
		return model, nil
	case tea.KeyLeft:
		buffer.MoveCaret(-1, false)
		return model, nil
	case tea.KeyRight:
		buffer.MoveCaret(1, false)
		return model, nil
	case tea.KeyUp:
		if model.scrollChatFromField(connection, overlay, buffer, -1) {
			return model, nil
		}
		if multiline {
			buffer.MoveLine(-1, false)
			return model, nil
		}
	case tea.KeyDown:
		if model.scrollChatFromField(connection, overlay, buffer, 1) {
			return model, nil
		}
		if multiline {
			buffer.MoveLine(1, false)
			return model, nil
		}
	case tea.KeyHome:
		if !model.jumpChatEnd(connection, overlay, -1) {
			buffer.MoveToStart(false)
		}
		return model, nil
	case tea.KeyEnd:
		if !model.jumpChatEnd(connection, overlay, 1) {
			buffer.MoveToEnd(false)
		}
		return model, nil
	}

	if key.Text != "" && !key.Mod.Contains(uv.ModCtrl) && !key.Mod.Contains(uv.ModAlt) {
		buffer.Insert(key.Text)
		model.resetOverlayCursor(connection, overlay)
	}
	if overlay.Kind == app.OverlayExport {
		ReadExportField(overlay, buffer.Text)
	}
	if overlay.Kind == app.OverlayImport {
		ReadImportField(overlay, buffer.Text)
	}
	return model, nil
}

// jumpChatEnd moves the conversation of the chat to its top or to its bottom. The card of the
// chat shows a conversation over a field, so the keys that reach the ends reach the ends of
// what is read and not of the question being written. It reports whether the panel took the
// key.
func (model *Model) jumpChatEnd(
	connection *app.Connection, overlay *app.Overlay, step int,
) bool {
	if overlay.Kind != app.OverlayAiChat {
		return false
	}
	chat := connection.Chat
	chat.HasTurn, chat.Notice = false, ""
	if step < 0 {
		chat.Offset, chat.Follow = 0, false
		return true
	}
	chat.Offset, chat.Follow = model.chatLastOffset(connection), true
	return true
}

// scrollChatFromField moves the conversation of the chat where the caret of the field cannot
// move any further: the field holds the keyboard, so an arrow that would leave it scrolls the
// reply instead. It reports whether the conversation took the key.
func (model *Model) scrollChatFromField(
	connection *app.Connection, overlay *app.Overlay, buffer *app.EditorBuffer, step int,
) bool {
	if overlay.Kind != app.OverlayAiChat {
		return false
	}
	line, _ := buffer.CaretPosition()
	if step < 0 && line > 0 {
		return false
	}
	if step > 0 && line < len(buffer.Lines())-1 {
		return false
	}
	model.rollChat(connection, step)
	return true
}

// resetOverlayCursor puts the cursor of a filtered list back on its first row.
func (model *Model) resetOverlayCursor(connection *app.Connection, overlay *app.Overlay) {
	switch overlay.Kind {
	case app.OverlayHistory, app.OverlaySaved, app.OverlayPalette, app.OverlayHelp:
		overlay.List.Cursor, overlay.List.Offset, overlay.List.Rolled = 0, 0, false
	}
}

// keepMatchingRows returns the rows the term keeps. `describeRow` writes the text of one row,
// which is what the term is matched against. An empty term keeps every row.
func keepMatchingRows[T any](rows []T, term string, describeRow func(T) string) []T {
	if term == "" {
		return rows
	}
	kept := make([]T, 0, len(rows))
	for _, row := range rows {
		if present.MatchesText(describeRow(row), term) {
			kept = append(kept, row)
		}
	}
	return kept
}

// filterHistory returns the statements the term at the top of the list keeps.
func (model *Model) filterHistory(overlay app.Overlay) []hist.HistoryEntry {
	return keepMatchingRows(overlay.Entries, model.readOverlayTerm(overlay),
		func(entry hist.HistoryEntry) string { return entry.SQL })
}

// filterSaved returns the saved statements the term keeps.
func (model *Model) filterSaved(overlay app.Overlay) []hist.SavedQuery {
	return keepMatchingRows(overlay.Saved, model.readOverlayTerm(overlay),
		func(saved hist.SavedQuery) string {
			return saved.Name + " " + present.TruncateText(
				core.CollapseWhitespace(saved.SQL), savedSQLWidth)
		})
}

// filterMenu returns the rows of a menu the term at the top of it keeps.
func (model *Model) filterMenu(overlay app.Overlay) []app.MenuAction {
	return keepMatchingRows(overlay.Actions, model.readOverlayTerm(overlay),
		func(action app.MenuAction) string { return action.Label + " " + action.Detail })
}

// filterThemes returns the themes the term at the top of the list keeps.
func (model *Model) filterThemes(overlay app.Overlay) []ThemeChoice {
	return keepMatchingRows(model.styles.registry.ListThemeChoices(),
		model.readOverlayTerm(overlay),
		func(choice ThemeChoice) string {
			return choice.Title + " " + choice.Name + " " + string(choice.Appearance)
		})
}

// filterPalette returns the actions the term keeps. A term is matched against the keys, the
// text and the detail of a row.
func (model *Model) filterPalette(overlay app.Overlay) []app.PaletteAction {
	return keepMatchingRows(overlay.Palette, model.readOverlayTerm(overlay),
		func(action app.PaletteAction) string {
			return action.Label + " " + action.Detail + " " + action.Chord
		})
}
