package mysql

import (
	"regexp"
	"strings"

	"github.com/turanmahmudov/masume/internal/db"
	"github.com/turanmahmudov/masume/internal/query"
)

// relationKindByTableType reads the table type `information_schema` reports.
var relationKindByTableType = map[string]db.RelationKind{
	"BASE TABLE": db.RelationTable, "VIEW": db.RelationView, "SYSTEM VIEW": db.RelationView,
}

// mysqlConstraintKinds read the constraint type `information_schema` reports.
var mysqlConstraintKinds = map[string]db.ConstraintKind{
	"PRIMARY KEY": db.ConstraintPrimaryKey, "FOREIGN KEY": db.ConstraintForeignKey,
	"UNIQUE": db.ConstraintUnique, "CHECK": db.ConstraintCheck,
}

var enumValues = regexp.MustCompile(`(?is)^\s*(?:enum|set)\s*\((.*)\)\s*$`)
var quotedChoice = regexp.MustCompile(`'((?:[^']|'')*)'`)

// ReadEnumChoices reads the values MySQL writes into the type, as
// `enum('draft','open')`. An inner quote is doubled.
func ReadEnumChoices(columnType string) []string {
	opened := enumValues.FindStringSubmatch(columnType)
	if opened == nil {
		return nil
	}
	choices := []string{}
	for _, match := range quotedChoice.FindAllStringSubmatch(opened[1], -1) {
		choices = append(choices, strings.ReplaceAll(match[1], "''", "'"))
	}
	return choices
}

// splitCommaList reads a `group_concat` column back into its parts.
func splitCommaList(value any) []string {
	written := db.ReadAnyText(value)
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

// FindDefinition reads the statement a SHOW CREATE answered. It names its column after
// the object, so the result is read by content.
func FindDefinition(row map[string]any) string {
	for key, value := range row {
		lowered := strings.ToLower(key)
		if !strings.HasPrefix(lowered, "create") && !strings.Contains(lowered, "statement") {
			continue
		}
		if written := db.ReadAnyText(value); written != "" {
			return written
		}
	}
	return ""
}

func readMysqlForeignKey(row map[string]any) db.ForeignKey {
	return db.ForeignKey{
		Name: db.ReadAnyText(row["name"]), Columns: splitCommaList(row["columns"]),
		TargetSchema:  db.ReadAnyText(row["target_schema"]),
		TargetTable:   db.ReadAnyText(row["target_table"]),
		TargetColumns: splitCommaList(row["target_columns"]),
	}
}

// RenderIndexDefinition builds a statement the server accepts, because MySQL keeps no
// text of an index.
func RenderIndexDefinition(
	table db.TableRef, name string, isPrimary, isUnique bool, columns any, dialect *query.Dialect,
) string {
	names := db.JoinQuoted(splitCommaList(columns), dialect.QuoteIdentifier)
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
func RenderConstraintDefinition(kind db.ConstraintKind, row map[string]any) string {
	columns := db.ReadAnyText(row["columns"])
	switch kind {
	case db.ConstraintPrimaryKey:
		return "primary key (" + columns + ")"
	case db.ConstraintUnique:
		return "unique (" + columns + ")"
	case db.ConstraintForeignKey:
		return "foreign key (" + columns + ") references " +
			db.ReadAnyText(row["target_table"]) + " (" + db.ReadAnyText(row["target_columns"]) + ")"
	}
	return db.ReadAnyText(row["check_clause"])
}
