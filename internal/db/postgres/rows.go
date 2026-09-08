package postgres

import (
	"strconv"
	"time"

	"github.com/turanmahmudov/masume/internal/db"
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

// readOptionalFloat reads a numeric measurement and distinguishes missing values from zero.
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
		// Drivers can return numeric values as text.
		read, err := strconv.ParseFloat(held, 64)
		return read, err == nil
	}
	// Other numeric types use their text representation.
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
