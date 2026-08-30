package db

import (
	"github.com/turanmahmudov/masume/internal/core"
	"github.com/turanmahmudov/masume/internal/query"
	"github.com/turanmahmudov/masume/internal/query/build"
	"github.com/turanmahmudov/masume/internal/query/statement"
)

type Composer interface {
	ComposeRelationRead(table TableRef, rewrite core.ReadRewrite) ComposedRead
	ComposeStatementRead(written BoundText, rewrite core.ReadRewrite) ComposedRead
	BindParameters(written string, values map[string]any) (BoundText, error)
	FindStatementSource(written string) (statement.SelectSource, bool)
	BuildChanges(target ChangeTarget, staged core.PendingChanges) ([]Change, error)
}

type SQLComposer struct {
	Dialect *query.Dialect
}

func NewSQLComposer(dialect *query.Dialect) SQLComposer {
	return SQLComposer{Dialect: dialect}
}

func (composer SQLComposer) ComposeRelationRead(
	table TableRef, rewrite core.ReadRewrite,
) ComposedRead {
	dialect := composer.Dialect
	bound := statement.BuildRelationSQL(
		table.Qualified(), build.ComposeFilter(rewrite.Filter, dialect, 1), rewrite.Sort, dialect)
	shown := statement.BuildRelationSQL(
		table.Qualified(), build.InlineFilter(rewrite.Filter, dialect), rewrite.Sort, dialect)
	return ComposedRead{
		Text: bound.SQL, Params: bound.Params, Display: shown.SQL, Pageable: true,
	}
}

// ComposeStatementRead wraps the statement of the user in the rewrite. It is wrapped, not
// merged, because its own WHERE, GROUP BY or LIMIT would change meaning.
func (composer SQLComposer) ComposeStatementRead(
	written BoundText, rewrite core.ReadRewrite,
) ComposedRead {
	dialect := composer.Dialect
	// The statement binds its own values first, so the filter numbers its marks after
	// them.
	filter := build.ComposeFilter(rewrite.Filter, dialect, len(written.Params)+1)
	bound := statement.BuildEffectiveSQL(written.Text, filter, rewrite.Sort, dialect)
	shown := statement.BuildEffectiveSQL(
		written.Text, build.InlineFilter(rewrite.Filter, dialect), rewrite.Sort, dialect)

	return ComposedRead{
		Text:     bound.SQL,
		Params:   append(append([]any{}, written.Params...), bound.Params...),
		Display:  shown.SQL,
		Pageable: statement.IsPageable(bound.SQL, dialect.Syntax),
	}
}

// SQL binds the marks, so nothing the user typed is ever read as SQL. A mark that cannot
// be bound is reported, because the statement would otherwise run with the mark in it.
func (composer SQLComposer) BindParameters(
	written string, values map[string]any,
) (BoundText, error) {
	bound, err := statement.BindQueryParameters(written, values, composer.Dialect, 1)
	if err != nil {
		return BoundText{}, err
	}
	return BoundText{Text: bound.SQL, Params: bound.Params}, nil
}

func (composer SQLComposer) FindStatementSource(written string) (statement.SelectSource, bool) {
	return statement.FindSingleSelectTable(written, composer.Dialect.Syntax)
}

func (composer SQLComposer) BuildChanges(
	target ChangeTarget, staged core.PendingChanges,
) ([]Change, error) {
	write := build.WriteTarget{
		Table: target.Table.Qualified(), Columns: target.Columns,
		KeyColumns: target.KeyColumns, Dialect: composer.Dialect,
	}
	statements, err := build.BuildChangeStatements(write, target.Rows, staged)
	if err != nil {
		return nil, err
	}

	changes := make([]Change, 0, len(statements))
	for _, built := range statements {
		change := Change{
			Description: built.Statement.Description,
			Display:     built.Statement.SQL,
			Params:      built.Statement.Params,
			Payload:     built.Statement,
			Expect:      built.Expect,
		}
		if built.Guard != nil {
			change.Guard = *built.Guard
		}
		changes = append(changes, change)
	}
	return changes, nil
}
