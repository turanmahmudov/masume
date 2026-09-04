// Package writeplan measures what a write does before it runs: the rows it lands on, the
// columns it changes, the relations it reaches through the server, and the statements that
// undo it afterwards. Nothing here decides whether a write may run.
package writeplan

import (
	"context"

	"github.com/turanmahmudov/masume/internal/cfg"
	"github.com/turanmahmudov/masume/internal/core"
	"github.com/turanmahmudov/masume/internal/db"
	"github.com/turanmahmudov/masume/internal/query/statement"
)

// Source is the part of a connection a plan reads.
type Source interface {
	db.SessionInfo
	db.CatalogReader
	db.QueryRunner
}

// Request is one plan to build.
type Request struct {
	SQL string
	// The relations of the connection, so the name in the statement leads to one of them.
	Tables []db.TableRef
	Mode   cfg.WritePlan
	// How many rows the undo may hold. A write over this many rows keeps none.
	UndoRows int
	// True while the user holds a transaction open, which the write joins.
	InTransaction bool
}

// Cascade is a relation a write reaches through the server rather than through the
// statement: a trigger on the relation, or a foreign key that points at it.
type Cascade struct {
	// What reaches the relation, such as `trigger t_order_audit`.
	Reason string
	// The relation it reaches, and nothing where that is the relation of the write.
	Table   string
	Rows    int64
	HasRows bool
}

// Plan is what one write would do, measured before it runs.
type Plan struct {
	SQL   string
	Kind  statement.WriteKind
	Table db.TableRef
	// The columns the write assigns. A delete assigns none.
	Columns []string
	// The rows the write lands on, counted on the server and not estimated.
	Rows    int64
	HasRows bool
	// Why the rows were not counted, where they were not.
	RowsReason string
	// The rows the whole relation holds, read only where the write names a subset.
	Total    int64
	HasTotal bool

	Cascades []Cascade
	// The relations that block the write while a row of theirs references it.
	Blockers []Cascade
	// Whether the write can be undone, and the read that takes the rows of the undo.
	Undo UndoPlan
	// True while the user holds a transaction open, which the write joins.
	InTransaction bool
}

// NamesEveryRow is true where the write lands on the whole relation.
func (plan Plan) NamesEveryRow() bool {
	return plan.HasRows && plan.HasTotal && plan.Rows == plan.Total && plan.Total > 0
}

// ReadShare returns the share of the relation the write lands on, from 0 to 1.
func (plan Plan) ReadShare() (float64, bool) {
	if !plan.HasRows || !plan.HasTotal || plan.Total <= 0 {
		return 0, false
	}
	return float64(plan.Rows) / float64(plan.Total), true
}

// Measures is true where this connection measures a write before it runs. Only one
// statement is measured: a plan names one relation and one set of rows.
func Measures(
	profile cfg.Profile, capabilities core.Capabilities,
	risk statement.WriteRisk, count int,
) bool {
	return profile.WritePlan != cfg.PlanOff && capabilities.PlansWrites &&
		risk != statement.RiskNone && count == 1
}

// Build measures the write. It returns nothing where the statement is no write this client
// can read as one relation and one predicate, because such a write cannot be counted.
func Build(ctx context.Context, session Source, request Request) (Plan, bool) {
	if request.Mode == cfg.PlanOff || !session.Capabilities().PlansWrites {
		return Plan{}, false
	}
	target, read := statement.ReadWriteTarget(request.SQL, session.Dialect().Syntax)
	if !read {
		return Plan{}, false
	}
	table, found := db.FindTableByName(
		request.Tables, target.Table, session.Describe().DefaultSchema)
	if !found {
		return Plan{}, false
	}

	plan := Plan{
		SQL: request.SQL, Kind: target.Kind, Table: table, Columns: target.Columns,
		InTransaction: request.InTransaction,
	}
	measure := measurer{session: session, target: target, table: table}
	measure.countRows(ctx, &plan)
	measure.readCascades(ctx, &plan)
	if request.Mode == cfg.PlanUndo {
		plan.Undo = measure.planUndo(ctx, plan, request.UndoRows)
	}
	return plan, true
}
