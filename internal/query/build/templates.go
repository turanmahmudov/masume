package build

import (
	"strings"

	"github.com/turanmahmudov/masume/internal/query"
)

// The statements the object menu writes into the editor. Each one is a starting
// point the user reads and changes, so the wording stays the same for every engine
// and only the dialect writes the names.

// TemplateColumn is one column of a relation, as a template reads it.
type TemplateColumn struct {
	Name string
	// True where the server fills the column in, so an insert leaves it out.
	HasDefault bool
}

// TemplateObject is a function, sequence, type or trigger, as a template reads it.
type TemplateObject struct {
	Schema string
	Name   string
	Kind   string
	// A trigger is dropped from the table this names.
	Detail string
	// A DDL lookup identifies a function by its argument types.
	Identity string
}

// The kinds a template writes a different statement for.
const (
	TemplateTable            = "table"
	TemplateView             = "view"
	TemplateMaterializedView = "materialized-view"
	TemplateFunction         = "function"
	TemplateSequence         = "sequence"
	TemplateType             = "type"
	TemplateTrigger          = "trigger"
)

// GenerateSelect writes the read of one relation, capped so a large table cannot
// fill the pane by accident.
func GenerateSelect(table query.QualifiedName, dialect *query.Dialect) string {
	return "select *\n  from " + dialect.BuildQualifiedName(table) + "\n limit 100;"
}

// GenerateInsert writes an INSERT that names the columns the user fills in. A column
// the server has a default for is left out, unless every column has one.
func GenerateInsert(
	table query.QualifiedName, columns []TemplateColumn, dialect *query.Dialect,
) string {
	writable := make([]TemplateColumn, 0, len(columns))
	for _, column := range columns {
		if !column.HasDefault {
			writable = append(writable, column)
		}
	}
	chosen := writable
	if len(chosen) == 0 {
		chosen = columns
	}

	names := make([]string, 0, len(chosen))
	marks := make([]string, 0, len(chosen))
	for _, column := range chosen {
		names = append(names, dialect.QuoteIdentifier(column.Name))
		marks = append(marks, ":"+column.Name)
	}
	return "insert into " + dialect.BuildQualifiedName(table) +
		" (" + strings.Join(names, ", ") + ")\nvalues (" + strings.Join(marks, ", ") + ");"
}

// GenerateAddColumn writes the ALTER that adds a column.
func GenerateAddColumn(table query.QualifiedName, dialect *query.Dialect) string {
	return "alter table " + dialect.BuildQualifiedName(table) + "\n  add column new_column text;"
}

// buildDerivedName names an object after another one. A name built from another is
// quoted like any other, because the server changes the case of a bare name.
func buildDerivedName(name, suffix string, dialect *query.Dialect) string {
	return dialect.QuoteIdentifierIfNeeded(name + suffix)
}

// GenerateCreateIndex writes the CREATE INDEX of one relation.
func GenerateCreateIndex(table query.QualifiedName, dialect *query.Dialect) string {
	index := buildDerivedName(table.Name, "_new_idx", dialect)
	return "create index " + index + "\n    on " + dialect.BuildQualifiedName(table) + " (column_name);"
}

// GenerateRenameTable writes the ALTER that renames a relation.
func GenerateRenameTable(table query.QualifiedName, dialect *query.Dialect) string {
	renamed := buildDerivedName(table.Name, "_renamed", dialect)
	return "alter table " + dialect.BuildQualifiedName(table) + "\n  rename to " + renamed + ";"
}

// GenerateTruncate writes the TRUNCATE of one relation.
func GenerateTruncate(table query.QualifiedName, dialect *query.Dialect) string {
	return "truncate table " + dialect.BuildQualifiedName(table) + ";"
}

// GenerateDrop writes the DROP of a relation, which names the kind it removes.
func GenerateDrop(table query.QualifiedName, kind string, dialect *query.Dialect) string {
	qualified := dialect.BuildQualifiedName(table)
	switch kind {
	case TemplateView:
		return "drop view " + qualified + ";"
	case TemplateMaterializedView:
		return "drop materialized view " + qualified + ";"
	}
	return "drop table " + qualified + ";"
}

// GenerateCreateTable writes a CREATE TABLE with the identity column of the engine.
func GenerateCreateTable(schema string, dialect *query.Dialect) string {
	target := dialect.BuildQualifiedName(query.QualifiedName{Schema: schema, Name: "new_table"})
	return "create table " + target + " (\n    " + dialect.IdentityColumn + ",\n    name text not null\n);"
}

// GenerateCreateView writes a CREATE VIEW.
func GenerateCreateView(schema string, dialect *query.Dialect) string {
	target := dialect.BuildQualifiedName(query.QualifiedName{Schema: schema, Name: "new_view"})
	return "create view " + target + " as\nselect 1 as id;"
}

// GenerateDropSchema writes the statement that removes a schema.
func GenerateDropSchema(schema string, dialect *query.Dialect) string {
	return dialect.BuildDropSchema(schema)
}

// GenerateDropObject writes the DROP of one object of a schema. A trigger is dropped
// from its table. The others are dropped by name.
func GenerateDropObject(object TemplateObject, dialect *query.Dialect) string {
	qualified := dialect.BuildQualifiedName(
		query.QualifiedName{Schema: object.Schema, Name: object.Name})
	switch object.Kind {
	case TemplateSequence:
		return "drop sequence " + qualified + ";"
	case TemplateType:
		return "drop type " + qualified + ";"
	case TemplateTrigger:
		return dialect.BuildDropTrigger(object.Schema, object.Name, object.Detail)
	}
	return dialect.BuildDropRoutine(object.Schema, object.Name, object.Identity)
}
