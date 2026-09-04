package build_test

import (
	"testing"

	"github.com/turanmahmudov/masume/internal/db/mysql"
	"github.com/turanmahmudov/masume/internal/db/postgres"
	"github.com/turanmahmudov/masume/internal/db/sqlite"
	"github.com/turanmahmudov/masume/internal/query"
	"github.com/turanmahmudov/masume/internal/query/build"
)

// The object menu writes these into the editor. The wording is the same for every
// engine, and only the dialect writes the names, so a template that quotes wrongly
// gives the user a statement the server refuses.

var (
	users   = query.QualifiedName{Schema: "public", Name: "users"}
	oddName = query.QualifiedName{Schema: "public", Name: "Odd Name"}
)

var templateColumns = []build.TemplateColumn{
	{Name: "id", HasDefault: true},
	{Name: "name"},
	{Name: "Odd Col"},
}

func TestGenerateSelectReadsOneRelationWithACap(t *testing.T) {
	for _, held := range []struct {
		name    string
		dialect *query.Dialect
		want    string
	}{
		{"postgres", postgres.Dialect, "select *\n  from \"public\".\"users\"\n limit 100;"},
		{"mysql", mysql.Dialect, "select *\n  from `public`.`users`\n limit 100;"},
		{"sqlite", sqlite.Dialect, "select *\n  from \"public\".\"users\"\n limit 100;"},
	} {
		t.Run(held.name, func(t *testing.T) {
			if got := build.GenerateSelect(users, held.dialect); got != held.want {
				t.Errorf("got %q, want %q", got, held.want)
			}
		})
	}
}

func TestGenerateInsertLeavesOutAColumnTheServerFillsIn(t *testing.T) {
	got := build.GenerateInsert(users, templateColumns, postgres.Dialect)
	want := "insert into \"public\".\"users\" (\"name\", \"Odd Col\")\n" +
		"values (:name, :Odd Col);"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestGenerateInsertNamesEveryColumnWhereTheServerFillsThemAll(t *testing.T) {
	// A table of nothing but defaults would give an INSERT with no column at all, so
	// every column is named instead and the user takes out what they do not write.
	every := []build.TemplateColumn{
		{Name: "id", HasDefault: true},
		{Name: "made_at", HasDefault: true},
	}
	got := build.GenerateInsert(users, every, postgres.Dialect)
	want := "insert into \"public\".\"users\" (\"id\", \"made_at\")\nvalues (:id, :made_at);"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestGenerateInsertWithNoColumnsWritesAnEmptyList(t *testing.T) {
	got := build.GenerateInsert(users, nil, postgres.Dialect)
	if want := "insert into \"public\".\"users\" ()\nvalues ();"; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestGenerateAddColumnWritesTheAlter(t *testing.T) {
	got := build.GenerateAddColumn(oddName, mysql.Dialect)
	if want := "alter table `public`.`Odd Name`\n  add column new_column text;"; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestGenerateCreateIndexQuotesADerivedNameOnlyWhereItNeedsIt(t *testing.T) {
	// A name built from another is quoted like any other, because the server changes
	// the case of a bare name.
	for _, held := range []struct {
		name  string
		table query.QualifiedName
		want  string
	}{
		{"a plain name needs no quotes", users,
			"create index users_new_idx\n    on \"public\".\"users\" (column_name);"},
		{"a name with a space is quoted", oddName,
			"create index \"Odd Name_new_idx\"\n    on \"public\".\"Odd Name\" (column_name);"},
	} {
		t.Run(held.name, func(t *testing.T) {
			if got := build.GenerateCreateIndex(held.table, postgres.Dialect); got != held.want {
				t.Errorf("got %q, want %q", got, held.want)
			}
		})
	}
}

func TestGenerateRenameTableQuotesTheNewNameOnlyWhereItNeedsIt(t *testing.T) {
	for _, held := range []struct {
		name  string
		table query.QualifiedName
		want  string
	}{
		{"a plain name", users, "alter table \"public\".\"users\"\n  rename to users_renamed;"},
		{"a name with a space", oddName,
			"alter table \"public\".\"Odd Name\"\n  rename to \"Odd Name_renamed\";"},
	} {
		t.Run(held.name, func(t *testing.T) {
			if got := build.GenerateRenameTable(held.table, postgres.Dialect); got != held.want {
				t.Errorf("got %q, want %q", got, held.want)
			}
		})
	}
}

func TestGenerateTruncateWritesTheStatement(t *testing.T) {
	got := build.GenerateTruncate(users, postgres.Dialect)
	if want := "truncate table \"public\".\"users\";"; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestGenerateDropNamesTheKindItRemoves(t *testing.T) {
	for _, held := range []struct {
		kind string
		want string
	}{
		{build.TemplateTable, "drop table \"public\".\"users\";"},
		{build.TemplateView, "drop view \"public\".\"users\";"},
		{build.TemplateMaterializedView, "drop materialized view \"public\".\"users\";"},
		{"a kind the menu does not know", "drop table \"public\".\"users\";"},
	} {
		t.Run(held.kind, func(t *testing.T) {
			if got := build.GenerateDrop(users, held.kind, postgres.Dialect); got != held.want {
				t.Errorf("got %q, want %q", got, held.want)
			}
		})
	}
}

func TestGenerateCreateTableWritesTheIdentityColumnOfTheEngine(t *testing.T) {
	for _, held := range []struct {
		name    string
		dialect *query.Dialect
		want    string
	}{
		{"postgres", postgres.Dialect,
			"create table \"sch\".\"new_table\" (\n    id bigserial primary key,\n" +
				"    name text not null\n);"},
		{"mysql", mysql.Dialect,
			"create table `sch`.`new_table` (\n    id bigint auto_increment primary key,\n" +
				"    name text not null\n);"},
		{"sqlite", sqlite.Dialect,
			"create table \"sch\".\"new_table\" (\n    id integer primary key autoincrement,\n" +
				"    name text not null\n);"},
	} {
		t.Run(held.name, func(t *testing.T) {
			if got := build.GenerateCreateTable("sch", held.dialect); got != held.want {
				t.Errorf("got %q, want %q", got, held.want)
			}
		})
	}
}

func TestGenerateCreateViewWritesTheSameStatementForEveryEngine(t *testing.T) {
	got := build.GenerateCreateView("sch", mysql.Dialect)
	if want := "create view `sch`.`new_view` as\nselect 1 as id;"; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestGenerateDropSchemaWritesWhatTheEngineCallsASchema(t *testing.T) {
	for _, held := range []struct {
		name    string
		dialect *query.Dialect
		want    string
	}{
		{"postgres removes a schema", postgres.Dialect, "drop schema \"sch\" restrict;"},
		{"mysql removes a database", mysql.Dialect, "drop database `sch`;"},
		{"sqlite lets a file go", sqlite.Dialect, "detach database \"sch\";"},
	} {
		t.Run(held.name, func(t *testing.T) {
			if got := build.GenerateDropSchema("sch", held.dialect); got != held.want {
				t.Errorf("got %q, want %q", got, held.want)
			}
		})
	}
}

func TestGenerateDropObjectWritesTheDropOfEachKind(t *testing.T) {
	for _, held := range []struct {
		name    string
		object  build.TemplateObject
		dialect *query.Dialect
		want    string
	}{
		{"a sequence", build.TemplateObject{
			Schema: "public", Name: "seq", Kind: build.TemplateSequence,
		}, postgres.Dialect, "drop sequence \"public\".\"seq\";"},
		{"a type", build.TemplateObject{
			Schema: "public", Name: "typ", Kind: build.TemplateType,
		}, postgres.Dialect, "drop type \"public\".\"typ\";"},
		{"a trigger names the table it hangs on", build.TemplateObject{
			Schema: "public", Name: "trg", Kind: build.TemplateTrigger, Detail: "users",
		}, postgres.Dialect, "drop trigger \"trg\" on \"public\".\"users\";"},
		{"a trigger on mysql needs no table", build.TemplateObject{
			Schema: "public", Name: "trg", Kind: build.TemplateTrigger, Detail: "users",
		}, mysql.Dialect, "drop trigger `public`.`trg`;"},
		{"a routine on postgres", build.TemplateObject{
			Schema: "public", Name: "fn", Kind: build.TemplateFunction,
			Identity: "function:public.fn",
		}, postgres.Dialect, "drop routine \"public\".\"fn\";"},
		{"a routine on mysql names the kind", build.TemplateObject{
			Schema: "public", Name: "fn", Kind: build.TemplateFunction,
			Identity: "function:public.fn",
		}, mysql.Dialect, "drop function `public`.`fn`;"},
	} {
		t.Run(held.name, func(t *testing.T) {
			if got := build.GenerateDropObject(held.object, held.dialect); got != held.want {
				t.Errorf("got %q, want %q", got, held.want)
			}
		})
	}
}
