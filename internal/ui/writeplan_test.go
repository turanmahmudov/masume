package ui

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/turanmahmudov/masume/internal/app"
	"github.com/turanmahmudov/masume/internal/cfg"
	"github.com/turanmahmudov/masume/internal/core"
	"github.com/turanmahmudov/masume/internal/db"
	"github.com/turanmahmudov/masume/internal/writeplan"
)

// undoingSession records the statements the undo applied.
type undoingSession struct {
	*offlineSession
	applied []db.Change
	problem error
}

func (session *undoingSession) ApplyChanges(_ context.Context, changes []db.Change) error {
	if session.problem != nil {
		return session.problem
	}
	session.applied = append(session.applied, changes...)
	return nil
}

// buildPlannedModel answers a model whose connection measures a write before it runs.
func buildPlannedModel(t *testing.T) (*Model, *app.Connection, *undoingSession) {
	t.Helper()
	model := buildOfflineModel(t, 120, 40)
	connection := model.Active()
	session := &undoingSession{offlineSession: connection.Session.(*offlineSession)}
	session.profile.WritePlan = cfg.PlanUndo
	session.profile.UndoRows = cfg.DefaultUndoRows
	session.capabilities = core.Capabilities{SortsRead: true, PlansWrites: true}
	connection.Session = session
	return model, connection, session
}

// buildTestUndo answers an undo of one row of orders.
func buildTestUndo() writeplan.Undo {
	return writeplan.Undo{
		Table: db.TableRef{Schema: "public", Name: "orders"},
		Rows:  1,
		Changes: []db.Change{{
			Description: "put status of one row of orders back",
			Display:     `update "public"."orders" set "status" = $1 where "id" = $2`,
		}},
		Display: []string{`update "public"."orders" set "status" = 'open' where "id" = 1`},
	}
}

// buildTestPlan answers a measured write over a quarter of a relation.
func buildTestPlan() writeplan.Plan {
	return writeplan.Plan{
		SQL:     "update orders set status = 'sent' where status = 'open'",
		Kind:    "update",
		Table:   db.TableRef{Schema: "public", Name: "orders"},
		Columns: []string{"status"},
		Rows:    3, HasRows: true, Total: 12, HasTotal: true,
		Cascades: []writeplan.Cascade{{Reason: "trigger t_order_audit", Table: "orders"}},
		Undo: writeplan.UndoPlan{
			Kept: true, Rows: 1, Table: db.TableRef{Schema: "public", Name: "orders"},
		},
	}
}

func TestAWriteThatCannotBeMeasuredKeepsThePlainQuestion(t *testing.T) {
	model, connection, _ := buildPlannedModel(t)
	connection.Overlay = app.Overlay{
		Kind: app.OverlayMessage, Title: measuringPlanTitle, Body: "measuring…",
	}
	held, _ := model.Update(writePlanBuiltMsg{
		ConnectionID: model.ActiveID(), TabID: connection.Active().ID,
		Written: "delete from orders using lines where lines.id = orders.line",
		SQL:     "delete from orders using lines where lines.id = orders.line",
	})
	model = held.(*Model)

	if model.Active().Overlay.Kind != app.OverlayConfirm {
		t.Fatalf("the card is %q, wanted the plain question",
			model.Active().Overlay.Kind)
	}
}

// An answer that lands after the reader closed the card is dropped.
func TestThePlanIsDrawnOverTheLineThatSaidItWasMeasured(t *testing.T) {
	model, connection, _ := buildPlannedModel(t)
	tab := connection.Active()
	connection.Overlay = app.Overlay{
		Kind: app.OverlayMessage, Title: measuringPlanTitle, Body: "measuring…",
	}

	answered := writePlanBuiltMsg{
		ConnectionID: model.ActiveID(), TabID: tab.ID,
		Written: buildTestPlan().SQL, SQL: buildTestPlan().SQL,
		Plan: buildTestPlan(), Measured: true,
	}
	held, _ := model.Update(answered)
	model = held.(*Model)
	if model.Active().Overlay.Kind != app.OverlayWritePlan {
		t.Fatalf("the card is %q", model.Active().Overlay.Kind)
	}

	model.Active().Overlay = app.Overlay{}
	held, _ = model.Update(answered)
	model = held.(*Model)
	if model.Active().Overlay.IsOpen() {
		t.Errorf("a closed card was drawn again as %q", model.Active().Overlay.Kind)
	}
}

func TestThePlanCardSaysWhatTheWriteDoes(t *testing.T) {
	model, connection, _ := buildPlannedModel(t)
	connection.Overlay = app.Overlay{
		Kind: app.OverlayWritePlan, Title: " write plan ", Plan: buildTestPlan(),
	}

	drawn := stripStyles(model.renderWritePlan(connection.Overlay, 100))
	for _, said := range []string{
		"3 of 12 in orders", "status", "t_order_audit", "1 row read", "run", "cancel",
	} {
		if !strings.Contains(drawn, said) {
			t.Errorf("the card says nothing of %q:\n%s", said, drawn)
		}
	}
}

func TestTheUndoIsOfferedAndRun(t *testing.T) {
	model, connection, session := buildPlannedModel(t)
	connection.KeepUndo(buildTestUndo(), "update orders set status = 'sent'", time.Now())

	held, _ := model.undoLastWrite(connection)
	model = held.(*Model)
	overlay := model.Active().Overlay
	if overlay.Kind != app.OverlayConfirm {
		t.Fatalf("the card is %q, wanted the question", overlay.Kind)
	}
	if !strings.Contains(overlay.Body, "orders") {
		t.Errorf("the question reads:\n%s", overlay.Body)
	}

	command := overlay.Answers.Answer(true)
	if command == nil {
		t.Fatal("a yes started nothing")
	}
	answer, is := command().(undoWrittenMsg)
	if !is {
		t.Fatalf("the answer is %T", command())
	}
	if answer.Problem != "" {
		t.Fatalf("the undo answered %q", answer.Problem)
	}
	if len(session.applied) != 1 {
		t.Fatalf("the undo applied %d changes", len(session.applied))
	}

	held, _ = model.Update(answer)
	model = held.(*Model)
	if model.Active().Undo != nil {
		t.Error("the undo was kept after it ran")
	}
}

func TestNothingIsUndoneWithoutAWriteThatKeptOne(t *testing.T) {
	model, connection, session := buildPlannedModel(t)
	held, _ := model.undoLastWrite(connection)
	model = held.(*Model)

	if model.Active().Overlay.IsOpen() {
		t.Error("a card was opened with no undo to run")
	}
	if len(session.applied) != 0 {
		t.Error("statements were applied with no undo to run")
	}
}

// A write the server refused reads no undo, so nothing is kept for it. The undo is read
// inside the transaction of the write and answered with it.
func TestAFailedWriteKeepsNoUndo(t *testing.T) {
	model, connection, _ := buildPlannedModel(t)
	tab := connection.Active()

	held, _ := model.Update(queryRanMsg{
		ConnectionID: model.ActiveID(), TabID: tab.ID, RunID: 1,
		Read:    db.ComposedRead{Display: "update orders set status = 'sent'"},
		Problem: "the server refused it",
	})
	model = held.(*Model)

	if model.Active().Undo != nil {
		t.Error("a write that failed left an undo")
	}
}
