package writeplan_test

import (
	"context"
	"strings"
	"testing"

	"github.com/turanmahmudov/masume/internal/cfg"
	"github.com/turanmahmudov/masume/internal/core"
	"github.com/turanmahmudov/masume/internal/db"
	"github.com/turanmahmudov/masume/internal/db/postgres"
	"github.com/turanmahmudov/masume/internal/query"
	"github.com/turanmahmudov/masume/internal/query/statement"
	"github.com/turanmahmudov/masume/internal/writeplan"
)

// orders is the relation every plan below is built against.
var orders = db.TableRef{Schema: "public", Name: "orders", Kind: db.RelationTable}

// planningSession answers the reads of a plan out of what the test set, and records every
// statement it was asked.
type planningSession struct {
	db.Session
	// What a count answers: the rows of the write, the rows of the whole relation, and
	// the rows of the relation a foreign key follows into.
	matched  int64
	total    int64
	followed int64
	// rows answers the read of the undo.
	columns []db.ResultColumn
	undo    [][]any
	// The relation as the catalog holds it, and what points at it.
	detail        db.TableDetail
	relationships []db.Relationship
	objects       []db.SchemaObject

	asked []string
	// True where the server takes no write the client can read as one relation.
	plansNoWrite bool
}

func (session *planningSession) Describe() db.SessionDescriptor {
	return db.SessionDescriptor{DefaultSchema: "public"}
}

func (session *planningSession) Dialect() *query.Dialect { return postgres.Dialect }

func (session *planningSession) Capabilities() core.Capabilities {
	return core.Capabilities{PlansWrites: !session.plansNoWrite}
}

func (session *planningSession) RunQuery(
	_ context.Context, sql string, _ int, _ []any,
) (db.QueryResult, error) {
	session.asked = append(session.asked, sql)
	if !strings.HasPrefix(sql, "select count(*)") {
		return db.QueryResult{Columns: session.columns, Rows: session.undo}, nil
	}
	return db.QueryResult{Rows: [][]any{{session.answerCount(sql)}}}, nil
}

// answerCount returns the count of the relation the statement reads.
func (session *planningSession) answerCount(sql string) int64 {
	_, counted, _ := strings.Cut(sql, " from ")
	switch {
	case !strings.HasPrefix(counted, `"public"."orders"`):
		return session.followed
	case strings.Contains(counted, " where "):
		return session.matched
	}
	return session.total
}

func (session *planningSession) DescribeTable(
	context.Context, db.TableRef,
) (db.TableDetail, error) {
	return session.detail, nil
}

func (session *planningSession) ListRelationships(
	context.Context,
) ([]db.Relationship, error) {
	return session.relationships, nil
}

func (session *planningSession) ListSchemaObjects(
	context.Context,
) ([]db.SchemaObject, error) {
	return session.objects, nil
}

// buildOrdersSession answers a relation of three columns, keyed by id.
func buildOrdersSession() *planningSession {
	return &planningSession{
		matched: 3, total: 12,
		columns: []db.ResultColumn{
			{Name: "id", DataType: "integer"}, {Name: "status", DataType: "text"},
		},
		undo: [][]any{
			{int64(1), "open"}, {int64(2), "open"}, {int64(3), "sent"},
		},
		detail: db.TableDetail{Table: orders, Columns: []db.ColumnDetail{
			{Name: "id", DataType: "integer", IsPrimaryKey: true},
			{Name: "status", DataType: "text"},
			{Name: "total", DataType: "numeric"},
		}},
	}
}

// buildPlan measures one write against that relation.
func buildPlan(
	t *testing.T, session *planningSession, sql string, mode cfg.WritePlan,
) writeplan.Plan {
	t.Helper()
	plan, measured := writeplan.Build(context.Background(), session, writeplan.Request{
		SQL: sql, Tables: []db.TableRef{orders}, Mode: mode,
		UndoRows: cfg.DefaultUndoRows,
	})
	if !measured {
		t.Fatalf("%q was not measured", sql)
	}
	return plan
}

func TestBuildCountsTheRowsAnUpdateLandsOn(t *testing.T) {
	session := buildOrdersSession()
	session.matched = 3
	plan := buildPlan(t, session,
		"update orders set status = 'sent' where status = 'open'", cfg.PlanCount)

	if !plan.HasRows || plan.Rows != 3 {
		t.Errorf("the plan counted %d rows, reported %v", plan.Rows, plan.HasRows)
	}
	if strings.Join(plan.Columns, ",") != "status" {
		t.Errorf("the plan touches %v", plan.Columns)
	}
	if plan.Table.Name != "orders" {
		t.Errorf("the plan names %q", plan.Table.Name)
	}
}

// Two counts, so the card can say how much of the relation the write takes.
func TestBuildCountsTheWholeRelationAsWell(t *testing.T) {
	session := buildOrdersSession()
	session.matched, session.total = 25, 100
	plan := buildPlan(t, session,
		"delete from orders where status = 'open'", cfg.PlanCount)

	if !plan.HasTotal {
		t.Fatal("the relation was not counted")
	}
	if share, held := plan.ReadShare(); !held || share <= 0 {
		t.Errorf("the share is %v, reported %v", share, held)
	}
}

func TestBuildReadsEveryRowOfAWriteWithoutAPredicate(t *testing.T) {
	session := buildOrdersSession()
	session.matched, session.total = 12, 12
	plan := buildPlan(t, session, "truncate table orders", cfg.PlanCount)

	if !plan.NamesEveryRow() {
		t.Errorf("the plan counted %d of %d rows", plan.Rows, plan.Total)
	}
}

// The plan reads no row of its own. The rows are read when the write runs, inside its
// transaction, so nothing changes them in between.
func TestBuildReadsNoRowOfTheWrite(t *testing.T) {
	session := buildOrdersSession()
	plan := buildPlan(t, session,
		"update orders set status = 'sent' where status = 'open'", cfg.PlanUndo)

	if !plan.Undo.Kept {
		t.Fatalf("no undo was planned: %s", plan.Undo.Reason)
	}
	for _, sql := range session.asked {
		if !strings.HasPrefix(sql, "select count(*)") {
			t.Errorf("the plan read rows as well:\n%s", sql)
		}
	}
}

// The read of the undo holds its rows until the transaction ends, so the write finds them
// as the undo read them.
func TestTheUndoReadHoldsItsRows(t *testing.T) {
	session := buildOrdersSession()
	plan := buildPlan(t, session,
		"update orders set status = 'sent' where status = 'open'", cfg.PlanUndo)

	if !strings.HasSuffix(plan.Undo.Read, " for update") {
		t.Errorf("the undo reads:\n%s", plan.Undo.Read)
	}
	if !strings.Contains(plan.Undo.Read, `"id", "status"`) {
		t.Errorf("the undo reads the columns:\n%s", plan.Undo.Read)
	}
}

func TestReadUndoBuildsTheStatementsOfAnUpdate(t *testing.T) {
	session := buildOrdersSession()
	plan := buildPlan(t, session,
		"update orders set status = 'sent' where status = 'open'", cfg.PlanUndo)

	undo, err := writeplan.ReadUndo(context.Background(), session, plan.Undo)
	if err != nil {
		t.Fatalf("the undo answered %v", err)
	}
	if !undo.IsHeld() || undo.Rows != 3 || len(undo.Changes) != 3 {
		t.Fatalf("the undo holds %d rows and %d statements", undo.Rows, len(undo.Changes))
	}
	if !strings.HasPrefix(undo.Display[0], `update "public"."orders" set "status" =`) {
		t.Errorf("the undo reads:\n%s", undo.Display[0])
	}
}

func TestReadUndoBuildsTheStatementsOfADelete(t *testing.T) {
	session := buildOrdersSession()
	session.columns = append(session.columns, db.ResultColumn{Name: "total", DataType: "numeric"})
	session.undo = [][]any{{int64(1), "open", "9.99"}}
	plan := buildPlan(t, session, "delete from orders where id = 1", cfg.PlanUndo)

	undo, err := writeplan.ReadUndo(context.Background(), session, plan.Undo)
	if err != nil {
		t.Fatalf("the undo answered %v", err)
	}
	if !strings.HasPrefix(undo.Display[0], `insert into "public"."orders"`) {
		t.Errorf("the undo reads:\n%s", undo.Display[0])
	}
}

// The mode that only counts plans no undo, so no row of the write is ever read.
func TestBuildPlansNoUndoWhereOnlyTheCountWasAskedFor(t *testing.T) {
	session := buildOrdersSession()
	plan := buildPlan(t, session, "delete from orders where id = 1", cfg.PlanCount)

	if plan.Undo.Kept {
		t.Error("the count mode planned an undo")
	}
	for _, sql := range session.asked {
		if !strings.HasPrefix(sql, "select count(*)") {
			t.Errorf("the plan read rows as well:\n%s", sql)
		}
	}
}

func TestBuildKeepsNoUndoWithoutAPrimaryKey(t *testing.T) {
	session := buildOrdersSession()
	session.detail.Columns[0].IsPrimaryKey = false
	plan := buildPlan(t, session, "delete from orders where id = 1", cfg.PlanUndo)

	if plan.Undo.Kept {
		t.Error("an undo was planned for a relation with no key")
	}
	if !strings.Contains(plan.Undo.Reason, "primary key") {
		t.Errorf("the reason is %q", plan.Undo.Reason)
	}
}

// An update that assigns the key moves the rows the undo would name, so there is none.
func TestBuildKeepsNoUndoWhereTheWriteAssignsTheKey(t *testing.T) {
	session := buildOrdersSession()
	plan := buildPlan(t, session, "update orders set id = 9 where id = 1", cfg.PlanUndo)

	if plan.Undo.Kept {
		t.Error("an undo was planned for a write that moves its own key")
	}
}

func TestBuildKeepsNoUndoOverTheRowLimit(t *testing.T) {
	session := buildOrdersSession()
	session.matched = 5000
	plan, measured := writeplan.Build(context.Background(), session, writeplan.Request{
		SQL: "delete from orders where id > 1", Tables: []db.TableRef{orders},
		Mode: cfg.PlanUndo, UndoRows: 10,
	})
	if !measured {
		t.Fatal("the delete was not measured")
	}
	if plan.Undo.Kept {
		t.Error("an undo was planned over the limit")
	}
	if !strings.Contains(plan.Undo.Reason, "undo_rows") {
		t.Errorf("the reason is %q", plan.Undo.Reason)
	}
}

func TestBuildReportsTheTriggersOfTheRelation(t *testing.T) {
	session := buildOrdersSession()
	session.objects = []db.SchemaObject{
		{
			Schema: "public", Name: "t_order_audit", Kind: db.ObjectTrigger,
			Detail: "orders", Events: "insert, update",
		},
		{
			Schema: "public", Name: "t_other", Kind: db.ObjectTrigger,
			Detail: "customers", Events: "update",
		},
	}
	plan := buildPlan(t, session,
		"update orders set status = 'sent' where id = 1", cfg.PlanCount)

	if len(plan.Cascades) != 1 {
		t.Fatalf("the plan reports %d cascades", len(plan.Cascades))
	}
	if !strings.Contains(plan.Cascades[0].Reason, "t_order_audit") {
		t.Errorf("the cascade reads %q", plan.Cascades[0].Reason)
	}
}

// A foreign key that refuses the delete writes nothing, so it is no cascade.
func TestBuildReportsOnlyTheKeysThatFollowARemovedRow(t *testing.T) {
	session := buildOrdersSession()
	session.matched, session.followed = 2, 8
	session.relationships = []db.Relationship{
		{
			ForeignKey: db.ForeignKey{
				Name: "lines_order", Columns: []string{"order_id"},
				TargetSchema: "public", TargetTable: "orders",
				TargetColumns: []string{"id"}, DeleteRule: query.DeleteRuleCascade,
			},
			Schema: "public", Table: "order_lines",
		},
		{
			ForeignKey: db.ForeignKey{
				Name: "notes_order", Columns: []string{"order_id"},
				TargetSchema: "public", TargetTable: "orders",
				TargetColumns: []string{"id"}, DeleteRule: query.DeleteRuleRestrict,
			},
			Schema: "public", Table: "order_notes",
		},
	}
	plan := buildPlan(t, session, "delete from orders where id = 1", cfg.PlanCount)

	if len(plan.Cascades) != 1 {
		t.Fatalf("the plan reports %d cascades: %v", len(plan.Cascades), plan.Cascades)
	}
	if plan.Cascades[0].Table != "public.order_lines" || !plan.Cascades[0].HasRows {
		t.Errorf("the cascade is %+v", plan.Cascades[0])
	}
	if len(plan.Blockers) != 1 || plan.Blockers[0].Table != "public.order_notes" {
		t.Errorf("the blockers are %+v", plan.Blockers)
	}
}

// The server rejects a delete while a restricting key references the rows, so the plan says
// so before the write runs.
func TestBuildReportsABlockerOnlyWhereRowsReferenceTheseRows(t *testing.T) {
	session := buildOrdersSession()
	session.relationships = []db.Relationship{{
		ForeignKey: db.ForeignKey{
			Name: "notes_order", Columns: []string{"order_id"},
			TargetSchema: "public", TargetTable: "orders",
			TargetColumns: []string{"id"}, DeleteRule: query.DeleteRuleRestrict,
		},
		Schema: "public", Table: "order_notes",
	}}

	session.matched, session.followed = 2, 4
	plan := buildPlan(t, session, "delete from orders where id = 1", cfg.PlanCount)
	if len(plan.Blockers) != 1 || plan.Blockers[0].Rows != 4 {
		t.Errorf("the blockers are %+v", plan.Blockers)
	}

	session.followed = 0
	quiet := buildPlan(t, session, "delete from orders where id = 1", cfg.PlanCount)
	if len(quiet.Blockers) != 0 {
		t.Errorf("a relation that references no row blocked: %+v", quiet.Blockers)
	}
}

// An update reaches no other relation through a key, so no key is reported for one.
func TestBuildReportsNoFollowingKeyForAnUpdate(t *testing.T) {
	session := buildOrdersSession()
	session.relationships = []db.Relationship{{
		ForeignKey: db.ForeignKey{
			Columns: []string{"order_id"}, TargetSchema: "public", TargetTable: "orders",
			TargetColumns: []string{"id"}, DeleteRule: query.DeleteRuleCascade,
		},
		Schema: "public", Table: "order_lines",
	}}
	plan := buildPlan(t, session,
		"update orders set status = 'sent' where id = 1", cfg.PlanCount)

	if len(plan.Cascades) != 0 {
		t.Errorf("the plan reports %v", plan.Cascades)
	}
}

func TestBuildMeasuresNothingWhereTheModeIsOff(t *testing.T) {
	session := buildOrdersSession()
	if _, measured := writeplan.Build(context.Background(), session, writeplan.Request{
		SQL: "delete from orders", Tables: []db.TableRef{orders}, Mode: cfg.PlanOff,
	}); measured {
		t.Error("a plan was built with the mode off")
	}
	if len(session.asked) != 0 {
		t.Errorf("the server was asked %v", session.asked)
	}
}

func TestBuildMeasuresNothingTheServerCannotRead(t *testing.T) {
	for _, written := range []string{
		"delete from orders using customers where customers.id = orders.customer",
		"select * from orders",
		"delete from unknown_relation where id = 1",
	} {
		if _, measured := writeplan.Build(
			context.Background(), buildOrdersSession(), writeplan.Request{
				SQL: written, Tables: []db.TableRef{orders}, Mode: cfg.PlanUndo,
			}); measured {
			t.Errorf("%q was measured", written)
		}
	}
}

func TestBuildMeasuresNothingOnAServerThatTakesNoSQL(t *testing.T) {
	session := buildOrdersSession()
	session.plansNoWrite = true
	if _, measured := writeplan.Build(context.Background(), session, writeplan.Request{
		SQL: "delete from orders", Tables: []db.TableRef{orders}, Mode: cfg.PlanUndo,
	}); measured {
		t.Error("a plan was built for a server that reads no write")
	}
}

func TestDescribeLinesWritesEveryPartOfThePlan(t *testing.T) {
	session := buildOrdersSession()
	plan := buildPlan(t, session,
		"update orders set status = 'sent' where status = 'open'", cfg.PlanUndo)

	written := strings.Join(writeplan.DescribeLines(plan), "\n")
	for _, part := range []string{
		writeplan.LabelRows, writeplan.LabelColumns,
		writeplan.LabelUndo, writeplan.LabelCommit, "status",
	} {
		if !strings.Contains(written, part) {
			t.Errorf("the plan says nothing of %q:\n%s", part, written)
		}
	}
}

// A batch reaches rows the count of one relation cannot answer for, so it is not measured.
func TestMeasuresOnlyOneWriteAtATime(t *testing.T) {
	profile := cfg.Profile{WritePlan: cfg.PlanUndo}
	takesSQL := core.Capabilities{PlansWrites: true}

	if !writeplan.Measures(profile, takesSQL, statement.RiskDelete, 1) {
		t.Error("one write was not measured")
	}
	if writeplan.Measures(profile, takesSQL, statement.RiskDelete, 2) {
		t.Error("a batch of two writes was measured")
	}
	if writeplan.Measures(profile, takesSQL, statement.RiskNone, 1) {
		t.Error("a read was measured")
	}
	if writeplan.Measures(profile, core.Capabilities{}, statement.RiskDelete, 1) {
		t.Error("a write was measured on a server that reads none")
	}
	if writeplan.Measures(
		cfg.Profile{WritePlan: cfg.PlanOff}, takesSQL, statement.RiskDelete, 1) {
		t.Error("a write was measured with the mode off")
	}
}

// A trigger the write does not run is no cascade of it. A server that names no event at all
// leaves the trigger in.
func TestBuildReportsOnlyTheTriggersTheWriteRuns(t *testing.T) {
	session := buildOrdersSession()
	session.objects = []db.SchemaObject{
		{
			Schema: "public", Name: "t_on_update", Kind: db.ObjectTrigger,
			Detail: "orders", Events: "update",
		},
		{
			Schema: "public", Name: "t_on_delete", Kind: db.ObjectTrigger,
			Detail: "orders", Events: "delete",
		},
		{
			Schema: "public", Name: "t_unnamed", Kind: db.ObjectTrigger,
			Detail: "orders",
		},
	}
	plan := buildPlan(t, session, "delete from orders where id = 1", cfg.PlanCount)

	named := []string{}
	for _, cascade := range plan.Cascades {
		named = append(named, cascade.Reason)
	}
	written := strings.Join(named, " ")
	if strings.Contains(written, "t_on_update") {
		t.Errorf("a trigger of another write was reported: %s", written)
	}
	if !strings.Contains(written, "t_on_delete") || !strings.Contains(written, "t_unnamed") {
		t.Errorf("the triggers of the delete are %s", written)
	}
}

// A write that matches no row reaches nothing, so nothing is named for it.
func TestBuildReportsNothingReachedByAWriteThatMatchesNoRow(t *testing.T) {
	session := buildOrdersSession()
	session.matched = 0
	session.objects = []db.SchemaObject{{
		Schema: "public", Name: "t_on_delete", Kind: db.ObjectTrigger,
		Detail: "orders", Events: "delete",
	}}
	session.relationships = []db.Relationship{{
		ForeignKey: db.ForeignKey{
			Columns: []string{"order_id"}, TargetSchema: "public", TargetTable: "orders",
			TargetColumns: []string{"id"}, DeleteRule: query.DeleteRuleRestrict,
		},
		Schema: "public", Table: "order_notes",
	}}
	plan := buildPlan(t, session, "delete from orders where id = 99", cfg.PlanCount)

	if len(plan.Cascades) != 0 || len(plan.Blockers) != 0 {
		t.Errorf("the plan reports %+v and %+v", plan.Cascades, plan.Blockers)
	}
}
