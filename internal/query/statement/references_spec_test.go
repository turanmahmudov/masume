package statement_test

import (
	"testing"

	"github.com/turanmahmudov/masume/internal/query/statement"
	"github.com/turanmahmudov/masume/internal/query/syntax"
)

// The relations a statement names decide what the editor checks against the catalog and which
// relation an edit is written to. A name missed here is a name nothing verifies.
func TestFindTableReferencesNamesEveryRelationOfAStatement(t *testing.T) {
	for _, held := range []struct {
		name string
		sql  string
		want []string
	}{
		{"one relation", "select * from orders", []string{"orders"}},
		{"a relation with its schema", "select * from public.orders", []string{"orders"}},
		{"two relations of a join",
			"select * from orders join customers on orders.id = customers.id",
			[]string{"orders", "customers"}},
		{"a relation of an insert", "insert into orders (id) values (1)", []string{"orders"}},
		{"a relation of an update", "update orders set id = 1", []string{"orders"}},
		{"a relation of a delete", "delete from orders", []string{"orders"}},
		{"two relations in a list", "select * from orders, customers",
			[]string{"orders", "customers"}},

		{"no relation", "select 1", nil},
		{"nothing", "", nil},
	} {
		t.Run(held.name, func(t *testing.T) {
			found := statement.FindTableReferences(held.sql, syntax.FlavourStandard)
			if len(found) != len(held.want) {
				names := []string{}
				for _, one := range found {
					names = append(names, one.Name)
				}
				t.Fatalf("%q names %q, wanted %q", held.sql, names, held.want)
			}
			for at, one := range found {
				if one.Name != held.want[at] {
					t.Errorf("reference %d names %q, wanted %q", at, one.Name, held.want[at])
				}
				// The place has to point inside the buffer, because a report marks it.
				if one.Start < 0 || one.End > len(held.sql) || one.End < one.Start {
					t.Errorf("reference %d covers %d to %d of %d cells",
						at, one.Start, one.End, len(held.sql))
				}
			}
		})
	}
}

// The schema of a name travels with it, because a relation of one name can sit in two schemas
// and an edit has to reach the right one.
func TestFindTableReferencesKeepsTheSchemaOfAName(t *testing.T) {
	found := statement.FindTableReferences(
		"select * from archive.orders", syntax.FlavourStandard)
	if len(found) != 1 {
		t.Fatalf("the statement names %d relations", len(found))
	}
	if !found[0].HasSchema || found[0].Schema != "archive" {
		t.Errorf("the schema reads %q and present=%v", found[0].Schema, found[0].HasSchema)
	}
	if found[0].Name != "orders" {
		t.Errorf("the name reads %q", found[0].Name)
	}
}

// An alias is how the columns of a relation are reached later in the statement, so it travels
// with the reference.
func TestFindTableReferencesKeepsTheAlias(t *testing.T) {
	for _, held := range []struct {
		name  string
		sql   string
		alias string
		has   bool
	}{
		{"an alias with as", "select * from orders as o", "o", true},
		{"an alias without as", "select * from orders o", "o", true},
		{"no alias", "select * from orders", "", false},
	} {
		t.Run(held.name, func(t *testing.T) {
			found := statement.FindTableReferences(held.sql, syntax.FlavourStandard)
			if len(found) != 1 {
				t.Fatalf("the statement names %d relations", len(found))
			}
			if found[0].HasAlias != held.has {
				t.Fatalf("an alias present=%v, wanted %v", found[0].HasAlias, held.has)
			}
			if held.has && found[0].Alias != held.alias {
				t.Errorf("the alias reads %q, wanted %q", found[0].Alias, held.alias)
			}
		})
	}
}

// A name a statement makes for itself is not in the catalog, so the editor must know them or
// every statement with a CTE would be marked as wrong.
func TestFindCteNamesNamesWhatTheStatementMakesItself(t *testing.T) {
	for _, held := range []struct {
		name  string
		sql   string
		names []string
	}{
		{"one name", "with recent as (select 1) select * from recent", []string{"recent"}},
		{"two names",
			"with a as (select 1), b as (select 2) select * from a join b on true",
			[]string{"a", "b"}},
		{"a recursive name",
			"with recursive tree as (select 1) select * from tree", []string{"tree"}},

		{"no name at all", "select * from orders", nil},
		{"nothing", "", nil},
	} {
		t.Run(held.name, func(t *testing.T) {
			found := statement.FindCteNames(held.sql, syntax.FlavourStandard)
			if len(found) != len(held.names) {
				t.Fatalf("%q makes %v, wanted %q", held.sql, found, held.names)
			}
			for _, name := range held.names {
				if !found[name] {
					t.Errorf("%q was not named", name)
				}
			}
		})
	}
}

// An edit is written to one relation, so a statement that reads several cannot be edited and
// has to say so rather than write to the first one it found.
func TestFindSingleSelectTableAnswersOnlyForOneRelation(t *testing.T) {
	for _, held := range []struct {
		name string
		sql  string
		want string
		is   bool
	}{
		{"one relation", "select * from orders", "orders", true},
		{"one relation with its schema", "select * from public.orders", "orders", true},
		{"one relation with an alias", "select * from orders o", "orders", true},

		{"a join reads two", "select * from orders join customers on true", "", false},
		{"a list reads two", "select * from orders, customers", "", false},
		{"no relation", "select 1", "", false},
		{"nothing", "", "", false},
	} {
		t.Run(held.name, func(t *testing.T) {
			found, is := statement.FindSingleSelectTable(held.sql, syntax.FlavourStandard)
			if is != held.is {
				t.Fatalf("read = %v, wanted %v", is, held.is)
			}
			if is && found.Name != held.want {
				t.Errorf("the relation reads %q, wanted %q", found.Name, held.want)
			}
		})
	}
}

// A name inside a string or a comment is not a relation, so the editor never checks it against
// the catalog and an edit is never written to it.
func TestFindTableReferencesIgnoresTextAndComments(t *testing.T) {
	for _, sql := range []string{
		"select 'from orders'",
		"select 1 -- from orders",
		"select /* from orders */ 1",
	} {
		if found := statement.FindTableReferences(sql, syntax.FlavourStandard); len(found) != 0 {
			names := []string{}
			for _, one := range found {
				names = append(names, one.Name)
			}
			t.Errorf("%q names %q, wanted none", sql, names)
		}
	}
}

func TestFindTableReferencesLeavesOutAQualifiedNameThatIsNotFinished(t *testing.T) {
	// The user is still typing. A schema with no relation after it names nothing yet.
	for _, sql := range []string{
		"select * from public.",
		"select * from public. where a = 1",
		"select * from .users",
	} {
		for _, found := range statement.FindTableReferences(sql, syntax.FlavourStandard) {
			if found.Name == "" {
				t.Errorf("%q gave a reference with no name: %+v", sql, found)
			}
		}
	}
}

func TestFindTableReferencesReadsAQualifiedNameOnceItIsFinished(t *testing.T) {
	found := statement.FindTableReferences("select * from public.users", syntax.FlavourStandard)
	if len(found) != 1 {
		t.Fatalf("got %d references, want 1", len(found))
	}
	if found[0].Schema != "public" || found[0].Name != "users" {
		t.Errorf("got %+v, want public.users", found[0])
	}
}

func TestFindSingleSelectTableAnswersNothingWhereMoreThanOneRelationIsRead(t *testing.T) {
	// The grid can stage an edit only where every row comes from one relation.
	for _, held := range []struct {
		name string
		sql  string
	}{
		{"a join", "select * from a join b on a.id = b.id"},
		{"two relations in the from clause", "select * from a, b"},
		{"a union", "select * from a union select * from b"},
		{"a subquery", "select * from (select 1) x"},
		{"a common table expression", "with c as (select 1) select * from c"},
		{"no from clause", "select 1"},
		{"an insert", "insert into t (a) values (1)"},
		{"an update", "update t set a = 1"},
		{"nothing", ""},
	} {
		t.Run(held.name, func(t *testing.T) {
			if _, found := statement.FindSingleSelectTable(held.sql, syntax.FlavourStandard); found {
				t.Errorf("%q was read as one relation", held.sql)
			}
		})
	}
}

func TestFindSingleSelectTableStopsTheNameAtTheClauseAfterIt(t *testing.T) {
	for _, held := range []struct {
		name   string
		sql    string
		schema string
		table  string
	}{
		{"a bare name", "select * from users", "", "users"},
		{"a qualified name", "select * from public.users", "public", "users"},
		{"a where clause after it", "select * from users where id = 1", "", "users"},
		{"an order by after it", "select * from users order by id", "", "users"},
		{"a limit after it", "select * from users limit 5", "", "users"},
		{"an alias", "select * from users u", "", "users"},
		{"a terminator", "select * from users;", "", "users"},
		{"blanks around it", "  select * from users  ", "", "users"},
	} {
		t.Run(held.name, func(t *testing.T) {
			found, is := statement.FindSingleSelectTable(held.sql, syntax.FlavourStandard)
			if !is {
				t.Fatalf("%q was not read as one relation", held.sql)
			}
			if found.Schema != held.schema || found.Name != held.table {
				t.Errorf("got %s.%s, want %s.%s",
					found.Schema, found.Name, held.schema, held.table)
			}
		})
	}
}
