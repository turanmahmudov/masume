package postgres

import (
	"strconv"
	"strings"
	"time"

	"github.com/turanmahmudov/masume/internal/db"
	"github.com/turanmahmudov/masume/internal/query"
)

// relationKindByRelkind reads the one-letter kind Postgres stores.
var relationKindByRelkind = map[string]db.RelationKind{
	"r": db.RelationTable, "p": db.RelationTable,
	"v": db.RelationView, "m": db.RelationMaterializedView,
}

// constraintKinds read the one-letter kind Postgres stores.
var constraintKinds = map[string]db.ConstraintKind{
	"p": db.ConstraintPrimaryKey, "f": db.ConstraintForeignKey, "u": db.ConstraintUnique,
	"c": db.ConstraintCheck, "x": db.ConstraintExclusion,
}

// MapRelationKind reads the relkind of a relation.
func MapRelationKind(code any) db.RelationKind {
	kind, known := relationKindByRelkind[db.ReadCatalogText(code)]
	if !known {
		return db.RelationTable
	}
	return kind
}

// MapConstraintKind reads the contype of a constraint.
func MapConstraintKind(code any) db.ConstraintKind {
	kind, known := constraintKinds[db.ReadCatalogText(code)]
	if !known {
		return db.ConstraintCheck
	}
	return kind
}

// ReadTextArray reads a `text[]` column, whichever shape the driver gave it.
func ReadTextArray(value any) []string {
	switch held := value.(type) {
	case nil:
		return nil
	case []string:
		return held
	case []any:
		texts := make([]string, 0, len(held))
		for _, entry := range held {
			texts = append(texts, db.ReadAnyText(entry))
		}
		return texts
	}
	return nil
}

func readFlag(value any) bool {
	held, isFlag := value.(bool)
	return held && isFlag
}

// readTimestamp returns a timestamp column as a time. A column the server left null, or
// one the driver gave in another shape, reads as the zero time, which every caller here
// takes as "the server reported nothing".
// readOptionalFloat reads a number the server can leave unset, and reports whether it was
// there. A server that answers nothing for a measure has not measured zero.
func readOptionalFloat(value any) (float64, bool) {
	switch held := value.(type) {
	case nil:
		return 0, false
	case float64:
		return held, true
	case float32:
		return float64(held), true
	case int64:
		return float64(held), true
	case int32:
		return float64(held), true
	case int:
		return float64(held), true
	case string:
		// A numeric of the server arrives as its own text where the driver has no type
		// for it. Read as nothing it would turn a measure the server did make into one
		// it did not.
		read, err := strconv.ParseFloat(held, 64)
		return read, err == nil
	}
	// Anything else carries its value in a form only its own type knows, and the text of
	// it is what every other reader of this package falls back to.
	if written := db.ReadAnyText(value); written != "" {
		read, err := strconv.ParseFloat(written, 64)
		return read, err == nil
	}
	return 0, false
}

func readTimestamp(value any) time.Time {
	held, isTime := value.(time.Time)
	if !isTime {
		return time.Time{}
	}
	return held
}

// RenderTableDDL builds a CREATE TABLE from the catalog rows the detail tabs already
// read, because PostgreSQL keeps no statement of its own for a table.
func RenderTableDDL(
	detail db.TableDetail, indexes []db.IndexDetail, constraints []db.ConstraintDetail,
	dialect *query.Dialect,
) []string {
	lines := []string{"create table " + dialect.BuildQualifiedName(detail.Table.Qualified()) + " ("}

	body := make([]string, 0, len(detail.Columns)+len(constraints))
	for _, column := range detail.Columns {
		parts := []string{"    " + dialect.QuoteIdentifier(column.Name) + " " + column.DataType}
		if column.HasDefault {
			parts = append(parts, "default "+column.DefaultValue)
		}
		if !column.Nullable {
			parts = append(parts, "not null")
		}
		body = append(body, strings.Join(parts, " "))
	}
	for _, constraint := range constraints {
		body = append(body, "    constraint "+dialect.QuoteIdentifier(constraint.Name)+" "+
			constraint.Definition)
	}

	for at, line := range body {
		if at == len(body)-1 {
			lines = append(lines, line)
			continue
		}
		lines = append(lines, line+",")
	}
	lines = append(lines, ");")

	secondary := make([]db.IndexDetail, 0, len(indexes))
	for _, index := range indexes {
		if !index.IsPrimary {
			secondary = append(secondary, index)
		}
	}
	if len(secondary) > 0 {
		lines = append(lines, "")
		for _, index := range secondary {
			lines = append(lines, index.Definition+";")
		}
	}
	return lines
}
