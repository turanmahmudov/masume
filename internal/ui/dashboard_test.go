package ui

import (
	"context"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/turanmahmudov/masume/internal/app"
	"github.com/turanmahmudov/masume/internal/core"
	"github.com/turanmahmudov/masume/internal/db"
)

// watchedSession counts what the dashboard asked the server, and can hold a read open so a
// test can see what the card does while one is on its way.
type watchedSession struct {
	*offlineSession
	guard    sync.Mutex
	reads    int
	sessions []db.Activity
	locks    []db.LockWait
	load     db.ServerLoad
	// hold is closed to let a read finish. A session with none answers at once.
	hold        chan struct{}
	slow        []db.StatementStat
	noLocks     bool
	noLoad      bool
	noSlow      bool
	listProblem error
}

func (session *watchedSession) ListActivity(context.Context) ([]db.Activity, error) {
	session.guard.Lock()
	session.reads++
	held := session.hold
	session.guard.Unlock()
	if held != nil {
		<-held
	}
	if session.listProblem != nil {
		return nil, session.listProblem
	}
	return session.sessions, nil
}

func (session *watchedSession) ListLockWaits(context.Context) ([]db.LockWait, error) {
	if session.noLocks {
		return nil, db.NewUnsupportedError("report which sessions wait for a lock")
	}
	return session.locks, nil
}

func (session *watchedSession) ReadServerLoad(context.Context) (db.ServerLoad, error) {
	if session.noLoad {
		return db.ServerLoad{}, db.NewUnsupportedError("report the load it is under")
	}
	return session.load, nil
}

func (session *watchedSession) ListSlowStatements(
	context.Context, int,
) ([]db.StatementStat, error) {
	if session.noSlow {
		return nil, db.NewUnsupportedError("report the statements it spends its time in")
	}
	return session.slow, nil
}

func (session *watchedSession) countReads() int {
	session.guard.Lock()
	defer session.guard.Unlock()
	return session.reads
}

// buildDashboardModel answers a model whose dashboard is open on three sessions.
func buildDashboardModel(t *testing.T) (*Model, *app.Connection, *watchedSession) {
	t.Helper()
	model := buildOfflineModel(t, 120, 40)
	connection := model.Active()
	session := &watchedSession{
		offlineSession: connection.Session.(*offlineSession),
		sessions: []db.Activity{
			{PID: 4417, User: "writer", State: "active", Query: "alter table orders"},
			{PID: 4520, User: "reader", State: "idle", Query: "select 1"},
			{PID: 4533, User: "reader", State: "active", Query: "update orders set x = 1"},
		},
		load: db.ServerLoad{Connections: 84, MaxConnections: 100},
		locks: []db.LockWait{{
			BlockedPID: 4520, BlockedQuery: "select 1", Waiting: 4 * time.Minute,
			Mode: "AccessExclusiveLock", Relation: "orders",
			BlockingPID: 4417, BlockingQuery: "alter table orders",
			BlockingFor: 5 * time.Minute,
		}},
	}
	connection.Session = session
	return model, connection, session
}

// openDashboard runs the read the card is opened by, and lands its answer.
func openDashboard(t *testing.T, model *Model, connection *app.Connection) {
	t.Helper()
	command := readActivity(model.ActiveID(), connection.Session, readAsked)
	held, _ := model.Update(command())
	if held != model {
		t.Fatal("the answer replaced the model")
	}
	if connection.Overlay.Kind != app.OverlayActivity {
		t.Fatalf("the card is %q, wanted the dashboard", connection.Overlay.Kind)
	}
}

// The card refreshes under the reader, so it must not move what the reader is on. Before
// this the answer built a fresh overlay and the cursor jumped back to the first row every
// two seconds.
func TestARefreshKeepsWhatTheReaderIsOn(t *testing.T) {
	model, connection, session := buildDashboardModel(t)
	openDashboard(t, model, connection)

	connection.Overlay.List.Cursor = 2
	connection.Overlay.List.Offset = 1
	connection.Overlay.View.FoldPanel(app.PanelBlocking, true)

	session.sessions = append(session.sessions, db.Activity{PID: 4600, State: "idle"})
	held, _ := model.Update(readActivity(
		model.ActiveID(), connection.Session, readRefresh)())
	model = held.(*Model)

	overlay := model.Active().Overlay
	if overlay.List.Cursor != 2 {
		t.Errorf("the cursor moved to row %d", overlay.List.Cursor)
	}
	if overlay.List.Offset != 1 {
		t.Errorf("the list scrolled to %d", overlay.List.Offset)
	}
	if !overlay.View.IsPanelFolded(app.PanelBlocking) {
		t.Error("the folded panel opened again")
	}
	if len(overlay.Sessions) != 4 {
		t.Errorf("the card holds %d sessions, wanted the four the server now has",
			len(overlay.Sessions))
	}
}

// A refresh that lands after sessions went away must not leave the cursor past the last row,
// or the keys act on a session that is no longer there.
func TestARefreshBringsTheCursorBackIntoTheList(t *testing.T) {
	model, connection, session := buildDashboardModel(t)
	openDashboard(t, model, connection)
	connection.Overlay.List.Cursor = 2

	session.sessions = session.sessions[:1]
	held, _ := model.Update(readActivity(
		model.ActiveID(), connection.Session, readRefresh)())
	model = held.(*Model)

	if cursor := model.Active().Overlay.List.Cursor; cursor != 0 {
		t.Errorf("the cursor stands at row %d, past the one row left", cursor)
	}
}

// A server slower than the interval must be asked once. Without the mark a read is started
// on every wake and they pile up behind the first.
func TestTheDashboardAsksASlowServerOnce(t *testing.T) {
	model, connection, session := buildDashboardModel(t)
	openDashboard(t, model, connection)

	session.hold = make(chan struct{})
	opened := session.countReads()
	now := time.Now().Add(time.Hour)

	command := model.refreshDashboard(now)
	if command == nil {
		t.Fatal("a stale card started no read")
	}
	if !connection.Overlay.View.Reading {
		t.Fatal("the card does not know a read is on its way")
	}
	landed := make(chan tea.Msg, 1)
	go func() { landed <- command() }()

	for range 5 {
		if again := model.refreshDashboard(now); again != nil {
			t.Fatal("a second read started while the first was on its way")
		}
	}
	close(session.hold)
	<-landed
	if asked := session.countReads() - opened; asked != 1 {
		t.Errorf("the server was asked %d times while one read was on its way", asked)
	}
}

// The refresh rides the wake the client already asks for. A card that is not open must not
// read the server at all.
func TestTheDashboardReadsOnlyWhileItIsOpen(t *testing.T) {
	model, connection, _ := buildDashboardModel(t)
	openDashboard(t, model, connection)
	stale := time.Now().Add(time.Hour)

	if command := model.refreshDashboard(stale); command == nil {
		t.Fatal("the open card started no read")
	}
	connection.Overlay.View.Reading = false

	connection.Overlay = app.Overlay{}
	if command := model.refreshDashboard(stale); command != nil {
		t.Error("a closed card read the server")
	}
}

// A read that has just landed is not read again. Without this the card would read the server
// on every wake, which is once a second.
func TestTheDashboardWaitsTheIntervalBetweenReads(t *testing.T) {
	model, connection, _ := buildDashboardModel(t)
	openDashboard(t, model, connection)

	readAt := connection.Overlay.Server.ReadAt
	if command := model.refreshDashboard(readAt.Add(dashboardRefreshWait - time.Millisecond)); command != nil {
		t.Error("the card read the server again before the interval had passed")
	}
	if command := model.refreshDashboard(readAt.Add(dashboardRefreshWait)); command == nil {
		t.Error("the card did not read the server once the interval had passed")
	}
}

// A refresh that fails leaves the last answer on screen and reports nothing. A report would
// fill the bar every two seconds and hide the rows under it.
func TestARefreshThatFailsKeepsTheLastAnswerAndIsQuiet(t *testing.T) {
	model, connection, session := buildDashboardModel(t)
	openDashboard(t, model, connection)
	connection.Overlay.View.Reading = true

	session.listProblem = db.NewDatabaseError("the server went away")
	held, _ := model.Update(readActivity(
		model.ActiveID(), connection.Session, readRefresh)())
	model = held.(*Model)

	connection = model.Active()
	if connection.Overlay.Kind != app.OverlayActivity {
		t.Fatalf("the card is %q, wanted the dashboard still on screen",
			connection.Overlay.Kind)
	}
	if len(connection.Overlay.Sessions) != 3 {
		t.Errorf("the card holds %d sessions, wanted the three of the last answer",
			len(connection.Overlay.Sessions))
	}
	if connection.Overlay.View.Reading {
		t.Error("the card still thinks a read is on its way, so it will never read again")
	}
	if connection.Notice != nil {
		t.Errorf("a refresh that failed reported %q", connection.Notice.Text)
	}
}

// A read the reader asked for is a different matter: a fault has to be reported, or the key
// looks like it did nothing.
func TestAReadTheReaderAskedForReportsAFault(t *testing.T) {
	model, connection, session := buildDashboardModel(t)
	session.listProblem = db.NewDatabaseError("the server went away")

	held, _ := model.Update(readActivity(model.ActiveID(), connection.Session, readAsked)())
	model = held.(*Model)

	notice := model.Active().Notice
	if notice == nil {
		t.Fatal("a read the reader asked for failed and reported nothing")
	}
	if !strings.Contains(notice.Text, "went away") {
		t.Errorf("the fault was reported as %q", notice.Text)
	}
}

// An engine that reports no locks and no load still lists its sessions. The panels it has no
// numbers for are left out, rather than drawn as zero or reported as a fault.
func TestTheDashboardLeavesOutWhatTheEngineDoesNotReport(t *testing.T) {
	model, connection, session := buildDashboardModel(t)
	session.noLocks, session.noLoad = true, true
	openDashboard(t, model, connection)

	overlay := connection.Overlay
	if overlay.Server.HasLoad || overlay.Server.HasLocks {
		t.Fatal("the card thinks the server answered for what it refused")
	}
	if connection.Notice != nil {
		t.Errorf("an engine without the two reads reported %q", connection.Notice.Text)
	}
	if len(overlay.Sessions) != 3 {
		t.Errorf("the card holds %d sessions, wanted three", len(overlay.Sessions))
	}

	frame := stripStyles(model.render())
	for _, absent := range []string{"conns", "blocking tree"} {
		if strings.Contains(frame, absent) {
			t.Errorf("the card draws %q for an engine that reports none of it", absent)
		}
	}
	if !strings.Contains(frame, "sessions 3") {
		t.Error("the card does not say how many sessions the server holds")
	}
}

// The panel folds with the keys that fold a schema, and it keeps the count of waiters while
// it is folded, or folding it away would hide that anything is blocked.
func TestThePanelFoldsAndKeepsItsCount(t *testing.T) {
	model, connection, _ := buildDashboardModel(t)
	openDashboard(t, model, connection)

	if frame := stripStyles(model.render()); !strings.Contains(frame, "select 1") {
		t.Fatal("the open panel does not draw the session that is waiting")
	}

	pressOnCard(t, model, tea.KeyPressMsg{Code: tea.KeyLeft})
	if !connection.Overlay.View.IsPanelFolded(app.PanelBlocking) {
		t.Fatal("the panel did not fold")
	}
	frame := stripStyles(model.render())
	if !strings.Contains(frame, "blocking tree · 1 waiting") {
		t.Error("the folded panel does not say how many sessions are waiting")
	}

	pressOnCard(t, model, tea.KeyPressMsg{Code: tea.KeyRight})
	if connection.Overlay.View.IsPanelFolded(app.PanelBlocking) {
		t.Error("the panel did not open again")
	}
}

// The title carries how old the numbers can be. A reader who cannot see the interval has to
// guess whether the rows on screen are from now or from a minute ago.
func TestTheCardNamesHowOftenItRefreshes(t *testing.T) {
	model, connection, _ := buildDashboardModel(t)
	openDashboard(t, model, connection)

	frame := stripStyles(model.render())
	want := "refreshing " + core.FormatLargestUnit(dashboardRefreshWait)
	if !strings.Contains(frame, want) {
		t.Errorf("the title does not say %q", want)
	}
}

// buildSlowReading returns a dashboard whose server counted some statements.
func buildSlowReading() app.Overlay {
	return app.Overlay{
		Kind: app.OverlayActivity,
		Server: app.ServerReading{
			HasSlow: true,
			Slow: []db.StatementStat{
				{Query: "select * from orders where id = $1", Calls: 9180,
					MeanTime: 412 * time.Millisecond},
				{Query: "update sessions set seen_at = now()", Calls: 41002,
					MeanTime: 88 * time.Millisecond},
			},
		},
	}
}

// The panel covers the statements the server spends its time in, so it draws each statement,
// how long it takes and how often it ran.
func TestTheSlowPanelDrawsTheSlowestStatements(t *testing.T) {
	model := NewModel(loadedConfigForTest("tokyonight"), nil, nil, nil)

	written := strings.Join(model.buildSlowPanel(buildSlowReading(), 100), "\n")
	for _, expected := range []string{
		"slowest statements", "412 ms", "88 ms", "9,180", "41,002",
		"select * from orders", "update sessions",
	} {
		if !strings.Contains(written, expected) {
			t.Errorf("the panel is missing %q:\n%s", expected, written)
		}
	}
}

// A server that keeps no count of its statements draws no panel. An empty panel would leave
// the reader unable to tell an idle server from a silent one.
func TestTheSlowPanelIsAbsentWhereTheServerCountsNothing(t *testing.T) {
	model := NewModel(loadedConfigForTest("tokyonight"), nil, nil, nil)

	silent := buildSlowReading()
	silent.Server.HasSlow = false
	if lines := model.buildSlowPanel(silent, 100); len(lines) != 0 {
		t.Errorf("a server that counts nothing drew %v", lines)
	}

	empty := buildSlowReading()
	empty.Server.Slow = nil
	if lines := model.buildSlowPanel(empty, 100); len(lines) != 0 {
		t.Errorf("a server that has run nothing drew %v", lines)
	}
}

// A folded panel is one row, and it keeps its name so the reader knows what they folded.
func TestTheSlowPanelFoldsToItsHeading(t *testing.T) {
	model := NewModel(loadedConfigForTest("tokyonight"), nil, nil, nil)

	overlay := buildSlowReading()
	overlay.View.FoldPanel(app.PanelSlow, true)
	lines := model.buildSlowPanel(overlay, 100)
	if len(lines) != 1 {
		t.Fatalf("a folded panel drew %d rows, wanted one", len(lines))
	}
	if !strings.Contains(lines[0], "slowest statements") {
		t.Errorf("a folded panel reads %q", lines[0])
	}
}

// The panel draws the worst few and not every statement the server holds, or one card would
// hold a report.
func TestTheSlowPanelDrawsTheWorstFew(t *testing.T) {
	model := NewModel(loadedConfigForTest("tokyonight"), nil, nil, nil)

	overlay := buildSlowReading()
	overlay.Server.Slow = nil
	for at := range 40 {
		overlay.Server.Slow = append(overlay.Server.Slow, db.StatementStat{
			Query: "select " + strconv.Itoa(at), Calls: 1,
			MeanTime: time.Duration(40-at) * time.Millisecond,
		})
	}
	// One row for the heading and one per statement it draws.
	if lines := model.buildSlowPanel(overlay, 100); len(lines) != slowStatementRows+1 {
		t.Errorf("the panel drew %d rows, wanted %d", len(lines), slowStatementRows+1)
	}
}
