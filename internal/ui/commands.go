package ui

import (
	"context"
	"encoding/json"
	"io"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/turanmahmudov/masume/internal/app"
	"github.com/turanmahmudov/masume/internal/cfg"
	"github.com/turanmahmudov/masume/internal/core"
	"github.com/turanmahmudov/masume/internal/db"
	"github.com/turanmahmudov/masume/internal/db/engines"
	"github.com/turanmahmudov/masume/internal/hist"
	"github.com/turanmahmudov/masume/internal/present"
	"github.com/turanmahmudov/masume/internal/query/editor"
	"github.com/turanmahmudov/masume/internal/query/statement"
	"github.com/turanmahmudov/masume/internal/writeplan"
)

// The work that reaches a server runs off the draw loop. Each command returns one message,
// and every message names the connection and the tab it belongs to, so an answer that
// arrives after the user moved on is dropped rather than drawn in the wrong place.

// connectedMsg returns one attempt at opening a connection.
type connectedMsg struct {
	Profile cfg.Profile
	Session db.Session
	Problem string
	// What was started to reach the server, which is stopped with the connection.
	PreConnect *cfg.PreConnectHandle
}

// catalogReadMsg returns a read of the object tree.
type catalogReadMsg struct {
	ConnectionID int
	Tables       []db.TableRef
	Objects      []db.SchemaObject
	Roles        []db.DbRole
	Problem      string
	PartProblem  string
	// True for a read the user asked for, which is reported when it is done. A read that
	// follows a connect or a DDL statement says nothing.
	Announce bool
}

// tableDetailMsg returns the columns and the keys of one relation.
type tableDetailMsg struct {
	ConnectionID int
	TableID      string
	Detail       db.TableDetail
	Problem      string
}

// queryRanMsg returns one statement of a run.
type queryRanMsg struct {
	ConnectionID int
	TabID        int
	// The run the statement belongs to, so an answer of a run that was replaced is
	// dropped rather than written into the run that took its place.
	RunID   int
	Index   int
	Read    db.ComposedRead
	Result  db.QueryResult
	Problem string
	// The whole run is done once the last statement answered.
	Last bool
	// The undo of a planned write, read inside the transaction of that write.
	Undo writeplan.Undo
}

// pageReadMsg returns the next page of rows.
type pageReadMsg struct {
	ConnectionID int
	TabID        int
	Index        int
	// The result the page belongs to. A tab that ran again holds another result at the
	// same position, so a page that lands late is dropped rather than added to it.
	ResultID int
	Result   db.QueryResult
	Problem  string
}

// countedMsg returns the count of the whole result.
type countedMsg struct {
	ConnectionID int
	TabID        int
	Index        int
	// The result the count belongs to, for the same reason as the page above.
	ResultID int
	Total    int64
	HasTotal bool
	Problem  string
}

// planReadMsg returns the plan of a statement.
type planReadMsg struct {
	ConnectionID int
	TabID        int
	// The result the plan belongs to, for the same reason as the page above.
	ResultID int
	Plan     db.QueryPlan
	Problem  string
}

// relationViewMsg returns what a view that describes a relation draws.
type relationViewMsg struct {
	ConnectionID int
	TabID        int
	View         app.ResultView
	Content      app.PaneContent
}

// changesAppliedMsg returns an attempt at applying the staged work.
type changesAppliedMsg struct {
	ConnectionID int
	TabID        int
	Applied      int
	Problem      string
}

// historyReadMsg returns the statements that ran on this profile.
type historyReadMsg struct {
	ConnectionID int
	Entries      []hist.HistoryEntry
}

// savedReadMsg returns the statements a reader kept by name.
type savedReadMsg struct {
	ConnectionID int
	Queries      []hist.SavedQuery
}

// activityReadMsg returns what the server is doing: its sessions, the sessions waiting for a
// lock, and the load it is under. The three arrive together.
type activityReadMsg struct {
	ConnectionID int
	Sessions     []db.Activity
	Locks        []db.LockWait
	Load         db.ServerLoad
	Slow         []db.StatementStat
	// Whether the server answered for the locks, the load and the counts.
	HasLocks bool
	HasLoad  bool
	HasSlow  bool
	// Problem is set only where the session list itself failed.
	Problem string
	// True where the read was asked for by the refresh rather than by the reader.
	Refresh bool
}

// marksReadMsg returns what the user marked on this profile.
type marksReadMsg struct {
	ConnectionID int
	Favourites   []core.Favourite
	Recent       []core.RecentSchema
}

// exportWrittenMsg returns an export.
type exportWrittenMsg struct {
	ConnectionID int
	Path         string
	Rows         int64
	Problem      string
}

// historyWrittenMsg reports a write of the history file that did not work. Nothing waits for
// such a write, so this report is the only sign the user gets that it was lost.
type historyWrittenMsg struct {
	ConnectionID int
	Problem      string
}

// readHistoryWritten reports a write of the history file that was lost.
func (model *Model) readHistoryWritten(held historyWrittenMsg) (tea.Model, tea.Cmd) {
	connection, _, found := model.findConnection(held.ConnectionID)
	if !found {
		return model, nil
	}
	connection.ShowError(held.Problem)
	return model, nil
}

// readSavedQueryRemoved reports the removal, and reads the list of saved statements again.
func (model *Model) readSavedQueryRemoved(held savedQueryRemovedMsg) (tea.Model, tea.Cmd) {
	connection, _, found := model.findConnection(held.ConnectionID)
	if !found {
		return model, nil
	}
	if held.Problem != "" {
		connection.ShowError(held.Problem)
		return model, nil
	}
	connection.Show("deleted " + held.Name)
	return model, readSaved(held.ConnectionID, model.log, connection.Profile().Name)
}

// writeHistory runs one write of the history file away from the loop. A key press must not
// wait for the file, which the agent server can hold at the same moment, and what the write
// stores is on screen already.
func writeHistory(connectionID int, what string, write func() error) tea.Cmd {
	return func() tea.Msg {
		if err := write(); err != nil {
			return historyWrittenMsg{
				ConnectionID: connectionID, Problem: what + ": " + db.DescribeError(err),
			}
		}
		return nil
	}
}

// tickMsg wakes the app so a report can be taken off the bar.
type tickMsg time.Time

// How often the app wakes itself. While something is waited for, the wheel of the wait turns
// on every frame of it; with nothing to wait for, the wake only has to take a report off the
// bar, so it is rare and the client draws nothing between one and the next.
const (
	spinnerFrameWait = 100 * time.Millisecond
	restingWait      = time.Second
)

// tick asks to be woken again, so a report leaves the bar without a key and the wheel of a
// wait keeps turning.
func tick(wait time.Duration) tea.Cmd {
	return tea.Tick(wait, func(at time.Time) tea.Msg { return tickMsg(at) })
}

// wakeMsg wakes the app so a mark that has run out is taken off the frame. It carries no work
// of its own and starts no wheel of its own, so a wake never leaves a second clock running.
type wakeMsg struct{}

// wake asks for one frame after this long, and nothing else.
func wake(after time.Duration) tea.Cmd {
	return tea.Tick(after, func(time.Time) tea.Msg { return wakeMsg{} })
}

// firstAnswerWait is how long the first frame waits for the terminal to report its colours. A
// terminal returns in about a third of a second. Without the wait a theme that follows the
// terminal is drawn first in the standard palette and then in the colours of the terminal.
const firstAnswerWait = 500 * time.Millisecond

// How long the client waits before it asks the terminal for its colours again, at first and
// at most. The wait doubles between the two.
const (
	firstAskAgainSeconds   = 1
	longestAskAgainSeconds = 64
)

// watchTerminalWait is how often the colours are read again once every one of them has
// arrived, so a palette the user edits is followed. A terminal reports a switch of its own
// theme, and reports nothing when a slot of its palette is changed.
const watchTerminalWait = 2 * time.Second

// terminalColorsWaitedMsg says the wait for the colours of the terminal has run out.
type terminalColorsWaitedMsg struct{}

// waitForTerminalColors holds the first frame back until the terminal has answered.
func waitForTerminalColors() tea.Cmd {
	return tea.Tick(firstAnswerWait, func(time.Time) tea.Msg {
		return terminalColorsWaitedMsg{}
	})
}

// connectTimeout is how long one attempt at a connection may take.
const connectTimeout = 30 * time.Second

// connect opens one connection, and returns why it could not.
func connect(adapters engines.Adapters, profile cfg.Profile, password string) tea.Cmd {
	return func() tea.Msg {
		// A tunnel or a proxy has to listen before the driver is asked, so the command
		// of the profile runs first and is stopped with the connection.
		preConnect, err := cfg.StartPreConnectCommand(profile)
		if err != nil {
			return connectedMsg{Profile: profile, Problem: err.Error()}
		}

		ctx, stop := context.WithTimeout(context.Background(), connectTimeout)
		defer stop()

		session, err := adapters.Open(ctx, profile, password)
		if err != nil {
			preConnect.Stop()
			return connectedMsg{Profile: profile, Problem: db.DescribeError(err)}
		}
		return connectedMsg{Profile: profile, Session: session, PreConnect: preConnect}
	}
}

// readTimeout is how long a catalog read may take.
const readTimeout = 30 * time.Second

// The two ways a catalog read is asked for. A read the user asked for reports when it lands,
// and one the client started itself says nothing.
const (
	announceCatalogRead = true
	quietCatalogRead    = false
)

// readCatalog asks the server what it holds.
func readCatalog(connectionID int, session db.CatalogReader, announce bool) tea.Cmd {
	return func() tea.Msg {
		ctx, stop := context.WithTimeout(context.Background(), readTimeout)
		defer stop()

		answered := catalogReadMsg{ConnectionID: connectionID, Announce: announce}
		tables, err := session.ListTables(ctx)
		if err != nil {
			answered.Problem = db.DescribeError(err)
			return answered
		}
		answered.Tables = tables
		objects, objectErr := session.ListSchemaObjects(ctx)
		roles, roleErr := session.ListRoles(ctx)
		answered.Objects, answered.Roles = objects, roles
		if objectErr != nil {
			answered.PartProblem = db.DescribeError(objectErr)
		} else if roleErr != nil {
			answered.PartProblem = db.DescribeError(roleErr)
		}
		return answered
	}
}

// readTableDetail asks the server for the columns and the keys of one relation.
func readTableDetail(connectionID int, session db.CatalogReader, table db.TableRef) tea.Cmd {
	tableID := present.BuildTableID(table)
	return func() tea.Msg {
		ctx, stop := context.WithTimeout(context.Background(), readTimeout)
		defer stop()

		detail, err := session.DescribeTable(ctx, table)
		if err != nil {
			return tableDetailMsg{
				ConnectionID: connectionID, TableID: tableID,
				Problem: db.DescribeError(err),
			}
		}
		return tableDetailMsg{ConnectionID: connectionID, TableID: tableID, Detail: detail}
	}
}

// runStatements asks the server for one statement of a buffer. The answer dispatches the
// next, so a statement that failed stops the batch: the ones after it were written for a
// state the server no longer holds.
func runStatements(
	connectionID, tabID, runID, index int, session db.Session, reads []db.ComposedRead,
	rowLimit int, undo writeplan.UndoPlan, log *hist.Store, profileName string,
) tea.Cmd {
	if index < 0 || index >= len(reads) {
		return nil
	}
	return runOneStatement(runOneStatementDeps{
		connectionID: connectionID, tabID: tabID, runID: runID, index: index,
		session: session, read: reads[index], rowLimit: rowLimit, undo: undo,
		last: index == len(reads)-1, log: log, profileName: profileName,
	})
}

// runOneStatementDeps is what one statement of a run needs.
type runOneStatementDeps struct {
	connectionID int
	tabID        int
	runID        int
	index        int
	session      db.Session
	read         db.ComposedRead
	rowLimit     int
	undo         writeplan.UndoPlan
	last         bool
	log          *hist.Store
	profileName  string
}

func runOneStatement(deps runOneStatementDeps) tea.Cmd {
	connectionID, tabID, runID, index := deps.connectionID, deps.tabID, deps.runID, deps.index
	session, read, rowLimit := deps.session, deps.read, deps.rowLimit
	log, profileName, last := deps.log, deps.profileName, deps.last
	return func() tea.Msg {
		ctx := context.Background()
		startedAt := time.Now()
		answered := queryRanMsg{
			ConnectionID: connectionID, TabID: tabID, RunID: runID, Index: index,
			Read: read, Last: last,
		}

		result, undo, err := writeplan.RunWithUndo(ctx, session, deps.undo,
			func(running context.Context) (db.QueryResult, error) {
				return session.ReadPage(running, read, db.ReadWindow{Limit: rowLimit})
			})
		answered.Undo = undo
		entry := hist.HistoryEntry{
			ProfileName: profileName, SQL: read.Display, RanAt: startedAt,
			Elapsed: time.Since(startedAt),
		}
		if err != nil {
			answered.Problem = db.DescribeError(err)
			entry.ErrorMessage = answered.Problem
			_ = log.Record(entry)
			return answered
		}
		answered.Result = result
		entry.Elapsed = result.Elapsed
		entry.RowCount, entry.HasRowCount = int64(len(result.Rows)), true
		_ = log.Record(entry)
		return answered
	}
}

// readNextPage asks the server for the rows after the ones already drawn.
func readNextPage(
	connectionID, tabID, index, resultID int, session db.QueryRunner, read db.ComposedRead,
	window db.ReadWindow,
) tea.Cmd {
	return func() tea.Msg {
		result, err := session.ReadPage(context.Background(), read, window)
		if err != nil {
			return pageReadMsg{
				ConnectionID: connectionID, TabID: tabID, Index: index, ResultID: resultID,
				Problem: db.DescribeError(err),
			}
		}
		return pageReadMsg{
			ConnectionID: connectionID, TabID: tabID, Index: index, ResultID: resultID,
			Result: result,
		}
	}
}

// countRows counts the whole result, once.
func countRows(
	connectionID, tabID, index, resultID int, session db.QueryRunner, read db.ComposedRead,
) tea.Cmd {
	return func() tea.Msg {
		total, counted, err := session.CountRead(context.Background(), read)
		if err != nil {
			return countedMsg{
				ConnectionID: connectionID, TabID: tabID, Index: index, ResultID: resultID,
				Problem: db.DescribeError(err),
			}
		}
		return countedMsg{
			ConnectionID: connectionID, TabID: tabID, Index: index, ResultID: resultID,
			Total: total, HasTotal: counted,
		}
	}
}

// readPlan asks the server how it would run the statement.
func readPlan(
	connectionID, tabID, resultID int, session db.QueryRunner, statement string, analyze bool,
) tea.Cmd {
	return func() tea.Msg {
		plan, err := session.ExplainQuery(context.Background(), statement, analyze)
		if err != nil {
			return planReadMsg{
				ConnectionID: connectionID, TabID: tabID, ResultID: resultID,
				Problem: db.DescribeError(err),
			}
		}
		return planReadMsg{
			ConnectionID: connectionID, TabID: tabID, ResultID: resultID, Plan: plan,
		}
	}
}

// readRelationView asks the server for what one view of a relation draws.
func readRelationView(
	connectionID, tabID int, session db.CatalogReader, table db.TableRef, view app.ResultView,
) tea.Cmd {
	return func() tea.Msg {
		ctx, stop := context.WithTimeout(context.Background(), readTimeout)
		defer stop()

		answered := relationViewMsg{ConnectionID: connectionID, TabID: tabID, View: view}
		fail := func(err error) tea.Msg {
			answered.Content = app.PaneContent{
				Kind: app.DataFailed, Message: db.DescribeError(err),
			}
			return answered
		}

		switch view {
		case app.ViewColumns:
			detail, err := session.DescribeTable(ctx, table)
			if err != nil {
				return fail(err)
			}
			answered.Content = app.PaneContent{
				Kind: app.DataColumns, Columns: detail.Columns,
			}
		case app.ViewIndexes:
			indexes, err := session.ListIndexes(ctx, table)
			if err != nil {
				return fail(err)
			}
			answered.Content = app.PaneContent{Kind: app.DataIndexes, Indexes: indexes}
		case app.ViewConstraints:
			constraints, err := session.ListConstraints(ctx, table)
			if err != nil {
				return fail(err)
			}
			answered.Content = app.PaneContent{
				Kind: app.DataConstraints, Constraints: constraints,
			}
		case app.ViewDDL:
			lines, err := session.BuildTableDDL(ctx, table)
			if err != nil {
				return fail(err)
			}
			answered.Content = app.PaneContent{Kind: app.DataDDL, Lines: lines}
		default:
			answered.Content = app.PaneContent{
				Kind: app.DataIdle, Reason: "nothing to describe here",
			}
		}
		return answered
	}
}

// readObjectDDL asks the server for the definition of one schema object.
func readObjectDDL(
	connectionID, tabID int, session db.CatalogReader, object db.SchemaObject,
) tea.Cmd {
	return func() tea.Msg {
		ctx, stop := context.WithTimeout(context.Background(), readTimeout)
		defer stop()

		answered := relationViewMsg{
			ConnectionID: connectionID, TabID: tabID, View: app.ViewDDL,
		}
		lines, err := session.BuildObjectDDL(ctx, object)
		if err != nil {
			answered.Content = app.PaneContent{
				Kind: app.DataFailed, Message: db.DescribeError(err),
			}
			return answered
		}
		answered.Content = app.PaneContent{Kind: app.DataDDL, Lines: lines}
		return answered
	}
}

// applyChanges runs the staged work, all of it or none.
func applyChanges(
	connectionID, tabID int, session db.TransactionKeeper, changes []db.Change,
) tea.Cmd {
	return func() tea.Msg {
		err := session.ApplyChanges(context.Background(), changes)
		if err != nil {
			return changesAppliedMsg{
				ConnectionID: connectionID, TabID: tabID, Problem: db.DescribeError(err),
			}
		}
		return changesAppliedMsg{
			ConnectionID: connectionID, TabID: tabID, Applied: len(changes),
		}
	}
}

// readHistory asks the file for the statements that ran on this profile.
func readHistory(connectionID int, log *hist.Store, profileName string, limit int) tea.Cmd {
	return func() tea.Msg {
		entries, _ := log.ListRecent(profileName, limit)
		return historyReadMsg{ConnectionID: connectionID, Entries: entries}
	}
}

// readSaved asks the file for the statements a reader kept by name.
func readSaved(connectionID int, log *hist.Store, profileName string) tea.Cmd {
	return func() tea.Msg {
		queries, _ := log.ListSaved(profileName)
		return savedReadMsg{ConnectionID: connectionID, Queries: queries}
	}
}

// savedQueryRemovedMsg reports the removal of one saved statement. The note stands only once
// the file has taken the removal.
type savedQueryRemovedMsg struct {
	ConnectionID int
	Name         string
	Problem      string
}

// dropSavedQuery removes one statement from the file away from the loop.
func dropSavedQuery(connectionID int, log *hist.Store, profileName, name string) tea.Cmd {
	return func() tea.Msg {
		answered := savedQueryRemovedMsg{ConnectionID: connectionID, Name: name}
		if err := log.DeleteSaved(profileName, name); err != nil {
			answered.Problem = "the query was not removed: " + db.DescribeError(err)
		}
		return answered
	}
}

// The two ways the dashboard is read.
const (
	readAsked   = false
	readRefresh = true
)

// readActivity asks the server what its other connections are doing, which sessions wait for
// a lock, and the load it is under. Only the session list is reported as a fault.
// dashboardReadTimeout is how long the four reads of one refresh have in all. A read longer
// than several intervals answers with numbers too old to act on.
const dashboardReadTimeout = 6 * time.Second

func readActivity(connectionID int, session db.ServerAdmin, refresh bool) tea.Cmd {
	return func() tea.Msg {
		ctx, stop := context.WithTimeout(context.Background(), dashboardReadTimeout)
		defer stop()

		sessions, err := session.ListActivity(ctx)
		if err != nil {
			return activityReadMsg{
				ConnectionID: connectionID, Problem: db.DescribeError(err),
				Refresh: refresh,
			}
		}
		answered := activityReadMsg{
			ConnectionID: connectionID, Sessions: sessions, Refresh: refresh,
		}
		if locks, lockErr := session.ListLockWaits(ctx); lockErr == nil {
			answered.Locks, answered.HasLocks = locks, true
		}
		if load, loadErr := session.ReadServerLoad(ctx); loadErr == nil {
			answered.Load, answered.HasLoad = load, true
		}
		if slow, slowErr := session.ListSlowStatements(
			ctx, slowStatementRows); slowErr == nil {
			answered.Slow, answered.HasSlow = slow, true
		}
		return answered
	}
}

// readMarks asks the file what the user marked on this profile.
func readMarks(connectionID int, log *hist.Store, profileName string) tea.Cmd {
	return func() tea.Msg {
		favourites, _ := log.ListFavourites(profileName)
		recent, _ := log.ListRecentSchemas(profileName, core.RecentLimit)
		return marksReadMsg{
			ConnectionID: connectionID, Favourites: favourites, Recent: recent,
		}
	}
}

// keepMarks writes a mark to the file. Nothing is drawn for it, so a lost write is the only
// thing it reports.
func keepMarks(
	connectionID int, log *hist.Store, profileName string, favourite core.Favourite,
) tea.Cmd {
	return writeHistory(connectionID, "the mark was not stored", func() error {
		return log.ToggleFavourite(profileName, favourite)
	})
}

// keepVisit records that the user opened a schema.
func keepVisit(connectionID int, log *hist.Store, profileName, schema string) tea.Cmd {
	return writeHistory(connectionID, "the schema visit was not stored", func() error {
		return log.VisitSchema(profileName, schema)
	})
}

// keepCatalog writes the last catalog read of a profile, so a reconnect draws it at once.
func keepCatalog(
	connectionID int, log *hist.Store, profileName string,
	tables []db.TableRef, objects []db.SchemaObject, roles []db.DbRole,
) tea.Cmd {
	return writeHistory(connectionID, "the catalog was not stored", func() error {
		snapshot := hist.CatalogSnapshot{}
		snapshot.Tables, _ = json.Marshal(tables)
		snapshot.Objects, _ = json.Marshal(objects)
		snapshot.Roles, _ = json.Marshal(roles)
		return log.SaveCatalog(profileName, snapshot)
	})
}

// keepSavedQuery writes one statement under a name.
func keepSavedQuery(
	connectionID int, log *hist.Store, profileName, name, statement string,
) tea.Cmd {
	return writeHistory(connectionID, "the query was not stored", func() error {
		return log.SaveQuery(profileName, name, statement)
	})
}

// closeSession ends a connection off the draw loop, so a socket that will not close does
// not hold the frame.
func closeSession(session io.Closer, preConnect *cfg.PreConnectHandle) tea.Cmd {
	return func() tea.Msg {
		_ = session.Close()
		preConnect.Stop()
		return nil
	}
}

// diagramMsg returns the lines of an ER diagram, or why it could not be drawn.
type diagramMsg struct {
	ConnectionID int
	Title        string
	Lines        []string
	Problem      string
}

// readDiagram asks the server for the relation and the relations a foreign key joins to it,
// and draws them. Every read is done here, off the frame, because a diagram reads one
// relation per neighbour.
func readDiagram(
	connectionID int, session db.CatalogReader, table db.TableRef, tables []db.TableRef,
) tea.Cmd {
	return func() tea.Msg {
		ctx, stop := context.WithTimeout(context.Background(), readTimeout)
		defer stop()

		answered := diagramMsg{
			ConnectionID: connectionID,
			Title:        present.QualifyDiagramTable(table.Schema, table.Name),
		}
		describe := func(ref db.TableRef) (present.DiagramTable, error) {
			detail, err := session.DescribeTable(ctx, ref)
			if err != nil {
				return present.DiagramTable{}, err
			}
			foreign := map[string]bool{}
			for _, key := range detail.ForeignKeys {
				for _, name := range key.Columns {
					foreign[strings.ToLower(name)] = true
				}
			}
			columns := make([]present.DiagramColumn, 0, len(detail.Columns))
			for _, column := range detail.Columns {
				columns = append(columns, present.DiagramColumn{
					Name: column.Name, Primary: column.IsPrimaryKey,
					Foreign: foreign[strings.ToLower(column.Name)],
				})
			}
			return present.DiagramTable{
				Schema: ref.Schema, Name: ref.Name,
				Columns: columns, ForeignKeys: detail.ForeignKeys,
			}, nil
		}

		root, err := describe(table)
		if err != nil {
			answered.Problem = db.DescribeError(err)
			return answered
		}
		// A server that cannot list the keys of every relation still draws the ones this
		// relation points at.
		relationships, _ := session.ListRelationships(ctx)
		neighbours := present.CollectDiagramNeighbours(root, relationships)

		related := []present.DiagramTable{}
		for _, ref := range tables {
			if ref.Kind != db.RelationTable ||
				(ref.Schema == table.Schema && ref.Name == table.Name) ||
				!neighbours[present.QualifyDiagramTable(ref.Schema, ref.Name)] {
				continue
			}
			described, err := describe(ref)
			if err != nil {
				continue
			}
			related = append(related, described)
		}
		answered.Lines = present.RenderErDiagram(root, related)
		return answered
	}
}

// The delay before the server is asked what it thinks of the buffer: long enough that no
// request follows a key press, short enough to feel live. Each statement over the count
// costs one request per pause, for faults nobody reads.
const (
	checkDelay        = 400 * time.Millisecond
	checkedStatements = 8
	checkTimeout      = 10 * time.Second
)

// checkDueMsg says the typing stopped, so the buffer may be sent to the server.
type checkDueMsg struct {
	ConnectionID int
	TabID        int
	SQL          string
}

// checkedMsg returns what the server thinks of the buffer.
type checkedMsg struct {
	ConnectionID int
	TabID        int
	SQL          string
	Found        []editor.Diagnostic
}

// scheduleStatementCheck waits for the typing to stop before the buffer is sent.
func scheduleStatementCheck(connectionID, tabID int, sql string) tea.Cmd {
	return tea.Tick(checkDelay, func(time.Time) tea.Msg {
		return checkDueMsg{ConnectionID: connectionID, TabID: tabID, SQL: sql}
	})
}

type statementChecker interface {
	db.SessionInfo
	db.QueryRunner
}

// checkStatements asks the server about each statement of the buffer in turn, and returns
// the faults it named.
func checkStatements(
	connectionID, tabID int, session statementChecker, sql string,
) tea.Cmd {
	return func() tea.Msg {
		ctx, stop := context.WithTimeout(context.Background(), checkTimeout)
		defer stop()

		answered := checkedMsg{ConnectionID: connectionID, TabID: tabID, SQL: sql}
		language := session.Language()
		ranges := language.SplitStatementRanges(sql)
		if len(ranges) > checkedStatements {
			ranges = ranges[:checkedStatements]
		}
		for _, held := range ranges {
			// No server reads a `:name` mark, so a statement with one is checked once
			// the marks have values. A statement that changes the catalog is checked
			// against the state it already changed, so it is left alone.
			if len(statement.FindQueryParameters(held.Text)) > 0 ||
				language.ChangesCatalog(held.Text) {
				continue
			}
			problem, faulty := session.CheckStatement(ctx, held.Text)
			if !faulty {
				continue
			}
			start := held.Start
			if problem.HasOffset {
				start = held.Start + problem.Offset
			}
			if start > held.End {
				start = held.End
			}
			answered.Found = append(answered.Found, editor.Diagnostic{
				Message: problem.Message, Start: start, End: held.End,
			})
		}
		return answered
	}
}

// The check that the server still responds. A tunnel can stop without warning, so the
// connection is asked on an interval and the wait doubles after each failure.
const (
	pingTimeout      = 10 * time.Second
	maxHealthBackoff = 60 * time.Second
)

// healthDueMsg says the wait passed, so the server can be asked whether it responds.
type healthDueMsg struct{ ConnectionID int }

// healthCheckedMsg returns whether the server is still there.
type healthCheckedMsg struct {
	ConnectionID int
	Problem      string
}

// reconnectedMsg returns what an attempt at opening the connection again did.
type reconnectedMsg struct {
	ConnectionID int
	Outcome      db.ReconnectOutcome
}

// ResolveHealthBackoff returns how long to wait before the next check. It doubles after
// each failure, up to a limit.
func ResolveHealthBackoff(failures int, interval time.Duration) time.Duration {
	base := interval
	if base <= 0 {
		base = time.Second
	}
	waited := base
	for at := 1; at < failures; at++ {
		waited *= 2
		if waited >= maxHealthBackoff {
			return maxHealthBackoff
		}
	}
	return waited
}

// scheduleHealthCheck waits before the server is asked again. A profile with no interval
// is never checked.
func scheduleHealthCheck(connectionID int, delay time.Duration) tea.Cmd {
	if delay <= 0 {
		return nil
	}
	return tea.Tick(delay, func(time.Time) tea.Msg {
		return healthDueMsg{ConnectionID: connectionID}
	})
}

// pingSession asks the server anything, to learn whether it still returns.
func pingSession(connectionID int, session db.SessionLifecycle) tea.Cmd {
	return func() tea.Msg {
		ctx, stop := context.WithTimeout(context.Background(), pingTimeout)
		defer stop()

		if err := session.Ping(ctx); err != nil {
			return healthCheckedMsg{
				ConnectionID: connectionID, Problem: db.DescribeError(err),
			}
		}
		return healthCheckedMsg{ConnectionID: connectionID}
	}
}

// reconnectSession opens a connection in place of the one that stopped answering. The
// connection is what comes back, not the work: the server rolled that back already.
func reconnectSession(connectionID int, session db.Session) tea.Cmd {
	reconnectable, canReopen := db.FindReconnectable(session)
	if !canReopen {
		return nil
	}
	return func() tea.Msg {
		ctx, stop := context.WithTimeout(context.Background(), connectTimeout)
		defer stop()

		return reconnectedMsg{
			ConnectionID: connectionID, Outcome: reconnectable.Reconnect(ctx),
		}
	}
}
