package ui

import (
	"testing"

	"github.com/turanmahmudov/masume/internal/app"
	"github.com/turanmahmudov/masume/internal/cfg"
)

func TestOpenConnectionsStepRoundTheList(t *testing.T) {
	list := openConnections{}
	first := app.NewConnection(&offlineSession{
		profile: cfg.Profile{Name: "first", Engine: "postgres"},
	}, nil, true)
	second := app.NewConnection(&offlineSession{
		profile: cfg.Profile{Name: "second", Engine: "postgres"},
	}, nil, true)
	list.open(first)
	list.open(second)
	if list.active().Profile().Name != "second" {
		t.Fatalf("the list opened on %q, wanted the last one added", list.active().Profile().Name)
	}

	list.step(1)
	if list.active().Profile().Name != "first" {
		t.Errorf("a step past the last connection left the screen on %q",
			list.active().Profile().Name)
	}
	list.step(-1)
	if list.active().Profile().Name != "second" {
		t.Errorf("a step before the first connection left the screen on %q",
			list.active().Profile().Name)
	}
}

func TestOpenConnectionsAnswerNothingOffTheList(t *testing.T) {
	list := openConnections{}
	if list.at(0) != nil || list.idAt(0) != 0 || list.active() != nil || list.activeID() != 0 {
		t.Error("an empty list answered a connection")
	}
	if _, _, found := list.closeActive(); found {
		t.Error("closing an empty list answered a connection")
	}

	held := app.NewConnection(&offlineSession{
		profile: cfg.Profile{Name: "only", Engine: "postgres"},
	}, nil, true)
	id := list.open(held)
	if list.idAt(-1) != 0 || list.idAt(1) != 0 {
		t.Error("an index off the list answered an id")
	}
	if list.idOf(app.NewConnection(&offlineSession{
		profile: cfg.Profile{Name: "other", Engine: "postgres"},
	}, nil, true)) != 0 {
		t.Error("a connection that is not open answered an id")
	}
	if list.idOf(held) != id {
		t.Errorf("the open connection answers id %d, wanted %d", list.idOf(held), id)
	}

	visited := 0
	for range list.all() {
		visited++
		break
	}
	if visited != 1 {
		t.Errorf("all visited %d connections after a stop, wanted 1", visited)
	}

	closed, closedID, found := list.closeActive()
	if !found || closed != held || closedID != id {
		t.Errorf("close answered %v, %d, %v", closed, closedID, found)
	}
	if list.count() != 0 {
		t.Errorf("the list still holds %d connections", list.count())
	}
}

func TestOpenConnectionsHoldsOnlyAnOpenTab(t *testing.T) {
	list := openConnections{}
	held := app.NewConnection(&offlineSession{
		profile: cfg.Profile{Name: "only", Engine: "postgres"},
	}, nil, true)
	id := list.open(held)
	tab := held.Active()
	if tab == nil {
		t.Fatal("the connection opened with no tab")
	}
	if !list.holdsTab(tabKey{connection: id, tab: tab.ID}) {
		t.Error("the open tab is not held")
	}
	if list.holdsTab(tabKey{connection: id, tab: tab.ID + 1}) {
		t.Error("a tab that was never opened is held")
	}
	if list.holdsTab(tabKey{connection: id + 1, tab: tab.ID}) {
		t.Error("a tab of a connection that is not open is held")
	}
}
