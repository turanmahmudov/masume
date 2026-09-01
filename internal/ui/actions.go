package ui

import (
	"github.com/turanmahmudov/masume/internal/cfg"
	"github.com/turanmahmudov/masume/internal/core"
)

// ActionID names one action the app returns for.
type ActionID string

// Capability names what a server must answer for before an action is offered.
type Capability string

// The capabilities an action can need.
const (
	NeedsNothing        Capability = ""
	NeedsPlansStatement Capability = "plansStatement"
	NeedsMeasuresPlan   Capability = "measuresPlan"
	NeedsServerSessions Capability = "hasServerSessions"
	NeedsCancelsRunning Capability = "cancelsRunningQuery"
	NeedsTransactions   Capability = "hasTransactions"
	NeedsSortsRead      Capability = "sortsRead"
	NeedsTruncatesTable Capability = "truncatesTable"
	NeedsWritesDDL      Capability = "writesDdl"
)

// AnswersFor is true if the server has the capability the action needs. An action that needs
// nothing is always true.
func AnswersFor(capabilities core.Capabilities, needs Capability) bool {
	switch needs {
	case NeedsNothing:
		return true
	case NeedsPlansStatement:
		return capabilities.PlansStatement
	case NeedsMeasuresPlan:
		return capabilities.MeasuresPlan
	case NeedsServerSessions:
		return capabilities.HasServerSessions
	case NeedsCancelsRunning:
		return capabilities.CancelsRunningQuery
	case NeedsTransactions:
		return capabilities.HasTransactions
	case NeedsSortsRead:
		return capabilities.SortsRead
	case NeedsTruncatesTable:
		return capabilities.TruncatesTable
	case NeedsWritesDDL:
		return capabilities.WritesDDL
	}
	// A capability no arm answers for is one the catalog names and this switch does not,
	// so the action stays out of reach rather than being offered on every server.
	return false
}

// ActionDefinition says what an action may do, apart from the chord that runs it. The
// registry holds the chord, so a rebound key keeps the same rules.
type ActionDefinition struct {
	ID    ActionID
	Scope cfg.KeyScope
	// A server without this capability leaves the chord unbound and the hint hidden.
	Needs Capability
	// True for an action that works while a query runs, such as a cancel.
	WhileRunning bool
	// True for an action that works while a field or the editor holds the caret.
	WhileTyping bool
	// True for an action on the statement in the editor. It works only while the editor
	// holds the caret.
	EditorOnly bool
	// True for an action whose result goes into the result pane. A hidden result is shown
	// again before one of these runs.
	AnswersInResult bool
}

// The ids of every action, so a typo is a build error rather than a key that silently does
// nothing.
const (
	ActionRunBatch            ActionID = "run-batch"
	ActionNewQueryTab         ActionID = "new-query-tab"
	ActionToggleSidebar       ActionID = "toggle-sidebar"
	ActionToggleResult        ActionID = "toggle-result"
	ActionRevealSQL           ActionID = "reveal-sql"
	ActionNameTab             ActionID = "name-tab"
	ActionCloseTab            ActionID = "close-tab"
	ActionReopenTab           ActionID = "reopen-tab"
	ActionPreviousTab         ActionID = "previous-tab"
	ActionNextTab             ActionID = "next-tab"
	ActionPreviousConnection  ActionID = "previous-connection"
	ActionNextConnection      ActionID = "next-connection"
	ActionActivateTab         ActionID = "activate-tab"
	ActionRunAtCursor         ActionID = "run-at-cursor"
	ActionExplain             ActionID = "explain"
	ActionExplainAnalyze      ActionID = "explain-analyze"
	ActionShowHistory         ActionID = "show-history"
	ActionShowSaved           ActionID = "show-saved"
	ActionCancelQuery         ActionID = "cancel-query"
	ActionShowPalette         ActionID = "show-palette"
	ActionShowAiChat          ActionID = "show-ai-chat"
	ActionAiFixError          ActionID = "ai-fix-error"
	ActionSendToAi            ActionID = "send-to-ai"
	ActionNextPage            ActionID = "next-page"
	ActionExportCSV           ActionID = "export-csv"
	ActionExportJSON          ActionID = "export-json"
	ActionBeginTransaction    ActionID = "begin-transaction"
	ActionCommitTransaction   ActionID = "commit-transaction"
	ActionRollbackTransaction ActionID = "rollback-transaction"
	ActionToggleAutocommit    ActionID = "toggle-autocommit"
	ActionOpenPicker          ActionID = "open-picker"
	ActionCloseConnection     ActionID = "close-connection"
	ActionShowHelp            ActionID = "show-help"
	ActionFocusNextPane       ActionID = "focus-next-pane"
	ActionFocusPreviousPane   ActionID = "focus-previous-pane"
	ActionPreviousStatement   ActionID = "previous-statement"
	ActionNextStatement       ActionID = "next-statement"
	ActionRefreshObjects      ActionID = "refresh-objects"
	ActionSelectView          ActionID = "select-view"
	ActionPreviousView        ActionID = "previous-view"
	ActionNextView            ActionID = "next-view"
	ActionSaveQuery           ActionID = "save-query"
	ActionFocusSidebar        ActionID = "focus-sidebar"
	ActionFocusEditor         ActionID = "focus-editor"
	ActionFocusResult         ActionID = "focus-result"
	ActionShowActivity        ActionID = "show-activity"
	ActionShowThemes          ActionID = "show-themes"

	// The statement being written. A move takes the selection with it while Shift is held,
	// so one action returns a chord and its shifted twin.
	ActionCaretLeft          ActionID = "caret-left"
	ActionCaretRight         ActionID = "caret-right"
	ActionCaretUp            ActionID = "caret-up"
	ActionCaretDown          ActionID = "caret-down"
	ActionCaretWordLeft      ActionID = "caret-word-left"
	ActionCaretWordRight     ActionID = "caret-word-right"
	ActionCaretLineStart     ActionID = "caret-line-start"
	ActionCaretLineEnd       ActionID = "caret-line-end"
	ActionCaretTextStart     ActionID = "caret-text-start"
	ActionCaretTextEnd       ActionID = "caret-text-end"
	ActionCaretPageUp        ActionID = "caret-page-up"
	ActionCaretPageDown      ActionID = "caret-page-down"
	ActionSelectAll          ActionID = "select-all"
	ActionDeleteBack         ActionID = "delete-back"
	ActionDeleteForward      ActionID = "delete-forward"
	ActionDeleteWordBack     ActionID = "delete-word-back"
	ActionDeleteWordForward  ActionID = "delete-word-forward"
	ActionOpenLine           ActionID = "open-line"
	ActionUndoEdit           ActionID = "undo-edit"
	ActionRedoEdit           ActionID = "redo-edit"
	ActionPasteText          ActionID = "paste-text"
	ActionFormatSQL          ActionID = "format-sql"
	ActionCommentLines       ActionID = "comment-lines"
	ActionIndentLines        ActionID = "indent-lines"
	ActionOutdentLines       ActionID = "outdent-lines"
	ActionFindInStatement    ActionID = "find-in-statement"
	ActionReplaceInStatement ActionID = "replace-in-statement"
	ActionNextMatch          ActionID = "next-match"
	ActionPreviousMatch      ActionID = "previous-match"
	ActionNextProblem        ActionID = "next-problem"

	ActionCursorUp         ActionID = "cursor-up"
	ActionCursorDown       ActionID = "cursor-down"
	ActionCursorPageUp     ActionID = "cursor-page-up"
	ActionCursorPageDown   ActionID = "cursor-page-down"
	ActionCursorFirstRow   ActionID = "cursor-first-row"
	ActionCursorLastRow    ActionID = "cursor-last-row"
	ActionCursorLeft       ActionID = "cursor-left"
	ActionCursorRight      ActionID = "cursor-right"
	ActionSortColumn       ActionID = "sort-column"
	ActionAddSortColumn    ActionID = "add-sort-column"
	ActionOpenRow          ActionID = "open-row"
	ActionViewCell         ActionID = "view-cell"
	ActionEditCell         ActionID = "edit-cell"
	ActionToggleDelete     ActionID = "toggle-delete"
	ActionDuplicateRow     ActionID = "duplicate-row"
	ActionReviewChanges    ActionID = "review-changes"
	ActionUndoChange       ActionID = "undo-change"
	ActionRedoChange       ActionID = "redo-change"
	ActionCountRows        ActionID = "count-rows"
	ActionFollowForeignKey ActionID = "follow-foreign-key"
	ActionInsertRow        ActionID = "insert-row"
	ActionCopyMenu         ActionID = "copy-menu"
	ActionCopyCSV          ActionID = "copy-csv"
	ActionCopyJSON         ActionID = "copy-json"
	ActionCopyMarkdown     ActionID = "copy-markdown"
	ActionCopyInserts      ActionID = "copy-inserts"
	ActionOpenMenu         ActionID = "open-menu"
	ActionFilterByCell     ActionID = "filter-by-cell"
	ActionFilterByValues   ActionID = "filter-by-values"
	ActionExcludeCell      ActionID = "exclude-cell"
	ActionClearRewrites    ActionID = "clear-rewrites"
	ActionPopFilter        ActionID = "pop-filter"
	ActionFreezeColumns    ActionID = "freeze-columns"
	ActionToggleMasking    ActionID = "toggle-masking"
	ActionGoToColumn       ActionID = "go-to-column"
	ActionSearchColumns    ActionID = "search-columns"
	ActionFilterWhere      ActionID = "filter-where"

	ActionToggleRawPlan ActionID = "toggle-raw-plan"
	ActionCopyPlan      ActionID = "copy-plan"
	ActionAiCheckPlan   ActionID = "ai-check-plan"

	ActionCopyPath ActionID = "copy-path"

	ActionFoldRow             ActionID = "fold-row"
	ActionUnfoldRow           ActionID = "unfold-row"
	ActionOpenNode            ActionID = "open-node"
	ActionOpenInNewTab        ActionID = "open-in-new-tab"
	ActionDescribeTable       ActionID = "describe-table"
	ActionObjectMenu          ActionID = "object-menu"
	ActionFilterTree          ActionID = "filter-tree"
	ActionToggleFavourite     ActionID = "toggle-favourite"
	ActionToggleSystemSchemas ActionID = "toggle-system-schemas"

	ActionChooseRow ActionID = "choose-row"

	ActionClose            ActionID = "close"
	ActionAnswerYes        ActionID = "answer-yes"
	ActionAnswerNo         ActionID = "answer-no"
	ActionNewConnection    ActionID = "new-connection"
	ActionEditConnection   ActionID = "edit-connection"
	ActionDeleteConnection ActionID = "delete-connection"
	ActionSaveForm         ActionID = "save-form"
	ActionTestConnection   ActionID = "test-connection"
	ActionSaveCell         ActionID = "save-cell"
	ActionPrettifyJSON     ActionID = "prettify-json"
	ActionSetNull          ActionID = "set-null"
	ActionSetEmpty         ActionID = "set-empty"
	ActionSetDefault       ActionID = "set-default"
	ActionRunWithValues    ActionID = "run-with-values"
	ActionWriteExport      ActionID = "write-export"
	ActionCopyValue        ActionID = "copy-value"
	ActionListSecondary    ActionID = "list-secondary"
	ActionToggleValue      ActionID = "toggle-value"
	ActionKeepAllValues    ActionID = "keep-all-values"
	ActionKeepOnlyValue    ActionID = "keep-only-value"
	ActionApplyChanges     ActionID = "apply-changes"
	ActionDiscardChanges   ActionID = "discard-changes"
	ActionInsertAiSQL      ActionID = "insert-ai-sql"
	ActionStopAiReply      ActionID = "stop-ai-reply"
	ActionNewAiChat        ActionID = "new-ai-chat"
	ActionShowAiChats      ActionID = "show-ai-chats"
	ActionScrollBack       ActionID = "scroll-back"
	ActionScrollForward    ActionID = "scroll-forward"
	ActionPreviousTurn     ActionID = "previous-turn"
	ActionNextTurn         ActionID = "next-turn"
)

// globalActions are the ones the workspace handles wherever the focus is.
var globalActions = []ActionDefinition{
	{ID: ActionRunBatch, WhileRunning: true, AnswersInResult: true},
	{ID: ActionNewQueryTab, WhileRunning: true},
	{ID: ActionToggleSidebar, WhileRunning: true},
	{ID: ActionToggleResult, WhileRunning: true},
	{ID: ActionRevealSQL, WhileRunning: true},
	{ID: ActionNameTab, WhileRunning: true},
	{ID: ActionCloseTab, WhileRunning: true},
	{ID: ActionReopenTab, WhileRunning: true},
	{ID: ActionPreviousTab, WhileRunning: true},
	{ID: ActionNextTab, WhileRunning: true},
	{ID: ActionPreviousConnection, WhileRunning: true},
	{ID: ActionNextConnection, WhileRunning: true},
	{ID: ActionActivateTab, WhileRunning: true},

	{ID: ActionRunAtCursor, WhileRunning: true, AnswersInResult: true},
	{ID: ActionExplain, Needs: NeedsPlansStatement, WhileRunning: true, AnswersInResult: true},
	{ID: ActionExplainAnalyze, Needs: NeedsMeasuresPlan, WhileRunning: true, AnswersInResult: true},
	{ID: ActionShowHistory, WhileRunning: true},
	{ID: ActionShowSaved, WhileRunning: true},
	{ID: ActionSaveQuery, WhileRunning: true, EditorOnly: true},
	{ID: ActionShowActivity, Needs: NeedsServerSessions, WhileRunning: true},
	{ID: ActionShowThemes, WhileRunning: true},
	{ID: ActionFocusSidebar, WhileRunning: true},
	{ID: ActionFocusEditor, WhileRunning: true},
	{ID: ActionFocusResult, WhileRunning: true},
	{ID: ActionCancelQuery, Needs: NeedsCancelsRunning, WhileRunning: true},
	{ID: ActionShowPalette, WhileRunning: true},
	{ID: ActionShowAiChat, WhileRunning: true},
	{ID: ActionAiFixError, WhileRunning: true},
	{ID: ActionSendToAi, WhileRunning: true, EditorOnly: true},
	{ID: ActionNextPage, WhileRunning: true, AnswersInResult: true},
	{ID: ActionExportCSV, WhileRunning: true},
	{ID: ActionExportJSON, WhileRunning: true},
	{ID: ActionBeginTransaction, Needs: NeedsTransactions, WhileRunning: true},
	{ID: ActionCommitTransaction, Needs: NeedsTransactions, WhileRunning: true},
	{ID: ActionRollbackTransaction, Needs: NeedsTransactions, WhileRunning: true},
	{ID: ActionToggleAutocommit, Needs: NeedsTransactions, WhileRunning: true},
	{ID: ActionOpenPicker, WhileRunning: true},
	{ID: ActionCloseConnection, WhileRunning: true},

	// The help is most useful while the user waits for the server.
	{ID: ActionShowHelp, WhileRunning: true},

	// These leave whatever holds the caret, so both work from inside it.
	{ID: ActionFocusNextPane, WhileRunning: true, WhileTyping: true},
	{ID: ActionFocusPreviousPane, WhileRunning: true, WhileTyping: true},

	{ID: ActionPreviousStatement},
	{ID: ActionNextStatement},
	{ID: ActionRefreshObjects},
	{ID: ActionSelectView},
	{ID: ActionPreviousView},
	{ID: ActionNextView},
}

// gridActions work only on a result the grid already drew.
var gridActions = []ActionDefinition{
	{ID: ActionCursorUp}, {ID: ActionCursorDown},
	{ID: ActionCursorPageUp}, {ID: ActionCursorPageDown},
	{ID: ActionCursorFirstRow}, {ID: ActionCursorLastRow},
	{ID: ActionCursorLeft}, {ID: ActionCursorRight},
	{ID: ActionSortColumn, Needs: NeedsSortsRead},
	{ID: ActionAddSortColumn, Needs: NeedsSortsRead},
	{ID: ActionOpenRow}, {ID: ActionViewCell}, {ID: ActionEditCell},
	{ID: ActionToggleDelete}, {ID: ActionDuplicateRow}, {ID: ActionReviewChanges},
	{ID: ActionUndoChange}, {ID: ActionRedoChange},
	{ID: ActionCountRows, AnswersInResult: true},
	{ID: ActionFollowForeignKey}, {ID: ActionInsertRow},
	{ID: ActionCopyMenu}, {ID: ActionOpenMenu},
	{ID: ActionCopyCSV}, {ID: ActionCopyJSON},
	{ID: ActionCopyMarkdown}, {ID: ActionCopyInserts},
	{ID: ActionDiscardChanges},
	{ID: ActionFilterByCell}, {ID: ActionFilterByValues}, {ID: ActionExcludeCell},
	{ID: ActionClearRewrites}, {ID: ActionPopFilter},
	{ID: ActionFreezeColumns}, {ID: ActionToggleMasking},
	{ID: ActionGoToColumn}, {ID: ActionSearchColumns}, {ID: ActionFilterWhere},
}

// planActions answer while the plan view is drawn in place of the grid.
var planActions = []ActionDefinition{
	{ID: ActionToggleRawPlan}, {ID: ActionCopyPlan},
	{ID: ActionAiCheckPlan, Needs: NeedsPlansStatement},
}

// documentActions answer while the tree that opens the rows as documents is drawn. It holds
// a cursor and folds like the object tree, and it copies like the grid, so it answers to both
// kinds of key under bindings of its own.
var documentActions = []ActionDefinition{
	{ID: ActionCursorUp}, {ID: ActionCursorDown},
	{ID: ActionCursorPageUp}, {ID: ActionCursorPageDown},
	{ID: ActionCursorFirstRow}, {ID: ActionCursorLastRow},
	{ID: ActionFoldRow}, {ID: ActionUnfoldRow}, {ID: ActionOpenNode},
	{ID: ActionCopyValue}, {ID: ActionCopyPath},
	{ID: ActionSearchColumns}, {ID: ActionCountRows},
	{ID: ActionClearRewrites}, {ID: ActionPopFilter},
}

// treeActions answer while the object tree holds the caret.
var treeActions = []ActionDefinition{
	{ID: ActionCursorUp}, {ID: ActionCursorDown},
	{ID: ActionCursorPageUp}, {ID: ActionCursorPageDown},
	{ID: ActionCursorFirstRow}, {ID: ActionCursorLastRow},
	{ID: ActionFoldRow}, {ID: ActionUnfoldRow},
	{ID: ActionOpenNode}, {ID: ActionOpenInNewTab}, {ID: ActionDescribeTable},
	{ID: ActionObjectMenu}, {ID: ActionFilterTree},
	{ID: ActionToggleFavourite}, {ID: ActionToggleSystemSchemas},
}

// editorActions answer while the statement being written holds the caret. A move is bound to
// its plain chord and to its shifted twin, and the shifted one takes the selection along.
var editorActions = []ActionDefinition{
	{ID: ActionCaretLeft}, {ID: ActionCaretRight},
	{ID: ActionCaretUp}, {ID: ActionCaretDown},
	{ID: ActionCaretWordLeft}, {ID: ActionCaretWordRight},
	{ID: ActionCaretLineStart}, {ID: ActionCaretLineEnd},
	{ID: ActionCaretTextStart}, {ID: ActionCaretTextEnd},
	{ID: ActionCaretPageUp}, {ID: ActionCaretPageDown},
	{ID: ActionSelectAll},
	{ID: ActionDeleteBack}, {ID: ActionDeleteForward},
	{ID: ActionDeleteWordBack}, {ID: ActionDeleteWordForward},
	{ID: ActionOpenLine},
	{ID: ActionUndoEdit}, {ID: ActionRedoEdit}, {ID: ActionPasteText},
	{ID: ActionFormatSQL}, {ID: ActionCommentLines},
	{ID: ActionIndentLines}, {ID: ActionOutdentLines},
	{ID: ActionFindInStatement},
	{ID: ActionNextMatch}, {ID: ActionPreviousMatch},
	{ID: ActionNextProblem},
}

// listActions move any list moved by keys that is not the grid or the tree: palette,
// history, saved queries, connections, column values. One preset moves them all.
var listActions = []ActionDefinition{
	{ID: ActionCursorUp}, {ID: ActionCursorDown},
	{ID: ActionCursorPageUp}, {ID: ActionCursorPageDown},
	{ID: ActionCursorFirstRow}, {ID: ActionCursorLastRow},
	{ID: ActionChooseRow},
}

// dialogActions answer while an overlay, the picker or a form is open, which owns the
// keyboard.
var dialogActions = []ActionDefinition{
	// The find field turns into the replace field, so replacing is bound where that
	// field stands rather than in the editor.
	{ID: ActionReplaceInStatement},
	{ID: ActionClose}, {ID: ActionAnswerYes}, {ID: ActionAnswerNo},
	{ID: ActionNewConnection}, {ID: ActionEditConnection}, {ID: ActionDeleteConnection},
	{ID: ActionSaveForm}, {ID: ActionTestConnection},
	{ID: ActionSaveCell}, {ID: ActionPrettifyJSON},
	{ID: ActionSetNull}, {ID: ActionSetEmpty}, {ID: ActionSetDefault},
	{ID: ActionRunWithValues}, {ID: ActionWriteExport}, {ID: ActionCopyValue},
	{ID: ActionOpenInNewTab}, {ID: ActionListSecondary},
	{ID: ActionToggleValue}, {ID: ActionKeepAllValues}, {ID: ActionKeepOnlyValue},
	{ID: ActionApplyChanges}, {ID: ActionDiscardChanges},
	{ID: ActionInsertAiSQL}, {ID: ActionStopAiReply},
	{ID: ActionNewAiChat}, {ID: ActionShowAiChats},
	{ID: ActionScrollBack}, {ID: ActionScrollForward},
	{ID: ActionPreviousTurn}, {ID: ActionNextTurn},
}

// ActionCatalog holds every action of every scope.
var ActionCatalog = func() []ActionDefinition {
	catalog := []ActionDefinition{}
	add := func(scope cfg.KeyScope, actions []ActionDefinition) {
		for _, action := range actions {
			action.Scope = scope
			catalog = append(catalog, action)
		}
	}
	add(cfg.ScopeGlobal, globalActions)
	add(cfg.ScopeGrid, gridActions)
	add(cfg.ScopePlan, planActions)
	add(cfg.ScopeDocument, documentActions)
	add(cfg.ScopeTree, treeActions)
	add(cfg.ScopeEditor, editorActions)
	add(cfg.ScopeList, listActions)
	add(cfg.ScopeDialog, dialogActions)
	return catalog
}()

var actionsByKey = func() map[string]ActionDefinition {
	byKey := map[string]ActionDefinition{}
	for _, action := range ActionCatalog {
		byKey[cfg.BuildActionKey(action.Scope, string(action.ID))] = action
	}
	return byKey
}()

// collectScopeActions returns the id of every action a scope holds.
func collectScopeActions(scope cfg.KeyScope) []ActionID {
	ids := []ActionID{}
	for _, action := range ActionCatalog {
		if action.Scope == scope {
			ids = append(ids, action.ID)
		}
	}
	return ids
}

// FindAction returns the definition of the action, or nothing if no scope has it.
func FindAction(scope cfg.KeyScope, id ActionID) (ActionDefinition, bool) {
	action, known := actionsByKey[cfg.BuildActionKey(scope, string(id))]
	return action, known
}

// FindActionID reads this text as an action of the catalog.
func FindActionID(written string) (ActionID, bool) {
	for _, action := range ActionCatalog {
		if string(action.ID) == written {
			return action.ID, true
		}
	}
	return "", false
}

// AnswersInResult is true if the result of this action goes into the result pane. The same id
// stands in more than one scope, so the scope picks which definition answers.
func AnswersInResult(scope cfg.KeyScope, id ActionID) bool {
	action, known := FindAction(scope, id)
	return known && action.AnswersInResult
}

// FindActionCapability returns the capability this action needs in this scope, or nothing if
// every server has it.
func FindActionCapability(scope cfg.KeyScope, id ActionID) Capability {
	action, known := FindAction(scope, id)
	if !known {
		return NeedsNothing
	}
	return action.Needs
}
