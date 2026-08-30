package ui

import (
	"testing"

	"github.com/turanmahmudov/masume/internal/app"
)

// A chat still asking the model holds the session and writes into a channel nobody reads
// once the connection is gone. Its goroutine fills that channel and then waits for ever, so
// closing the connection has to stop the run.
func TestClosingAConnectionStopsItsChat(t *testing.T) {
	model := buildOfflineModel(t, 160, 48)
	connection := model.Active()

	stopped := false
	run, events := connection.Chat.Begin(func() { stopped = true })
	if run == 0 || events == nil {
		t.Fatal("the chat did not open a run")
	}
	if connection.Chat.Status != app.ChatStreaming {
		t.Fatalf("the chat reads %q, wanted it streaming", connection.Chat.Status)
	}

	model.closeActiveConnection()

	if !stopped {
		t.Error("the connection closed and its chat was left asking")
	}
	if connection.Chat.Status != app.ChatIdle {
		t.Errorf("the chat reads %q after the close", connection.Chat.Status)
	}
}

// The same holds when the program ends: every chat is stopped before its session closes.
func TestShuttingDownStopsEveryChat(t *testing.T) {
	model := buildOfflineModel(t, 160, 48)
	connection := model.Active()

	stopped := false
	connection.Chat.Begin(func() { stopped = true })

	model.shutDown()

	if !stopped {
		t.Error("the program ended and a chat was left asking")
	}
}
