package build

import (
	"strings"

	"github.com/turanmahmudov/masume/internal/core"
	"github.com/turanmahmudov/masume/internal/query"
)

// Editable SQL templates for the object menu.

// TemplateColumn is a table column for template generation.
type TemplateColumn struct {
	Name string
	// True when the column has a server default.
	HasDefault bool
}

// TemplateObject is a function, sequence, type or trigger, as a template reads it.
type TemplateObject struct {
	Schema string
	Name   string
	Kind   string
	// The trigger target table.
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

// generatedSelectRows is the row cap of the generated read.
const generatedSelectRows = 100

// GenerateSelect builds a query with a 100-row limit.
func GenerateSelect(table query.QualifiedName, dialect *query.Dialect) string {
	return dialect.BuildCappedRead(dialect.BuildQualifiedName(table), generatedSelectRows)
}

// GenerateInsert builds an INSERT with named parameters. Columns with defaults are omitted unless every column has a default.
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
	return dialect.BuildAddColumn(table)
}

// buildDerivedName appends a suffix and quotes the identifier when required.
func buildDerivedName(name, suffix string, dialect *query.Dialect) string {
	return dialect.QuoteIdentifierIfNeeded(name + suffix)
}

// GenerateCreateIndex builds a CREATE INDEX template for a table.
func GenerateCreateIndex(table query.QualifiedName, dialect *query.Dialect) string {
	index := buildDerivedName(table.Name, "_new_idx", dialect)
	return "create index " + index + "\n    on " + dialect.BuildQualifiedName(table) + " (column_name);"
}

// GenerateRenameTable builds an ALTER TABLE rename template.
func GenerateRenameTable(table query.QualifiedName, dialect *query.Dialect) string {
	return dialect.BuildRenameTable(table, buildDerivedName(table.Name, "_renamed", dialect))
}

// GenerateTruncate builds a TRUNCATE TABLE statement.
func GenerateTruncate(table query.QualifiedName, dialect *query.Dialect) string {
	return "truncate table " + dialect.BuildQualifiedName(table) + ";"
}

// GenerateDrop builds a DROP statement for a table or view.
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
	return "create table " + target + " (\n    " + dialect.IdentityColumn +
		",\n    name " + dialect.BuildColumnType(core.KindText) + " not null\n);"
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

// GenerateDropObject builds a DROP statement for a schema object.
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
