//go:build integration

// An integration test: it reads a real MySQL, named through MASUME_TEST_MYSQL.
package mysql_test

import (
	"context"
	"testing"

	"github.com/turanmahmudov/masume/internal/db"
	"github.com/turanmahmudov/masume/internal/db/dbtest"
	"github.com/turanmahmudov/masume/internal/query"
)

const keySchema = `
drop table if exists key_lines;
drop table if exists key_notes;
drop table if exists key_orders;
create table key_orders (id int auto_increment primary key);
create table key_lines (
  id       int auto_increment primary key,
  order_id int,
  foreign key (order_id) references key_orders (id) on delete cascade
);
create table key_notes (
  id       int auto_increment primary key,
  order_id int,
  foreign key (order_id) references key_orders (id) on delete restrict
);
`

// openKeys answers a session with one relation that follows a removed row and one that
// refuses the delete.
func openKeys(t *testing.T) db.Session {
	t.Helper()
	session := dbtest.Open(t, dbtest.MySQL)
	dbtest.RunStatements(t, session, keySchema)
	t.Cleanup(func() {
		dbtest.RunStatements(t, session,
			"drop table if exists key_lines; drop table if exists key_notes;"+
				"drop table if exists key_orders;")
	})
	return session
}

// findRule returns the delete rule of the key that starts from that relation.
func findRule(keys []db.Relationship, table string) (query.DeleteRule, bool) {
	for _, key := range keys {
		if key.Table == table {
			return key.DeleteRule, true
		}
	}
	return query.DeleteRuleUnknown, false
}

// A write plan names the relations a delete reaches, so every rule comes from the catalog.
func TestServerReadsTheDeleteRuleOfEveryKey(t *testing.T) {
	session := openKeys(t)

	keys, err := session.ListRelationships(context.Background())
	if err != nil {
		t.Fatalf("the relationships answered %v", err)
	}
	cascading, held := findRule(keys, "key_lines")
	if !held || cascading != query.DeleteRuleCascade {
		t.Errorf("the cascading key answered %q, present %v", cascading, held)
	}
	refusing, held := findRule(keys, "key_notes")
	if !held || refusing != query.DeleteRuleRestrict {
		t.Errorf("the refusing key answered %q, present %v", refusing, held)
	}
	if !cascading.ReachesRows() || refusing.ReachesRows() {
		t.Error("the rules were read but not told apart")
	}
}

func TestServerReadsTheDeleteRuleOfATableItDescribes(t *testing.T) {
	session := openKeys(t)

	detail, err := session.DescribeTable(context.Background(),
		db.TableRef{Schema: session.Describe().DefaultSchema, Name: "key_lines",
			Kind: db.RelationTable})
	if err != nil {
		t.Fatalf("the description answered %v", err)
	}
	if len(detail.ForeignKeys) != 1 {
		t.Fatalf("the relation holds %d keys", len(detail.ForeignKeys))
	}
	if detail.ForeignKeys[0].DeleteRule != query.DeleteRuleCascade {
		t.Errorf("the key answered %q", detail.ForeignKeys[0].DeleteRule)
	}
}
