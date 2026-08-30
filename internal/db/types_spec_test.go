package db_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/turanmahmudov/masume/internal/cfg"
	"github.com/turanmahmudov/masume/internal/db"
)

func TestTableRefQualifiedKeepsSchemaAndName(t *testing.T) {
	held := db.TableRef{Schema: "public", Name: "orders"}.Qualified()
	if held.Schema != "public" || held.Name != "orders" {
		t.Errorf("the qualified name reads %+v", held)
	}
}

func TestIsSchemaObjectKindReadsTheFourKinds(t *testing.T) {
	for _, kind := range db.SchemaObjectKinds {
		if !db.IsSchemaObjectKind(string(kind)) {
			t.Errorf("%q is not a kind", kind)
		}
	}
	if db.IsSchemaObjectKind("table") {
		t.Error("a relation was read as a schema object")
	}
}

func TestBuildConnectMessageNamesTheTarget(t *testing.T) {
	profile := cfg.Profile{
		Engine: "postgres", User: "ada", Host: "db.example", Port: 5432, Database: "shop",
	}
	held := db.BuildConnectMessage(profile, db.NewDatabaseError("password authentication failed"))
	if !strings.Contains(held, "ada@db.example:5432/shop") {
		t.Errorf("the message does not name the target: %q", held)
	}
	if !strings.Contains(held, "password authentication failed") {
		t.Errorf("the message does not name the reason: %q", held)
	}
	if errors.Is(errors.New(held), db.ErrDatabase) {
		t.Error("the connect message itself reads as a database error")
	}
}
