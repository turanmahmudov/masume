package present_test

import (
	"testing"
	"time"

	"github.com/turanmahmudov/masume/internal/core"
	"github.com/turanmahmudov/masume/internal/db"
	"github.com/turanmahmudov/masume/internal/present"
)

// buildTreeInput returns a small catalog: two schemas of the user and one system schema.
func buildTreeInput() present.TreeInput {
	return present.TreeInput{
		Engine: core.EnginePostgres,
		Tables: []db.TableRef{
			{Schema: "public", Name: "orders", Kind: db.RelationTable},
			{Schema: "public", Name: "customers", Kind: db.RelationTable},
			{Schema: "public", Name: "paid_orders", Kind: db.RelationView},
			{Schema: "archive", Name: "orders_2024", Kind: db.RelationTable},
			{Schema: "pg_catalog", Name: "pg_class", Kind: db.RelationTable},
		},
		Expanded: map[string]bool{},
		Details:  map[string]present.TableDetailState{},
		Now:      time.Now(),
	}
}

// findRow returns the row with that label.
func findRow(rows []present.TreeRow, label string) (present.TreeRow, bool) {
	for _, row := range rows {
		if row.Label == label {
			return row, true
		}
	}
	return present.TreeRow{}, false
}

// A folded tree shows the schemas and nothing below them, so a server with many tables opens
// without a long list of rows.
func TestBuildTreeShowsTheSchemasFolded(t *testing.T) {
	held := present.BuildTree(buildTreeInput())

	if _, there := findRow(held.Rows, "public"); !there {
		t.Error("the tree does not hold the schema of the user")
	}
	// A folded schema draws no row below itself.
	if _, there := findRow(held.Rows, "orders"); there {
		t.Error("a relation is drawn under a folded schema")
	}
	for _, row := range held.Rows {
		if row.Depth < 0 {
			t.Errorf("the row %q sits at depth %d", row.Label, row.Depth)
		}
	}
}

// An open schema shows its content, and the rows below it are one level deeper.
func TestBuildTreeShowsWhatAnOpenSchemaHolds(t *testing.T) {
	input := buildTreeInput()
	schema, there := findRow(present.BuildTree(input).Rows, "public")
	if !there {
		t.Fatal("the tree does not hold the schema")
	}
	input.Expanded[schema.ID] = true

	held := present.BuildTree(input)
	orders, there := findRow(held.Rows, "orders")
	if !there {
		t.Fatal("the relation is not drawn under an open schema")
	}
	if orders.Depth <= schema.Depth {
		t.Errorf("the relation sits at depth %d, not deeper than the schema at %d",
			orders.Depth, schema.Depth)
	}
	// A view is also drawn, with its own kind.
	if _, there := findRow(held.Rows, "paid_orders"); !there {
		t.Error("the view is not drawn")
	}
}

// The system schemas are hidden by default, because the user opens the tree for their own
// data.
func TestBuildTreeHidesTheSchemasTheServerKeeps(t *testing.T) {
	input := buildTreeInput()
	input.HideSystemSchemas = true

	if _, there := findRow(present.BuildTree(input).Rows, "pg_catalog"); there {
		t.Error("a schema the server keeps is drawn although they are hidden")
	}

	// With the setting on, they are drawn.
	input.HideSystemSchemas = false
	if _, there := findRow(present.BuildTree(input).Rows, "pg_catalog"); !there {
		t.Error("a schema the server keeps is not drawn although they are shown")
	}
}

// A filter searches the whole tree and shows the matches, so the user finds a table without
// opening every schema.
func TestBuildTreeFiltersTheWholeTree(t *testing.T) {
	input := buildTreeInput()
	input.Filter = "customers"

	held := present.BuildTree(input)
	if _, there := findRow(held.Rows, "customers"); !there {
		t.Error("the relation searched for is not drawn")
	}
	// A table without a match is not drawn.
	if _, there := findRow(held.Rows, "paid_orders"); there {
		t.Error("a relation that does not match the filter is drawn")
	}
}

// A filter opened inside a schema searches that schema only, because the user opened it
// there.
func TestBuildTreeHoldsAFilterToItsSchema(t *testing.T) {
	input := buildTreeInput()
	schema, there := findRow(present.BuildTree(input).Rows, "archive")
	if !there {
		t.Fatal("the tree does not hold the second schema")
	}

	input.Filter = "orders"
	input.FilterScope = schema.ID

	held := present.BuildTree(input)
	if _, there := findRow(held.Rows, "orders_2024"); !there {
		t.Error("the relation of the searched schema is not drawn")
	}
	// The table of the other schema matches the text and is outside the scope.
	if _, there := findRow(held.Rows, "orders"); there {
		t.Error("a relation of another schema is drawn although the search is held to one")
	}
}

// A filter matches without case comparison, because the user types in the fastest form.
func TestBuildTreeMatchesAFilterWithoutCase(t *testing.T) {
	input := buildTreeInput()
	input.Filter = "CUSTOMERS"
	if _, there := findRow(present.BuildTree(input).Rows, "customers"); !there {
		t.Error("a filter in capitals matched nothing")
	}
}

// A filter without a match draws no table and must still return a tree.
func TestBuildTreeAnswersATreeForAFilterThatMatchesNothing(t *testing.T) {
	input := buildTreeInput()
	input.Filter = "nothing_here_at_all"

	held := present.BuildTree(input)
	for _, row := range held.Rows {
		if row.Label == "orders" || row.Label == "customers" {
			t.Errorf("the row %q is drawn although nothing matches", row.Label)
		}
	}
}

// A marked table shows its mark at every position, so the mark is the same in the favourites
// folder and below its schema.
func TestBuildTreeMarksAFavourite(t *testing.T) {
	input := buildTreeInput()
	input.Favourites = []core.Favourite{
		{Kind: core.FavouriteTable, Schema: "public", Name: "orders"},
	}
	schema, _ := findRow(present.BuildTree(input).Rows, "public")
	input.Expanded[schema.ID] = true

	held := present.BuildTree(input)
	orders, there := findRow(held.Rows, "orders")
	if !there {
		t.Fatal("the relation is not drawn")
	}
	if !orders.Marked {
		t.Error("the relation was marked and is not drawn as marked")
	}
	// An unmarked table shows no mark.
	customers, there := findRow(held.Rows, "customers")
	if there && customers.Marked {
		t.Error("a relation nobody marked is drawn as marked")
	}
}

// Every tree row needs an id, because the open state and the cursor use it as a key. Two rows
// with one id would open and close together.
func TestBuildTreeGivesEveryRowAnIdOfItsOwn(t *testing.T) {
	input := buildTreeInput()
	// Open every node, so the deepest rows are also drawn.
	first := present.BuildTree(input)
	for _, row := range first.Rows {
		if row.Expandable {
			input.Expanded[row.ID] = true
		}
	}

	held := present.BuildTree(input)
	seen := map[string]string{}
	for _, row := range held.Rows {
		if row.ID == "" {
			t.Errorf("the row %q carries no id", row.Label)
			continue
		}
		if taken, there := seen[row.ID]; there {
			t.Errorf("the id %q is used by %q and by %q", row.ID, taken, row.Label)
		}
		seen[row.ID] = row.Label
	}
}

// The tree draws an empty catalog before the first read returns, and it must draw the
// folders and not an empty pane.
func TestBuildTreeHoldsAnEmptyCatalog(t *testing.T) {
	held := present.BuildTree(present.TreeInput{
		Engine:   core.EnginePostgres,
		Expanded: map[string]bool{},
		Details:  map[string]present.TableDetailState{},
		Now:      time.Now(),
	})
	for _, row := range held.Rows {
		if row.Label == "" {
			t.Error("a row of the empty tree carries no label")
		}
	}
}

// The summary counts the schemas and the hidden system schemas, so the user knows that the
// tree does not show everything.
func TestBuildTreeCountsTheSchemasItShowedAndHid(t *testing.T) {
	input := buildTreeInput()
	input.HideSystemSchemas = true

	held := present.BuildTree(input)
	// Two schemas of the user are eligible, and both are drawn, because no filter is on.
	if held.Summary.TotalSchemas != 2 {
		t.Errorf("%d schemas are eligible, wanted the 2 of the user",
			held.Summary.TotalSchemas)
	}
	if held.Summary.ShownSchemas != held.Summary.TotalSchemas {
		t.Errorf("the tree draws %d of %d schemas although no filter is on",
			held.Summary.ShownSchemas, held.Summary.TotalSchemas)
	}
	// The system schema is counted separately, so the status bar can report it as hidden.
	if held.Summary.HiddenSystemSchemas != 1 {
		t.Errorf("%d schemas were folded away, wanted the one the server keeps",
			held.Summary.HiddenSystemSchemas)
	}

	// With the system schemas shown, nothing is hidden and the counts match.
	input.HideSystemSchemas = false
	shown := present.BuildTree(input)
	if shown.Summary.HiddenSystemSchemas != 0 {
		t.Errorf("%d schemas were folded away although they are shown",
			shown.Summary.HiddenSystemSchemas)
	}
	if shown.Summary.TotalSchemas != 3 {
		t.Errorf("%d schemas are eligible with none hidden, wanted 3",
			shown.Summary.TotalSchemas)
	}
}

// A filter draws fewer schemas than are eligible, which tells the status bar to report the
// tree as filtered and not as empty.
func TestBuildTreeCountsFewerShownThanEligibleUnderAFilter(t *testing.T) {
	input := buildTreeInput()
	input.HideSystemSchemas = true
	input.Filter = "customers"

	held := present.BuildTree(input)
	if held.Summary.ShownSchemas >= held.Summary.TotalSchemas {
		t.Errorf("the tree draws %d of %d schemas under a filter that matches one",
			held.Summary.ShownSchemas, held.Summary.TotalSchemas)
	}
}
