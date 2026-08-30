package ui

import (
	"testing"

	"github.com/turanmahmudov/masume/internal/app"
)

func TestWaitForChatEventsTakesEveryPieceWaiting(t *testing.T) {
	events := make(chan app.ChatEvent, app.ChatEventRoom)
	for range 12 {
		events <- app.ChatEvent{Run: 1, Kind: app.ChatTextArrived, Text: "a"}
	}

	held := waitForChatEvents(7, 1, events)()
	batch, is := held.(chatEventsMsg)
	if !is {
		t.Fatalf("the wait answered %T, want a batch", held)
	}
	if len(batch.Events) != 12 {
		t.Errorf("the batch held %d pieces, want 12", len(batch.Events))
	}
	if batch.ConnectionID != 7 || batch.Run != 1 {
		t.Errorf("the batch named connection %d run %d", batch.ConnectionID, batch.Run)
	}
}

func TestWaitForChatEventsReportsAClosedRun(t *testing.T) {
	events := make(chan app.ChatEvent)
	close(events)
	held := waitForChatEvents(7, 3, events)()
	if _, is := held.(chatClosedMsg); !is {
		t.Fatalf("a closed run answered %T, want the closed message", held)
	}
}

func TestResolveTickWaitFollowsWhatIsWaitedFor(t *testing.T) {
	model := &Model{}
	if wait := model.resolveTickWait(); wait != restingWait {
		t.Errorf("a client with nothing to wait for asked for %v, want %v", wait, restingWait)
	}
	model.runs.start(batchKey{connectionID: 1, tabID: 1}, &runBatch{})
	if wait := model.resolveTickWait(); wait != spinnerFrameWait {
		t.Errorf("a running statement asked for %v, want %v", wait, spinnerFrameWait)
	}
}
