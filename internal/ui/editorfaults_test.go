package ui

import (
	"strings"
	"testing"
	"time"

	"github.com/turanmahmudov/masume/internal/app"
	"github.com/turanmahmudov/masume/internal/db"
	"github.com/turanmahmudov/masume/internal/present"
	"github.com/turanmahmudov/masume/internal/query/editor"
)

// buildScannedModel answers a model whose editor holds a statement over a catalog that was
// read, which is what the scanner checks a buffer against.
func buildScannedModel(t *testing.T) (*Model, *app.Connection, *app.Tab) {
	t.Helper()
	model := buildOfflineModel(t, 160, 48)
	connection := model.Active()
	tab := connection.Active()

	connection.Catalog.Tables = []db.TableRef{
		{Schema: "public", Name: "orders", Kind: db.RelationTable},
		{Schema: "public", Name: "customers", Kind: db.RelationTable},
	}
	connection.Catalog.ReadAt = time.Unix(1_700_000_000, 0)
	connection.Catalog.Details = map[string]present.TableDetailState{
		present.BuildTableID(connection.Catalog.Tables[0]): {
			Kind: present.DetailReady,
			Detail: db.TableDetail{
				Table: connection.Catalog.Tables[0],
				Columns: []db.ColumnDetail{
					{Name: "id", DataType: "integer", IsPrimaryKey: true},
					{Name: "placed_at", DataType: "timestamp"},
				},
			},
		},
	}
	tab.Editor = app.NewEditorBuffer("select o.id, o.placed_at from public.orders as o", 0)
	return model, connection, tab
}

// markedFault is laid over the faults that were kept. A scan of the buffer writes what the
// scanner found, so the mark is gone wherever the buffer was scanned again.
const markedFault = "marked"

// markKeptFaults lays the mark over the faults the caches hold.
func markKeptFaults(model *Model, key tabKey) {
	held, found := model.caches.readFaults(key)
	if !found {
		return
	}
	held.faults = []editor.Diagnostic{{Message: markedFault}}
	model.caches.keepFaults(key, held)
}

// holdsMarkedFaults is true while the faults the caches hold are still the marked ones.
func holdsMarkedFaults(model *Model, key tabKey) bool {
	held, found := model.caches.readFaults(key)
	return found && len(held.faults) == 1 && held.faults[0].Message == markedFault
}

// The fault row, the marks over the text and the gutter each read the faults, so one frame
// scanned the buffer three times. It is scanned once and kept.
func TestEditorFaultsAreKeptWhileNothingChanges(t *testing.T) {
	model, connection, tab := buildScannedModel(t)
	model.findDiagnostics(connection, tab)
	key := model.buildTabKey(connection, tab)
	markKeptFaults(model, key)

	for range 20 {
		model.findDiagnostics(connection, tab)
	}
	if !holdsMarkedFaults(model, key) {
		t.Error("the buffer was scanned again although nothing changed")
	}
}

// A whole frame reads the faults from three places. The first read keeps them, so the two
// after it read what was kept rather than scanning the buffer again.
func TestOneFrameKeepsTheFaultsItScanned(t *testing.T) {
	model, connection, tab := buildScannedModel(t)
	model.View()

	key := model.buildTabKey(connection, tab)
	held, kept := model.caches.readFaults(key)
	if !kept || !held.found || held.text != tab.Editor.Text {
		t.Fatal("the frame kept no faults, so every reader of them scanned the buffer")
	}
	markKeptFaults(model, key)
	model.View()
	if !holdsMarkedFaults(model, key) {
		t.Error("a frame scanned the buffer again although the faults were kept")
	}
}

// One test per input the faults are found from. A cache that misses one of these reports a
// fault against a catalog the client no longer holds, or hides one it does.
func TestEditorFaultsAreFoundAgainForEveryChange(t *testing.T) {
	cases := []struct {
		name   string
		change func(connection *app.Connection, tab *app.Tab)
	}{
		{"the buffer changed", func(_ *app.Connection, tab *app.Tab) {
			tab.Editor = app.NewEditorBuffer("select * from public.customers", 0)
		}},
		{"the catalog was read again", func(connection *app.Connection, _ *app.Tab) {
			connection.Catalog.ReadAt = connection.Catalog.ReadAt.Add(time.Minute)
		}},
		{"a relation was added to the catalog", func(
			connection *app.Connection, _ *app.Tab,
		) {
			connection.Catalog.Tables = append(connection.Catalog.Tables,
				db.TableRef{Schema: "public", Name: "roles", Kind: db.RelationTable})
		}},
		{"the detail of a relation was asked for", func(
			connection *app.Connection, _ *app.Tab,
		) {
			id := present.BuildTableID(connection.Catalog.Tables[1])
			connection.Catalog.Details[id] = present.TableDetailState{
				Kind: present.DetailLoading,
			}
		}},
		// The count of the details does not move when one of them turns from loading to
		// read, and the count of the columns does not move when one is renamed. A key
		// built from counts alone would miss both.
		{"the detail of a relation turned from loading to read", func(
			connection *app.Connection, _ *app.Tab,
		) {
			id := present.BuildTableID(connection.Catalog.Tables[0])
			held := connection.Catalog.Details[id]
			held.Kind = present.DetailLoading
			connection.Catalog.Details[id] = held
		}},
		{"a column of a relation was renamed", func(
			connection *app.Connection, _ *app.Tab,
		) {
			id := present.BuildTableID(connection.Catalog.Tables[0])
			held := connection.Catalog.Details[id]
			held.Detail.Columns[1].Name = "created_at"
			connection.Catalog.Details[id] = held
		}},
		{"the type of a column changed", func(connection *app.Connection, _ *app.Tab) {
			id := present.BuildTableID(connection.Catalog.Tables[0])
			held := connection.Catalog.Details[id]
			held.Detail.Columns[1].DataType = "date"
			connection.Catalog.Details[id] = held
		}},
		{"a column became a key", func(connection *app.Connection, _ *app.Tab) {
			id := present.BuildTableID(connection.Catalog.Tables[0])
			held := connection.Catalog.Details[id]
			held.Detail.Columns[1].IsPrimaryKey = true
			connection.Catalog.Details[id] = held
		}},
		{"the read of a relation failed", func(connection *app.Connection, _ *app.Tab) {
			id := present.BuildTableID(connection.Catalog.Tables[0])
			held := connection.Catalog.Details[id]
			held.Kind, held.Message = present.DetailFailed, "no rights on it"
			connection.Catalog.Details[id] = held
		}},
	}

	for _, held := range cases {
		t.Run(held.name, func(t *testing.T) {
			model, connection, tab := buildScannedModel(t)
			model.findDiagnostics(connection, tab)
			key := model.buildTabKey(connection, tab)
			markKeptFaults(model, key)

			held.change(connection, tab)
			model.findDiagnostics(connection, tab)
			if holdsMarkedFaults(model, key) {
				t.Error("the faults were kept although " + held.name)
			}
		})
	}
}

// A cache that scans again and answers what it held before passes a test that only counts the
// scans, so the faults themselves are read: a column the relation does not hold is reported,
// and the report goes once the column is named right.
func TestEditorFaultsFollowTheBuffer(t *testing.T) {
	model, connection, tab := buildScannedModel(t)
	if faults := model.findDiagnostics(connection, tab); len(faults) > 0 {
		t.Fatalf("a statement over the columns of the relation was faulted: %v", faults)
	}

	tab.Editor = app.NewEditorBuffer("select o.nothing_here from public.orders as o", 0)
	found := model.findDiagnostics(connection, tab)
	if len(found) == 0 {
		t.Fatal("a column the relation does not hold was not reported")
	}
	if !strings.Contains(strings.ToLower(found[0].Message), "nothing_here") {
		t.Errorf("the fault reads %q, and does not name the column", found[0].Message)
	}

	tab.Editor = app.NewEditorBuffer("select o.placed_at from public.orders as o", 0)
	if faults := model.findDiagnostics(connection, tab); len(faults) > 0 {
		t.Errorf("the fault is still reported with the column named right: %v", faults)
	}
}

// The faults are kept per tab, so the buffer of one tab never answers for another.
func TestEditorFaultsAreKeptPerTab(t *testing.T) {
	model, connection, tab := buildScannedModel(t)
	model.findDiagnostics(connection, tab)

	other := connection.OpenQueryTab("select o.nothing_here from public.orders as o")
	if other == tab {
		t.Fatal("the second tab is the first one")
	}

	markKeptFaults(model, model.buildTabKey(connection, tab))
	found := model.findDiagnostics(connection, other)
	if len(found) == 1 && found[0].Message == markedFault {
		t.Error("the second tab answered with the faults of the first")
	}
	if len(found) == 0 {
		t.Error("the buffer of the second tab was not faulted")
	}
}
