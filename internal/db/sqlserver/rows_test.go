package sqlserver

import (
	"testing"

	"github.com/turanmahmudov/masume/internal/db"
	"github.com/turanmahmudov/masume/internal/query"
	"github.com/turanmahmudov/masume/internal/query/syntax"
)

// An OUTPUT clause makes a write answer with rows, so that write is read and not counted.
func TestHoldsOutputClauseFindsTheClauseOfAWrite(t *testing.T) {
	for _, held := range []struct {
		sql   string
		holds bool
	}{
		{"insert into dbo.orders (customer) output inserted.id values (N'ada')", true},
		{"delete from dbo.orders output deleted.id where id = 1", true},
		{"insert into dbo.orders (customer) values (N'ada')", false},
		// A name inside brackets is not the clause.
		{"select [output] from dbo.orders", false},
	} {
		if answered := HoldsOutputClause(
			held.sql, syntax.FlavourSqlserver); answered != held.holds {
			t.Errorf("%q holds the clause %v, wanted %v", held.sql, answered, held.holds)
		}
	}
}

// The catalog stores the kind of a relation as one letter.
func TestMapRelationKindReadsTheLetterOfTheCatalog(t *testing.T) {
	for _, held := range []struct {
		code string
		want db.RelationKind
	}{
		{"U", db.RelationTable},
		{"V", db.RelationView},
		{"", db.RelationTable},
	} {
		if answered := MapRelationKind(held.code); answered != held.want {
			t.Errorf("%q reads as %q, wanted %q", held.code, answered, held.want)
		}
	}
}

// A foreign key of the catalog carries its columns as one text, and the action of a delete
// as the server names it.
func TestReadForeignKeyReadsTheColumnsAndTheAction(t *testing.T) {
	key := ReadForeignKey(map[string]any{
		"name": "FK_orders_users", "columns": "user_id,tenant_id",
		"target_schema": "dbo", "target_table": "users",
		"target_columns": "id,tenant_id", "delete_rule": "cascade",
	})
	if len(key.Columns) != 2 || key.Columns[1] != "tenant_id" {
		t.Errorf("the key holds the columns %v", key.Columns)
	}
	if key.DeleteRule != query.DeleteRuleCascade {
		t.Errorf("the delete reads as %q, wanted cascade", key.DeleteRule)
	}
}

// The driver hands a uniqueidentifier over as 16 bytes, which the server itself writes as a
// name of five groups.
func TestReadGUIDWritesTheIdentifierAsTheServerWritesIt(t *testing.T) {
	held := readGUID([]byte{
		0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08,
		0x09, 0x0A, 0x0B, 0x0C, 0x0D, 0x0E, 0x0F, 0x10,
	})
	written, isText := held.(string)
	if !isText {
		t.Fatalf("the identifier reads as %T, wanted text", held)
	}
	if len(written) != 36 {
		t.Errorf("the identifier reads %q", written)
	}
}

// Bytes that are not an identifier stay as they are.
func TestReadGUIDLeavesOtherValuesAlone(t *testing.T) {
	if held := readGUID([]byte{1, 2, 3}); len(held.([]byte)) != 3 {
		t.Errorf("three bytes read as %v", held)
	}
	if held := readGUID("ada"); held != "ada" {
		t.Errorf("a text read as %v", held)
	}
}
