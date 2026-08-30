package ui

import (
	"context"
	"testing"

	"github.com/turanmahmudov/masume/internal/app"
	"github.com/turanmahmudov/masume/internal/cfg"
	"github.com/turanmahmudov/masume/internal/core"
	"github.com/turanmahmudov/masume/internal/db"
)

// planningSession records what the plan view asked the server for.
type planningSession struct {
	offlineSession
	planned  []string
	analyzed bool
}

func (session *planningSession) ExplainQuery(
	_ context.Context, sql string, analyze bool,
) (db.QueryPlan, error) {
	session.planned = append(session.planned, sql)
	session.analyzed = analyze
	return db.QueryPlan{}, nil
}

// buildPlanningModel answers a model on one connection whose server plans every statement.
func buildPlanningModel(t *testing.T) (*Model, *planningSession, *app.Connection, *app.Tab) {
	t.Helper()
	model := buildOfflineModel(t, 160, 48)
	session := &planningSession{
		profile: cfg.Profile{Name: "offline", Engine: "postgres"},
		capabilities: core.Capabilities{
			PlansStatement: true, PlansEveryStatement: true,
		},
	}
	connection := app.NewConnection(session, nil, true)
	model.connections.open(connection)
	tab := connection.Active()
	if tab == nil {
		t.Fatal("the connection opened with no tab")
	}
	return model, session, connection, tab
}

// A measured plan runs the statement it measures, so the key that asks for one only ever
// estimates a write, and the user is told the write did not run.
func TestExplainAnalyzeOnlyEstimatesAWrite(t *testing.T) {
	model, session, connection, tab := buildPlanningModel(t)
	tab.Editor = app.NewEditorBuffer("delete from orders", 0)

	_, command := model.explain(connection, tab, true)
	if command == nil {
		t.Fatal("the plan was not asked for")
	}
	command()

	if len(session.planned) != 1 {
		t.Fatalf("the server was asked %v, wanted the one statement", session.planned)
	}
	if session.analyzed {
		t.Error("a write was measured, so it ran")
	}
	if connection.Notice == nil {
		t.Error("the write was estimated without a word")
	}
}

// A read is measured as asked, because measuring it changes nothing.
func TestExplainAnalyzeMeasuresARead(t *testing.T) {
	model, session, connection, tab := buildPlanningModel(t)
	tab.Editor = app.NewEditorBuffer("select * from orders", 0)

	_, command := model.explain(connection, tab, true)
	if command == nil {
		t.Fatal("the plan was not asked for")
	}
	command()

	if !session.analyzed {
		t.Error("a read was only estimated")
	}
}
