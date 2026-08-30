package ui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	uv "github.com/charmbracelet/ultraviolet"

	"github.com/turanmahmudov/masume/internal/app"
)

// buildListingModel answers a model with the suggestions standing over this statement.
func buildListingModel(t *testing.T, written string) (*Model, *app.Connection, *app.Tab) {
	t.Helper()
	model, connection, tab := buildScannedModel(t)
	tab.Focus = app.PaneEditor
	tab.Editor = app.NewEditorBuffer(written, len(written))
	model.refreshCompletion(connection, tab)
	if !tab.Completion.IsListing() {
		t.Skipf("the statement %q offered nothing", written)
	}
	return model, connection, tab
}

// A list that stands over the statement owns the arrows, whether the word under the caret is
// written or empty. The arrows used to move the caret over an empty word, which left the list
// standing where the caret no longer was.
func TestTheArrowsStepTheListWhereverItOpened(t *testing.T) {
	for _, written := range []string{
		"select * from ",
		"select * from ord",
	} {
		t.Run(written, func(t *testing.T) {
			model, _, tab := buildListingModel(t, written)
			caret := tab.Editor.Caret

			model.readWorkspaceKey(tea.Key{Code: tea.KeyDown})
			if tab.Completion.Selected != 1 {
				t.Errorf("Down marked candidate %d", tab.Completion.Selected)
			}
			model.readWorkspaceKey(tea.Key{Code: tea.KeyUp})
			if tab.Completion.Selected != 0 {
				t.Errorf("Up marked candidate %d", tab.Completion.Selected)
			}
			if tab.Editor.Caret != caret {
				t.Errorf("the arrows moved the caret to %d, and it was at %d",
					tab.Editor.Caret, caret)
			}
			if !tab.Completion.IsListing() {
				t.Error("the arrows took the list off the statement")
			}
		})
	}
}

// Tab takes the candidate the arrows marked, so the two work together.
func TestTabTakesTheCandidateTheArrowsMarked(t *testing.T) {
	model, _, tab := buildListingModel(t, "select * from ")
	model.readWorkspaceKey(tea.Key{Code: tea.KeyDown})
	wanted := tab.Completion.Candidates[tab.Completion.Selected].Text

	model.readWorkspaceKey(tea.Key{Code: tea.KeyTab})
	if !strings.Contains(tab.Editor.Text, wanted) {
		t.Errorf("the statement reads %q and does not hold %q", tab.Editor.Text, wanted)
	}
	if tab.Completion.IsListing() {
		t.Error("the list still stands over the statement")
	}
}

// Escape takes the list off the statement, and the arrows move the caret again once it is
// gone.
func TestEscapeGivesTheArrowsBackToTheEditor(t *testing.T) {
	model, _, tab := buildListingModel(t, "select id,\nname from ")
	model.readWorkspaceKey(tea.Key{Code: tea.KeyEscape})
	if tab.Completion.IsListing() {
		t.Fatal("Escape left the list standing")
	}

	caret := tab.Editor.Caret
	model.readWorkspaceKey(tea.Key{Code: tea.KeyUp})
	if tab.Editor.Caret == caret {
		t.Error("the caret stayed where it was once the list was gone")
	}
}

// The client offers the list itself, so there is no key that asks for one.
func TestNoKeyAsksForTheList(t *testing.T) {
	model, _, tab := buildScannedModel(t)
	tab.Focus = app.PaneEditor
	tab.Editor = app.NewEditorBuffer("select * from ", len("select * from "))
	tab.Completion.Dismiss()

	model.readWorkspaceKey(tea.Key{Code: ' ', Text: " ", Mod: uv.ModCtrl})
	if tab.Completion.IsListing() {
		t.Error("a key asked for the list the client offers itself")
	}
	if tab.Editor.Text != "select * from " {
		t.Errorf("the statement reads %q", tab.Editor.Text)
	}
}

// The title of the pane names the keys of the list while it stands, and the same ones however
// the list opened.
func TestTheTitleNamesTheKeysOfTheList(t *testing.T) {
	for _, written := range []string{"select * from ", "select * from ord"} {
		model, _, tab := buildListingModel(t, written)
		frame := strings.Split(model.render(), "\n")
		title := stripStyles(frame[firstPaneRow])
		for _, key := range []string{"↑↓", "Tab take", "Esc"} {
			if !strings.Contains(title, key) {
				t.Errorf("the title of %q reads %q and does not name %q",
					written, strings.TrimSpace(title), key)
			}
		}
		if strings.Contains(title, "Space") {
			t.Errorf("the title of %q still names a key that asks for the list", written)
		}
		_ = tab
	}
}

// A terminal reports the locks it has on as modifiers of every press. Caps Lock and Num Lock
// change nothing about an arrow, so the list still answers one, and a letter is still typed.
func TestTheLocksOfTheTerminalDoNotTakeTheKeysOfTheList(t *testing.T) {
	for _, lock := range []struct {
		name string
		mod  uv.KeyMod
	}{{"caps lock", uv.ModCapsLock}, {"num lock", uv.ModNumLock}} {
		t.Run(lock.name, func(t *testing.T) {
			model, _, tab := buildListingModel(t, "select * from ")
			caret := tab.Editor.Caret

			model.readWorkspaceKey(tea.Key{Code: tea.KeyDown, Mod: lock.mod})
			if tab.Completion.Selected != 1 {
				t.Errorf("Down with %s marked candidate %d",
					lock.name, tab.Completion.Selected)
			}
			if tab.Editor.Caret != caret {
				t.Errorf("Down with %s moved the caret to %d", lock.name, tab.Editor.Caret)
			}

			model.readWorkspaceKey(tea.Key{Code: 'a', Text: "a", Mod: lock.mod})
			if !strings.HasSuffix(tab.Editor.Text, "a") {
				t.Errorf("a letter with %s wrote %q", lock.name, tab.Editor.Text)
			}
		})
	}
}

// A key that carries Shift grows the selection of the statement, so the list leaves it to the
// editor rather than stepping.
func TestShiftAndAnArrowStillGrowTheSelection(t *testing.T) {
	model, _, tab := buildListingModel(t, "select id,\nname from ")
	model.readWorkspaceKey(tea.Key{Code: tea.KeyUp, Mod: uv.ModShift})
	if tab.Completion.Selected != 0 {
		t.Errorf("Shift and Up stepped the list to %d", tab.Completion.Selected)
	}
	if !tab.Editor.HasSelection() {
		t.Error("Shift and Up selected nothing")
	}
}
