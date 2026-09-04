package app

import (
	"slices"
	"time"

	"github.com/turanmahmudov/masume/internal/cfg"
	"github.com/turanmahmudov/masume/internal/core"
	"github.com/turanmahmudov/masume/internal/db"
	"github.com/turanmahmudov/masume/internal/present"
	"github.com/turanmahmudov/masume/internal/writeplan"
)

// HealthState says whether the server still responds.
type HealthState string

// The three states a connection can be in.
const (
	HealthOk           HealthState = "ok"
	HealthReconnecting HealthState = "reconnecting"
	HealthDown         HealthState = "down"
)

// NoticeTone says how strongly a report is drawn.
type NoticeTone string

// The three tones a report can have. An active report says something stands now, such as the
// work staged on a tab.
const (
	NoticeInfo   NoticeTone = "info"
	NoticeActive NoticeTone = "active"
	NoticeError  NoticeTone = "error"
)

// Notice is the short-lived report the status bar shows. An outcome the app decided itself
// takes no key to dismiss.
type Notice struct {
	Text    string
	Tone    NoticeTone
	ShownAt time.Time
}

// How long a report of each tone stays in the bar. An error stays longer, because the user
// can have to act on it.
const (
	NoticeLife      = 4 * time.Second
	NoticeErrorLife = 8 * time.Second
)

// ReadLife returns how long this report stays in the bar.
func (notice *Notice) ReadLife() time.Duration {
	if notice.Tone == NoticeError {
		return NoticeErrorLife
	}
	return NoticeLife
}

// Catalog holds what the server holds: the schemas, the relations and the objects.
type Catalog struct {
	Tables  []db.TableRef
	Objects []db.SchemaObject
	Roles   []db.DbRole
	// The columns of each table, once read. Keyed by the table row id.
	Details map[string]present.TableDetailState
	// When the table list was last read, so it is not trusted for ever.
	ReadAt time.Time
	// True while the first read is still running.
	Loading bool
	// Why the read failed, where it did.
	Problem string
}

// NewCatalog starts a catalog with nothing read yet.
func NewCatalog() *Catalog {
	return &Catalog{Details: map[string]present.TableDetailState{}, Loading: true}
}

// IsStale is true where the table list is old enough to read again.
func (catalog *Catalog) IsStale(now time.Time) bool {
	return catalog.ReadAt.IsZero() || now.Sub(catalog.ReadAt) > core.CatalogTTL
}

// FindTable returns the relation of that name, and whether the catalog holds it.
func (catalog *Catalog) FindTable(schema, name string) (db.TableRef, bool) {
	for _, table := range catalog.Tables {
		if table.Schema == schema && table.Name == name {
			return table, true
		}
	}
	return db.TableRef{}, false
}

// Marks holds what the user marked on this profile: the favourites and the schemas opened
// lately.
type Marks struct {
	Favourites []core.Favourite
	Recent     []core.RecentSchema
}

// ToggleFavourite marks the object, or takes the mark off.
func (marks *Marks) ToggleFavourite(favourite core.Favourite) {
	id := core.BuildFavouriteID(favourite)
	for at, held := range marks.Favourites {
		if core.BuildFavouriteID(held) == id {
			marks.Favourites = append(marks.Favourites[:at], marks.Favourites[at+1:]...)
			return
		}
	}
	marks.Favourites = append(marks.Favourites, favourite)
}

// VisitSchema records that the user opened a schema, so the recent folder lists it.
func (marks *Marks) VisitSchema(schema string, now time.Time) {
	kept := make([]core.RecentSchema, 0, len(marks.Recent)+1)
	kept = append(kept, core.RecentSchema{Schema: schema, VisitedAt: now})
	for _, entry := range marks.Recent {
		if entry.Schema != schema {
			kept = append(kept, entry)
		}
	}
	if len(kept) > core.RecentLimit {
		kept = kept[:core.RecentLimit]
	}
	marks.Recent = kept
}

// Connection is one open connection, with the tabs and the state that belong to it.
type Connection struct {
	Session db.Session
	// What was started to reach the server, which is stopped with the connection.
	PreConnect *cfg.PreConnectHandle
	Catalog    *Catalog
	Marks      *Marks
	Health     HealthState
	// Why the last check failed, and how many failed in a row, which decides the wait.
	HealthProblem  string
	HealthFailures int
	Notice         *Notice

	Tabs        []*Tab
	ActiveIndex int
	// The restored tabs that have not read what they describe yet. A tab reads the
	// first time it is shown, so a connect asks the server for one relation only.
	Unread    map[int]bool
	nextTabID int
	// The number of the last snapshot of the tabs. It rises with every snapshot, so the
	// history file can drop a save that lands after a newer one.
	workspaceChange uint64
	// The tabs closed lately, so the last one can be opened again with what it held.
	closed []*Tab

	// The first tab of the row on screen. It is kept, so the row moves only when the tab
	// the cursor is on would fall outside it.
	TabOffset int

	// True while the object tree is drawn. `Alt+S` gives its columns to the grid.
	SidebarVisible bool
	// False while the result is hidden and the editor has the whole pane.
	ResultVisible bool
	// The rows the editor takes when the result is drawn under it. Zero means the rows the
	// client opens with; a drag of the border between the two panes sets it.
	EditorHeight int
	// True while the user holds a transaction open by hand.
	Autocommit bool

	// The last write that was measured, held so it can be undone.
	Undo *HeldUndo

	// The overlay on top, which owns the keyboard while it is open.
	Overlay Overlay
	// The tree of this connection: which rows are folded, and where the cursor is.
	Tree TreeState
	// The chat of this connection: what was said, and what the reply is doing.
	Chat *Chat
	// stopExport ends the export that streams now. Reading every row can take minutes, so
	// the cancel key stops it as it stops a statement.
	stopExport func()
	stopImport func()

	// The object tree as it was last built, and the state it was built from. The whole
	// catalog goes into one, so it is kept until something it reads changes.
	treeAt     treeFingerprint
	treeResult present.TreeResult
	treeBuilt  bool
}

// HeldUndo is the undo of one write, and the write it reverses.
type HeldUndo struct {
	Undo  writeplan.Undo
	SQL   string
	RanAt time.Time
}

// KeepUndo holds the undo of the write that just ran. Only the last one is kept: an older
// one restores rows the newer write has changed since. A write that kept no undo is held as
// well, so the key that runs one can say why there is none.
func (connection *Connection) KeepUndo(undo writeplan.Undo, sql string, now time.Time) {
	connection.Undo = &HeldUndo{Undo: undo, SQL: sql, RanAt: now}
}

// TreeState holds the view state of the object tree, which is not application state.
type TreeState struct {
	Expanded map[string]bool
	Cursor   int
	Offset   int
	// True while the wheel moved the rows away from the cursor, so the cursor may stand
	// off screen until it moves again.
	Rolled bool
	Filter string
	// The schema a filter was opened inside, so it searches that schema alone.
	FilterScope string
	// True while the filter field holds the keyboard.
	Filtering bool
	// True while the system schemas are folded away.
	HideSystemSchemas bool
	// True once the first read seeded the folds.
	seeded bool
}

// NewConnection opens the state of one connection, with one query tab.
func NewConnection(
	session db.Session, preConnect *cfg.PreConnectHandle, hideSystemSchemas bool,
) *Connection {
	connection := &Connection{
		Session: session, PreConnect: preConnect,
		Catalog: NewCatalog(), Marks: &Marks{}, Health: HealthOk,
		SidebarVisible: true, ResultVisible: true,
		Autocommit: session.Describe().Profile.Autocommit,
		Tree: TreeState{
			Expanded: map[string]bool{}, HideSystemSchemas: hideSystemSchemas,
		},
		Chat: NewChat(),
	}
	connection.nextTabID = 1
	connection.Tabs = []*Tab{NewQueryTab(1, "")}
	connection.Unread = map[int]bool{}
	return connection
}

// Profile returns the profile this connection was opened from.
func (connection *Connection) Profile() cfg.Profile {
	return connection.Session.Describe().Profile
}

// Active returns the tab on screen. A connection always holds one tab: NewConnection opens
// one, CloseTab refuses to close the last, and RestoreTabs keeps the one it opened with
// where the file named none. The empty answer is the floor under those three, and no caller
// is written for it.
func (connection *Connection) Active() *Tab {
	if len(connection.Tabs) == 0 {
		return nil
	}
	return connection.Tabs[core.ClampIndex(connection.ActiveIndex, len(connection.Tabs))]
}

// BeginExport keeps the way to stop the export that starts now, and ends the one before it.
func (connection *Connection) BeginExport(stop func()) {
	connection.StopExport()
	connection.stopExport = stop
}

// StopExport ends the export that streams now, where one does.
func (connection *Connection) StopExport() {
	if connection.stopExport == nil {
		return
	}
	connection.stopExport()
	connection.stopExport = nil
}

// BeginImport holds what ends the import that writes now, so closing the card stops it.
func (connection *Connection) BeginImport(stop func()) {
	connection.StopImport()
	connection.stopImport = stop
}

// StopImport ends the import that writes now, where one does. The rows already written are
// inside a transaction that the server rolls back when the connection drops the statement.
func (connection *Connection) StopImport() {
	if connection.stopImport == nil {
		return
	}
	connection.stopImport()
	connection.stopImport = nil
}

// Show reports an outcome the app decided itself, which takes no key to dismiss.
func (connection *Connection) Show(text string) {
	connection.Notice = &Notice{Text: text, Tone: NoticeInfo, ShownAt: time.Now()}
}

// ShowError reports a failure in the bar.
func (connection *Connection) ShowError(text string) {
	connection.Notice = &Notice{Text: text, Tone: NoticeError, ShownAt: time.Now()}
}

// DropStaleNotice takes the report away once it has been on screen long enough.
func (connection *Connection) DropStaleNotice(now time.Time) {
	if connection.Notice != nil && now.Sub(connection.Notice.ShownAt) > connection.Notice.ReadLife() {
		connection.Notice = nil
	}
}

// appendTab puts the tab after the last one and moves to it.
func (connection *Connection) appendTab(tab *Tab) *Tab {
	connection.Tabs = append(connection.Tabs, tab)
	connection.ActiveIndex = len(connection.Tabs) - 1
	return tab
}

// showTab puts the tab in the place of the blank one on show, or after the last one. A read
// opened from the tree or the history leaves no empty tab behind it.
func (connection *Connection) showTab(tab *Tab) *Tab {
	standing := connection.Active()
	if standing == nil || !standing.IsBlank() {
		return connection.appendTab(tab)
	}
	connection.Tabs[connection.ActiveIndex] = tab
	return tab
}

// OpenQueryTab opens a tab bound to the text in its editor. An empty tab is always a new
// one, and a statement takes the place of a blank tab.
func (connection *Connection) OpenQueryTab(sql string) *Tab {
	connection.nextTabID++
	tab := NewQueryTab(connection.nextTabID, sql)
	if sql == "" {
		return connection.appendTab(tab)
	}
	return connection.showTab(tab)
}

// OpenTable opens the relation, or focuses the tab that already holds it. Walking the tree
// must not leave a row of identical tabs behind.
func (connection *Connection) OpenTable(table db.TableRef, preview string) *Tab {
	for at, tab := range connection.Tabs {
		if tab.Kind == TabTable && tab.Table.Schema == table.Schema && tab.Table.Name == table.Name {
			connection.ActiveIndex = at
			return tab
		}
	}
	return connection.OpenTableInNewTab(table, preview)
}

// OpenTableInNewTab asks for a second tab on the same relation on purpose, which is how two
// filters of one table are compared side by side.
func (connection *Connection) OpenTableInNewTab(table db.TableRef, preview string) *Tab {
	connection.nextTabID++
	return connection.showTab(NewTableTab(connection.nextTabID, table, preview))
}

// OpenObject opens a tab that shows the definition of one schema object.
func (connection *Connection) OpenObject(object db.SchemaObject) *Tab {
	for at, tab := range connection.Tabs {
		if tab.Kind == TabObject && tab.Object.Schema == object.Schema &&
			tab.Object.Name == object.Name && tab.Object.Kind == object.Kind {
			connection.ActiveIndex = at
			return tab
		}
	}
	connection.nextTabID++
	return connection.showTab(NewObjectTab(connection.nextTabID, object))
}

// CloseTab closes the tab at that position. The last tab cannot be closed.
func (connection *Connection) CloseTab(index int) {
	if len(connection.Tabs) <= 1 || index < 0 || index >= len(connection.Tabs) {
		return
	}
	connection.closed = append(connection.closed, connection.Tabs[index])
	connection.Tabs = append(connection.Tabs[:index], connection.Tabs[index+1:]...)
	connection.ActiveIndex = core.ClampIndex(connection.ActiveIndex, len(connection.Tabs))
}

// IndexOfTab returns where the tab of that id stands, and -1 where the connection has none.
func (connection *Connection) IndexOfTab(id int) int {
	for at, tab := range connection.Tabs {
		if tab.ID == id {
			return at
		}
	}
	return -1
}

// HasClosedTab is true where a tab was closed and can be opened again.
func (connection *Connection) HasClosedTab() bool {
	return len(connection.closed) > 0
}

// ReopenTab opens the tab closed last, with the statement it held.
func (connection *Connection) ReopenTab() bool {
	if len(connection.closed) == 0 {
		return false
	}
	tab := connection.closed[len(connection.closed)-1]
	connection.closed = connection.closed[:len(connection.closed)-1]
	connection.showTab(tab)
	return true
}

// ActivateTab moves to the tab at that position.
func (connection *Connection) ActivateTab(index int) {
	connection.ActiveIndex = core.ClampIndex(index, len(connection.Tabs))
}

// StepTab moves to the tab before or after the one on screen, and wraps at the ends.
func (connection *Connection) StepTab(step int) {
	connection.ActiveIndex = core.WrapIndex(connection.ActiveIndex+step, len(connection.Tabs))
}

// SeedTree opens the folds a first read of the catalog asks for.
func (connection *Connection) SeedTree() {
	if connection.Tree.seeded {
		return
	}
	if len(connection.Catalog.Tables) == 0 && len(connection.Catalog.Objects) == 0 {
		return
	}
	connection.Tree.seeded = true
	connection.Tree.Expanded = present.CollectDefaultExpanded(
		connection.Catalog.Tables, connection.Catalog.Objects,
		connection.Marks.Favourites, connection.Marks.Recent)
}

// CompletionSources returns the part of the completion that belongs to the connection. Each
// tab adds its own columns.
func (connection *Connection) CompletionSources() (schemas, tables, functions []string) {
	offered := func(schema string) bool {
		if !connection.Tree.HideSystemSchemas {
			return true
		}
		return !present.IsSystemSchema(schema, connection.Profile().Engine)
	}

	seenSchemas := map[string]bool{}
	for _, table := range connection.Catalog.Tables {
		if !offered(table.Schema) {
			continue
		}
		if !seenSchemas[table.Schema] {
			seenSchemas[table.Schema] = true
			schemas = append(schemas, table.Schema)
		}
		// Both forms, because a table outside the search path needs its schema.
		tables = append(tables, table.Name, table.Schema+"."+table.Name)
	}
	slices.Sort(schemas)

	for _, object := range connection.Catalog.Objects {
		if object.Kind != db.ObjectFunction || !offered(object.Schema) {
			continue
		}
		// A routine is called by name, and the list was already read for the tree.
		functions = append(functions, object.Name, object.Schema+"."+object.Name)
	}
	return schemas, tables, functions
}
