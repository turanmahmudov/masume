package ui

import (
	"strings"
	"testing"

	"github.com/turanmahmudov/masume/internal/app"
	"github.com/turanmahmudov/masume/internal/core"
)

// startRunOn puts a running statement on the tab of an engine with these capabilities.
func startRunOn(t *testing.T, cancels bool) *Model {
	t.Helper()
	model := buildOfflineModel(t, 120, 30)
	connection := model.Active()
	connection.Session.(*offlineSession).capabilities = core.Capabilities{
		SortsRead: true, CancelsRunningQuery: cancels,
	}
	tab := connection.Active()
	tab.Editor = app.NewEditorBuffer("select 1", 0)
	tab.Results.Start([]string{"select 1"}, 200)
	return model
}

// SQLite, CockroachDB, PlanetScale, SQL Server and MongoDB take no cancel. The wheel of the
// run shows the note, where the engines that take one show the key.
func TestARunOnAnEngineWithoutCancelShowsTheNote(t *testing.T) {
	frame := stripEscapes(startRunOn(t, false).render())

	if !strings.Contains(frame, noCancelNote) {
		t.Errorf("the frame of the run drew no note for an engine without cancel:\n%s", frame)
	}
	if strings.Contains(frame, "^X stop") {
		t.Errorf("the frame of the run drew a key that stops it:\n%s", frame)
	}
}

// An engine that takes a cancel shows the key beside the wheel, and no note.
func TestARunOnAnEngineWithCancelShowsTheKey(t *testing.T) {
	frame := stripEscapes(startRunOn(t, true).render())

	if !strings.Contains(frame, "^X stop") {
		t.Errorf("the frame of the run drew no key that stops it:\n%s", frame)
	}
	if strings.Contains(frame, noCancelNote) {
		t.Errorf("the frame of the run drew the note beside the key:\n%s", frame)
	}
}

// The bar under the pane shows the cancel key only where the engine takes one.
func TestTheBarShowsTheCancelKeyOnlyWhereTheEngineTakesOne(t *testing.T) {
	if bar := readStatusBar(t, startRunOn(t, true)); !strings.Contains(bar, "cancel") {
		t.Errorf("the status bar drew %q, wanted the cancel key", bar)
	}
	if bar := readStatusBar(t, startRunOn(t, false)); strings.Contains(bar, "cancel") {
		t.Errorf("the status bar drew %q, wanted no cancel key", bar)
	}
}
