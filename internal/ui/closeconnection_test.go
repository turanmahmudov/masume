package ui

import (
	"strings"
	"testing"

	"github.com/turanmahmudov/masume/internal/app"
	"github.com/turanmahmudov/masume/internal/cfg"
)

// closingSession records that the connection was closed.
type closingSession struct {
	*offlineSession
	closed bool
}

func (session *closingSession) Close() error {
	session.closed = true
	return nil
}

// A connection the user was asked about must be closed the same way as one that needed no
// question: the session ends and the command that opened the port is stopped. Only taking
// the connection off the screen would leave both behind.
func TestRequestCloseConnectionClosesTheSessionItWasAskedAbout(t *testing.T) {
	model := buildOfflineModel(t, 160, 48)
	connection := model.Active()
	session := &closingSession{offlineSession: connection.Session.(*offlineSession)}
	connection.Session = session
	connection.Tabs = append(connection.Tabs, app.NewQueryTab(connection.Tabs[0].ID+1, ""))

	model.requestCloseConnection(connection)
	if connection.Overlay.Kind != app.OverlayConfirm {
		t.Fatalf("the question was not asked; the card is %q", connection.Overlay.Kind)
	}
	if connection.Overlay.Answers.Answer == nil {
		t.Fatal("the question holds no answer")
	}

	started := connection.Overlay.Answers.Answer(true)
	if model.connections.count() != 0 {
		t.Fatalf("the client holds %d connections, wanted none", model.connections.count())
	}
	if started == nil {
		t.Fatal("the answer started nothing, so the session was never closed")
	}
	started()
	if !session.closed {
		t.Error("the session was left open")
	}
}

// A question the user said no to leaves the connection open.
func TestRequestCloseConnectionKeepsTheConnectionOnANo(t *testing.T) {
	model := buildOfflineModel(t, 160, 48)
	connection := model.Active()
	session := &closingSession{offlineSession: connection.Session.(*offlineSession)}
	connection.Session = session
	connection.Tabs = append(connection.Tabs, app.NewQueryTab(connection.Tabs[0].ID+1, ""))

	model.requestCloseConnection(connection)
	connection.Overlay.Answers.Answer(false)

	if model.connections.count() != 1 {
		t.Error("the connection was closed although the answer was no")
	}
	if session.closed {
		t.Error("the session was closed although the answer was no")
	}
}

// A read-only connection refuses a staged write in the client. MongoDB and a key store hold
// no read-only session, so a check only in the server would let the write through.
func TestApplyStagedChangesRefusesAReadOnlyConnection(t *testing.T) {
	model := buildOfflineModel(t, 160, 48)
	connection := model.Active()
	tab := connection.Active()
	profile := connection.Session.Describe().Profile
	profile.AccessMode = cfg.AccessReadOnly
	connection.Session.(*offlineSession).profile = profile

	_, command := model.applyStagedChanges(connection, tab)

	if command != nil {
		t.Error("a staged write was sent on a read-only connection")
	}
	if connection.Notice == nil || connection.Notice.Tone != app.NoticeError {
		t.Fatalf("the refusal was not reported; the report is %v", connection.Notice)
	}
	if !strings.Contains(connection.Notice.Text, "read-only") {
		t.Errorf("the report reads %q, wanted it to say the connection is read-only",
			connection.Notice.Text)
	}
}
