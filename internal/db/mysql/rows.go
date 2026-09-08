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

// FindDefinition finds the CREATE statement in a SHOW CREATE result. The result column name varies by object type.
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
		Name: db.ReadAnyText(row["name"]), Columns: db.SplitCommaList(row["columns"]),
		TargetSchema:  db.ReadAnyText(row["target_schema"]),
		TargetTable:   db.ReadAnyText(row["target_table"]),
		TargetColumns: db.SplitCommaList(row["target_columns"]),
		DeleteRule:    query.ParseDeleteRule(db.ReadAnyText(row["delete_rule"])),
	}
}
