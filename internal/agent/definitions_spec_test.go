package agent_test

import (
	"context"
	"strings"
	"testing"

	"github.com/turanmahmudov/masume/internal/agent"
	"github.com/turanmahmudov/masume/internal/core"
	"github.com/turanmahmudov/masume/internal/db"
	"github.com/turanmahmudov/masume/internal/query/language"
	"github.com/turanmahmudov/masume/internal/query/statement"
)

// Every tool is described to the model as JSON Schema, and the model can send any input. A
// tool without a name or without a schema cannot be called, and a tool that reads its input
// without validation accepts any value.
func TestEveryDefinitionIsCallableAndDescribed(t *testing.T) {
	held := agent.Definitions()
	if len(held) == 0 {
		t.Fatal("the catalogue holds no tool")
	}

	names := map[string]bool{}
	for _, tool := range held {
		if tool.Name == "" {
			t.Error("a tool carries no name")
			continue
		}
		if names[tool.Name] {
			t.Errorf("two tools are named %q", tool.Name)
		}
		names[tool.Name] = true

		if tool.Description == "" {
			t.Errorf("%q is described to the model as nothing", tool.Name)
		}
		if tool.Call == nil {
			t.Errorf("%q cannot be called", tool.Name)
		}
		if tool.InputSchema == nil {
			t.Errorf("%q describes no input", tool.Name)
		}
		if held := tool.InputSchema["type"]; held != "object" {
			t.Errorf("%q describes its input as %v, wanted an object", tool.Name, held)
		}
		// A caller must not be able to send a field the tool does not read.
		if allowed, there := tool.InputSchema["additionalProperties"]; !there || allowed != false {
			t.Errorf("%q takes fields it does not describe", tool.Name)
		}
	}
}

// A tool that uses the server must reject an invalid input and must not run with a part of
// it. No test here opens a connection, because the validation runs first.
func TestEveryDefinitionRefusesAnInputItCannotRead(t *testing.T) {
	for _, tool := range agent.Definitions() {
		t.Run(tool.Name, func(t *testing.T) {
			answered := tool.Call(context.Background(), agent.ToolDeps{},
				map[string]any{"a_field_no_tool_takes": "held"})
			if answered == nil {
				t.Fatal("an input it cannot read answered nothing")
			}
			if !strings.Contains(strings.ToLower(describeAnswer(answered)), "field") {
				t.Errorf("the refusal reads %v and does not name the field", answered)
			}
		})
	}
}

// The row limit from a model must be a number above zero. A limit of zero or less would read
// one row or every row, and neither is the request.
func TestRunQueryRefusesALimitBelowOne(t *testing.T) {
	tool := findTool(t, "run_query")

	for _, limit := range []any{float64(0), float64(-5)} {
		answered := tool.Call(context.Background(), agent.ToolDeps{},
			map[string]any{"sql": "select 1", "limit": limit})
		if answered == nil {
			t.Fatalf("a limit of %v answered nothing", limit)
		}
		if !strings.Contains(strings.ToLower(describeAnswer(answered)), "limit") {
			t.Errorf("a limit of %v gave %v, wanted the limit named", limit, answered)
		}
	}
}

// A number with a fraction is not a row count, and a truncation would give the model a
// different limit from the one it sent.
func TestRunQueryRefusesALimitWithAFraction(t *testing.T) {
	tool := findTool(t, "run_query")
	answered := tool.Call(context.Background(), agent.ToolDeps{},
		map[string]any{"sql": "select 1", "limit": float64(2.5)})
	if answered == nil {
		t.Fatal("a limit with a fraction answered nothing")
	}
}

// The statement is the input of the tool, so a call without one gives an error and does not
// run as an empty statement.
func TestRunQueryWantsAStatement(t *testing.T) {
	tool := findTool(t, "run_query")
	answered := tool.Call(context.Background(), agent.ToolDeps{}, map[string]any{})
	if answered == nil {
		t.Fatal("a call with no statement answered nothing")
	}
	if !strings.Contains(strings.ToLower(describeAnswer(answered)), "sql") {
		t.Errorf("the refusal reads %v and does not name the field", answered)
	}
}

// A statement from the model is checked before anything reaches the server, and the answer
// of the user decides. A refusal must stop the run and must not be reported next to a
// result.
func TestRunQueryStopsWhereTheUserSaysNo(t *testing.T) {
	tool := findTool(t, "run_query")
	ran := false

	question := askedQuestion{}
	deps := buildRefusingDeps(&recordingSession{}, &question)
	deps.Runner.RunStatement = func(
		context.Context, string, int,
	) (agent.StatementAnswer, error) {
		ran = true
		return agent.StatementAnswer{}, nil
	}

	answered := tool.Call(context.Background(), deps,
		map[string]any{"sql": "delete from orders"})
	if !question.asked {
		t.Error("the statement reached the server without the user being asked")
	}
	if ran {
		t.Error("the statement ran although the user said no")
	}
	if answered == nil {
		t.Fatal("the refusal answered nothing")
	}
}

// The question to the user names the real risk of the statement, so a delete is never shown
// as a read.
func TestRunQueryWeighsTheStatementItAsksAbout(t *testing.T) {
	tool := findTool(t, "run_query")
	question := askedQuestion{}
	deps := buildRefusingDeps(&recordingSession{}, &question)

	tool.Call(context.Background(), deps, map[string]any{"sql": "delete from orders"})
	if question.weighed != statement.RiskEveryRow {
		t.Errorf("a delete with no where was weighed as %q", question.weighed)
	}
}

// A plan of a write still sends the statement, so the check that stops a run also stops a
// plan. A refusal must stop the plan and must not be reported next to an estimate.
func TestExplainQueryStopsWhereTheUserSaysNo(t *testing.T) {
	tool := findTool(t, "explain_query")
	question := askedQuestion{}
	session := &recordingSession{}
	deps := buildRefusingDeps(session, &question)

	answered := tool.Call(context.Background(), deps,
		map[string]any{"sql": "delete from orders"})
	if !question.asked {
		t.Error("the statement reached the server without the user being asked")
	}
	if len(session.explained) != 0 {
		t.Error("the statement was planned although the user said no")
	}
	if answered == nil {
		t.Fatal("the refusal answered nothing")
	}
}

// A batch that starts with a read still writes, so it is checked as a write and is not sent.
// Otherwise the planner would add the EXPLAIN prefix to the first statement only and run the
// rest.
func TestExplainQueryWeighsABatchAsAWrite(t *testing.T) {
	tool := findTool(t, "explain_query")
	question := askedQuestion{}
	session := &recordingSession{}
	deps := buildRefusingDeps(session, &question)

	tool.Call(context.Background(), deps,
		map[string]any{"sql": "select 1; delete from orders"})
	if question.weighed != statement.RiskEveryRow {
		t.Errorf("a batch holding a delete was weighed as %q", question.weighed)
	}
	if len(session.explained) != 0 {
		t.Error("the batch was planned")
	}
}

// A read is only estimated, so the user is not asked and the plan still reaches the
// server.
func TestExplainQueryPlansAReadWithoutAsking(t *testing.T) {
	tool := findTool(t, "explain_query")
	question := askedQuestion{}
	session := &recordingSession{}
	deps := buildAllowingDeps(session, &question)

	tool.Call(context.Background(), deps, map[string]any{"sql": "select 1"})
	if question.asked {
		t.Error("a read was asked about before it was planned")
	}
	if len(session.explained) != 1 {
		t.Errorf("the server was asked %v, wanted the one read", session.explained)
	}
}

// A write that the user allows to plan is still only estimated, so the statement does not run
// even if the model asked for a measured plan.
func TestExplainQueryEstimatesAWriteAfterItIsAllowed(t *testing.T) {
	tool := findTool(t, "explain_query")
	question := askedQuestion{}
	session := &recordingSession{}
	deps := buildAllowingDeps(session, &question)

	answered := tool.Call(context.Background(), deps,
		map[string]any{"sql": "delete from orders where id = 1", "analyze": true})
	if len(session.explained) != 1 {
		t.Fatalf("the server was asked %v, wanted the one write", session.explained)
	}
	if session.analyzed {
		t.Error("a write was measured")
	}
	held, is := answered.(map[string]any)
	if !is {
		t.Fatalf("the answer was %T, wanted a map", answered)
	}
	note, _ := held["note"].(string)
	if !strings.Contains(note, "nothing ran") {
		t.Errorf("the answer reads %v and does not say nothing ran", answered)
	}
}

// A server without plan support gives an error before the question, because the user would
// be asked about a statement that is never sent.
func TestExplainQueryAnswersAServerThatPlansNothingWithoutAsking(t *testing.T) {
	tool := findTool(t, "explain_query")
	question := askedQuestion{}
	session := &recordingSession{cannotPlan: true}
	deps := buildRefusingDeps(session, &question)

	answered := tool.Call(context.Background(), deps,
		map[string]any{"sql": "delete from orders"})
	if question.asked {
		t.Error("the user was asked about a statement no server would plan")
	}
	if len(session.explained) != 0 {
		t.Error("a server that plans nothing was asked for a plan")
	}
	held, is := answered.(map[string]any)
	if !is || held["error"] == nil {
		t.Errorf("the answer was %v, wanted the refusal of the server", answered)
	}
}

// askedQuestion holds the question the user got before a statement ran.
type askedQuestion struct {
	asked   bool
	weighed statement.WriteRisk
}

// buildRefusingDeps builds the tools of this session with a user who refuses every statement
// the runner asks about.
func buildRefusingDeps(session db.Session, question *askedQuestion) agent.ToolDeps {
	return buildAskingDeps(session, question, "the user did not allow this statement to run")
}

// buildAllowingDeps builds the tools of this session with a user who allows every statement
// the runner asks about.
func buildAllowingDeps(session db.Session, question *askedQuestion) agent.ToolDeps {
	return buildAskingDeps(session, question, "")
}

// buildAskingDeps builds the tools of this session with a runner that records the question
// and answers it with this refusal. Nothing runs unless the test adds RunStatement itself.
func buildAskingDeps(
	session db.Session, question *askedQuestion, refusal string,
) agent.ToolDeps {
	return agent.ToolDeps{
		Session: session,
		Runner: agent.StatementRunner{
			RowLimit: 100,
			AskToRun: func(
				_ context.Context, risk statement.WriteRisk, _ []string,
			) agent.RunPermission {
				question.asked, question.weighed = true, risk
				return agent.RunPermission{Refusal: refusal}
			},
		},
	}
}

// findTool returns the definition with that name.
func findTool(t *testing.T, name string) agent.ToolDefinition {
	t.Helper()
	for _, tool := range agent.Definitions() {
		if tool.Name == name {
			return tool
		}
	}
	t.Fatalf("the catalogue holds no tool named %q", name)
	return agent.ToolDefinition{}
}

// describeAnswer returns the answer of a tool as text, so a test can search it for a word.
func describeAnswer(answered any) string {
	if held, is := answered.(map[string]any); is {
		written := []string{}
		for _, value := range held {
			if text, isText := value.(string); isText {
				written = append(written, text)
			}
		}
		return strings.Join(written, " ")
	}
	return ""
}

// recordingSession implements only the calls a tool makes before it reaches the server.
type recordingSession struct {
	db.Session
	explained  []string
	analyzed   bool
	cannotPlan bool
}

func (session *recordingSession) Language() language.Language { return language.SQL }

func (session *recordingSession) Capabilities() core.Capabilities {
	return core.Capabilities{PlansStatement: !session.cannotPlan}
}

func (session *recordingSession) ExplainQuery(
	_ context.Context, sql string, analyze bool,
) (db.QueryPlan, error) {
	session.explained = append(session.explained, sql)
	session.analyzed = analyze
	return db.QueryPlan{}, nil
}
