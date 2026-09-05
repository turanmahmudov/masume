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
			{Scope: cfg.ScopeGlobal, Actions: []ActionID{ActionActivateTab}, Text: "go to a tab by its number"},
			{Scope: cfg.ScopeGlobal, Actions: []ActionID{ActionPreviousTab, ActionNextTab}, Text: "previous or next tab"},
			{Scope: cfg.ScopeGlobal, Actions: []ActionID{ActionNameTab}, Text: "name this tab"},
			{Scope: cfg.ScopeGlobal, Actions: []ActionID{ActionCloseTab}, Text: "close the tab"},
			{Scope: cfg.ScopeGlobal, Actions: []ActionID{ActionReopenTab}, Text: "reopen the last closed tab"},
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
			{Scope: cfg.ScopeGlobal, Actions: []ActionID{ActionFocusPreviousPane}, Text: "move the focus to the previous pane"},
			{Scope: cfg.ScopeTree, Actions: []ActionID{ActionCursorUp, ActionCursorDown, ActionCursorPageUp, ActionCursorPageDown}, Text: "move in the tree"},
			{Scope: cfg.ScopeTree, Actions: []ActionID{ActionCursorFirstRow, ActionCursorLastRow}, Text: "go to the first or the last row"},
			{Scope: cfg.ScopeTree, Actions: []ActionID{ActionFoldRow, ActionUnfoldRow}, Text: "fold and unfold a row"},
			{Scope: cfg.ScopeTree, Actions: []ActionID{ActionOpenNode}, Text: "open what is under the cursor"},
			{Scope: cfg.ScopeTree, Actions: []ActionID{ActionOpenInNewTab}, Text: "open a second tab on the same table"},
			{Scope: cfg.ScopeTree, Actions: []ActionID{ActionDescribeTable}, Text: "describe the table"},
			{Scope: cfg.ScopeTree, Actions: []ActionID{ActionObjectMenu}, Text: "open the menu of this object"},
			{Scope: cfg.ScopeTree, Actions: []ActionID{ActionFilterTree}, Text: "filter the tree"},
			{Scope: cfg.ScopeTree, Actions: []ActionID{ActionToggleFavourite}, Text: "mark this object as a favourite"},
			{Scope: cfg.ScopeTree, Actions: []ActionID{ActionToggleSystemSchemas}, Text: "show or hide the system schemas"},
			{Scope: cfg.ScopeGlobal, Actions: []ActionID{ActionRefreshObjects}, Text: "read the object tree again"},
		},
	},
	{
		Title: "the grid",
		Entries: []HelpEntry{
			{Scope: cfg.ScopeGrid, Actions: []ActionID{ActionCursorUp, ActionCursorDown, ActionCursorPageUp, ActionCursorPageDown}, Text: "go to another row"},
			{Scope: cfg.ScopeGrid, Actions: []ActionID{ActionCursorLeft, ActionCursorRight}, Text: "go to another column"},
			{Scope: cfg.ScopeGrid, Actions: []ActionID{ActionCursorFirstRow, ActionCursorLastRow}, Text: "go to the first or the last row"},
			{Scope: cfg.ScopeGrid, Actions: []ActionID{ActionOpenRow}, Text: "open the row under the cursor"},
			{Scope: cfg.ScopeGrid, Actions: []ActionID{ActionViewCell}, Text: "open the cell viewer"},
			{Scope: cfg.ScopeDialog, Actions: []ActionID{ActionCopyValue}, Text: "copy the cell, in the cell viewer"},
			{Scope: cfg.ScopeGrid, Actions: []ActionID{ActionFollowForeignKey}, Text: "open the row the foreign key points at"},
			{Scope: cfg.ScopeGrid, Actions: []ActionID{ActionCopyMenu}, Text: "copy: cell, row, or the whole result"},
			{Scope: cfg.ScopeGrid, Actions: []ActionID{ActionOpenMenu}, Text: "open the menu of the row and the cell"},
			{Scope: cfg.ScopeGrid, Actions: []ActionID{ActionCountRows}, Text: "count every row of the result"},
			{Scope: cfg.ScopeGrid, Actions: []ActionID{ActionGoToColumn}, Text: "go to a column by name"},
			{Scope: cfg.ScopeGrid, Actions: []ActionID{ActionSearchColumns}, Text: "search the rows on screen"},
			{Scope: cfg.ScopeGrid, Actions: []ActionID{ActionFreezeColumns}, Text: "freeze the column under the cursor"},
			{Scope: cfg.ScopeGrid, Actions: []ActionID{ActionToggleMasking}, Text: "show or hide a masked value"},
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
			{Scope: cfg.ScopeGrid, Actions: []ActionID{ActionPopFilter}, Text: "remove the last filter"},
			{Scope: cfg.ScopeGrid, Actions: []ActionID{ActionClearRewrites}, Text: "clear the sort and the filters"},
		},
	},
	{
		Title: "query",
		Entries: []HelpEntry{
			{Scope: cfg.ScopeGlobal, Actions: []ActionID{ActionRunAtCursor}, Text: "run the selection, or the statement at the caret"},
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
			{Scope: cfg.ScopePlan, Actions: []ActionID{ActionToggleRawPlan}, Text: "switch between the plan tree and the raw plan"},
			{Scope: cfg.ScopePlan, Actions: []ActionID{ActionCopyPlan}, Text: "copy the plan as the server sent it"},
			{Scope: cfg.ScopeGlobal, Actions: []ActionID{ActionShowPalette}, Text: "command palette"},
			{Scope: cfg.ScopeGlobal, Actions: []ActionID{ActionShowHistory}, Text: "query history"},
			{Scope: cfg.ScopeGlobal, Actions: []ActionID{ActionShowSaved}, Text: "saved queries"},
			{Scope: cfg.ScopeGlobal, Actions: []ActionID{ActionExportCSV}, Text: "export the result as CSV"},
			{Scope: cfg.ScopeGlobal, Actions: []ActionID{ActionExportJSON}, Text: "export the result as JSON"},
			{Scope: cfg.ScopeGlobal, Actions: []ActionID{ActionShowHelp}, Text: "this help"},
			{Scope: cfg.ScopeGlobal, Actions: []ActionID{ActionSelectView}, Text: "go to a view of the result by its number"},
			{Scope: cfg.ScopeGlobal, Actions: []ActionID{ActionPreviousView, ActionNextView}, Text: "go to the previous or the next view"},
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
			{Scope: cfg.ScopeGrid, Actions: []ActionID{ActionUndoChange}, Text: "undo the last staged change"},
			{Scope: cfg.ScopeGrid, Actions: []ActionID{ActionRedoChange}, Text: "redo the last undone change"},
			{Scope: cfg.ScopeGrid, Actions: []ActionID{ActionReviewChanges}, Text: "review the staged changes"},
			{Scope: cfg.ScopeDialog, Actions: []ActionID{ActionApplyChanges}, Text: "apply them, in the review card"},
			{Scope: cfg.ScopeDialog, Actions: []ActionID{ActionDiscardChanges}, Text: "discard them, in the review card"},
			{Scope: cfg.ScopeGlobal, Actions: []ActionID{ActionUndoWrite}, Text: "undo the last write"},
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
			{Scope: cfg.ScopeEditor, Actions: []ActionID{ActionCaretLineStart}, Text: "go to the first word of the line, then to the start of the line"},
			{Scope: cfg.ScopeEditor, Actions: []ActionID{ActionCaretLineEnd}, Text: "go to the end of the line"},
			{Scope: cfg.ScopeEditor, Actions: []ActionID{ActionCaretTextStart, ActionCaretTextEnd}, Text: "go to the top or the bottom of the statement"},
			{Scope: cfg.ScopeEditor, Actions: []ActionID{ActionCaretWordLeft, ActionCaretWordRight}, Text: "go one word back or forward"},
			{Scope: cfg.ScopeEditor, Actions: []ActionID{ActionCaretPageUp, ActionCaretPageDown}, Text: "go a page up or down"},
			{Scope: cfg.ScopeEditor, Actions: []ActionID{ActionDeleteWordBack}, Text: "delete the word before the caret"},
			{Scope: cfg.ScopeEditor, Actions: []ActionID{ActionDeleteWordForward}, Text: "delete the word after the caret"},
			{Scope: cfg.ScopeEditor, Actions: []ActionID{ActionUndoEdit}, Text: "undo the last edit"},
			{Scope: cfg.ScopeEditor, Actions: []ActionID{ActionRedoEdit}, Text: "redo the last edit"},
			{Scope: cfg.ScopeEditor, Actions: []ActionID{ActionPasteText}, Text: "paste what this client last copied"},
			{Scope: cfg.ScopeEditor, Actions: []ActionID{ActionFormatSQL}, Text: "format the statement, one clause per line"},
			{Scope: cfg.ScopeEditor, Actions: []ActionID{ActionCommentLines}, Text: "comment or uncomment the lines"},
			{Scope: cfg.ScopeEditor, Actions: []ActionID{ActionIndentLines, ActionOutdentLines}, Text: "indent or outdent the lines"},
			{Keys: "Tab", Text: "accept a completion"},
		},
	},
	{
		Title: "searching the statement",
		Entries: []HelpEntry{
			{Scope: cfg.ScopeEditor, Actions: []ActionID{ActionFindInStatement}, Text: "find text in the statement"},
			{Scope: cfg.ScopeDialog, Actions: []ActionID{ActionReplaceInStatement}, Text: "replace every match, in the find field"},
			{Scope: cfg.ScopeEditor, Actions: []ActionID{ActionNextMatch, ActionPreviousMatch}, Text: "go to the next or the previous match"},
		},
	},
	{
		Title: "selecting text",
		Entries: []HelpEntry{
			{Scope: cfg.ScopeEditor, Actions: []ActionID{ActionSelectAll}, Text: "select the whole statement"},
			{Keys: "Shift+←", Text: "extend the selection one cell left, in the editor"},
			{Keys: "Shift+→", Text: "extend it one cell right"},
			{Keys: "Shift+↑", Text: "extend it one line up"},
			{Keys: "Shift+↓", Text: "extend it one line down"},
			{Keys: "Shift+Home", Text: "extend it to the start of the line"},
			{Keys: "Shift+End", Text: "extend it to the end of the line"},
			{Keys: "Ctrl+Shift+←", Text: "extend it one word back"},
			{Keys: "Ctrl+Shift+→", Text: "extend it one word forward"},
			{Keys: "double click", Text: "select the word under the pointer"},
			{Keys: "triple click", Text: "select the whole line"},
			{Keys: "Ctrl+C", Text: "copy the selection, or quit when nothing is selected"},
		},
	},
	{
		Title: "a list walked by key",
		Entries: []HelpEntry{
			{Scope: cfg.ScopeList, Actions: []ActionID{ActionCursorUp, ActionCursorDown, ActionCursorPageUp, ActionCursorPageDown}, Text: "move in the list of a card"},
			{Scope: cfg.ScopeList, Actions: []ActionID{ActionCursorFirstRow, ActionCursorLastRow}, Text: "go to the first or the last row"},
			{Scope: cfg.ScopeList, Actions: []ActionID{ActionChooseRow}, Text: "choose the row under the cursor"},
		},
	},
	{
		Title: "cards, the picker and the forms",
		Entries: []HelpEntry{
			{Scope: cfg.ScopeDialog, Actions: []ActionID{ActionClose}, Text: "close the card, or cancel"},
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
			{Scope: cfg.ScopeDialog, Actions: []ActionID{ActionListSecondary}, Text: "the second action a list row offers"},
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
			{Scope: cfg.ScopeGlobal, Actions: []ActionID{ActionSendToAi}, Text: "put the statement of the editor into the chat field"},
			{Keys: "Enter", Text: "send the question"},
			{Keys: "Shift+Enter", Text: "a newline in the question"},
			{Keys: "Alt+Enter", Text: "a newline in the question"},
			{Scope: cfg.ScopeDialog, Actions: []ActionID{ActionInsertAiSQL}, Text: "put the query of the last reply into the editor"},
			{Scope: cfg.ScopeDialog, Actions: []ActionID{ActionStopAiReply}, Text: "stop the reply"},
			{Scope: cfg.ScopeDialog, Actions: []ActionID{ActionNewAiChat}, Text: "start a new conversation"},
			{Scope: cfg.ScopeDialog, Actions: []ActionID{ActionShowAiChats}, Text: "list the conversations of this profile"},
			{Scope: cfg.ScopeDialog, Actions: []ActionID{ActionPreviousTurn, ActionNextTurn}, Text: "go to the previous or the next turn"},
			{Scope: cfg.ScopeDialog, Actions: []ActionID{ActionScrollBack, ActionScrollForward}, Text: "scroll a page back or forward"},
			{Scope: cfg.ScopePlan, Actions: []ActionID{ActionAiCheckPlan}, Text: "send the plan to the chat, in the plan view"},
			{Scope: cfg.ScopeGlobal, Actions: []ActionID{ActionAiFixError}, Text: "explain the query that just failed"},
			{Keys: "", Text: "the command palette holds more AI prompts"},
		},
	},
}
