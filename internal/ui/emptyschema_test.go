package ui

import (
	"testing"
	"time"

	"github.com/turanmahmudov/masume/internal/db"
)

// A database that holds no relation reaches the tree through the schema list of the catalog
// read, so a user can open it and create the first table in it.
func TestACatalogReadDrawsADatabaseThatHoldsNothing(t *testing.T) {
	model := buildOfflineModel(t, 160, 48)
	connection := model.Active()
	if connection == nil {
		t.Fatal("the model holds no connection")
	}

	model.readCatalogAnswer(catalogReadMsg{
		ConnectionID: model.ActiveID(),
		Schemas:      []string{"shop", "empty_one"},
		Tables:       []db.TableRef{{Schema: "shop", Name: "orders", Kind: db.RelationTable}},
	})

	if held := connection.Catalog.Schemas; len(held) != 2 {
		t.Fatalf("the catalog holds the schemas %v", held)
	}
	labels := map[string]bool{}
	for _, row := range connection.BuildTree(time.Now()).Rows {
		labels[row.Label] = true
	}
	if !labels["empty_one"] {
		t.Error("the tree draws no row for the database that holds nothing")
	}
	if !labels["shop"] {
		t.Error("the tree draws no row for the database of the relation")
	}
}
