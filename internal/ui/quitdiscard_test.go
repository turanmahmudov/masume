package ui

import (
	"strings"
	"testing"

	"github.com/turanmahmudov/masume/internal/app"
)

// Staged changes are lost with the client, so the press that quits must ask first.
func TestQuittingAsksAboutStagedChanges(t *testing.T) {
	model, _, tab := buildTableTabModel(t)
	stageCellEdits(tab, 2)

	model.readKey(pressCtrlC())

	if model.confirm == nil {
		t.Fatal("the client ended without asking about the staged changes")
	}
	if model.quitting {
		t.Error("the client ended before the question was answered")
	}
	if !strings.Contains(model.confirm.Body, "2 changes staged") {
		t.Errorf("the question does not count the changes: %q", model.confirm.Body)
	}
	if !model.confirm.Destructive {
		t.Error("the question is not drawn as one that cannot be taken back")
	}

	model.confirm.Answer(false)
	if model.quitting {
		t.Error("the client ended after the answer that keeps the work")
	}
}

// A notebook transaction is rolled back by the server when the connection closes.
func TestQuittingAsksAboutAnOpenTransaction(t *testing.T) {
	model := buildOfflineModel(t, 160, 48)
	tab := model.Active().Active()
	tab.Notebook = &app.Notebook{HoldsTransaction: true}

	model.readKey(pressCtrlC())

	if model.confirm == nil {
		t.Fatal("the client ended without asking about the open transaction")
	}
	if !strings.Contains(model.confirm.Body, "1 open transaction") {
		t.Errorf("the question does not name the transaction: %q", model.confirm.Body)
	}
}

// The answer that discards the work leads to the question about the connections that are
// in no config file, and the client ends after both.
func TestDiscardingStagedChangesAsksToSaveTheConnection(t *testing.T) {
	model, _, tab := buildTableTabModel(t)
	stageCellEdits(tab, 1)
	model.recordUnsavedConnection(buildUnsavedProfile("shop"))

	model.readKey(pressCtrlC())
	if model.confirm == nil {
		t.Fatal("the client ended without asking about the staged changes")
	}
	held := model.confirm
	model.confirm = nil
	held.Answer(true)

	if model.confirm == nil {
		t.Fatal("the client ended without offering to save the connection")
	}
	if model.quitting {
		t.Error("the client ended before the save question was answered")
	}
}

// A client without staged work and without an unsaved connection ends on the press.
func TestQuittingAsksNothingWithoutUnwrittenWork(t *testing.T) {
	model := buildOfflineModel(t, 160, 48)

	model.readKey(pressCtrlC())

	if model.confirm != nil {
		t.Fatalf("the client asked %q with nothing to lose", model.confirm.Body)
	}
	if !model.quitting {
		t.Error("the client did not end")
	}
}
