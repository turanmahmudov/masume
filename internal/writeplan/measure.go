package writeplan

import (
	"context"
	"strings"

	"github.com/turanmahmudov/masume/internal/db"
	"github.com/turanmahmudov/masume/internal/query"
	"github.com/turanmahmudov/masume/internal/query/statement"
)

// measurer holds what every read of one plan needs. A read that fails leaves its part of
// the plan unmeasured and says why.
type measurer struct {
	session Source
	target  statement.WriteTarget
	table   db.TableRef
}

func (measure measurer) dialect() *query.Dialect { return measure.session.Dialect() }

func (measure measurer) quotedTable() string {
	return measure.dialect().BuildQualifiedName(measure.table.Qualified())
}

// buildPredicate returns the WHERE of the write, and nothing where it names every row.
func (measure measurer) buildPredicate() string {
	if !measure.target.HasWhere {
		return ""
	}
	return " where " + measure.target.Where
}

func (measure measurer) readOneCount(ctx context.Context, sql string) (int64, error) {
	answered, err := measure.session.RunQuery(ctx, sql, 1, nil)
	if err != nil {
		return 0, err
	}
	if len(answered.Rows) == 0 || len(answered.Rows[0]) == 0 {
		return 0, db.NewDatabaseError("the server answered the count with no row")
	}
	return db.ReadNonNegativeCount(answered.Rows[0][0]), nil
}

// countRows counts the rows the write lands on, and the rows the whole relation holds.
func (measure measurer) countRows(ctx context.Context, plan *Plan) {
	if measure.target.Kind == statement.WriteInsert {
		plan.RowsReason = "an insert names the rows it writes"
		return
	}

	counted := "select " + measure.dialect().CountExpression + " from " + measure.quotedTable()
	rows, err := measure.readOneCount(ctx, counted+measure.buildPredicate())
	if err != nil {
		plan.RowsReason = db.DescribeError(err)
		return
	}
	plan.Rows, plan.HasRows = rows, true

	if !measure.target.HasWhere {
		plan.Total, plan.HasTotal = rows, true
		return
	}
	if total, totalErr := measure.readOneCount(ctx, counted); totalErr == nil {
		plan.Total, plan.HasTotal = total, true
	}
}

// readCascades returns what the write reaches through the server rather than through the
// statement: its triggers, and the relations its foreign keys reach.
func (measure measurer) readCascades(ctx context.Context, plan *Plan) {
	// A write that matches no row reaches nothing.
	if plan.HasRows && plan.Rows == 0 {
		return
	}
	plan.Cascades = append(plan.Cascades, measure.readTriggers(ctx)...)
	if measure.target.Kind == statement.WriteUpdate ||
		measure.target.Kind == statement.WriteInsert {
		return
	}
	measure.readKeysPointingHere(ctx, plan)
}

// readTriggers returns the triggers the server runs for this write. Every SQL engine names
// the relation of a trigger in its detail, and the writes it runs for in its events.
func (measure measurer) readTriggers(ctx context.Context) []Cascade {
	objects, err := measure.session.ListSchemaObjects(ctx)
	if err != nil {
		return nil
	}
	cascades := []Cascade{}
	for _, object := range objects {
		if object.Kind != db.ObjectTrigger || object.Schema != measure.table.Schema {
			continue
		}
		if !strings.EqualFold(object.Detail, measure.table.Name) {
			continue
		}
		if !measure.runsTrigger(object) {
			continue
		}
		// The relation is the one the write names, so it is left out.
		cascades = append(cascades, Cascade{Reason: "trigger " + object.Name})
	}
	return cascades
}

// runsTrigger is true where this write runs that trigger. A server that names no event is
// answered with a yes: a trigger that may run is worth reading.
func (measure measurer) runsTrigger(object db.SchemaObject) bool {
	if object.Events == "" {
		return true
	}
	for _, event := range strings.Split(object.Events, ",") {
		if strings.TrimSpace(event) == string(measure.target.Kind) {
			return true
		}
	}
	return false
}

// readKeysPointingHere sorts the foreign keys that reference this relation: the ones the
// server follows into their own relation, and the ones that block the delete.
func (measure measurer) readKeysPointingHere(ctx context.Context, plan *Plan) {
	relationships, err := measure.session.ListRelationships(ctx)
	if err != nil {
		return
	}

	for _, relationship := range relationships {
		if !measure.pointsAtTable(relationship) {
			continue
		}
		cascade := Cascade{
			Reason: "on delete " + string(relationship.DeleteRule),
			Table:  relationship.Schema + "." + relationship.Table,
		}
		if plan.HasRows && plan.Rows > 0 {
			if rows, counted := measure.countFollowedRows(ctx, relationship); counted {
				cascade.Rows, cascade.HasRows = rows, true
			}
		}
		if relationship.DeleteRule.ReachesRows() {
			plan.Cascades = append(plan.Cascades, cascade)
			continue
		}
		// A relation that references none of these rows blocks nothing. One that could
		// not be counted may still block.
		if cascade.HasRows && cascade.Rows == 0 {
			continue
		}
		plan.Blockers = append(plan.Blockers, cascade)
	}
}

func (measure measurer) pointsAtTable(relationship db.Relationship) bool {
	return relationship.TargetSchema == measure.table.Schema &&
		relationship.TargetTable == measure.table.Name
}

// countFollowedRows counts the rows of the other relation that reference the rows the write
// removes. Only a key of one column is counted: a key of several is written differently by
// each server.
func (measure measurer) countFollowedRows(
	ctx context.Context, relationship db.Relationship,
) (int64, bool) {
	if len(relationship.Columns) != 1 || len(relationship.TargetColumns) != 1 {
		return 0, false
	}
	dialect := measure.dialect()
	child := dialect.BuildQualifiedName(
		query.QualifiedName{Schema: relationship.Schema, Name: relationship.Table})

	counted := "select " + dialect.CountExpression + " from " + child +
		" where " + dialect.QuoteIdentifier(relationship.Columns[0]) + " in (select " +
		dialect.QuoteIdentifier(relationship.TargetColumns[0]) + " from " +
		measure.quotedTable() + measure.buildPredicate() + ")"

	rows, err := measure.readOneCount(ctx, counted)
	if err != nil {
		return 0, false
	}
	return rows, true
}
