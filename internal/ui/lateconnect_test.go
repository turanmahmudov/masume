package ui

import (
	"testing"

	"github.com/turanmahmudov/masume/internal/cfg"
)

// A slow server that answers after the user went back and asked for another profile must
// not take the place of the one they asked for. The answer names the profile it opened.
func TestAConnectionOfAnotherProfileIsNotTakenForTheOneAskedFor(t *testing.T) {
	model := buildOfflineModel(t, 160, 48)
	model.connections = openConnections{}
	model.screen = ScreenConnecting
	model.picker.pending = cfg.Profile{Name: "second", Engine: "postgres"}

	// The first profile answers late, after the user asked for the second.
	late := &offlineSession{profile: cfg.Profile{Name: "first", Engine: "postgres"}}
	held, _ := model.Update(connectedMsg{
		Profile: cfg.Profile{Name: "first", Engine: "postgres"}, Session: late,
	})
	model = held.(*Model)

	if model.connections.count() != 0 {
		t.Errorf("the connection of another profile was opened: %d", model.connections.count())
	}
	if model.screen != ScreenConnecting {
		t.Errorf("the screen reads %v, wanted the wait for the profile asked for", model.screen)
	}

	// A failure of that same profile is not reported as the failure of this one.
	held, _ = model.Update(connectedMsg{
		Profile: cfg.Profile{Name: "first", Engine: "postgres"}, Problem: "no route to host",
	})
	model = held.(*Model)
	if model.picker.problem != "" {
		t.Errorf("the failure of another profile was reported: %q", model.picker.problem)
	}

	// The profile the user asked for is opened.
	wanted := &offlineSession{profile: cfg.Profile{Name: "second", Engine: "postgres"}}
	held, _ = model.Update(connectedMsg{
		Profile: cfg.Profile{Name: "second", Engine: "postgres"}, Session: wanted,
	})
	model = held.(*Model)
	if model.connections.count() != 1 {
		t.Fatalf("the profile asked for was not opened: %d", model.connections.count())
	}
	if model.connections.at(0).Profile().Name != "second" {
		t.Errorf("the connection reads %q", model.connections.at(0).Profile().Name)
	}
}
