package writeplan

import (
	"context"
	"strings"

	"github.com/turanmahmudov/masume/internal/db"
	"github.com/turanmahmudov/masume/internal/query"
	"github.com/turanmahmudov/masume/internal/query/statement"
)

// measurer is the connection and target for one write plan.
type measurer struct {
	session Source
	target  statement.WriteTarget
	table   db.TableRef
}

func (measure measurer) dialect() *query.Dialect { return measure.session.Dialect() }

func (measure measurer) quotedTable() string {
	return measure.dialect().BuildQualifiedName(measure.table.Qualified())
}

// buildPredicate returns the write WHERE clause, or an empty string if absent.
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
		return 0, db.NewDatabaseError("the count query returned no value")
	}
	return db.ReadNonNegativeCount(answered.Rows[0][0]), nil
}

// countRows counts matching rows and total table rows.
func (measure measurer) countRows(ctx context.Context, plan *Plan) {
	if measure.target.Kind == statement.WriteInsert {
		plan.RowsReason = "insert row counts are unavailable before execution"
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

// readCascades collects triggers and foreign key effects.
func (measure measurer) readCascades(ctx context.Context, plan *Plan) {
	// Skip trigger and foreign key lookup when no rows match.
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

// readTriggers returns matching triggers. Engine adapters return the target table in Detail and trigger events in Events.
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
		// Omit the target table for triggers.
		cascades = append(cascades, Cascade{Reason: "trigger " + object.Name})
	}
	return cascades
}

// runsTrigger is true when the trigger matches the write or its events are unknown.
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

// readKeysPointingHere separates cascading foreign keys from references that may block a delete.
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
		// Unknown reference counts remain possible blockers.
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

// countFollowedRows counts referencing rows for single-column foreign keys.
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
