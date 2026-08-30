package mongo

import (
	"github.com/turanmahmudov/masume/internal/core"
	"github.com/turanmahmudov/masume/internal/db"
	"github.com/turanmahmudov/masume/internal/query"
	"github.com/turanmahmudov/masume/internal/query/statement"
)

type Composer struct{ dialect *query.Dialect }

func NewComposer(dialect *query.Dialect) Composer {
	return Composer{dialect: dialect}
}

func (composer Composer) ComposeRelationRead(
	table db.TableRef, rewrite core.ReadRewrite,
) db.ComposedRead {
	return ComposeRelationRead(table, rewrite)
}

func (composer Composer) ComposeStatementRead(
	written db.BoundText, rewrite core.ReadRewrite,
) db.ComposedRead {
	return ComposeStatementRead(written, rewrite)
}

// The server binds no value, so a `:name` mark is written into the statement itself, quoted
// by the dialect. A mark that cannot be written is reported, because the statement would
// otherwise run with the mark in it.
func (composer Composer) BindParameters(
	written string, values map[string]any,
) (db.BoundText, error) {
	inlined, err := statement.InlineQueryParameters(written, values, composer.dialect)
	if err != nil {
		return db.BoundText{}, err
	}
	return db.BoundText{Text: inlined}, nil
}

func (composer Composer) FindStatementSource(written string) (statement.SelectSource, bool) {
	return FindStatementSource(written)
}

func (composer Composer) BuildChanges(
	target db.ChangeTarget, staged core.PendingChanges,
) ([]db.Change, error) {
	return BuildChanges(target, staged)
}
