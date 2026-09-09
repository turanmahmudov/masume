package app

import (
	"slices"
	"time"

	"github.com/turanmahmudov/masume/internal/cfg"
	"github.com/turanmahmudov/masume/internal/core"
	"github.com/turanmahmudov/masume/internal/db"
	"github.com/turanmahmudov/masume/internal/notebook"
	"github.com/turanmahmudov/masume/internal/present"
	"github.com/turanmahmudov/masume/internal/writeplan"
)

// HealthState is the connection health status.
type HealthState string

// The three states a connection can be in.
const (
	HealthOk           HealthState = "ok"
	HealthReconnecting HealthState = "reconnecting"
	HealthDown         HealthState = "down"
)

// NoticeTone is the notice display category.
type NoticeTone string

// Notice categories. Active notices describe current state, such as staged changes.
const (
	NoticeInfo   NoticeTone = "info"
	NoticeActive NoticeTone = "active"
	NoticeError  NoticeTone = "error"
)

// Notice is a temporary status bar message.
type Notice struct {
	Text    string
	Tone    NoticeTone
	ShownAt time.Time
}

// Status bar notice lifetimes.
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

// Catalog is the loaded server metadata.
type Catalog struct {
	// The schemas the server holds, including one that holds no relation and no object.
	Schemas []string
	Tables  []db.TableRef
	Objects []db.SchemaObject
	Roles   []db.DbRole
	// The columns of each table, once read. Keyed by the table row id.
	Details map[string]present.TableDetailState
	// The last catalog refresh time.
	ReadAt time.Time
	// True while the first read is still running.
	Loading bool
	// The catalog loading error, if any.
	Problem string
}

// NewCatalog creates an unloaded catalog.
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

// Marks is the profile favourites and recently visited schemas.
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

// VisitSchema updates the recent schema list.
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
	// The last health error and consecutive failure count.
	HealthProblem  string
	HealthFailures int
	Notice         *Notice

	Tabs        []*Tab
	ActiveIndex int
	// Restored tabs awaiting their first data request.
	Unread    map[int]bool
	nextTabID int
	// The increasing snapshot sequence for rejecting stale saves.
	workspaceChange uint64
	// Recently closed tabs available for reopening.
	closed []*Tab

	// The first visible tab index.
	TabOffset int

	// True while the object tree is drawn. `Alt+S` gives its columns to the grid.
	SidebarVisible bool
	// The columns the object tree asks for, which a drag of its border sets. Zero asks for
	// the width the client opens with.
	SidebarWidth int
	// False while the result is hidden and the editor has the whole pane.
	ResultVisible bool
	// The editor height in rows. Zero uses the default height.
	EditorHeight int
	// True when statements use autocommit mode.
	Autocommit bool

	// Undo data or an unavailable reason for the last recorded write.
	Undo *HeldUndo

	// The cell a copy or a cut of the cell list took, which a paste inserts.
	NotebookClip *NotebookCell

	// The overlay on top, which owns the keyboard while it is open.
	Overlay Overlay
	// The tree of this connection: which rows are folded, and where the cursor is.
	Tree TreeState
	// The connection chat and current response state.
	Chat *Chat
	// stopExport is the active export cancellation function.
	stopExport func()
	stopImport func()

	// The cached object tree and its input fingerprint.
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

// KeepUndo replaces the stored undo with the latest write, including the reason when undo is unavailable.
func (connection *Connection) KeepUndo(undo writeplan.Undo, sql string, now time.Time) {
	connection.Undo = &HeldUndo{Undo: undo, SQL: sql, RanAt: now}
}

// TreeState holds the view state of the object tree, which is not application state.
type TreeState struct {
	Expanded map[string]bool
	Cursor   int
	Offset   int
	// True after scrolling independently of the cursor.
	Rolled bool
	Filter string
	// The schema restriction for the tree filter.
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

// Active returns the selected tab, or nil for an empty tab list.
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

// BeginImport replaces the active import cancellation function and stops the previous import.
func (connection *Connection) BeginImport(stop func()) {
	connection.StopImport()
	connection.stopImport = stop
}

// StopImport requests cancellation of the active import.
func (connection *Connection) StopImport() {
	if connection.stopImport == nil {
		return
	}
	connection.stopImport()
	connection.stopImport = nil
}

// Show displays an informational notice.
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

// showTab replaces a blank active tab or appends a new tab.
func (connection *Connection) showTab(tab *Tab) *Tab {
	standing := connection.Active()
	if standing == nil || !standing.IsBlank() {
		return connection.appendTab(tab)
	}
	connection.Tabs[connection.ActiveIndex] = tab
	return tab
}

// OpenQueryTab opens a query tab. Nonempty statements can replace a blank active tab.
func (connection *Connection) OpenQueryTab(sql string) *Tab {
	connection.nextTabID++
	tab := NewQueryTab(connection.nextTabID, sql)
	if sql == "" {
		return connection.appendTab(tab)
	}
	return connection.showTab(tab)
}

// OpenNotebook opens a notebook tab. A notebook already open in a tab comes forward.
func (connection *Connection) OpenNotebook(
	book notebook.Notebook, path string, origin notebook.Origin,
) *Tab {
	if path != "" {
		for at, tab := range connection.Tabs {
			if tab.Kind == TabNotebook && tab.Notebook != nil && tab.Notebook.Path == path {
				connection.ActiveIndex = at
				return tab
			}
		}
	}
	connection.nextTabID++
	return connection.showTab(NewNotebookTab(connection.nextTabID, book, path, origin))
}

// OpenNotebookInNewTab opens a notebook in a tab of its own, even where another tab holds
// the same notebook.
func (connection *Connection) OpenNotebookInNewTab(
	book notebook.Notebook, path string, origin notebook.Origin,
) *Tab {
	connection.nextTabID++
	return connection.showTab(NewNotebookTab(connection.nextTabID, book, path, origin))
}

// OpenTable focuses an existing table tab or opens a new table tab.
func (connection *Connection) OpenTable(table db.TableRef, preview string) *Tab {
	for at, tab := range connection.Tabs {
		if tab.Kind == TabTable && tab.Table.Schema == table.Schema && tab.Table.Name == table.Name {
			connection.ActiveIndex = at
			return tab
		}
	}
	return connection.OpenTableInNewTab(table, preview)
}

// OpenTableInNewTab opens a separate table tab even when another tab has the same table.
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

// closedTabDepth is the maximum retained closed tabs, including their results.
const closedTabDepth = 10

// CloseTab closes the tab at that position. The last tab cannot be closed.
func (connection *Connection) CloseTab(index int) {
	if len(connection.Tabs) <= 1 || index < 0 || index >= len(connection.Tabs) {
		return
	}
	connection.closed = append(connection.closed, connection.Tabs[index])
	if len(connection.closed) > closedTabDepth {
		copy(connection.closed, connection.closed[1:])
		connection.closed[len(connection.closed)-1] = nil
		connection.closed = connection.closed[:len(connection.closed)-1]
	}
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
	connection.closed[len(connection.closed)-1] = nil
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

// CompletionSources returns schema, table, and routine suggestions from the connection catalog.
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
		// Include qualified and unqualified table names.
		tables = append(tables, table.Name, table.Schema+"."+table.Name)
	}
	slices.Sort(schemas)

	for _, object := range connection.Catalog.Objects {
		if object.Kind != db.ObjectFunction || !offered(object.Schema) {
			continue
		}
		// Include qualified and unqualified routine names.
		functions = append(functions, object.Name, object.Schema+"."+object.Name)
	}
	return schemas, tables, functions
}
