package app

import (
	"testing"
	"time"

	"github.com/turanmahmudov/masume/internal/cfg"
	"github.com/turanmahmudov/masume/internal/core"
	"github.com/turanmahmudov/masume/internal/db"
	"github.com/turanmahmudov/masume/internal/present"
)

// holdsSameRows is true where two answers are the one list of rows, which says the second
// call answered the tree the first one built rather than building it again.
func holdsSameRows(left, right present.TreeResult) bool {
	if len(left.Rows) != len(right.Rows) || len(left.Rows) == 0 {
		return false
	}
	return &left.Rows[0] == &right.Rows[0]
}

// treeCacheSession answers only what a connection reads to build its tree.
type treeCacheSession struct {
	db.Session
}

func (session *treeCacheSession) Describe() db.SessionDescriptor {
	return db.SessionDescriptor{Profile: cfg.Profile{Name: "held", Engine: core.EnginePostgres}}
}

// buildCachedConnection answers a connection whose catalog holds a few relations.
func buildCachedConnection(t *testing.T) *Connection {
	t.Helper()
	connection := NewConnection(&treeCacheSession{}, nil, false)
	connection.Catalog.Tables = []db.TableRef{
		{Schema: "public", Name: "orders", Kind: db.RelationTable},
		{Schema: "public", Name: "customers", Kind: db.RelationTable},
		{Schema: "archive", Name: "orders_2024", Kind: db.RelationTable},
	}
	connection.Catalog.ReadAt = time.Now()
	return connection
}

// The tree is kept between frames, because one key press draws one frame and reads the rows
// more than once. Nothing changed, so the same tree is answered.
func TestBuildTreeIsKeptWhileNothingChanges(t *testing.T) {
	connection := buildCachedConnection(t)
	now := time.Now()

	first := connection.BuildTree(now)
	for range 20 {
		if !holdsSameRows(first, connection.BuildTree(now)) {
			t.Fatal("the tree was built again although nothing changed")
		}
	}
	if len(connection.BuildTree(now).Rows) != len(first.Rows) {
		t.Error("the tree that was kept holds a different number of rows")
	}
}

// A later moment inside the same second draws the same tree, because the only thing the moment
// changes is the age beside a recent schema and that is written to the second.
func TestBuildTreeIsKeptForALaterMomentInTheSameSecond(t *testing.T) {
	connection := buildCachedConnection(t)
	now := time.Unix(1_700_000_000, 0)

	first := connection.BuildTree(now)
	if !holdsSameRows(first, connection.BuildTree(now.Add(300*time.Millisecond))) {
		t.Error("the tree was built again for a later moment in the same second")
	}

	// The next second draws it again, so the age moves on.
	if holdsSameRows(first, connection.BuildTree(now.Add(time.Second))) {
		t.Error("the tree was not built again for the next second")
	}
}

// Every piece of state the tree is built from has to draw it again, or the pane would show a
// catalog, a fold or a mark that is no longer there.
func TestBuildTreeIsBuiltAgainForEveryChange(t *testing.T) {
	now := time.Now()

	for _, held := range []struct {
		name   string
		change func(connection *Connection)
	}{
		{"a catalog read", func(connection *Connection) {
			connection.Catalog.ReadAt = connection.Catalog.ReadAt.Add(time.Second)
			connection.Catalog.Tables = append(connection.Catalog.Tables,
				db.TableRef{Schema: "public", Name: "invoices", Kind: db.RelationTable})
		}},
		{"a schema opened", func(connection *Connection) {
			for _, row := range connection.BuildTree(now).Rows {
				if row.Expandable {
					connection.Tree.Expanded[row.ID] = true
					return
				}
			}
			t.Fatal("the tree holds no row that opens")
		}},
		{"a schema folded again", func(connection *Connection) {
			for _, row := range connection.BuildTree(now).Rows {
				if row.Expandable {
					connection.Tree.Expanded[row.ID] = true
					connection.BuildTree(now)
					delete(connection.Tree.Expanded, row.ID)
					return
				}
			}
			t.Fatal("the tree holds no row that opens")
		}},
		{"a filter typed", func(connection *Connection) {
			connection.Tree.Filter = "orders"
		}},
		{"a filter held to one schema", func(connection *Connection) {
			connection.Tree.Filter = "orders"
			connection.BuildTree(now)
			connection.Tree.FilterScope = "schema:public"
		}},
		{"the system schemas hidden", func(connection *Connection) {
			connection.Tree.HideSystemSchemas = !connection.Tree.HideSystemSchemas
		}},
		{"a relation marked", func(connection *Connection) {
			connection.Marks.Favourites = append(connection.Marks.Favourites,
				core.Favourite{Kind: core.FavouriteTable, Schema: "public", Name: "orders"})
		}},
		{"a schema opened lately", func(connection *Connection) {
			connection.Marks.Recent = append(connection.Marks.Recent,
				core.RecentSchema{Schema: "public", VisitedAt: now})
		}},
		{"the order of the recent schemas", func(connection *Connection) {
			connection.Marks.Recent = []core.RecentSchema{
				{Schema: "public"}, {Schema: "archive"},
			}
			connection.BuildTree(now)
			// The same two schemas the other way round is a different list.
			connection.Marks.Recent = []core.RecentSchema{
				{Schema: "archive"}, {Schema: "public"},
			}
		}},
		{"the columns of a relation arriving", func(connection *Connection) {
			id := present.BuildTableID(connection.Catalog.Tables[0])
			connection.Catalog.Details[id] = present.TableDetailState{
				Kind: present.DetailLoading,
			}
			connection.BuildTree(now)
			// The same relation, read now rather than reading: the count does not change.
			connection.Catalog.Details[id] = present.TableDetailState{
				Kind:   present.DetailReady,
				Detail: db.TableDetail{Columns: []db.ColumnDetail{{Name: "id"}}},
			}
		}},
	} {
		t.Run(held.name, func(t *testing.T) {
			connection := buildCachedConnection(t)
			connection.BuildTree(now)

			held.change(connection)
			kept := connection.treeResult
			if holdsSameRows(kept, connection.BuildTree(now)) {
				t.Error("the tree was kept although the state it is built from changed")
			}
		})
	}
}

// A mark that goes on and comes off again leaves the tree as it was, and the rows have to say
// so rather than keeping the mark.
func TestBuildTreeDropsAMarkThatWasTakenOff(t *testing.T) {
	connection := buildCachedConnection(t)
	now := time.Now()

	// Open the schema that holds the relation, so it is drawn.
	opened := false
	for _, row := range connection.BuildTree(now).Rows {
		if row.Label == "public" && row.Expandable {
			connection.Tree.Expanded[row.ID] = true
			opened = true
			break
		}
	}
	if !opened {
		t.Fatal("the tree holds no schema named public")
	}
	favourite := core.Favourite{Kind: core.FavouriteTable, Schema: "public", Name: "orders"}
	connection.Marks.Favourites = []core.Favourite{favourite}

	marked := false
	for _, row := range connection.BuildTree(now).Rows {
		if row.Label == "orders" && row.Marked {
			marked = true
		}
	}
	if !marked {
		t.Fatal("the relation was marked and the tree does not draw it as marked")
	}

	connection.Marks.Favourites = nil
	for _, row := range connection.BuildTree(now).Rows {
		if row.Label == "orders" && row.Marked {
			t.Error("the mark was taken off and the tree still draws it")
		}
	}
}
