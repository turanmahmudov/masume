package ui

import (
	"context"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/turanmahmudov/masume/internal/app"
	"github.com/turanmahmudov/masume/internal/db"
)

// stoppingSession records what the card asked the server to stop.
type stoppingSession struct {
	*offlineSession
	pid   int64
	ends  bool
	asked bool
}

func (session *stoppingSession) CancelBackend(
	_ context.Context, pid int64, terminate bool,
) (bool, error) {
	session.pid, session.ends, session.asked = pid, terminate, true
	return true, nil
}

// buildActivityModel answers a model whose activity card holds two sessions.
func buildActivityModel(t *testing.T) (*Model, *app.Connection, *stoppingSession) {
	t.Helper()
	model := buildOfflineModel(t, 160, 48)
	connection := model.Active()
	session := &stoppingSession{offlineSession: connection.Session.(*offlineSession)}
	connection.Session = session
	connection.Overlay = app.Overlay{
		Kind: app.OverlayActivity,
		Sessions: []db.Activity{
			{PID: 4417, User: "writer", State: "active", Query: "alter table orders add column tax int"},
			{PID: 4520, User: "reader", State: "idle", Query: "select id from orders where id = 1"},
		},
	}
	return model, connection, session
}

// pressOnCard presses one key while the card owns the keyboard, and answers what the press
// started.
func pressOnCard(t *testing.T, model *Model, key tea.KeyPressMsg) tea.Cmd {
	t.Helper()
	held, command := model.Update(key)
	if held != model {
		t.Fatal("a press on the card replaced the model")
	}
	return command
}

// Every other list card loads the thing under the cursor on Enter: the history and the saved
// queries load their statement, the theme picker keeps its theme. The activity card was the
// only one where Enter started a destructive question instead.
func TestEnterOnTheActivityCardLoadsTheStatement(t *testing.T) {
	model, connection, session := buildActivityModel(t)
	connection.Overlay.List.Cursor = 1

	pressOnCard(t, model, tea.KeyPressMsg{Code: tea.KeyEnter})

	if connection.Overlay.IsOpen() {
		t.Errorf("the card is still open as %q", connection.Overlay.Kind)
	}
	if session.asked {
		t.Error("Enter asked the server to stop a session")
	}
	written := connection.Active().Editor.Text
	if written != "select id from orders where id = 1" {
		t.Errorf("the editor holds %q", written)
	}
}

// A session that runs nothing has no statement to open, so the card reports it and stays open.
func TestEnterOnASessionWithNoStatementReportsIt(t *testing.T) {
	model, connection, _ := buildActivityModel(t)
	connection.Overlay.Sessions[0].Query = "   "

	pressOnCard(t, model, tea.KeyPressMsg{Code: tea.KeyEnter})

	if connection.Overlay.Kind != app.OverlayActivity {
		t.Errorf("the card is %q, wanted the activity card", connection.Overlay.Kind)
	}
	if connection.Active().Editor.Text != "" {
		t.Error("a session with no statement wrote into the editor")
	}
}

// The two keys that stop a session ask first, and each way names what it does: cancelling
// ends the statement, ending the session closes its connection as well.
func TestTheActivityCardStopsASessionBothWays(t *testing.T) {
	for _, held := range []struct {
		name  string
		key   tea.KeyPressMsg
		title string
		ends  bool
	}{
		{
			name: "stop the statement", key: tea.KeyPressMsg{Code: 'x', Text: "x"},
			title: " stop the statement ", ends: false,
		},
		{
			name: "end the session",
			key:  tea.KeyPressMsg{Code: 'd', Mod: tea.ModCtrl},
			// The card advertised this key and ran nothing: there was no branch for
			// it, and the flag the port takes was never passed as true.
			title: " end the session ", ends: true,
		},
	} {
		t.Run(held.name, func(t *testing.T) {
			model, connection, session := buildActivityModel(t)

			pressOnCard(t, model, held.key)

			if connection.Overlay.Kind != app.OverlayConfirm {
				t.Fatalf("the card is %q, wanted a question", connection.Overlay.Kind)
			}
			if connection.Overlay.Title != held.title {
				t.Errorf("the question is titled %q, wanted %q",
					connection.Overlay.Title, held.title)
			}
			// A PID is an identifier, so it is written as it stands. A thousands
			// separator would make it read as a quantity.
			if !strings.Contains(connection.Overlay.Body, "4417") {
				t.Errorf("the question names no session: %q", connection.Overlay.Body)
			}

			answer := connection.Overlay.Answers.Answer
			if answer == nil {
				t.Fatal("the question holds no answer")
			}
			started := answer(true)
			if started == nil {
				t.Fatal("the answer started nothing, so no session was stopped")
			}
			started()

			if !session.asked {
				t.Fatal("the server was never asked to stop the session")
			}
			if session.pid != 4417 {
				t.Errorf("the server was asked about session %d", session.pid)
			}
			if session.ends != held.ends {
				t.Errorf("the server was asked with terminate %v, wanted %v",
					session.ends, held.ends)
			}
		})
	}
}

// A no leaves the session alone.
func TestTheActivityCardStopsNothingOnANo(t *testing.T) {
	model, connection, session := buildActivityModel(t)

	pressOnCard(t, model, tea.KeyPressMsg{Code: 'x', Text: "x"})
	if connection.Overlay.Answers.Answer == nil {
		t.Fatalf("the card is %q and holds no question", connection.Overlay.Kind)
	}
	if started := connection.Overlay.Answers.Answer(false); started != nil {
		started()
	}
	if session.asked {
		t.Error("a no still asked the server to stop the session")
	}
}
