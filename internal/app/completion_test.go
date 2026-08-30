package app

import (
	"testing"

	"github.com/turanmahmudov/masume/internal/query/editor"
)

func TestCompletionListStepsRoundTheEnds(t *testing.T) {
	list := CompletionList{Candidates: []editor.Completion{
		{Text: "id"}, {Text: "name"}, {Text: "total"},
	}}
	if !list.IsListing() {
		t.Fatal("a list with candidates is not listing")
	}
	chosen, found := list.Chosen()
	if !found || chosen.Text != "id" {
		t.Errorf("the marked candidate is %+v, found=%v", chosen, found)
	}
	list.Step(1)
	if list.Selected != 1 {
		t.Errorf("a step down left the mark on %d", list.Selected)
	}
	list.Step(2)
	if list.Selected != 0 {
		t.Errorf("a step past the last candidate left the mark on %d", list.Selected)
	}
	list.Step(-1)
	if list.Selected != 2 {
		t.Errorf("a step before the first candidate left the mark on %d", list.Selected)
	}
}

func TestCompletionListDismissClosesAndRemembers(t *testing.T) {
	list := CompletionList{Candidates: []editor.Completion{{Text: "id"}}, Selected: 0}
	list.Dismiss()
	if list.IsListing() || len(list.Candidates) != 0 {
		t.Error("a dismissed list still stands")
	}
	if !list.Dismissed {
		t.Error("a dismiss was not remembered")
	}
	if _, found := list.Chosen(); found {
		t.Error("a closed list still has a chosen candidate")
	}
	list.Step(1)
	if list.Selected != 0 {
		t.Error("a step on an empty list moved the mark")
	}
}
