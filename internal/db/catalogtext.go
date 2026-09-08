package db

import (
	"strings"

	"github.com/turanmahmudov/masume/internal/query"
)

// SplitCommaList reads a comma-separated catalog column back into its parts.
func SplitCommaList(value any) []string {
	written := ReadAnyText(value)
	if written == "" {
		return nil
	}
	parts := []string{}
	for part := range strings.SplitSeq(written, ",") {
		trimmed := strings.TrimSpace(part)
		if trimmed != "" {
			parts = append(parts, trimmed)
		}
	}
	return parts
}

// RenderIndexDefinition builds index SQL from catalog metadata.
func RenderIndexDefinition(
	table TableRef, name string, isPrimary, isUnique bool, columns any, dialect *query.Dialect,
) string {
	names := JoinQuoted(SplitCommaList(columns), dialect.QuoteIdentifier)
	if isPrimary {
		return "primary key (" + names + ")"
	}
	unique := ""
	if isUnique {
		unique = "unique "
	}
	target := dialect.BuildQualifiedName(table.Qualified())
	return "create " + unique + "index " + dialect.QuoteIdentifier(name) +
		" on " + target + " (" + names + ")"
}

// RenderConstraintDefinition writes the constraint as a statement reads it.
func RenderConstraintDefinition(kind ConstraintKind, row map[string]any) string {
	columns := ReadAnyText(row["columns"])
	switch kind {
	case ConstraintPrimaryKey:
		return "primary key (" + columns + ")"
	case ConstraintUnique:
		return "unique (" + columns + ")"
	case ConstraintForeignKey:
		return "foreign key (" + columns + ") references " +
			ReadAnyText(row["target_table"]) + " (" + ReadAnyText(row["target_columns"]) + ")"
	}
	return ReadAnyText(row["check_clause"])
}
