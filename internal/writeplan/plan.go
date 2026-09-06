// Package writeplan measures matching rows, assigned columns, triggers, foreign key effects, and undo availability before a write runs.
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
	// Available tables for target resolution.
	Tables []db.TableRef
	Mode   cfg.WritePlan
	// Maximum rows to capture for undo.
	UndoRows int
	// True when the write uses an existing transaction.
	InTransaction bool
}

// Cascade is a trigger or foreign key effect associated with a write.
type Cascade struct {
	// The trigger or foreign key action, such as trigger t_order_audit.
	Reason string
	// The affected table, or empty for a trigger on the target table.
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
	// Matching rows counted on the server.
	Rows    int64
	HasRows bool
	// The reason the row count is unavailable.
	RowsReason string
	// Total table rows, counted separately when the write has a predicate.
	Total    int64
	HasTotal bool

	Cascades []Cascade
	// Tables with references that may block the write.
	Blockers []Cascade
	// Undo availability and the query for the original rows.
	Undo UndoPlan
	// True when the write uses an existing transaction.
	InTransaction bool
}

// NamesEveryRow is true when the write matches every row in a nonempty table.
func (plan Plan) NamesEveryRow() bool {
	return plan.HasRows && plan.HasTotal && plan.Rows == plan.Total && plan.Total > 0
}

// ReadShare returns the matching fraction of table rows, from 0 to 1.
func (plan Plan) ReadShare() (float64, bool) {
	if !plan.HasRows || !plan.HasTotal || plan.Total <= 0 {
		return 0, false
	}
	return float64(plan.Rows) / float64(plan.Total), true
}

// Measures is true when the profile and server support measurement of a single write statement.
func Measures(
	profile cfg.Profile, capabilities core.Capabilities,
	risk statement.WriteRisk, count int,
) bool {
	return profile.WritePlan != cfg.PlanOff && capabilities.PlansWrites &&
		risk != statement.RiskNone && count == 1
}

// Build measures a supported write with one known target table and one predicate.
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
