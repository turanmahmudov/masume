package ui

import (
	"testing"

	"github.com/turanmahmudov/masume/internal/app"
	"github.com/turanmahmudov/masume/internal/db"
	"github.com/turanmahmudov/masume/internal/writeplan"
)

// buildAnswer answers what one statement of a run reports back.
func buildAnswer(connectionID, tabID, runID int, text string) queryRanMsg {
	return queryRanMsg{
		ConnectionID: connectionID, TabID: tabID, RunID: runID, Index: 0, Last: true,
		Read: db.ComposedRead{Text: text, Display: text},
		Result: db.QueryResult{
			Columns: []db.ResultColumn{{Name: "id", DataType: "integer"}},
			Rows:    [][]any{{int64(1)}},
		},
	}
}

// A statement of a run that was replaced still answers, because the server was already
// reading it. Its rows belong to a result the tab no longer holds, so writing them into the
// run that replaced it would draw rows nobody asked for.
func TestReadQueryAnswerDropsTheAnswerOfAReplacedRun(t *testing.T) {
	model := buildOfflineModel(t, 160, 48)
	connection := model.Active()
	tab := connection.Active()
	id := model.ActiveID()

	tab.Results.Start([]string{"select 1"}, 200)
	first := model.startBatch(connection, tab, []db.ComposedRead{{Text: "select 1"}}, 200, writeplan.UndoPlan{})
	tab.Results.Start([]string{"select 2"}, 200)
	second := model.startBatch(connection, tab, []db.ComposedRead{{Text: "select 2"}}, 200, writeplan.UndoPlan{})

	model.readQueryAnswer(buildAnswer(id, tab.ID, first, "select 1"))

	if active := tab.Results.Active(); active != nil && active.State.Kind == app.QuerySucceeded {
		t.Error("the answer of the replaced run was written into the run that replaced it")
	}
	if _, held := model.findBatch(id, tab.ID, second); !held {
		t.Fatal("the answer of the replaced run stopped the run that replaced it")
	}

	model.readQueryAnswer(buildAnswer(id, tab.ID, second, "select 2"))

	active := tab.Results.Active()
	if active == nil || active.State.Kind != app.QuerySucceeded {
		t.Error("the answer of the run in hand was not kept")
	}
	if _, held := model.findBatch(id, tab.ID, second); held {
		t.Error("the run was not stopped by its last answer")
	}
}

// Each tab of a connection carries its own run. A run started in one tab must not stop the
// statements the other tab is still waiting for.
func TestStartBatchKeepsTheRunOfEveryTab(t *testing.T) {
	model := buildOfflineModel(t, 160, 48)
	connection := model.Active()
	first := connection.Active()
	second := &app.Tab{ID: first.ID + 1}
	id := model.ActiveID()

	firstRun := model.startBatch(connection, first, []db.ComposedRead{{Text: "select 1"}}, 200, writeplan.UndoPlan{})
	secondRun := model.startBatch(connection, second, []db.ComposedRead{{Text: "select 2"}}, 200, writeplan.UndoPlan{})

	if _, held := model.findBatch(id, first.ID, firstRun); !held {
		t.Error("the run of the second tab took the place of the run of the first")
	}
	if _, held := model.findBatch(id, second.ID, secondRun); !held {
		t.Error("the run of the second tab was not opened")
	}

	model.stopBatch(id, second.ID)
	if _, held := model.findBatch(id, first.ID, firstRun); !held {
		t.Error("stopping the run of one tab stopped the run of the other")
	}
}

// A tab closed while its statement ran leaves no run behind, because a run nothing can
// answer into would turn the wheel for ever.
func TestReadQueryAnswerStopsTheRunOfAClosedTab(t *testing.T) {
	model := buildOfflineModel(t, 160, 48)
	connection := model.Active()
	tab := connection.Active()
	id := model.ActiveID()

	runID := model.startBatch(connection, tab, []db.ComposedRead{{Text: "select 1"}}, 200, writeplan.UndoPlan{})
	closedID := tab.ID + 7
	model.startBatch(connection, &app.Tab{ID: closedID},
		[]db.ComposedRead{{Text: "select 2"}}, 200, writeplan.UndoPlan{})

	model.readQueryAnswer(buildAnswer(id, closedID, runID+1, "select 2"))

	if model.runs.count() != 1 {
		t.Errorf("the client holds %d runs, wanted only the run of the open tab",
			model.runs.count())
	}
	if _, held := model.findBatch(id, tab.ID, runID); !held {
		t.Error("the run of the open tab was stopped with the run of the closed one")
	}
}
