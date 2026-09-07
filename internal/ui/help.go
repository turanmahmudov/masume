package ui

import "github.com/turanmahmudov/masume/internal/cfg"

// Help groups actions by task and reads key bindings from the registry.

// HelpEntry is one help row with an action and its keys.
type HelpEntry struct {
	Scope   cfg.KeyScope
	Actions []ActionID
	// Keys is the displayed text for keys outside the registry.
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
			{Scope: cfg.ScopeGlobal, Actions: []ActionID{ActionRevealSQL}, Text: "edit the query for this result"},
			{Scope: cfg.ScopeGlobal, Actions: []ActionID{ActionToggleSidebar}, Text: "show or hide the object tree"},
			{Scope: cfg.ScopeGlobal, Actions: []ActionID{ActionToggleResult}, Text: "show or hide the result"},
		},
	},
	{
		Title: "connections",
		Entries: []HelpEntry{
			{Scope: cfg.ScopeGlobal, Actions: []ActionID{ActionOpenPicker}, Text: "open the connection picker"},
			{Scope: cfg.ScopeGlobal, Actions: []ActionID{ActionPreviousConnection, ActionNextConnection}, Text: "switch connection"},
			{Scope: cfg.ScopeGlobal, Actions: []ActionID{ActionCloseConnection}, Text: "close the connection and all its tabs"},
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
			{Scope: cfg.ScopeTree, Actions: []ActionID{ActionOpenNode}, Text: "open the object under the cursor"},
			{Scope: cfg.ScopeTree, Actions: []ActionID{ActionOpenInNewTab}, Text: "open a second tab on the same table"},
			{Scope: cfg.ScopeTree, Actions: []ActionID{ActionDescribeTable}, Text: "describe the table"},
			{Scope: cfg.ScopeTree, Actions: []ActionID{ActionObjectMenu}, Text: "open the object menu"},
			{Scope: cfg.ScopeTree, Actions: []ActionID{ActionFilterTree}, Text: "filter the tree"},
			{Scope: cfg.ScopeTree, Actions: []ActionID{ActionToggleFavourite}, Text: "mark this object as a favourite"},
			{Scope: cfg.ScopeTree, Actions: []ActionID{ActionToggleSystemSchemas}, Text: "show or hide the system schemas"},
			{Scope: cfg.ScopeGlobal, Actions: []ActionID{ActionRefreshObjects}, Text: "refresh the object tree"},
		},
	},
	{
		Title: "grid",
		Entries: []HelpEntry{
			{Scope: cfg.ScopeGrid, Actions: []ActionID{ActionCursorUp, ActionCursorDown, ActionCursorPageUp, ActionCursorPageDown}, Text: "go to another row"},
			{Scope: cfg.ScopeGrid, Actions: []ActionID{ActionCursorLeft, ActionCursorRight}, Text: "go to another column"},
			{Scope: cfg.ScopeGrid, Actions: []ActionID{ActionCursorFirstRow, ActionCursorLastRow}, Text: "go to the first or the last row"},
			{Scope: cfg.ScopeGrid, Actions: []ActionID{ActionOpenRow}, Text: "open the row under the cursor"},
			{Scope: cfg.ScopeGrid, Actions: []ActionID{ActionViewCell}, Text: "open the cell viewer"},
			{Scope: cfg.ScopeDialog, Actions: []ActionID{ActionCopyValue}, Text: "copy the value from the cell viewer"},
			{Scope: cfg.ScopeGrid, Actions: []ActionID{ActionFollowForeignKey}, Text: "open the row referenced by the foreign key"},
			{Scope: cfg.ScopeGrid, Actions: []ActionID{ActionCopyMenu}, Text: "copy: cell, row, or the whole result"},
			{Scope: cfg.ScopeGrid, Actions: []ActionID{ActionOpenMenu}, Text: "open the row and cell menu"},
			{Scope: cfg.ScopeGrid, Actions: []ActionID{ActionCountRows}, Text: "count all result rows"},
			{Scope: cfg.ScopeGrid, Actions: []ActionID{ActionGoToColumn}, Text: "go to a column by name"},
			{Scope: cfg.ScopeGrid, Actions: []ActionID{ActionSearchColumns}, Text: "search the rows on screen"},
			{Scope: cfg.ScopeGrid, Actions: []ActionID{ActionFreezeColumns}, Text: "freeze the column under the cursor"},
			{Scope: cfg.ScopeGrid, Actions: []ActionID{ActionToggleMasking}, Text: "show or hide masked values"},
			{Keys: "drag", Text: "select text under the pointer"},
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
			{Scope: cfg.ScopeGlobal, Actions: []ActionID{ActionPreviousStatement, ActionNextStatement}, Text: "previous or next statement in a batch"},
			{Scope: cfg.ScopeGlobal, Actions: []ActionID{ActionExplain}, Text: "explain the plan"},
			{Scope: cfg.ScopeGlobal, Actions: []ActionID{ActionExplainAnalyze}, Text: "explain analyze"},
			{Scope: cfg.ScopeGlobal, Actions: []ActionID{ActionCancelQuery}, Text: "cancel the running query"},
			{Scope: cfg.ScopeGlobal, Actions: []ActionID{ActionNextPage}, Text: "fetch more rows"},
			{Scope: cfg.ScopeDocument, Actions: []ActionID{ActionOpenNode}, Text: "open or fold the current document"},
			{Scope: cfg.ScopeDocument, Actions: []ActionID{ActionUnfoldRow, ActionFoldRow}, Text: "open a field, or fold it and step out"},
			{Scope: cfg.ScopeDocument, Actions: []ActionID{ActionCopyValue}, Text: "copy the value under the cursor"},
			{Scope: cfg.ScopeDocument, Actions: []ActionID{ActionCopyPath}, Text: "copy the field name under the cursor"},
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
		Title: "notebooks",
		Entries: []HelpEntry{
			{Scope: cfg.ScopeGlobal, Actions: []ActionID{ActionNewNotebookTab}, Text: "new notebook tab"},
			{Scope: cfg.ScopeGlobal, Actions: []ActionID{ActionShowNotebooks}, Text: "the notebooks of the project and of the user"},
			{Scope: cfg.ScopeGlobal, Actions: []ActionID{ActionSaveQuery}, Text: "save this notebook to its file"},
			{Scope: cfg.ScopeGlobal, Actions: []ActionID{ActionNotebookRunPolicy}, Text: "transaction and error policy of this notebook"},
			{Scope: cfg.ScopeNotebook, Actions: []ActionID{ActionCursorUp, ActionCursorDown}, Text: "move between cells"},
			{Scope: cfg.ScopeNotebook, Actions: []ActionID{ActionEditCellSource}, Text: "put the caret in the focused cell"},
			{Keys: "Esc", Text: "leave the cell and go back to the list"},
			{Scope: cfg.ScopeNotebook, Actions: []ActionID{ActionRunCell}, Text: "run the focused cell"},
			{Scope: cfg.ScopeNotebook, Actions: []ActionID{ActionRunFromCell}, Text: "run the focused cell and every cell below it"},
			{Scope: cfg.ScopeGlobal, Actions: []ActionID{ActionRunBatch}, Text: "run every cell"},
			{Scope: cfg.ScopeNotebook, Actions: []ActionID{ActionMarkCell}, Text: "mark a cell for a partial run"},
			{Scope: cfg.ScopeNotebook, Actions: []ActionID{ActionRunMarkedCells}, Text: "run the marked cells"},
			{Scope: cfg.ScopeNotebook, Actions: []ActionID{ActionAddCellBelow, ActionAddCellAbove}, Text: "add a cell below or above"},
			{Scope: cfg.ScopeNotebook, Actions: []ActionID{ActionSetCellKind}, Text: "sql, md, param, or chart"},
			{Scope: cfg.ScopeNotebook, Actions: []ActionID{ActionNameCell}, Text: "name the focused cell"},
			{Scope: cfg.ScopeNotebook, Actions: []ActionID{ActionDeleteCell}, Text: "delete the focused cell"},
			{Scope: cfg.ScopeNotebook, Actions: []ActionID{ActionUndoCellChange, ActionRedoCellChange}, Text: "undo or redo a change of the cell list"},
			{Scope: cfg.ScopeNotebook, Actions: []ActionID{ActionMoveCellUp, ActionMoveCellDown}, Text: "move the focused cell"},
			{Scope: cfg.ScopeNotebook, Actions: []ActionID{ActionCopyCell, ActionCutCell}, Text: "copy or cut the focused cell"},
			{Scope: cfg.ScopeNotebook, Actions: []ActionID{ActionPasteCell}, Text: "paste the cell below the focused one"},
			{Scope: cfg.ScopeNotebook, Actions: []ActionID{ActionToggleCellOutput, ActionToggleEveryOutput}, Text: "fold one cell, or every cell"},
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
			{Scope: cfg.ScopeDialog, Actions: []ActionID{ActionApplyChanges}, Text: "apply staged changes from the review"},
			{Scope: cfg.ScopeDialog, Actions: []ActionID{ActionDiscardChanges}, Text: "discard staged changes from the review"},
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
			{Scope: cfg.ScopeEditor, Actions: []ActionID{ActionPasteText}, Text: "paste the text last copied in this client"},
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
		Title: "lists",
		Entries: []HelpEntry{
			{Scope: cfg.ScopeList, Actions: []ActionID{ActionCursorUp, ActionCursorDown, ActionCursorPageUp, ActionCursorPageDown}, Text: "move through a dialog list"},
			{Scope: cfg.ScopeList, Actions: []ActionID{ActionCursorFirstRow, ActionCursorLastRow}, Text: "go to the first or the last row"},
			{Scope: cfg.ScopeList, Actions: []ActionID{ActionChooseRow}, Text: "choose the row under the cursor"},
		},
	},
	{
		Title: "dialogs and forms",
		Entries: []HelpEntry{
			{Scope: cfg.ScopeDialog, Actions: []ActionID{ActionClose}, Text: "close the card, or cancel"},
			{Scope: cfg.ScopeDialog, Actions: []ActionID{ActionAnswerYes}, Text: "confirm the action"},
			{Scope: cfg.ScopeDialog, Actions: []ActionID{ActionAnswerNo}, Text: "decline the action"},
			{Scope: cfg.ScopeDialog, Actions: []ActionID{ActionNewConnection}, Text: "add a connection in the picker"},
			{Scope: cfg.ScopeDialog, Actions: []ActionID{ActionEditConnection}, Text: "edit the selected connection"},
			{Scope: cfg.ScopeDialog, Actions: []ActionID{ActionDeleteConnection}, Text: "delete the selected connection"},
			{Scope: cfg.ScopeDialog, Actions: []ActionID{ActionTestConnection}, Text: "test the connection, in the form"},
			{Scope: cfg.ScopeDialog, Actions: []ActionID{ActionSaveForm}, Text: "save the connection form"},
			{Scope: cfg.ScopeDialog, Actions: []ActionID{ActionSaveCell}, Text: "stage the cell edit"},
			{Scope: cfg.ScopeDialog, Actions: []ActionID{ActionSetNull}, Text: "stage NULL for the cell"},
			{Scope: cfg.ScopeDialog, Actions: []ActionID{ActionSetEmpty}, Text: "stage an empty value for the cell"},
			{Scope: cfg.ScopeDialog, Actions: []ActionID{ActionSetDefault}, Text: "stage DEFAULT for the cell"},
			{Scope: cfg.ScopeDialog, Actions: []ActionID{ActionPrettifyJSON}, Text: "format the JSON"},
			{Scope: cfg.ScopeDialog, Actions: []ActionID{ActionRunWithValues}, Text: "run, in the values form"},
			{Scope: cfg.ScopeDialog, Actions: []ActionID{ActionWriteExport}, Text: "write the file, in the export form"},
			{Scope: cfg.ScopeDialog, Actions: []ActionID{ActionOpenInNewTab}, Text: "open a history query in a new tab"},
			{Scope: cfg.ScopeDialog, Actions: []ActionID{ActionListSecondary}, Text: "run the secondary list action"},
			{Scope: cfg.ScopeDialog, Actions: []ActionID{ActionStopSession}, Text: "stop the selected session's statement"},
			{Scope: cfg.ScopeDialog, Actions: []ActionID{ActionToggleValue}, Text: "keep or drop a value, in the picker"},
			{Scope: cfg.ScopeDialog, Actions: []ActionID{ActionKeepOnlyValue}, Text: "keep only the value under the cursor"},
			{Scope: cfg.ScopeDialog, Actions: []ActionID{ActionKeepAllValues}, Text: "keep every value again"},
		},
	},
	{
		Title: "ai chat",
		Entries: []HelpEntry{
			{Scope: cfg.ScopeGlobal, Actions: []ActionID{ActionShowAiChat}, Text: "open AI chat"},
			{Scope: cfg.ScopeGlobal, Actions: []ActionID{ActionSendToAi}, Text: "copy the editor query into the chat field"},
			{Keys: "Enter", Text: "send the question"},
			{Keys: "Shift+Enter", Text: "a newline in the question"},
			{Keys: "Alt+Enter", Text: "a newline in the question"},
			{Scope: cfg.ScopeDialog, Actions: []ActionID{ActionInsertAiSQL}, Text: "insert the last reply's query into the editor, or as a cell"},
			{Scope: cfg.ScopeDialog, Actions: []ActionID{ActionChatToNotebook}, Text: "turn this conversation into a notebook"},
			{Scope: cfg.ScopeDialog, Actions: []ActionID{ActionStopAiReply}, Text: "stop the reply"},
			{Scope: cfg.ScopeDialog, Actions: []ActionID{ActionNewAiChat}, Text: "start a new conversation"},
			{Scope: cfg.ScopeDialog, Actions: []ActionID{ActionShowAiChats}, Text: "list conversations for this profile"},
			{Scope: cfg.ScopeDialog, Actions: []ActionID{ActionPreviousTurn, ActionNextTurn}, Text: "go to the previous or the next turn"},
			{Scope: cfg.ScopeDialog, Actions: []ActionID{ActionScrollBack, ActionScrollForward}, Text: "scroll a page back or forward"},
			{Scope: cfg.ScopePlan, Actions: []ActionID{ActionAiCheckPlan}, Text: "send the plan to the chat, in the plan view"},
			{Scope: cfg.ScopeGlobal, Actions: []ActionID{ActionAiFixError}, Text: "explain the query that just failed"},
			{Keys: "", Text: "more AI prompts in the command palette"},
		},
	},
}
