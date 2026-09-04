package mcp_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/turanmahmudov/masume/internal/cfg"
	"github.com/turanmahmudov/masume/internal/core"
	"github.com/turanmahmudov/masume/internal/db/engines"
	"github.com/turanmahmudov/masume/internal/mcp"
)

// The two steps an agent takes on a client that cannot show a question: measure the write,
// show the plan, then run it with the token. A SQLite file needs no server, so the whole
// flow runs in the ordinary suite.

// buildPlanTools answers the tools of a server on a file with four orders, under a profile
// that lets an agent carry the answer of the user.
func buildPlanTools(t *testing.T, confirm cfg.ConfirmWrites) ([]mcp.Tool, string) {
	t.Helper()
	// An asker of a client that reported no elicitation, as a client with no dialog is.
	tools, path, _ := buildPlanToolsAsking(t, confirm, mcp.CreateAsker(func(string) {}))
	return tools, path
}

// buildPlanToolsAsking answers the same server with an asker of the test, and the token
// store it issues from.
func buildPlanToolsAsking(
	t *testing.T, confirm cfg.ConfirmWrites, asker *mcp.Asker,
) ([]mcp.Tool, string, *mcp.PlanTokens) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "shop.db")
	profile := cfg.Profile{
		Name: "shop", Engine: core.EngineSqlite, Database: path,
		Environment: cfg.EnvironmentDev, AccessMode: cfg.AccessWrite,
		ConfirmWrites: confirm, WritePlan: cfg.PlanUndo, UndoRows: cfg.DefaultUndoRows,
		PageSize: cfg.DefaultPageSize,
	}
	// The relations are laid out before the server opens the file, because a connection
	// reads its table list when it opens.
	buildPlanFile(t, profile)

	tokens := mcp.CreatePlanTokens()
	tools := mcp.BuildTools(mcp.ToolDeps{
		AccessDeps: mcp.AccessDeps{
			Profiles: []cfg.Profile{profile},
			Config: cfg.McpConfig{
				Profiles: []string{"shop"}, Access: cfg.McpFull, RowLimit: 100,
				Timeout: cfg.DefaultMcpTimeout,
			},
			Sessions: mcp.CreateSessions(engines.CreateAdapters()),
		},
		Asker: asker,
		Plans: tokens,
	})

	return tools, path, tokens
}

// buildPlanFile makes the file the server opens, with four orders in it.
func buildPlanFile(t *testing.T, profile cfg.Profile) {
	t.Helper()
	made, err := os.Create(profile.Database)
	if err != nil {
		t.Fatalf("the file was not made: %v", err)
	}
	_ = made.Close()

	session, err := engines.CreateAdapters().Open(context.Background(), profile, "")
	if err != nil {
		t.Fatalf("the file was not opened: %v", err)
	}
	defer func() { _ = session.Close() }()

	for _, written := range []string{
		"create table orders (id integer primary key, status text not null)",
		"insert into orders (status) values ('open'), ('open'), ('sent'), ('open')",
	} {
		if _, err := session.RunQuery(context.Background(), written, 100, nil); err != nil {
			t.Fatalf("%q answered %v", written, err)
		}
	}
}

// runTool calls one tool of the server and returns what it answered.
func runTool(t *testing.T, tools []mcp.Tool, name string, input map[string]any) map[string]any {
	t.Helper()
	for _, tool := range tools {
		if tool.Name != name {
			continue
		}
		answered, err := tool.Call(context.Background(), input)
		if err != nil {
			t.Fatalf("%s answered %v", name, err)
		}
		held, is := answered.(map[string]any)
		if !is {
			t.Fatalf("%s answered %T", name, answered)
		}
		return held
	}
	t.Fatalf("the server has no tool named %q", name)
	return nil
}

func TestPlanWriteMeasuresTheWriteAndRunsNothing(t *testing.T) {
	tools, _ := buildPlanTools(t, cfg.ConfirmAgent)
	written := "delete from orders where status = 'open'"

	measured := runTool(t, tools, "plan_write",
		map[string]any{"profile": "shop", "sql": written})
	if measured["measured"] != true {
		t.Fatalf("the write was not measured: %v", measured)
	}
	if measured["rows"] != int64(3) || measured["total"] != int64(4) {
		t.Errorf("the plan counted %v of %v", measured["rows"], measured["total"])
	}
	if measured["token"] == "" || measured["token"] == nil {
		t.Error("a profile that lets an agent carry the answer issued no token")
	}

	counted := runTool(t, tools, "run_query",
		map[string]any{"profile": "shop", "sql": "select count(*) as held from orders"})
	if rows, is := counted["rows"].([][]any); !is || len(rows) != 1 ||
		core.FormatCell(rows[0][0], "") != "4" {
		t.Errorf("measuring the write changed the rows: %v", counted["rows"])
	}
}

func TestATokenRunsTheWriteWithoutAQuestion(t *testing.T) {
	tools, _ := buildPlanTools(t, cfg.ConfirmAgent)
	written := "delete from orders where status = 'open'"

	measured := runTool(t, tools, "plan_write",
		map[string]any{"profile": "shop", "sql": written})
	answered := runTool(t, tools, "run_query", map[string]any{
		"profile": "shop", "sql": written, "plan_token": measured["token"],
	})

	if answered["ran"] != true || answered["error"] != nil {
		t.Fatalf("the write did not run: %v", answered)
	}
	undo, is := answered["undo"].([]string)
	if !is || len(undo) != 3 {
		t.Errorf("the answer carries %v", answered["undo"])
	}
	if !strings.HasPrefix(undo[0], `insert into "main"."orders"`) {
		t.Errorf("the undo reads %q", undo[0])
	}
}

// Without the token the write still needs a question, and a client that cannot be asked
// does not run it.
func TestTheSameWriteWithoutATokenIsRefused(t *testing.T) {
	tools, _ := buildPlanTools(t, cfg.ConfirmAgent)
	answered := runTool(t, tools, "run_query", map[string]any{
		"profile": "shop", "sql": "delete from orders where status = 'open'",
	})

	if answered["ran"] != false {
		t.Fatalf("the write ran unasked: %v", answered)
	}
	if reason, _ := answered["reason"].(string); !strings.Contains(reason, "cannot ask you") {
		t.Errorf("the refusal reads %q", reason)
	}
}

// A token is worth nothing on a profile that asks the user itself, so the answer of the
// user is never carried by the agent where a dialog is what the profile asked for.
func TestATokenRunsNothingOnAProfileThatAsksItsUser(t *testing.T) {
	tools, _ := buildPlanTools(t, cfg.ConfirmWrite)
	written := "delete from orders where status = 'open'"

	measured := runTool(t, tools, "plan_write",
		map[string]any{"profile": "shop", "sql": written})
	if measured["token"] != nil {
		t.Errorf("a profile that asks its user issued a token: %v", measured["token"])
	}

	answered := runTool(t, tools, "run_query", map[string]any{
		"profile": "shop", "sql": written, "plan_token": "plan-1",
	})
	if answered["ran"] != false {
		t.Errorf("a made-up token ran the write: %v", answered)
	}
}

func TestPlanWriteSaysWhatItCannotMeasure(t *testing.T) {
	tools, _ := buildPlanTools(t, cfg.ConfirmAgent)

	read := runTool(t, tools, "plan_write",
		map[string]any{"profile": "shop", "sql": "select 1"})
	if read["measured"] != false ||
		!strings.Contains(read["reason"].(string), "writes nothing") {
		t.Errorf("a read answered %v", read)
	}

	joined := runTool(t, tools, "plan_write", map[string]any{
		"profile": "shop",
		"sql":     "update orders set status = 'x' from other where other.id = orders.id",
	})
	if joined["measured"] != false ||
		!strings.Contains(joined["reason"].(string), "one relation") {
		t.Errorf("a write of two relations answered %v", joined)
	}
}

// A profile that lets an agent carry the answer is told how to carry it, and not to turn
// the confirmation off.
func TestARefusalNamesTheStepThatRunsTheWrite(t *testing.T) {
	tools, _ := buildPlanTools(t, cfg.ConfirmAgent)
	answered := runTool(t, tools, "run_query", map[string]any{
		"profile": "shop", "sql": "delete from orders where status = 'open'",
	})

	reason, _ := answered["reason"].(string)
	if !strings.Contains(reason, "plan_write") || !strings.Contains(reason, "plan_token") {
		t.Errorf("the refusal reads %q", reason)
	}
}

// buildAnsweringAsker returns an asker of a client that shows a dialog. Every question it
// is sent lands on the channel, and is answered with this word.
func buildAnsweringAsker(confirmed bool) (*mcp.Asker, chan string) {
	asker := mcp.CreateAsker(func(string) {})
	asker.RememberClient(map[string]any{"elicitation": map[string]any{}})
	asked := make(chan string, 4)

	asker.AttachWriter(func(message any) {
		written, err := json.Marshal(message)
		if err != nil {
			return
		}
		var held struct {
			ID     string         `json:"id"`
			Params map[string]any `json:"params"`
		}
		if json.Unmarshal(written, &held) != nil {
			return
		}
		question, _ := held.Params["message"].(string)
		asked <- question
		go asker.ReceiveAnswer(map[string]any{
			"id": held.ID,
			"result": map[string]any{
				"action":  "accept",
				"content": map[string]any{"confirm": confirmed},
			},
		})
	})
	return asker, asked
}

// A token is what an agent brings back where there is no other way to reach the user. A
// client that shows a dialog is always asked, whatever token the agent sends.
func TestAClientThatShowsADialogIsAlwaysAsked(t *testing.T) {
	asker, asked := buildAnsweringAsker(false)
	tools, _, tokens := buildPlanToolsAsking(t, cfg.ConfirmAgent, asker)
	written := "delete from orders where status = 'open'"

	measured := runTool(t, tools, "plan_write",
		map[string]any{"profile": "shop", "sql": written})
	if measured["token"] != nil {
		t.Errorf("a client that can be asked was issued a token: %v", measured["token"])
	}

	// A token that is good in every other way still does not stand in for the dialog.
	answered := runTool(t, tools, "run_query", map[string]any{
		"profile": "shop", "sql": written,
		"plan_token": tokens.Issue("shop", written),
	})
	select {
	case question := <-asked:
		if !strings.Contains(question, written) {
			t.Errorf("the question reads %q", question)
		}
	default:
		t.Fatal("the write ran without the user being asked")
	}
	if answered["ran"] != false {
		t.Errorf("the write ran although the user said no: %v", answered)
	}
}

// The same write with the same token, answered yes, runs and keeps its undo.
func TestAClientThatShowsADialogRunsWhatTheUserAllows(t *testing.T) {
	asker, asked := buildAnsweringAsker(true)
	tools, _, _ := buildPlanToolsAsking(t, cfg.ConfirmAgent, asker)

	answered := runTool(t, tools, "run_query", map[string]any{
		"profile": "shop", "sql": "delete from orders where status = 'open'",
	})
	if len(asked) == 0 {
		t.Fatal("the write ran without the user being asked")
	}
	if answered["ran"] != true || answered["error"] != nil {
		t.Fatalf("the write did not run: %v", answered)
	}
	if undo, _ := answered["undo"].([]string); len(undo) != 3 {
		t.Errorf("the answer carries %v", answered["undo"])
	}
}
