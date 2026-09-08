package clickhouse

import (
	"testing"

	"github.com/turanmahmudov/masume/internal/db"
)

// A command that answers with rows is read; every other command is run.
func TestReadsRowsSeparatesAReadFromAWrite(t *testing.T) {
	for _, held := range []struct {
		command string
		reads   bool
	}{
		{"select", true}, {"with", true}, {"show", true}, {"describe", true},
		{"explain", true}, {"SELECT", true},
		{"insert", false}, {"alter", false}, {"create", false}, {"drop", false},
		{"delete", false}, {"truncate", false}, {"optimize", false}, {"", false},
	} {
		if answered := ReadsRows(held.command); answered != held.reads {
			t.Errorf("%q reads rows %v, wanted %v", held.command, answered, held.reads)
		}
	}
}

// The server plans a read without running it, and that is how a statement is checked.
func TestPlansStatementTakesAReadOnly(t *testing.T) {
	for _, held := range []struct {
		command string
		planned bool
	}{
		{"select", true}, {"with", true},
		{"insert", false}, {"show", false}, {"alter", false},
	} {
		if answered := PlansStatement(held.command); answered != held.planned {
			t.Errorf("%q is planned %v, wanted %v", held.command, answered, held.planned)
		}
	}
}

// The engine of a relation says whether it holds rows of its own or reads them again.
func TestMapRelationKindReadsTheEngine(t *testing.T) {
	for _, held := range []struct {
		engine string
		want   db.RelationKind
	}{
		{"MergeTree", db.RelationTable},
		{"ReplicatedMergeTree", db.RelationTable},
		{"View", db.RelationView},
		{"MaterializedView", db.RelationMaterializedView},
		{"", db.RelationTable},
	} {
		if answered := MapRelationKind(held.engine); answered != held.want {
			t.Errorf("%q reads as %q, wanted %q", held.engine, answered, held.want)
		}
	}
}

// A column takes a null only where its type says so.
func TestReadsNullReadsTheTypeOfTheColumn(t *testing.T) {
	for _, held := range []struct {
		dataType string
		nullable bool
	}{
		{"Nullable(String)", true},
		{"nullable(Int64)", true},
		{"String", false},
		{"Array(Nullable(String))", false},
	} {
		if answered := ReadsNull(held.dataType); answered != held.nullable {
			t.Errorf("%q takes a null %v, wanted %v", held.dataType, answered, held.nullable)
		}
	}
}
