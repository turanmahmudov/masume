package core_test

import (
	"testing"

	"github.com/turanmahmudov/masume/internal/core"
)

func TestBuildSchemaIDRoundTrips(t *testing.T) {
	id := core.BuildSchemaID("public")
	if id != "schema:public" {
		t.Errorf("the id reads %q", id)
	}
	if core.FindSchemaOfID(id) != "public" {
		t.Errorf("the schema reads %q", core.FindSchemaOfID(id))
	}
	if core.FindSchemaOfID("table:public.orders") != "" {
		t.Error("a table row was read as a schema")
	}
}

func TestBuildFavouriteIDNamesTheKind(t *testing.T) {
	schema := core.BuildFavouriteID(core.Favourite{
		Kind: core.FavouriteSchema, Schema: "public",
	})
	if schema != "favourite:schema:public" {
		t.Errorf("a schema mark reads %q", schema)
	}
	table := core.BuildFavouriteID(core.Favourite{
		Kind: core.FavouriteTable, Schema: "public", Name: "orders",
	})
	if table != "favourite:table:public.orders" {
		t.Errorf("a table mark reads %q", table)
	}
}
