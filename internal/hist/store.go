// Package hist stores the data this client writes itself: the statements that ran, the
// queries the user saved, the open tabs of a connection, the last catalog read of a profile,
// the chats and the favourites. One SQLite file holds all of it.
package hist

import (
	"database/sql"
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"time"

	_ "modernc.org/sqlite"

	"github.com/turanmahmudov/masume/internal/core"
)

// HistoryEntry is one statement that ran, with the id from the file.
type HistoryEntry struct {
	ID           int64
	ProfileName  string
	SQL          string
	RanAt        time.Time
	Elapsed      time.Duration
	RowCount     int64
	HasRowCount  bool
	ErrorMessage string
}

// SavedQuery is one statement the user saved under a name.
type SavedQuery struct {
	Name    string
	SQL     string
	SavedAt time.Time
}

// SavedTabState is the sort and the filter the grid applies to a statement. The buffer does
// not hold them.
type SavedTabState struct {
	// The caret position, so a restored tab opens at the same place.
	Caret  int               `json:"caret"`
	Sort   []core.SortState  `json:"sort"`
	Filter []core.FilterStep `json:"filter"`
}

// SavedTab is one tab in stored form. A query tab holds its buffer. A table tab holds the
// table, and an object tab holds the object. The statements of those two are generated.
type SavedTab struct {
	Kind   string `json:"kind"`
	SQL    string `json:"sql,omitempty"`
	Schema string `json:"schema,omitempty"`
	Name   string `json:"name,omitempty"`
	// The kind of table a table tab was opened on.
	TableKind string `json:"tableKind,omitempty"`
	// The kind of object an object tab shows.
	ObjectKind string `json:"objectKind,omitempty"`
	// The handle the server uses to find the definition. A name is not sufficient.
	Identity string        `json:"identity,omitempty"`
	State    SavedTabState `json:"state"`
}

// SavedWorkspace holds the open tabs and the one that was active.
type SavedWorkspace struct {
	Tabs        []SavedTab `json:"tabs"`
	ActiveIndex int        `json:"activeIndex"`
	// Change is the number of this snapshot. It increases with every snapshot, so a save
	// that arrives after a newer one is skipped. A zero is always written, so a caller
	// that sets no number keeps the previous behaviour.
	Change uint64 `json:"-"`
}

// CatalogSnapshot is the object tree of the last read, so a reconnect can draw it
// immediately.
type CatalogSnapshot struct {
	Tables  json.RawMessage `json:"tables"`
	Objects json.RawMessage `json:"objects"`
	Roles   json.RawMessage `json:"roles"`
	Version int             `json:"version"`
}

// cacheVersion is written into the cached payload, so a payload of an older format is
// discarded and not parsed.
const cacheVersion = 1

// Store is one history file and its content.
//
// The client holds a nil store if the file could not be opened. Every method works on a nil
// store: a read returns nothing and a write is skipped, both without an error. A user who
// cannot write a history can still open every connection, so the nil check is here and not
// at every call site.
type Store struct {
	file *sql.DB
	// workspaceGuard allows one workspace save at a time, and workspaceChange holds the
	// number of the last snapshot written for each profile. Every save runs in its own
	// goroutine, so without these a late snapshot would overwrite a newer one.
	workspaceGuard  sync.Mutex
	workspaceChange map[string]uint64
}

// schema creates every table the store reads and writes. `CREATE TABLE IF NOT EXISTS`
// keeps an existing table unchanged, so a file from an earlier version is upgraded here.
const schema = `
  CREATE TABLE IF NOT EXISTS query_history (
    id            INTEGER PRIMARY KEY AUTOINCREMENT,
    profile_name  TEXT    NOT NULL,
    sql           TEXT    NOT NULL,
    ran_at        INTEGER NOT NULL,
    duration_ms   REAL    NOT NULL,
    row_count     INTEGER,
    error_message TEXT
  );
  CREATE INDEX IF NOT EXISTS query_history_profile_idx
    ON query_history (profile_name, ran_at DESC);

  CREATE TABLE IF NOT EXISTS saved_query (
    profile_name TEXT    NOT NULL,
    name         TEXT    NOT NULL,
    sql          TEXT    NOT NULL,
    saved_at     INTEGER NOT NULL,
    PRIMARY KEY (profile_name, name)
  );

  -- One row per tab.
  CREATE TABLE IF NOT EXISTS workspace_tab (
    profile_name TEXT    NOT NULL,
    position     INTEGER NOT NULL,
    kind         TEXT    NOT NULL,
    sql          TEXT    NOT NULL,
    schema_name  TEXT,
    table_name   TEXT,
    table_kind   TEXT,
    identity     TEXT,
    is_active    INTEGER NOT NULL,
    saved_at     INTEGER NOT NULL,
    state        TEXT,
    PRIMARY KEY (profile_name, position)
  );

  CREATE TABLE IF NOT EXISTS catalog_cache (
    profile_name TEXT    PRIMARY KEY,
    payload      TEXT    NOT NULL,
    cached_at    INTEGER NOT NULL
  );

  -- A schema is stored with an empty table name, so one table holds both kinds of mark
  -- and the primary key keeps a mark from being made twice.
  CREATE TABLE IF NOT EXISTS favourite_object (
    profile_name TEXT    NOT NULL,
    schema_name  TEXT    NOT NULL,
    table_name   TEXT    NOT NULL,
    marked_at    INTEGER NOT NULL,
    PRIMARY KEY (profile_name, schema_name, table_name)
  );

  -- The order is kept by a counter rather than by the clock: two schemas opened inside
  -- one millisecond read as equal, and which of them came last is what this table is for.
  CREATE TABLE IF NOT EXISTS recent_schema (
    profile_name TEXT    NOT NULL,
    schema_name  TEXT    NOT NULL,
    visited_at   INTEGER NOT NULL,
    visit_seq    INTEGER NOT NULL,
    PRIMARY KEY (profile_name, schema_name)
  );

  CREATE TABLE IF NOT EXISTS chat_conversation (
    id           INTEGER PRIMARY KEY AUTOINCREMENT,
    profile_name TEXT    NOT NULL,
    title        TEXT    NOT NULL,
    started_at   INTEGER NOT NULL,
    updated_at   INTEGER NOT NULL
  );

  CREATE TABLE IF NOT EXISTS chat_turn (
    conversation_id INTEGER NOT NULL,
    position        INTEGER NOT NULL,
    role            TEXT    NOT NULL,
    content         TEXT    NOT NULL,
    context         TEXT,
    PRIMARY KEY (conversation_id, position)
  );
`

// Open opens the history file and creates it if there is none. The file holds every
// statement that ran, and with it the values in those statements, so only the owner can read
// it.
func Open(path string) (*Store, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, err
	}
	file, err := sql.Open("sqlite", "file:"+path+"?_pragma=journal_mode(WAL)")
	if err != nil {
		return nil, err
	}
	file.SetMaxOpenConns(1)
	if _, execErr := file.Exec(schema); execErr != nil {
		_ = file.Close()
		return nil, execErr
	}
	// WAL mode writes two files next to the database, and both hold rows until a
	// checkpoint.
	for _, beside := range []string{path, path + "-wal", path + "-shm"} {
		if err := os.Chmod(beside, 0o600); err != nil && !os.IsNotExist(err) {
			_ = file.Close()
			return nil, err
		}
	}
	return &Store{file: file, workspaceChange: map[string]uint64{}}, nil
}

// DefaultPath returns the path of the history file.
func DefaultPath() string {
	return core.ResolveStatePath("history.sqlite")
}

// Close releases the resources of the store.
func (store *Store) Close() error {
	if store == nil || store.file == nil {
		return nil
	}
	return store.file.Close()
}

// Record stores one statement that ran.
func (store *Store) Record(entry HistoryEntry) error {
	if store == nil {
		return nil
	}
	var rowCount any
	if entry.HasRowCount {
		rowCount = entry.RowCount
	}
	var message any
	if entry.ErrorMessage != "" {
		message = entry.ErrorMessage
	}
	_, err := store.file.Exec(
		`INSERT INTO query_history
		   (profile_name, sql, ran_at, duration_ms, row_count, error_message)
		 VALUES (?, ?, ?, ?, ?, ?)`,
		entry.ProfileName, entry.SQL, entry.RanAt.UnixMilli(),
		float64(entry.Elapsed)/float64(time.Millisecond), rowCount, message)
	return err
}

// ListRecent returns the newest statements of one profile.
func (store *Store) ListRecent(profileName string, limit int) ([]HistoryEntry, error) {
	if store == nil {
		return nil, nil
	}
	rows, err := store.file.Query(
		`SELECT id, profile_name, sql, ran_at, duration_ms, row_count, error_message
		   FROM query_history
		  WHERE profile_name = ?
		  ORDER BY ran_at DESC, id DESC
		  LIMIT ?`, profileName, limit)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	entries := []HistoryEntry{}
	for rows.Next() {
		entry := HistoryEntry{}
		var ranAt int64
		var elapsed float64
		var rowCount sql.NullInt64
		var message sql.NullString
		if scanErr := rows.Scan(&entry.ID, &entry.ProfileName, &entry.SQL,
			&ranAt, &elapsed, &rowCount, &message); scanErr != nil {
			return nil, scanErr
		}
		entry.RanAt = time.UnixMilli(ranAt)
		entry.Elapsed = time.Duration(elapsed * float64(time.Millisecond))
		entry.RowCount, entry.HasRowCount = rowCount.Int64, rowCount.Valid
		entry.ErrorMessage = message.String
		entries = append(entries, entry)
	}
	return entries, rows.Err()
}

// SaveQuery stores one statement under a name and replaces the statement with that name.
func (store *Store) SaveQuery(profileName, name, statement string) error {
	if store == nil {
		return nil
	}
	_, err := store.file.Exec(
		`INSERT INTO saved_query (profile_name, name, sql, saved_at)
		 VALUES (?, ?, ?, ?)
		 ON CONFLICT (profile_name, name)
		 DO UPDATE SET sql = excluded.sql, saved_at = excluded.saved_at`,
		profileName, name, statement, time.Now().UnixMilli())
	return err
}

// ListSaved returns the saved statements of one profile, sorted by name.
func (store *Store) ListSaved(profileName string) ([]SavedQuery, error) {
	if store == nil {
		return nil, nil
	}
	rows, err := store.file.Query(
		`SELECT name, sql, saved_at FROM saved_query WHERE profile_name = ? ORDER BY name`,
		profileName)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	queries := []SavedQuery{}
	for rows.Next() {
		saved := SavedQuery{}
		var savedAt int64
		if scanErr := rows.Scan(&saved.Name, &saved.SQL, &savedAt); scanErr != nil {
			return nil, scanErr
		}
		saved.SavedAt = time.UnixMilli(savedAt)
		queries = append(queries, saved)
	}
	return queries, rows.Err()
}

// DeleteSaved removes one saved statement.
func (store *Store) DeleteSaved(profileName, name string) error {
	if store == nil {
		return nil
	}
	_, err := store.file.Exec(
		`DELETE FROM saved_query WHERE profile_name = ? AND name = ?`, profileName, name)
	return err
}

// tabStateVersion is written into the payload, so a payload of an older format is discarded
// and not parsed.
const tabStateVersion = 2

// savedTabPayload is the sort and the filter of a tab in the form the history file stores.
type savedTabPayload struct {
	Version int               `json:"version"`
	Caret   int               `json:"caret"`
	Sort    []core.SortState  `json:"sort"`
	Filter  []core.FilterStep `json:"filter"`
}

// SaveWorkspace stores the open tabs of a connection. It replaces the stored tabs, so a
// closed tab is deleted from the file. A snapshot older than the stored one is skipped, and
// only one save runs at a time.
func (store *Store) SaveWorkspace(profileName string, workspace SavedWorkspace) error {
	if store == nil {
		return nil
	}
	store.workspaceGuard.Lock()
	defer store.workspaceGuard.Unlock()
	if workspace.Change != 0 {
		if workspace.Change <= store.workspaceChange[profileName] {
			return nil
		}
		store.workspaceChange[profileName] = workspace.Change
	}

	transaction, err := store.file.Begin()
	if err != nil {
		return err
	}
	defer func() { _ = transaction.Rollback() }()

	if _, err := transaction.Exec(
		`DELETE FROM workspace_tab WHERE profile_name = ?`, profileName); err != nil {
		return err
	}

	savedAt := time.Now().UnixMilli()
	for position, tab := range workspace.Tabs {
		payload, marshalErr := json.Marshal(savedTabPayload{
			Version: tabStateVersion, Caret: tab.State.Caret,
			Sort: tab.State.Sort, Filter: tab.State.Filter,
		})
		if marshalErr != nil {
			return marshalErr
		}
		active := 0
		if position == workspace.ActiveIndex {
			active = 1
		}
		if _, err := transaction.Exec(
			`INSERT INTO workspace_tab
			   (profile_name, position, kind, sql, schema_name, table_name, table_kind,
			    identity, is_active, saved_at, state)
			 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			profileName, position, tab.Kind, tab.SQL,
			nullableText(tab.Schema), nullableText(tab.Name),
			nullableText(readSavedTabKind(tab)), nullableText(tab.Identity),
			active, savedAt, string(payload)); err != nil {
			return err
		}
	}
	return transaction.Commit()
}

// readSavedTabKind returns the kind of the table or of the object. Both use the same
// column.
func readSavedTabKind(tab SavedTab) string {
	if tab.Kind == "table" {
		return tab.TableKind
	}
	if tab.Kind == "object" {
		return tab.ObjectKind
	}
	return ""
}

// nullableText converts an empty text into a null.
func nullableText(written string) any {
	if written == "" {
		return nil
	}
	return written
}

// FindWorkspace returns the tabs of the profile, and nothing for a profile that was never
// opened. A file that cannot be read gives an error, because the tabs are the work of the
// user and a silent loss looks like a bug in the client.
func (store *Store) FindWorkspace(profileName string) (SavedWorkspace, bool, error) {
	if store == nil {
		return SavedWorkspace{}, false, nil
	}
	rows, err := store.file.Query(
		`SELECT kind, sql, schema_name, table_name, table_kind, identity, is_active, state
		   FROM workspace_tab
		  WHERE profile_name = ?
		  ORDER BY position`, profileName)
	if err != nil {
		return SavedWorkspace{}, false, err
	}
	defer func() { _ = rows.Close() }()

	workspace := SavedWorkspace{}
	active := -1
	for rows.Next() {
		tab := SavedTab{}
		var schema, name, kind, identity, payload sql.NullString
		var isActive int
		if scanErr := rows.Scan(&tab.Kind, &tab.SQL, &schema, &name, &kind, &identity,
			&isActive, &payload); scanErr != nil {
			return SavedWorkspace{}, false, scanErr
		}
		tab.Schema, tab.Name, tab.Identity = schema.String, name.String, identity.String
		switch tab.Kind {
		case "object":
			tab.ObjectKind = kind.String
		case "table":
			tab.TableKind = kind.String
			if tab.TableKind == "" {
				tab.TableKind = "table"
			}
		}
		tab.State = readSavedTabState(payload.String)
		if isActive == 1 && active == -1 {
			active = len(workspace.Tabs)
		}
		workspace.Tabs = append(workspace.Tabs, tab)
	}
	if rowsErr := rows.Err(); rowsErr != nil {
		return SavedWorkspace{}, false, rowsErr
	}
	if len(workspace.Tabs) == 0 {
		return SavedWorkspace{}, false, nil
	}
	if active > 0 {
		workspace.ActiveIndex = active
	}
	return workspace, true, nil
}

// readSavedTabState parses the payload. A payload of another version gives an empty state,
// so a format this client does not know is discarded and not parsed.
func readSavedTabState(payload string) SavedTabState {
	if payload == "" {
		return SavedTabState{}
	}
	read := savedTabPayload{}
	if json.Unmarshal([]byte(payload), &read) != nil {
		return SavedTabState{}
	}
	if read.Version != tabStateVersion {
		return SavedTabState{}
	}
	caret := max(read.Caret, 0)
	return SavedTabState{Caret: caret, Sort: read.Sort, Filter: read.Filter}
}

// SaveCatalog stores the last catalog read of a profile.
func (store *Store) SaveCatalog(profileName string, snapshot CatalogSnapshot) error {
	if store == nil {
		return nil
	}
	snapshot.Version = cacheVersion
	payload, err := json.Marshal(snapshot)
	if err != nil {
		return err
	}
	_, execErr := store.file.Exec(
		`INSERT INTO catalog_cache (profile_name, payload, cached_at)
		 VALUES (?, ?, ?)
		 ON CONFLICT (profile_name)
		 DO UPDATE SET payload = excluded.payload, cached_at = excluded.cached_at`,
		profileName, string(payload), time.Now().UnixMilli())
	return execErr
}

// FindCatalog returns the last catalog read of a profile, so a connection draws the tree
// before the server responds.
func (store *Store) FindCatalog(profileName string) (CatalogSnapshot, bool) {
	if store == nil {
		return CatalogSnapshot{}, false
	}
	var payload string
	row := store.file.QueryRow(
		`SELECT payload FROM catalog_cache WHERE profile_name = ?`, profileName)
	if row.Scan(&payload) != nil {
		return CatalogSnapshot{}, false
	}
	snapshot := CatalogSnapshot{}
	if json.Unmarshal([]byte(payload), &snapshot) != nil {
		return CatalogSnapshot{}, false
	}
	if snapshot.Version != cacheVersion {
		return CatalogSnapshot{}, false
	}
	return snapshot, true
}

// ListFavourites returns the objects the user marked on that profile.
func (store *Store) ListFavourites(profileName string) ([]core.Favourite, error) {
	if store == nil {
		return nil, nil
	}
	rows, err := store.file.Query(
		`SELECT schema_name, table_name FROM favourite_object
		  WHERE profile_name = ?
		  ORDER BY marked_at, schema_name, table_name`, profileName)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	favourites := []core.Favourite{}
	for rows.Next() {
		var schema, name string
		if scanErr := rows.Scan(&schema, &name); scanErr != nil {
			return nil, scanErr
		}
		// An empty table name means a schema and not a table.
		if name == "" {
			favourites = append(favourites,
				core.Favourite{Kind: core.FavouriteSchema, Schema: schema})
			continue
		}
		favourites = append(favourites,
			core.Favourite{Kind: core.FavouriteTable, Schema: schema, Name: name})
	}
	return favourites, rows.Err()
}

// ToggleFavourite adds the mark to the object, or removes an existing mark.
func (store *Store) ToggleFavourite(profileName string, favourite core.Favourite) error {
	if store == nil {
		return nil
	}
	name := favourite.Name
	if favourite.Kind == core.FavouriteSchema {
		name = ""
	}
	removed, err := store.file.Exec(
		`DELETE FROM favourite_object
		  WHERE profile_name = ? AND schema_name = ? AND table_name = ?`,
		profileName, favourite.Schema, name)
	if err != nil {
		return err
	}
	if changed, _ := removed.RowsAffected(); changed > 0 {
		return nil
	}
	_, insertErr := store.file.Exec(
		`INSERT OR IGNORE INTO favourite_object
		   (profile_name, schema_name, table_name, marked_at)
		 VALUES (?, ?, ?, ?)`,
		profileName, favourite.Schema, name, time.Now().UnixMilli())
	return insertErr
}

// ListRecentSchemas returns the schemas the user opened recently, newest first.
func (store *Store) ListRecentSchemas(profileName string, limit int) ([]core.RecentSchema, error) {
	if store == nil {
		return nil, nil
	}
	rows, err := store.file.Query(
		`SELECT schema_name, visited_at FROM recent_schema
		  WHERE profile_name = ?
		  ORDER BY visit_seq DESC
		  LIMIT ?`, profileName, limit)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	recent := []core.RecentSchema{}
	for rows.Next() {
		var schema string
		var visitedAt int64
		if scanErr := rows.Scan(&schema, &visitedAt); scanErr != nil {
			return nil, scanErr
		}
		recent = append(recent,
			core.RecentSchema{Schema: schema, VisitedAt: time.UnixMilli(visitedAt)})
	}
	return recent, rows.Err()
}

// VisitSchema records that the user opened a schema.
func (store *Store) VisitSchema(profileName, schema string) error {
	if store == nil {
		return nil
	}
	_, err := store.file.Exec(
		`INSERT INTO recent_schema (profile_name, schema_name, visited_at, visit_seq)
		 VALUES (?, ?, ?,
		   (SELECT COALESCE(MAX(visit_seq), 0) + 1 FROM recent_schema WHERE profile_name = ?))
		 ON CONFLICT (profile_name, schema_name) DO UPDATE SET
		   visited_at = excluded.visited_at,
		   visit_seq  = excluded.visit_seq`,
		profileName, schema, time.Now().UnixMilli(), profileName)
	return err
}
