package ui

import "github.com/turanmahmudov/masume/internal/cfg"

// The help, grouped the way a reader looks for a key: by what they are doing, not by
// the scope the registry keeps it in. The keys come from the registry, so a rebound
// chord moves the help with it. A row with no action holds its keys as text.

// HelpEntry is one row of the help: what it does, and the keys that do it.
type HelpEntry struct {
	Scope   cfg.KeyScope
	Actions []ActionID
	// Keys stands where no action is bound, such as a key a field returns itself.
	Keys string
	Text string
}

// HelpSection is one titled group of help rows.
type HelpSection struct {
	Title   string
	Entries []HelpEntry
}

// HelpSections are the groups the help draws, in order.
var HelpSections = []HelpSection{
	{
		Title: "tabs",
		Entries: []HelpEntry{
			{Scope: cfg.ScopeGlobal, Actions: []ActionID{ActionNewQueryTab}, Text: "new query tab"},
			{Scope: cfg.ScopeGlobal, Actions: []ActionID{ActionActivateTab}, Text: "go to that tab, by position"},
			{Scope: cfg.ScopeGlobal, Actions: []ActionID{ActionPreviousTab, ActionNextTab}, Text: "previous or next tab"},
			{Scope: cfg.ScopeGlobal, Actions: []ActionID{ActionNameTab}, Text: "name this tab, as a comment on the first line"},
			{Scope: cfg.ScopeGlobal, Actions: []ActionID{ActionCloseTab}, Text: "close the tab"},
			{Scope: cfg.ScopeGlobal, Actions: []ActionID{ActionReopenTab}, Text: "reopen the tab closed last"},
			{Scope: cfg.ScopeGlobal, Actions: []ActionID{ActionRevealSQL}, Text: "edit the query behind the result"},
			{Scope: cfg.ScopeGlobal, Actions: []ActionID{ActionToggleSidebar}, Text: "show or hide the object tree"},
			{Scope: cfg.ScopeGlobal, Actions: []ActionID{ActionToggleResult}, Text: "show or hide the result"},
		},
	},
	{
		Title: "connections",
		Entries: []HelpEntry{
			{Scope: cfg.ScopeGlobal, Actions: []ActionID{ActionOpenPicker}, Text: "open the connection picker"},
			{Scope: cfg.ScopeGlobal, Actions: []ActionID{ActionPreviousConnection, ActionNextConnection}, Text: "switch connection"},
			{Scope: cfg.ScopeGlobal, Actions: []ActionID{ActionCloseConnection}, Text: "close the connection and every tab of it"},
		},
	},
	{
		Title: "panes",
		Entries: []HelpEntry{
			{Scope: cfg.ScopeGlobal, Actions: []ActionID{ActionFocusNextPane}, Text: "move the focus to the next pane"},
			{Scope: cfg.ScopeGlobal, Actions: []ActionID{ActionFocusPreviousPane}, Text: "move it to the pane before"},
			{Scope: cfg.ScopeTree, Actions: []ActionID{ActionCursorUp, ActionCursorDown, ActionCursorPageUp, ActionCursorPageDown}, Text: "move in the tree"},
			{Scope: cfg.ScopeTree, Actions: []ActionID{ActionCursorFirstRow, ActionCursorLastRow}, Text: "to either end"},
			{Scope: cfg.ScopeTree, Actions: []ActionID{ActionFoldRow, ActionUnfoldRow}, Text: "fold and unfold a row"},
			{Scope: cfg.ScopeTree, Actions: []ActionID{ActionOpenNode}, Text: "open what is under the cursor"},
			{Scope: cfg.ScopeTree, Actions: []ActionID{ActionOpenInNewTab}, Text: "open a second tab on the same table"},
			{Scope: cfg.ScopeTree, Actions: []ActionID{ActionDescribeTable}, Text: "describe the table"},
			{Scope: cfg.ScopeTree, Actions: []ActionID{ActionObjectMenu}, Text: "the menu of this object"},
			{Scope: cfg.ScopeTree, Actions: []ActionID{ActionFilterTree}, Text: "filter the tree"},
			{Scope: cfg.ScopeTree, Actions: []ActionID{ActionToggleFavourite}, Text: "keep this object as a favourite"},
			{Scope: cfg.ScopeTree, Actions: []ActionID{ActionToggleSystemSchemas}, Text: "show or hide the system schemas"},
			{Scope: cfg.ScopeGlobal, Actions: []ActionID{ActionRefreshObjects}, Text: "read the object tree again"},
		},
	},
	{
		Title: "the grid",
		Entries: []HelpEntry{
			{Scope: cfg.ScopeGrid, Actions: []ActionID{ActionCursorUp, ActionCursorDown, ActionCursorPageUp, ActionCursorPageDown}, Text: "another row"},
			{Scope: cfg.ScopeGrid, Actions: []ActionID{ActionCursorLeft, ActionCursorRight}, Text: "another column"},
			{Scope: cfg.ScopeGrid, Actions: []ActionID{ActionCursorFirstRow, ActionCursorLastRow}, Text: "to either end"},
			{Scope: cfg.ScopeGrid, Actions: []ActionID{ActionOpenRow}, Text: "open the row under the cursor"},
			{Scope: cfg.ScopeGrid, Actions: []ActionID{ActionViewCell}, Text: "view the cell full size"},
			{Scope: cfg.ScopeDialog, Actions: []ActionID{ActionCopyValue}, Text: "copy the cell, in the cell viewer"},
			{Scope: cfg.ScopeGrid, Actions: []ActionID{ActionFollowForeignKey}, Text: "open the row the foreign key points at"},
			{Scope: cfg.ScopeGrid, Actions: []ActionID{ActionCopyMenu}, Text: "copy: cell, row, or the whole result"},
			{Scope: cfg.ScopeGrid, Actions: []ActionID{ActionOpenMenu}, Text: "the menu of actions on the row and cell"},
			{Scope: cfg.ScopeGrid, Actions: []ActionID{ActionCountRows}, Text: "count every row of the result"},
			{Scope: cfg.ScopeGrid, Actions: []ActionID{ActionGoToColumn}, Text: "go to a column by name"},
			{Scope: cfg.ScopeGrid, Actions: []ActionID{ActionSearchColumns}, Text: "search the rows on screen"},
			{Scope: cfg.ScopeGrid, Actions: []ActionID{ActionFreezeColumns}, Text: "freeze the column under the cursor"},
			{Scope: cfg.ScopeGrid, Actions: []ActionID{ActionToggleMasking}, Text: "show or hide a hidden value"},
			{Keys: "drag", Text: "select what the pointer is dragged over"},
		},
	},
	{
		Title: "sort and filter",
		Entries: []HelpEntry{
			{Scope: cfg.ScopeGrid, Actions: []ActionID{ActionSortColumn}, Text: "sort by the column under the cursor"},
			{Scope: cfg.ScopeGrid, Actions: []ActionID{ActionAddSortColumn}, Text: "add that column to the sort"},
			{Scope: cfg.ScopeGrid, Actions: []ActionID{ActionFilterByCell}, Text: "filter by the cell under the cursor"},
			{Scope: cfg.ScopeGrid, Actions: []ActionID{ActionFilterByValues}, Text: "filter by values chosen from a list"},
			{Scope: cfg.ScopeGrid, Actions: []ActionID{ActionExcludeCell}, Text: "exclude that value"},
			{Scope: cfg.ScopeGrid, Actions: []ActionID{ActionFilterWhere}, Text: "filter with a WHERE predicate"},
			{Scope: cfg.ScopeGrid, Actions: []ActionID{ActionPopFilter}, Text: "take the last filter off"},
			{Scope: cfg.ScopeGrid, Actions: []ActionID{ActionClearRewrites}, Text: "clear the sort and the filters"},
		},
	},
	{
		Title: "query",
		Entries: []HelpEntry{
			{Scope: cfg.ScopeGlobal, Actions: []ActionID{ActionRunAtCursor}, Text: "run the selection, or the statement at it"},
			{Scope: cfg.ScopeGlobal, Actions: []ActionID{ActionRunBatch}, Text: "run every statement"},
			{Scope: cfg.ScopeGlobal, Actions: []ActionID{ActionPreviousStatement, ActionNextStatement}, Text: "previous or next statement of a batch"},
			{Scope: cfg.ScopeGlobal, Actions: []ActionID{ActionExplain}, Text: "explain the plan"},
			{Scope: cfg.ScopeGlobal, Actions: []ActionID{ActionExplainAnalyze}, Text: "explain analyze"},
			{Scope: cfg.ScopeGlobal, Actions: []ActionID{ActionCancelQuery}, Text: "cancel the running query"},
			{Scope: cfg.ScopeGlobal, Actions: []ActionID{ActionNextPage}, Text: "fetch more rows"},
			{Scope: cfg.ScopeDocument, Actions: []ActionID{ActionOpenNode}, Text: "open or fold the document under the cursor, in the tree view"},
			{Scope: cfg.ScopeDocument, Actions: []ActionID{ActionUnfoldRow, ActionFoldRow}, Text: "open a field, or fold it and step out"},
			{Scope: cfg.ScopeDocument, Actions: []ActionID{ActionCopyValue}, Text: "copy the value under the cursor"},
			{Scope: cfg.ScopeDocument, Actions: []ActionID{ActionCopyPath}, Text: "copy the name of the field under the cursor"},
			{Scope: cfg.ScopePlan, Actions: []ActionID{ActionToggleRawPlan}, Text: "the raw plan or the tree, in the plan view"},
			{Scope: cfg.ScopePlan, Actions: []ActionID{ActionCopyPlan}, Text: "copy the plan as the server sent it"},
			{Scope: cfg.ScopeGlobal, Actions: []ActionID{ActionShowPalette}, Text: "command palette"},
			{Scope: cfg.ScopeGlobal, Actions: []ActionID{ActionShowHistory}, Text: "query history"},
			{Scope: cfg.ScopeGlobal, Actions: []ActionID{ActionShowSaved}, Text: "saved queries"},
			{Scope: cfg.ScopeGlobal, Actions: []ActionID{ActionExportCSV}, Text: "export the result as CSV"},
			{Scope: cfg.ScopeGlobal, Actions: []ActionID{ActionExportJSON}, Text: "export the result as JSON"},
			{Scope: cfg.ScopeGlobal, Actions: []ActionID{ActionShowHelp}, Text: "this help"},
			{Scope: cfg.ScopeGlobal, Actions: []ActionID{ActionSelectView}, Text: "the views of the result, by position"},
			{Scope: cfg.ScopeGlobal, Actions: []ActionID{ActionPreviousView, ActionNextView}, Text: "the previous or next view of the result"},
			{Scope: cfg.ScopeGlobal, Actions: []ActionID{ActionSaveQuery}, Text: "save this query under a name"},
		},
	},
	{
		Title: "editing rows",
		Entries: []HelpEntry{
			{Scope: cfg.ScopeGrid, Actions: []ActionID{ActionEditCell}, Text: "edit the cell under the cursor"},
			{Scope: cfg.ScopeGrid, Actions: []ActionID{ActionInsertRow}, Text: "insert a row"},
			{Scope: cfg.ScopeGrid, Actions: []ActionID{ActionToggleDelete}, Text: "mark the row for deletion"},
			{Scope: cfg.ScopeGrid, Actions: []ActionID{ActionDuplicateRow}, Text: "duplicate the row"},
			{Scope: cfg.ScopeGrid, Actions: []ActionID{ActionUndoChange}, Text: "take the last staged change back"},
			{Scope: cfg.ScopeGrid, Actions: []ActionID{ActionRedoChange}, Text: "put it on again"},
			{Scope: cfg.ScopeGrid, Actions: []ActionID{ActionReviewChanges}, Text: "review the staged changes"},
			{Scope: cfg.ScopeDialog, Actions: []ActionID{ActionApplyChanges}, Text: "apply them, in the review overlay"},
			{Scope: cfg.ScopeDialog, Actions: []ActionID{ActionDiscardChanges}, Text: "discard them, there"},
		},
	},
	{
		Title: "transaction",
		Entries: []HelpEntry{
			{Scope: cfg.ScopeGlobal, Actions: []ActionID{ActionBeginTransaction}, Text: "begin"},
			{Scope: cfg.ScopeGlobal, Actions: []ActionID{ActionCommitTransaction}, Text: "commit"},
			{Scope: cfg.ScopeGlobal, Actions: []ActionID{ActionRollbackTransaction}, Text: "rollback"},
			{Scope: cfg.ScopeGlobal, Actions: []ActionID{ActionToggleAutocommit}, Text: "toggle autocommit"},
		},
	},
	{
		Title: "writing a statement",
		Entries: []HelpEntry{
			{Scope: cfg.ScopeEditor, Actions: []ActionID{ActionCaretLineStart}, Text: "to the first word of the line, and to its first cell again"},
			{Scope: cfg.ScopeEditor, Actions: []ActionID{ActionCaretLineEnd}, Text: "to the end of the line"},
			{Scope: cfg.ScopeEditor, Actions: []ActionID{ActionCaretTextStart, ActionCaretTextEnd}, Text: "to the top or the foot of the statement"},
			{Scope: cfg.ScopeEditor, Actions: []ActionID{ActionCaretWordLeft, ActionCaretWordRight}, Text: "one word back or on"},
			{Scope: cfg.ScopeEditor, Actions: []ActionID{ActionCaretPageUp, ActionCaretPageDown}, Text: "a page up or down"},
			{Scope: cfg.ScopeEditor, Actions: []ActionID{ActionDeleteWordBack}, Text: "take the word before the caret"},
			{Scope: cfg.ScopeEditor, Actions: []ActionID{ActionDeleteWordForward}, Text: "take the word after it"},
			{Scope: cfg.ScopeEditor, Actions: []ActionID{ActionUndoEdit}, Text: "take the last edit back"},
			{Scope: cfg.ScopeEditor, Actions: []ActionID{ActionRedoEdit}, Text: "write it again"},
			{Scope: cfg.ScopeEditor, Actions: []ActionID{ActionPasteText}, Text: "paste what this client last copied"},
			{Scope: cfg.ScopeEditor, Actions: []ActionID{ActionFormatSQL}, Text: "write the statement out again, one clause per line"},
			{Scope: cfg.ScopeEditor, Actions: []ActionID{ActionCommentLines}, Text: "comment the lines out, and back in again"},
			{Scope: cfg.ScopeEditor, Actions: []ActionID{ActionIndentLines, ActionOutdentLines}, Text: "move the lines one step right or left"},
			{Keys: "Tab", Text: "accept a completion"},
		},
	},
	{
		Title: "searching the statement",
		Entries: []HelpEntry{
			{Scope: cfg.ScopeEditor, Actions: []ActionID{ActionFindInStatement}, Text: "look for text in the statement"},
			{Scope: cfg.ScopeDialog, Actions: []ActionID{ActionReplaceInStatement}, Text: "in the find field, replace every match instead"},
			{Scope: cfg.ScopeEditor, Actions: []ActionID{ActionNextMatch, ActionPreviousMatch}, Text: "the next or the previous match"},
		},
	},
	{
		Title: "selecting text",
		Entries: []HelpEntry{
			{Scope: cfg.ScopeEditor, Actions: []ActionID{ActionSelectAll}, Text: "select the whole statement"},
			{Keys: "Shift+←", Text: "grow a selection left, in the editor"},
			{Keys: "Shift+→", Text: "grow it to the right"},
			{Keys: "Shift+↑", Text: "carry it onto the line above"},
			{Keys: "Shift+↓", Text: "carry it onto the line below"},
			{Keys: "Shift+Home", Text: "grow it to the start of the line"},
			{Keys: "Shift+End", Text: "grow it to the end of the line"},
			{Keys: "Ctrl+Shift+←", Text: "grow it one word back"},
			{Keys: "Ctrl+Shift+→", Text: "grow it one word on"},
			{Keys: "double click", Text: "take the word under the pointer"},
			{Keys: "triple click", Text: "take the whole line"},
			{Keys: "Ctrl+C", Text: "copy the selection, or quit with none"},
		},
	},
	{
		Title: "a list walked by key",
		Entries: []HelpEntry{
			{Scope: cfg.ScopeList, Actions: []ActionID{ActionCursorUp, ActionCursorDown, ActionCursorPageUp, ActionCursorPageDown}, Text: "move in a list an overlay draws"},
			{Scope: cfg.ScopeList, Actions: []ActionID{ActionCursorFirstRow, ActionCursorLastRow}, Text: "to either end"},
			{Scope: cfg.ScopeList, Actions: []ActionID{ActionChooseRow}, Text: "take the row under the cursor"},
		},
	},
	{
		Title: "overlays, the picker and the forms",
		Entries: []HelpEntry{
			{Scope: cfg.ScopeDialog, Actions: []ActionID{ActionClose}, Text: "close the overlay, or cancel"},
			{Scope: cfg.ScopeDialog, Actions: []ActionID{ActionAnswerYes}, Text: "yes, to a question before a write"},
			{Scope: cfg.ScopeDialog, Actions: []ActionID{ActionAnswerNo}, Text: "no, to the same question"},
			{Scope: cfg.ScopeDialog, Actions: []ActionID{ActionNewConnection}, Text: "a new connection, in the picker"},
			{Scope: cfg.ScopeDialog, Actions: []ActionID{ActionEditConnection}, Text: "edit the one under the cursor"},
			{Scope: cfg.ScopeDialog, Actions: []ActionID{ActionDeleteConnection}, Text: "delete it"},
			{Scope: cfg.ScopeDialog, Actions: []ActionID{ActionTestConnection}, Text: "test the connection, in the form"},
			{Scope: cfg.ScopeDialog, Actions: []ActionID{ActionSaveForm}, Text: "save it"},
			{Scope: cfg.ScopeDialog, Actions: []ActionID{ActionSaveCell}, Text: "save the cell, in the cell editor"},
			{Scope: cfg.ScopeDialog, Actions: []ActionID{ActionSetNull}, Text: "write NULL there"},
			{Scope: cfg.ScopeDialog, Actions: []ActionID{ActionSetEmpty}, Text: "write an empty value"},
			{Scope: cfg.ScopeDialog, Actions: []ActionID{ActionSetDefault}, Text: "write DEFAULT"},
			{Scope: cfg.ScopeDialog, Actions: []ActionID{ActionPrettifyJSON}, Text: "prettify the JSON"},
			{Scope: cfg.ScopeDialog, Actions: []ActionID{ActionRunWithValues}, Text: "run, in the values form"},
			{Scope: cfg.ScopeDialog, Actions: []ActionID{ActionWriteExport}, Text: "write the file, in the export form"},
			{Scope: cfg.ScopeDialog, Actions: []ActionID{ActionOpenInNewTab}, Text: "into a new tab, in the history"},
			{Scope: cfg.ScopeDialog, Actions: []ActionID{ActionListSecondary}, Text: "the second choice a list offers"},
			{Scope: cfg.ScopeDialog, Actions: []ActionID{ActionStopSession}, Text: "stop the statement of a session, in the server activity"},
			{Scope: cfg.ScopeDialog, Actions: []ActionID{ActionToggleValue}, Text: "keep or drop a value, in the picker"},
			{Scope: cfg.ScopeDialog, Actions: []ActionID{ActionKeepOnlyValue}, Text: "keep only the one under the cursor"},
			{Scope: cfg.ScopeDialog, Actions: []ActionID{ActionKeepAllValues}, Text: "keep every value again"},
		},
	},
	{
		Title: "ai chat",
		Entries: []HelpEntry{
			{Scope: cfg.ScopeGlobal, Actions: []ActionID{ActionShowAiChat}, Text: "open it"},
			{Scope: cfg.ScopeGlobal, Actions: []ActionID{ActionSendToAi}, Text: "the editor's statement into the chat's field"},
			{Keys: "Enter", Text: "send the question"},
			{Keys: "Shift+Enter", Text: "a newline in the question"},
			{Keys: "Alt+Enter", Text: "a newline in the question"},
			{Scope: cfg.ScopeDialog, Actions: []ActionID{ActionInsertAiSQL}, Text: "put the last reply's query into the editor"},
			{Scope: cfg.ScopeDialog, Actions: []ActionID{ActionStopAiReply}, Text: "stop the reply being written"},
			{Scope: cfg.ScopeDialog, Actions: []ActionID{ActionNewAiChat}, Text: "start a new conversation"},
			{Scope: cfg.ScopeDialog, Actions: []ActionID{ActionShowAiChats}, Text: "the conversations of this profile"},
			{Scope: cfg.ScopeDialog, Actions: []ActionID{ActionPreviousTurn, ActionNextTurn}, Text: "to the turn before or after, over a long reply"},
			{Scope: cfg.ScopeDialog, Actions: []ActionID{ActionScrollBack, ActionScrollForward}, Text: "read a page back or forward"},
			{Scope: cfg.ScopePlan, Actions: []ActionID{ActionAiCheckPlan}, Text: "send the plan to it, in the plan view"},
			{Scope: cfg.ScopeGlobal, Actions: []ActionID{ActionAiFixError}, Text: "explain a query that just failed"},
			{Keys: "", Text: "more prompts for it live in the palette"},
		},
	},
}
