package app

import (
	"github.com/turanmahmudov/masume/internal/query/editor"
)

// CompletionList is the suggestions over the statement, and which one is marked.
type CompletionList struct {
	Candidates []editor.Completion
	Selected   int
	// True while the user closed the list for the word under the caret.
	Dismissed bool
}

// IsListing is true while the list stands over the statement. A list that stands over the
// statement owns the arrows: it is drawn where the reader is looking, and an arrow that moved
// the caret instead would leave the list behind.
func (list *CompletionList) IsListing() bool {
	return len(list.Candidates) > 0
}

// Close takes the list off the statement.
func (list *CompletionList) Close() {
	list.Candidates = nil
	list.Selected = 0
}

// Dismiss closes the list for the word under the caret, and remembers it, so a list
// is not offered again until the word changes.
func (list *CompletionList) Dismiss() {
	list.Dismissed = true
	list.Close()
}

// Step marks another candidate, and wraps at each end.
func (list *CompletionList) Step(delta int) {
	count := len(list.Candidates)
	if count == 0 {
		return
	}
	list.Selected = ((list.Selected+delta)%count + count) % count
}

// Chosen returns the marked candidate, and whether there is one.
func (list *CompletionList) Chosen() (editor.Completion, bool) {
	if list.Selected < 0 || list.Selected >= len(list.Candidates) {
		return editor.Completion{}, false
	}
	return list.Candidates[list.Selected], true
}
