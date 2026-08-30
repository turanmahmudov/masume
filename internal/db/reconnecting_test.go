package db

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/turanmahmudov/masume/internal/cfg"
)

// stubSession answers only the parts of a session this test asks for.
type stubSession struct {
	Session
	profile cfg.Profile
	state   TransactionState
	closed  bool
}

func (session *stubSession) Describe() SessionDescriptor {
	return SessionDescriptor{Profile: session.profile}
}

func (session *stubSession) ReadTransactionState() TransactionState {
	return session.state
}

func (session *stubSession) Close() error {
	session.closed = true
	return nil
}

// stubAdapter opens the sessions this test names, in order.
type stubAdapter struct {
	opens []*stubSession
	err   error
	calls int
}

func (adapter *stubAdapter) Connect(
	_ context.Context, _ cfg.Profile, _ string,
) (Session, error) {
	adapter.calls++
	if adapter.err != nil {
		return nil, adapter.err
	}
	return adapter.opens[adapter.calls-1], nil
}

func buildProfile(keepalive time.Duration) cfg.Profile {
	return cfg.Profile{Name: "held", Keepalive: keepalive}
}

func TestMakeReconnectableLeavesAProfileWithNoKeepalive(t *testing.T) {
	inner := &stubSession{profile: buildProfile(0)}
	if wrapped := MakeReconnectable(inner, &stubAdapter{}, ""); wrapped != Session(inner) {
		t.Error("a profile with the keepalive off was wrapped")
	}
	held := &stubSession{profile: buildProfile(30 * time.Second)}
	if wrapped := MakeReconnectable(held, &stubAdapter{}, ""); wrapped == Session(held) {
		t.Error("a profile with a keepalive was not wrapped")
	}
}

func TestReconnectReplacesTheSession(t *testing.T) {
	first := &stubSession{profile: buildProfile(time.Second)}
	second := &stubSession{profile: buildProfile(time.Second)}
	adapter := &stubAdapter{opens: []*stubSession{second}}

	wrapped := MakeReconnectable(first, adapter, "secret")
	session, is := FindReconnectable(wrapped)
	if !is {
		t.Fatal("the wrapped session is not a reconnecting one")
	}

	outcome := session.Reconnect(context.Background())
	if !outcome.Reconnected || outcome.Problem != "" {
		t.Errorf("the reconnect answered %+v, wanted it to work", outcome)
	}
	if !first.closed {
		t.Error("the old session was left open")
	}
	held, done := session.hold()
	done()
	if held != Session(second) {
		t.Error("the new session is not the one every call goes to")
	}
}

func TestReconnectReportsAnOpenTransaction(t *testing.T) {
	first := &stubSession{profile: buildProfile(time.Second), state: TransactionOpen}
	adapter := &stubAdapter{opens: []*stubSession{{profile: buildProfile(time.Second)}}}

	session, _ := FindReconnectable(MakeReconnectable(first, adapter, ""))
	outcome := session.Reconnect(context.Background())
	if !outcome.TransactionLost {
		t.Error("the transaction that was open was not reported as lost")
	}
}

func TestReconnectKeepsTheSessionWhereItCannotOpenOne(t *testing.T) {
	first := &stubSession{profile: buildProfile(time.Second)}
	adapter := &stubAdapter{err: errors.New("no route")}

	session, _ := FindReconnectable(MakeReconnectable(first, adapter, ""))
	outcome := session.Reconnect(context.Background())
	if outcome.Reconnected || outcome.Problem == "" {
		t.Errorf("the reconnect answered %+v, wanted a problem", outcome)
	}
	if first.closed {
		t.Error("the old session was closed after a failed reconnect")
	}
	held, done := session.hold()
	done()
	if held != Session(first) {
		t.Error("the old session was replaced after a failed reconnect")
	}
}

// blockingSession holds a call until the test lets it go, so a reconnect can be run while
// one is still inside the session.
type blockingSession struct {
	Session
	profile cfg.Profile
	entered chan struct{}
	release chan struct{}
	closed  chan struct{}
}

func (session *blockingSession) Describe() SessionDescriptor {
	return SessionDescriptor{Profile: session.profile}
}

func (session *blockingSession) ReadTransactionState() TransactionState {
	return TransactionNone
}

// ListTables reports that it is inside the session, waits, and then answers whether the
// session was closed while it was reading.
func (session *blockingSession) ListTables(context.Context) ([]TableRef, error) {
	close(session.entered)
	<-session.release
	select {
	case <-session.closed:
		return nil, errors.New("the connection closed while the read was still on it")
	default:
		return nil, nil
	}
}

func (session *blockingSession) Close() error {
	close(session.closed)
	return nil
}

// A reconnect must not close the session a call is still reading, because the driver would
// answer that call with a closed connection rather than with its rows.
func TestReconnectWaitsForACallBeforeItClosesTheOldSession(t *testing.T) {
	first := &blockingSession{
		profile: buildProfile(time.Second),
		entered: make(chan struct{}), release: make(chan struct{}),
		closed: make(chan struct{}),
	}
	second := &stubSession{profile: buildProfile(time.Second)}
	adapter := &stubAdapter{opens: []*stubSession{second}}

	session, is := FindReconnectable(MakeReconnectable(first, adapter, ""))
	if !is {
		t.Fatal("the wrapped session is not a reconnecting one")
	}

	read := make(chan error, 1)
	go func() {
		_, err := session.ListTables(context.Background())
		read <- err
	}()

	// The read is inside the old session, and the reconnect happens under it.
	<-first.entered
	outcome := session.Reconnect(context.Background())
	if !outcome.Reconnected {
		t.Fatalf("the reconnect answered %+v", outcome)
	}

	select {
	case <-first.closed:
		t.Fatal("the old session closed while a call was still reading it")
	default:
	}

	close(first.release)
	if err := <-read; err != nil {
		t.Errorf("the read answered %v", err)
	}

	// Once the last call is out, the session that was replaced closes.
	select {
	case <-first.closed:
	case <-time.After(time.Second):
		t.Error("the old session never closed after its last call left")
	}
}

// A call that starts after the swap goes to the new session.
func TestReconnectSendsALaterCallToTheNewSession(t *testing.T) {
	first := &stubSession{profile: buildProfile(time.Second)}
	second := &stubSession{profile: buildProfile(time.Second)}
	adapter := &stubAdapter{opens: []*stubSession{second}}

	session, _ := FindReconnectable(MakeReconnectable(first, adapter, ""))
	if outcome := session.Reconnect(context.Background()); !outcome.Reconnected {
		t.Fatalf("the reconnect answered %+v", outcome)
	}
	if !first.closed {
		t.Error("the old session was left open although no call held it")
	}
	held, done := session.hold()
	done()
	if held != Session(second) {
		t.Error("a later call did not go to the new session")
	}
}
