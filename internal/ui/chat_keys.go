package ui

import (
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/turanmahmudov/masume/internal/app"
	"github.com/turanmahmudov/masume/internal/core"
	"github.com/turanmahmudov/masume/internal/db"
	"github.com/turanmahmudov/masume/internal/present"
	"github.com/turanmahmudov/masume/internal/query"
)

// What the panel of the chat does: the keys that open it, the keys it returns to, and what one
// event of a run changes.

// openAiChat opens the panel, with the question already in the field.
func (model *Model) openAiChat(
	connection *app.Connection, asked string,
) (tea.Model, tea.Cmd) {
	connection.Chat.Notice = ""
	connection.Overlay = app.Overlay{
		Kind: app.OverlayAiChat, Draft: app.NewEditorBuffer(asked, len(asked)),
	}
	model.readConversations(connection)
	return model, nil
}

// askAi opens the panel and asks about the contents of the editor at once.
func (model *Model) askAi(
	connection *app.Connection, tab *app.Tab, prompt string,
) (tea.Model, tea.Cmd) {
	if strings.TrimSpace(tab.Editor.Text) == "" {
		connection.Show("the editor is empty")
		return model, nil
	}
	model.openAiChat(connection, "")
	return model.sendChatMessage(connection, tab, prompt)
}

// runAiAction returns the keys that reach the chat from the workspace.
func (model *Model) runAiAction(
	connection *app.Connection, tab *app.Tab, match Match,
) (tea.Model, tea.Cmd) {
	// No chord and no palette row reaches these while the features are off. The guard
	// stands here so a surface added later cannot open the chat by accident.
	if !model.offersAi() {
		return model, nil
	}
	switch match.Action {
	case ActionShowAiChat:
		return model.openAiChat(connection, "")
	case ActionSendToAi:
		// The statement goes into the field and is not sent, so the user can write the
		// question around it. An empty editor opens the chat with an empty field.
		return model.openAiChat(connection, strings.TrimSpace(tab.Editor.Text))
	case ActionAiFixError:
		// A refused run has a message from the server. A statement marked before it runs has
		// none, so the model reads the statement and can call validate_query itself.
		if findLastRunError(tab) != "" {
			return model.askAi(connection, tab,
				"The query in the editor failed. Explain why, and propose a fix.")
		}
		return model.askAi(connection, tab,
			"The query in the editor does not check out. Explain what is wrong, and fix it.")
	case ActionAiCheckPlan:
		return model.askAiToCheckPlan(connection, tab)
	}
	return model, nil
}

// askAiToCheckPlan sends the plan on screen. A new explain_query could answer with another one.
func (model *Model) askAiToCheckPlan(
	connection *app.Connection, tab *app.Tab,
) (tea.Model, tea.Cmd) {
	if tab.ViewData.Kind != app.DataPlan {
		return model, nil
	}
	heading := "Here is the estimated plan for this query, not run for real:"
	if tab.ViewData.Plan.Analyzed {
		heading = "Here is the plan already run for this query, with actual timings:"
	}
	return model.askAi(connection, tab, heading+"\n\n"+tab.ViewData.Plan.Raw)
}

// runChatAction returns the keys of the panel, and reports whether the key belonged to it. A
// key the panel does not answer to is a character of the question, because the field holds the
// keyboard: the two answers to a statement are read only while one waits, and the key that
// stops a reply only while one is written.
func (model *Model) runChatAction(
	connection *app.Connection, tab *app.Tab, match Match,
) (bool, tea.Model, tea.Cmd) {
	chat := connection.Chat
	switch match.Action {
	case ActionClose:
		connection.Overlay = app.Overlay{}
		return true, model, nil
	case ActionAnswerYes, ActionAnswerNo:
		if chat.Pending == nil {
			return false, model, nil
		}
		chat.AnswerPending(match.Action == ActionAnswerYes)
		return true, model, nil
	case ActionStopAiReply:
		if !chat.IsStreaming() {
			return false, model, nil
		}
		chat.Stopped()
		chat.Notice = "stopped; what it had written is kept"
		return true, model, model.keepConversation(connection)
	case ActionInsertAiSQL:
		held, command := model.insertAiSQL(connection, tab)
		return true, held, command
	case ActionNewAiChat:
		// The conversation on screen stays in the file, so nothing is lost.
		chat.Stopped()
		chat.Messages, chat.OpenID = nil, 0
		chat.Notice, chat.Problem, chat.Status = "", "", app.ChatIdle
		chat.HasTurn, chat.Offset, chat.Follow = false, 0, true
		return true, model, nil
	case ActionShowAiChats:
		connection.Overlay = app.Overlay{
			Kind: app.OverlayAiChats, Draft: app.NewEditorBuffer("", 0),
		}
		model.readConversations(connection)
		return true, model, nil
	case ActionScrollBack:
		held, command := model.scrollChat(connection, -1)
		return true, held, command
	case ActionScrollForward:
		held, command := model.scrollChat(connection, 1)
		return true, held, command
	case ActionPreviousTurn:
		held, command := model.jumpChatTurn(connection, -1)
		return true, held, command
	case ActionNextTurn:
		held, command := model.jumpChatTurn(connection, 1)
		return true, held, command
	}
	return false, model, nil
}

// submitChatQuestion sends what the field holds. The field returns this key itself, because a
// press of Enter alone writes a line into the question.
func (model *Model) submitChatQuestion(
	connection *app.Connection, tab *app.Tab,
) (tea.Model, tea.Cmd) {
	chat := connection.Chat
	asked := connection.Overlay.Draft.Text
	if strings.TrimSpace(asked) == "" || chat.IsStreaming() {
		return model, nil
	}
	connection.Overlay.Draft = app.NewEditorBuffer("", 0)
	// The jump ends, because the next turn is the one about to be written.
	chat.Notice, chat.HasTurn, chat.Follow = "", false, true
	return model.sendChatMessage(connection, tab, asked)
}

// insertAiSQL puts the statement of the last reply into the editor.
func (model *Model) insertAiSQL(
	connection *app.Connection, tab *app.Tab,
) (tea.Model, tea.Cmd) {
	chat := connection.Chat
	reply, found := chat.FindLastReply()
	if found {
		if sql, wrote := query.FindSQLBlock(reply); wrote {
			connection.Overlay = app.Overlay{}
			// A tab that shows a relation has no editor, so the statement opens a query
			// tab of its own rather than a buffer nothing draws.
			return model.loadSQL(connection, tab, sql, false)
		}
	}
	chat.Notice = "no query in the last reply yet"
	return model, nil
}

// scrollChat moves the conversation by a page.
func (model *Model) scrollChat(connection *app.Connection, pages int) (tea.Model, tea.Cmd) {
	chat := connection.Chat
	// The mark belongs to the jump, and this is a page.
	chat.HasTurn, chat.Notice = false, ""
	chat.Offset += pages * model.chatViewRows(connection)
	if chat.Offset < 0 {
		chat.Offset = 0
	}
	chat.Follow = pages > 0 && chat.Offset >= model.chatLastOffset(connection)
	return model, nil
}

// rollChat moves the conversation by rows, which is what one turn of the wheel does.
func (model *Model) rollChat(connection *app.Connection, rows int) (tea.Model, tea.Cmd) {
	chat := connection.Chat
	// The mark belongs to the jump, and this is a scroll.
	chat.HasTurn, chat.Notice = false, ""
	last := model.chatLastOffset(connection)
	chat.Offset = core.ClampWithin(chat.Offset+rows, last)
	chat.Follow = chat.Offset >= last
	return model, nil
}

// jumpChatTurn steps from turn to turn, so a long reply need not be scrolled. The first jump
// lands on the last turn, in either direction.
func (model *Model) jumpChatTurn(connection *app.Connection, step int) (tea.Model, tea.Cmd) {
	chat := connection.Chat
	count := len(chat.Messages)
	if count == 0 {
		return model, nil
	}
	from := count
	if chat.HasTurn {
		from = chat.TurnAt
	}
	chat.TurnAt = core.ClampIndex(from+step, count)
	chat.HasTurn, chat.Follow = true, false
	chat.Notice = "turn " + present.FormatCount(int64(chat.TurnAt+1)) +
		" of " + present.FormatCount(int64(count))
	// The turn goes to the top of the view, so every press moves the page.
	chat.Offset = model.findChatTurnRow(connection, chat.TurnAt)
	return model, nil
}

// chooseConversation puts the conversation under the cursor of the list on screen.
func (model *Model) chooseConversation(connection *app.Connection) (tea.Model, tea.Cmd) {
	held := model.filterConversations(connection.Overlay, connection.Chat)
	if connection.Overlay.List.Cursor >= len(held) {
		return model, nil
	}
	return model.openConversation(connection, held[connection.Overlay.List.Cursor].ID)
}

// removeChosenConversation removes the conversation under the cursor of the list.
func (model *Model) removeChosenConversation(
	connection *app.Connection,
) (tea.Model, tea.Cmd) {
	held := model.filterConversations(connection.Overlay, connection.Chat)
	if connection.Overlay.List.Cursor >= len(held) {
		return model, nil
	}
	return model.dropConversation(connection, held[connection.Overlay.List.Cursor].ID)
}

// openConversation puts one conversation of this profile back on screen.
func (model *Model) openConversation(
	connection *app.Connection, id int64,
) (tea.Model, tea.Cmd) {
	chat := connection.Chat
	chat.Stopped()
	chat.Problem, chat.Status, chat.Notice = "", app.ChatIdle, ""

	turns, err := model.log.ListChatTurns(id)
	if err != nil {
		chat.Fail("that conversation cannot be read back")
		return model.openAiChat(connection, "")
	}
	chat.OpenConversation(id, turns)
	return model.openAiChat(connection, "")
}

// dropConversation removes one conversation from the file. The panel opens an empty one where
// the conversation removed was the one on screen.
func (model *Model) dropConversation(
	connection *app.Connection, id int64,
) (tea.Model, tea.Cmd) {
	chat := connection.Chat
	log := model.log
	dropped := writeHistory(model.ActiveID(), "the conversation was not removed", func() error {
		return log.DeleteConversation(id)
	})
	if chat.OpenID == id {
		chat.Messages, chat.OpenID = nil, 0
	}
	connection.Overlay.List.Cursor = 0
	model.readConversations(connection)
	return model, dropped
}

// readConversations reads the list of conversations of this profile again, so a title or a
// count of turns is never stale.
func (model *Model) readConversations(connection *app.Connection) {
	conversations, err := model.log.ListConversations(connection.Profile().Name)
	if err != nil {
		// A file that cannot be read must not break the chat.
		connection.Chat.Conversations = nil
		return
	}
	connection.Chat.Conversations = conversations
}

// keepConversation writes the turns of the conversation into the file, and opens one for them
// where there is none. The turns are read here and written away from the loop, which holds a
// transaction the agent server can be waiting on.
func (model *Model) keepConversation(connection *app.Connection) tea.Cmd {
	chat := connection.Chat
	if len(chat.Messages) == 0 {
		return nil
	}
	log, connectionID := model.log, model.ActiveID()
	name, openID, turns := connection.Profile().Name, chat.OpenID, chat.WriteTurns()
	return func() tea.Msg {
		id, err := log.SaveConversation(name, openID, turns)
		if err != nil {
			// A turn that cannot be stored is still part of the conversation on screen.
			return conversationKeptMsg{
				ConnectionID: connectionID,
				Problem:      "the conversation was not stored: " + db.DescribeError(err),
			}
		}
		return conversationKeptMsg{ConnectionID: connectionID, ID: id}
	}
}

// readConversationKept takes the id the file gave the conversation, so the turns that follow
// are written into the same one.
func (model *Model) readConversationKept(held conversationKeptMsg) (tea.Model, tea.Cmd) {
	connection, _, found := model.findConnection(held.ConnectionID)
	if !found {
		return model, nil
	}
	if held.Problem != "" {
		connection.ShowError(held.Problem)
		return model, nil
	}
	connection.Chat.OpenID = held.ID
	model.readConversations(connection)
	return model, nil
}

// readChatEvents applies everything a run reported since the last frame.
func (model *Model) readChatEvents(held chatEventsMsg) (tea.Model, tea.Cmd) {
	connection, _, found := model.findConnection(held.ConnectionID)
	if !found {
		return model, nil
	}
	for _, event := range held.Events {
		model.applyChatEvent(connection, event)
	}
	if held.Source == nil {
		return model, nil
	}
	return model, waitForChatEvents(held.ConnectionID, held.Run, held.Source)
}

// applyChatEvent applies one thing a run reported.
func (model *Model) applyChatEvent(connection *app.Connection, event app.ChatEvent) {
	chat := connection.Chat
	// A run that was stopped goes on producing until the provider notices.
	if event.Run != chat.Run {
		if event.Allowed != nil {
			event.Allowed <- false
		}
		return
	}

	switch event.Kind {
	case app.ChatTextStarted:
		chat.StartTextBlock()
	case app.ChatTextArrived:
		chat.AppendDelta(event.Text)
	case app.ChatStepStarted:
		chat.StartStep(event.Text)
	case app.ChatStepFinished:
		chat.FinishStep()
	case app.ChatRunAsked:
		chat.Ask(event.Ask, event.Allowed)
	case app.ChatTableRead:
		connection.Catalog.Details[present.BuildTableID(event.Table)] =
			present.TableDetailState{Kind: present.DetailReady, Detail: event.Detail}
	case app.ChatUndoKept:
		connection.KeepUndo(event.Undo, event.Text, time.Now())
		chat.Notice = model.describeWriteOutcome(event.Undo)
	case app.ChatEnded:
		chat.Usage = chat.Usage.Add(event.Usage)
		if event.Problem != "" {
			chat.Fail(event.Problem)
		} else {
			chat.Status = app.ChatIdle
		}
	}
}

// readChatClosed ends the run and writes the conversation into the file.
func (model *Model) readChatClosed(held chatClosedMsg) (tea.Model, tea.Cmd) {
	connection, _, found := model.findConnection(held.ConnectionID)
	if !found {
		return model, nil
	}
	if held.Run != connection.Chat.Run {
		return model, nil
	}
	connection.Chat.End(held.Run)
	// Written at the end of the turn, because the next run reads only this.
	return model, model.keepConversation(connection)
}
