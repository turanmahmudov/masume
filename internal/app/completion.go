package app

import (
	"github.com/turanmahmudov/masume/internal/query/editor"
)

// CompletionList is the list of suggestions over the statement and the selected entry.
type CompletionList struct {
	Candidates []editor.Completion
	Selected   int
	// True while the user closed the list for the word at the caret.
	Dismissed bool
}

// IsListing is true while the list is displayed over the statement. An open list takes the
// arrow keys: it is at the position of the caret, and an arrow key that moved the caret would
// leave the list behind.
func (list *CompletionList) IsListing() bool {
	return len(list.Candidates) > 0
}

// Close hides the list.
func (list *CompletionList) Close() {
	list.Candidates = nil
	list.Selected = 0
}

// Dismiss closes the list for the word at the caret and stores that word, so the list does
// not open again before the word changes.
func (list *CompletionList) Dismiss() {
	list.Dismissed = true
	list.Close()
}

// Step selects another candidate and wraps at both ends.
func (list *CompletionList) Step(delta int) {
	count := len(list.Candidates)
	if count == 0 {
		return
	}
	list.Selected = ((list.Selected+delta)%count + count) % count
}

// Chosen returns the selected candidate, and whether there is one.
func (list *CompletionList) Chosen() (editor.Completion, bool) {
	if list.Selected < 0 || list.Selected >= len(list.Candidates) {
		return editor.Completion{}, false
	}
	return list.Candidates[list.Selected], true
}
