package redis

import (
	"github.com/turanmahmudov/masume/internal/core"
	"github.com/turanmahmudov/masume/internal/db"
	"github.com/turanmahmudov/masume/internal/query"
)

type Composer struct{ db.SQLComposer }

func NewComposer(dialect *query.Dialect) Composer {
	return Composer{SQLComposer: db.NewSQLComposer(dialect)}
}

func (composer Composer) ComposeRelationRead(
	table db.TableRef, rewrite core.ReadRewrite,
) db.ComposedRead {
	return ComposeRelationRead(table, rewrite)
}

func (composer Composer) ComposeStatementRead(
	written db.BoundText, _ core.ReadRewrite,
) db.ComposedRead {
	return ComposeStatementRead(written)
}

func (composer Composer) BuildChanges(
	target db.ChangeTarget, staged core.PendingChanges,
) ([]db.Change, error) {
	return BuildChanges(target, staged)
}
