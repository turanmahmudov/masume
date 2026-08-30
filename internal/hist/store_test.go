package hist

import (
	"encoding/json"
	"os"
	"testing"
	"time"

	"github.com/turanmahmudov/masume/internal/core"
)

// A nil store is what the client holds where the history file could not be opened. Every call
// has to answer rather than stop the client, because a reader who cannot write a history still
// opens every connection.
func TestANilStoreAnswersEveryCall(t *testing.T) {
	var store *Store

	if err := store.Record(HistoryEntry{ProfileName: "shop", SQL: "select 1"}); err != nil {
		t.Errorf("Record answered %v", err)
	}
	if err := store.SaveQuery("shop", "held", "select 1"); err != nil {
		t.Errorf("SaveQuery answered %v", err)
	}
	if err := store.Close(); err != nil {
		t.Errorf("Close answered %v", err)
	}
	if held, err := store.ListRecent("shop", 10); err != nil || len(held) != 0 {
		t.Errorf("ListRecent answered %v and %d entries", err, len(held))
	}
	if held, err := store.ListSaved("shop"); err != nil || len(held) != 0 {
		t.Errorf("ListSaved answered %v and %d entries", err, len(held))
	}
	if _, held, err := store.FindWorkspace("shop"); held || err != nil {
		t.Errorf("FindWorkspace answered %v out of no file", err)
	}
	if _, held := store.FindCatalog("shop"); held {
		t.Error("FindCatalog answered a catalog out of no file")
	}
}

func TestRecordAndListRecentKeepTheNewestFirst(t *testing.T) {
	store := openTestStore(t)
	base := time.Now().Add(-time.Hour)

	for at, sql := range []string{"select 1", "select 2", "select 3"} {
		if err := store.Record(HistoryEntry{
			ProfileName: "shop", SQL: sql,
			RanAt:   base.Add(time.Duration(at) * time.Minute),
			Elapsed: time.Millisecond, RowCount: int64(at), HasRowCount: true,
		}); err != nil {
			t.Fatalf("the record answered %v", err)
		}
	}

	held, err := store.ListRecent("shop", 10)
	if err != nil {
		t.Fatalf("the list answered %v", err)
	}
	if len(held) != 3 {
		t.Fatalf("the file holds %d entries, wanted 3", len(held))
	}
	if held[0].SQL != "select 3" {
		t.Errorf("the first entry is %q, wanted the one that ran last", held[0].SQL)
	}
	if held[0].ID == 0 {
		t.Error("the entry carries no id from the file")
	}
	if !held[0].HasRowCount || held[0].RowCount != 2 {
		t.Errorf("the count of rows reads %d", held[0].RowCount)
	}
}

// The history of one profile is not the history of another, or a reader would see statements
// that ran somewhere else.
func TestListRecentKeepsTheProfilesApart(t *testing.T) {
	store := openTestStore(t)

	for _, profile := range []string{"shop", "warehouse"} {
		if err := store.Record(HistoryEntry{
			ProfileName: profile, SQL: "select from " + profile, RanAt: time.Now(),
		}); err != nil {
			t.Fatalf("the record answered %v", err)
		}
	}

	held, err := store.ListRecent("shop", 10)
	if err != nil {
		t.Fatalf("the list answered %v", err)
	}
	if len(held) != 1 {
		t.Fatalf("the profile holds %d entries, wanted its own", len(held))
	}
	if held[0].SQL != "select from shop" {
		t.Errorf("the entry reads %q", held[0].SQL)
	}
}

func TestListRecentHoldsToItsLimit(t *testing.T) {
	store := openTestStore(t)
	for range 5 {
		if err := store.Record(HistoryEntry{
			ProfileName: "shop", SQL: "select 1", RanAt: time.Now(),
		}); err != nil {
			t.Fatalf("the record answered %v", err)
		}
	}
	held, err := store.ListRecent("shop", 2)
	if err != nil {
		t.Fatalf("the list answered %v", err)
	}
	if len(held) != 2 {
		t.Errorf("a limit of 2 gave %d entries", len(held))
	}
}

// A statement that failed is kept with its message, so a reader can see what went wrong.
func TestRecordKeepsTheMessageOfAStatementThatFailed(t *testing.T) {
	store := openTestStore(t)
	if err := store.Record(HistoryEntry{
		ProfileName: "shop", SQL: "select * from nothing", RanAt: time.Now(),
		ErrorMessage: "relation does not exist",
	}); err != nil {
		t.Fatalf("the record answered %v", err)
	}

	held, err := store.ListRecent("shop", 10)
	if err != nil {
		t.Fatalf("the list answered %v", err)
	}
	if len(held) != 1 || held[0].ErrorMessage != "relation does not exist" {
		t.Errorf("the entry reads %+v", held)
	}
}

// A query kept by name is replaced when it is kept again, so a reader never has two of one
// name and never has to remove one first.
func TestSaveQueryReplacesTheOneOfThatName(t *testing.T) {
	store := openTestStore(t)

	if err := store.SaveQuery("shop", "totals", "select 1"); err != nil {
		t.Fatalf("the save answered %v", err)
	}
	if err := store.SaveQuery("shop", "totals", "select 2"); err != nil {
		t.Fatalf("the second save answered %v", err)
	}

	held, err := store.ListSaved("shop")
	if err != nil {
		t.Fatalf("the list answered %v", err)
	}
	if len(held) != 1 {
		t.Fatalf("the file holds %d queries, wanted the one name once", len(held))
	}
	if held[0].SQL != "select 2" {
		t.Errorf("the query reads %q, wanted the one kept last", held[0].SQL)
	}
}

func TestDeleteSavedRemovesOnlyThatQuery(t *testing.T) {
	store := openTestStore(t)
	for _, name := range []string{"totals", "counts"} {
		if err := store.SaveQuery("shop", name, "select 1"); err != nil {
			t.Fatalf("the save answered %v", err)
		}
	}
	if err := store.DeleteSaved("shop", "totals"); err != nil {
		t.Fatalf("the delete answered %v", err)
	}

	held, err := store.ListSaved("shop")
	if err != nil {
		t.Fatalf("the list answered %v", err)
	}
	if len(held) != 1 || held[0].Name != "counts" {
		t.Errorf("the file holds %+v, wanted counts alone", held)
	}
}

// The tabs are read back as they were left, because the next connect opens them again.
func TestSaveAndFindWorkspaceRoundTrip(t *testing.T) {
	store := openTestStore(t)

	saved := SavedWorkspace{
		ActiveIndex: 1,
		Tabs: []SavedTab{
			{Kind: "query", SQL: "select 1", State: SavedTabState{Caret: 4}},
			{Kind: "table", Schema: "public", Name: "orders", TableKind: "table"},
		},
	}
	if err := store.SaveWorkspace("shop", saved); err != nil {
		t.Fatalf("the save answered %v", err)
	}

	held, found, err := store.FindWorkspace("shop")
	if err != nil {
		t.Fatalf("the read answered %v", err)
	}
	if !found {
		t.Fatal("the workspace was not read back")
	}
	if len(held.Tabs) != 2 {
		t.Fatalf("the workspace holds %d tabs, wanted 2", len(held.Tabs))
	}
	if held.ActiveIndex != 1 {
		t.Errorf("the active tab reads %d, wanted 1", held.ActiveIndex)
	}
	if held.Tabs[0].SQL != "select 1" || held.Tabs[0].State.Caret != 4 {
		t.Errorf("the first tab reads %+v", held.Tabs[0])
	}
	if held.Tabs[1].Name != "orders" || held.Tabs[1].Kind != "table" {
		t.Errorf("the second tab reads %+v", held.Tabs[1])
	}
}

// Saving again replaces the tabs, so a workspace never grows the tabs of an earlier run.
func TestSaveWorkspaceReplacesWhatWasThere(t *testing.T) {
	store := openTestStore(t)

	if err := store.SaveWorkspace("shop", SavedWorkspace{
		Tabs: []SavedTab{{Kind: "query", SQL: "one"}, {Kind: "query", SQL: "two"}},
	}); err != nil {
		t.Fatalf("the save answered %v", err)
	}
	if err := store.SaveWorkspace("shop", SavedWorkspace{
		Tabs: []SavedTab{{Kind: "query", SQL: "only"}},
	}); err != nil {
		t.Fatalf("the second save answered %v", err)
	}

	held, _, _ := store.FindWorkspace("shop")
	if len(held.Tabs) != 1 || held.Tabs[0].SQL != "only" {
		t.Errorf("the workspace holds %+v, wanted the one tab saved last", held.Tabs)
	}
}

// Every save runs on its own goroutine, so one can reach the file after a newer one. The
// number of the snapshot decides, or a press would bring back the tabs of an earlier one.
func TestSaveWorkspaceDropsASnapshotOlderThanTheOneWritten(t *testing.T) {
	store := openTestStore(t)

	if err := store.SaveWorkspace("shop", SavedWorkspace{
		Change: 2, Tabs: []SavedTab{{Kind: "query", SQL: "newer"}},
	}); err != nil {
		t.Fatalf("the save answered %v", err)
	}
	if err := store.SaveWorkspace("shop", SavedWorkspace{
		Change: 1, Tabs: []SavedTab{{Kind: "query", SQL: "older"}},
	}); err != nil {
		t.Fatalf("the late save answered %v", err)
	}

	held, _, _ := store.FindWorkspace("shop")
	if len(held.Tabs) != 1 || held.Tabs[0].SQL != "newer" {
		t.Errorf("the workspace holds %+v, wanted the newer snapshot", held.Tabs)
	}

	// A newer one still writes.
	if err := store.SaveWorkspace("shop", SavedWorkspace{
		Change: 3, Tabs: []SavedTab{{Kind: "query", SQL: "newest"}},
	}); err != nil {
		t.Fatalf("the third save answered %v", err)
	}
	held, _, _ = store.FindWorkspace("shop")
	if len(held.Tabs) != 1 || held.Tabs[0].SQL != "newest" {
		t.Errorf("the workspace holds %+v, wanted the newest snapshot", held.Tabs)
	}

	// Each profile is numbered on its own.
	if err := store.SaveWorkspace("other", SavedWorkspace{
		Change: 1, Tabs: []SavedTab{{Kind: "query", SQL: "first"}},
	}); err != nil {
		t.Fatalf("the save of the other profile answered %v", err)
	}
	other, found, _ := store.FindWorkspace("other")
	if !found || len(other.Tabs) != 1 {
		t.Errorf("the other profile holds %+v", other.Tabs)
	}
}

// A file this client cannot read is reported, because the tabs of the user are answered as
// nothing otherwise, which reads as a profile that was never opened.
func TestFindWorkspaceReportsAFileItCannotRead(t *testing.T) {
	store := openTestStore(t)
	if err := store.SaveWorkspace("shop", SavedWorkspace{
		Tabs: []SavedTab{{Kind: "query", SQL: "select 1"}},
	}); err != nil {
		t.Fatalf("the save answered %v", err)
	}
	if _, err := store.file.Exec("DROP TABLE workspace_tab"); err != nil {
		t.Fatalf("the table was not dropped: %v", err)
	}

	held, found, err := store.FindWorkspace("shop")
	if err == nil {
		t.Errorf("a file that cannot be read answered %+v and %v", held, found)
	}
}

func TestFindWorkspaceAnswersNothingForAProfileNeverSaved(t *testing.T) {
	store := openTestStore(t)
	if _, found, _ := store.FindWorkspace("never"); found {
		t.Error("a profile never saved answered a workspace")
	}
}

// The catalog is cached so a reconnect draws the tree at once. A payload written by an older
// version carries another shape, so the version is checked and an old one is dropped.
func TestSaveAndFindCatalogRoundTrip(t *testing.T) {
	store := openTestStore(t)

	tables, err := json.Marshal([]string{"orders", "customers"})
	if err != nil {
		t.Fatalf("cannot write the tables: %v", err)
	}
	if err := store.SaveCatalog("shop", CatalogSnapshot{Tables: tables}); err != nil {
		t.Fatalf("the save answered %v", err)
	}

	held, found := store.FindCatalog("shop")
	if !found {
		t.Fatal("the catalog was not read back")
	}
	var names []string
	if err := json.Unmarshal(held.Tables, &names); err != nil {
		t.Fatalf("the tables do not read back: %v", err)
	}
	if len(names) != 2 || names[0] != "orders" {
		t.Errorf("the tables read %v", names)
	}
}

// A mark goes on and comes off with the same call, so one key does both.
func TestToggleFavouriteMarksAndUnmarks(t *testing.T) {
	store := openTestStore(t)
	favourite := core.Favourite{Kind: core.FavouriteTable, Schema: "public", Name: "orders"}

	if err := store.ToggleFavourite("shop", favourite); err != nil {
		t.Fatalf("the mark answered %v", err)
	}
	held, err := store.ListFavourites("shop")
	if err != nil {
		t.Fatalf("the list answered %v", err)
	}
	if len(held) != 1 || held[0].Name != "orders" {
		t.Fatalf("the marks read %+v, wanted the one relation", held)
	}

	if err := store.ToggleFavourite("shop", favourite); err != nil {
		t.Fatalf("the second call answered %v", err)
	}
	held, err = store.ListFavourites("shop")
	if err != nil {
		t.Fatalf("the list answered %v", err)
	}
	if len(held) != 0 {
		t.Errorf("the mark stayed on: %+v", held)
	}
}

// The recent schemas are ordered by a counter and not by the clock, because two visits inside
// one millisecond read as equal and which came last is the whole point of the list.
func TestVisitSchemaKeepsTheOrderOfTwoVisitsInOneMoment(t *testing.T) {
	store := openTestStore(t)

	for _, schema := range []string{"first", "second", "third"} {
		if err := store.VisitSchema("shop", schema); err != nil {
			t.Fatalf("the visit answered %v", err)
		}
	}

	held, err := store.ListRecentSchemas("shop", 10)
	if err != nil {
		t.Fatalf("the list answered %v", err)
	}
	if len(held) != 3 {
		t.Fatalf("the list holds %d schemas, wanted 3", len(held))
	}
	if held[0].Schema != "third" {
		t.Errorf("the first is %q, wanted the one visited last", held[0].Schema)
	}
}

// Visiting a schema again moves it to the front rather than listing it twice.
func TestVisitSchemaAgainMovesItToTheFront(t *testing.T) {
	store := openTestStore(t)
	for _, schema := range []string{"first", "second", "first"} {
		if err := store.VisitSchema("shop", schema); err != nil {
			t.Fatalf("the visit answered %v", err)
		}
	}

	held, err := store.ListRecentSchemas("shop", 10)
	if err != nil {
		t.Fatalf("the list answered %v", err)
	}
	if len(held) != 2 {
		t.Fatalf("the list holds %d schemas, wanted 2", len(held))
	}
	if held[0].Schema != "first" {
		t.Errorf("the first is %q, wanted the one visited again", held[0].Schema)
	}
}

// The file is made where its directory does not exist yet, because the first run of the client
// has neither.
func TestOpenMakesTheDirectoryOfTheFile(t *testing.T) {
	path := t.TempDir() + "/state/masume/history.sqlite"
	store, err := Open(path)
	if err != nil {
		t.Fatalf("the file did not open: %v", err)
	}
	defer func() { _ = store.Close() }()

	if err := store.Record(HistoryEntry{
		ProfileName: "shop", SQL: "select 1", RanAt: time.Now(),
	}); err != nil {
		t.Errorf("the file cannot be written: %v", err)
	}
}

// The file holds every statement that ran, so it holds the values written into one. On a
// machine with more than one user, a file another user can read hands them that.
func TestOpenKeepsTheFilePrivateToItsOwner(t *testing.T) {
	directory := t.TempDir() + "/state/masume"
	path := directory + "/history.sqlite"
	store, err := Open(path)
	if err != nil {
		t.Fatalf("the file did not open: %v", err)
	}
	defer func() { _ = store.Close() }()

	found, err := os.Stat(directory)
	if err != nil {
		t.Fatalf("the directory cannot be read: %v", err)
	}
	if found.Mode().Perm() != 0o700 {
		t.Errorf("the directory reads %o, wanted 700", found.Mode().Perm())
	}

	for _, beside := range []string{path, path + "-wal", path + "-shm"} {
		held, err := os.Stat(beside)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			t.Fatalf("%s cannot be read: %v", beside, err)
		}
		if held.Mode().Perm() != 0o600 {
			t.Errorf("%s reads %o, wanted 600", beside, held.Mode().Perm())
		}
	}
}

// A file an earlier run left readable by everyone is narrowed when it is opened again.
func TestOpenNarrowsAFileAnEarlierRunLeftOpen(t *testing.T) {
	path := t.TempDir() + "/history.sqlite"
	first, err := Open(path)
	if err != nil {
		t.Fatalf("the file did not open: %v", err)
	}
	if err := first.Close(); err != nil {
		t.Fatalf("the file did not close: %v", err)
	}
	if err := os.Chmod(path, 0o644); err != nil {
		t.Fatalf("the mode did not change: %v", err)
	}

	second, err := Open(path)
	if err != nil {
		t.Fatalf("the file did not open again: %v", err)
	}
	defer func() { _ = second.Close() }()

	found, err := os.Stat(path)
	if err != nil {
		t.Fatalf("the file cannot be read: %v", err)
	}
	if found.Mode().Perm() != 0o600 {
		t.Errorf("the file reads %o, wanted 600", found.Mode().Perm())
	}
}

// A file written by an earlier run is opened as it is, so a reader never loses a history to an
// upgrade.
func TestOpenReadsAFileWrittenByAnEarlierRun(t *testing.T) {
	path := t.TempDir() + "/history.sqlite"

	first, err := Open(path)
	if err != nil {
		t.Fatalf("the file did not open: %v", err)
	}
	if err := first.Record(HistoryEntry{
		ProfileName: "shop", SQL: "select 1", RanAt: time.Now(),
	}); err != nil {
		t.Fatalf("the record answered %v", err)
	}
	if err := first.Close(); err != nil {
		t.Fatalf("the file did not close: %v", err)
	}

	second, err := Open(path)
	if err != nil {
		t.Fatalf("the file did not open again: %v", err)
	}
	defer func() { _ = second.Close() }()

	held, err := second.ListRecent("shop", 10)
	if err != nil {
		t.Fatalf("the list answered %v", err)
	}
	if len(held) != 1 {
		t.Errorf("the file holds %d entries after opening again, wanted 1", len(held))
	}
}
