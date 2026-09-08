package clickhouse

import (
	"github.com/turanmahmudov/masume/internal/core"
	"github.com/turanmahmudov/masume/internal/db"
	"github.com/turanmahmudov/masume/internal/query"
)

// Composer reads a relation and a statement as the shared SQL composer does. A staged
// change is a mutation of the table, which the dialect writes.
type Composer struct {
	db.SQLComposer
}

// NewComposer returns the composer of a ClickHouse connection.
func NewComposer(dialect *query.Dialect) Composer {
	return Composer{SQLComposer: db.NewSQLComposer(dialect)}
}

// BuildChanges refuses an insert of a row into a view, which the server cannot write, and
// hands every other change to the shared builder.
func (composer Composer) BuildChanges(
	target db.ChangeTarget, staged core.PendingChanges,
) ([]db.Change, error) {
	if target.Table.Kind != db.RelationTable && len(staged.Inserts) > 0 {
		return nil, core.NewEditError("this server writes no row into a view")
	}
	return composer.SQLComposer.BuildChanges(target, staged)
}

// Compile-time Composer interface check.
var _ db.Composer = Composer{}
