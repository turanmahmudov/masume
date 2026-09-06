//go:build integration

// An integration test of the MCP server against a real MongoDB. The server is started
// outside this code and named through MASUME_TEST_MONGO.
package mcp_test

import (
	"context"
	"testing"

	"github.com/turanmahmudov/masume/internal/cfg"
	"github.com/turanmahmudov/masume/internal/db/dbtest"
	"github.com/turanmahmudov/masume/internal/db/engines"
	"github.com/turanmahmudov/masume/internal/mcp"
)

// buildMongoTools answers the tools of a server on a collection of two documents, in a
// database of this test alone.
func buildMongoTools(t *testing.T) []mcp.Tool {
	t.Helper()
	profile, password := dbtest.BuildProfile(t, dbtest.Mongo)
	profile.Name = "shop"
	profile.WritePlan = cfg.PlanUndo
	profile.UndoRows = cfg.DefaultUndoRows
	// A database of its own, so the collections of this test are not the collections
	// another package counts.
	profile.Database = "mcp_shop"

	session, err := engines.CreateAdapters().Open(context.Background(), profile, password)
	if err != nil {
		t.Fatalf("cannot reach the server: %v", err)
	}
	t.Cleanup(func() { _ = session.Close() })
	dbtest.RunStatements(t, session,
		"db.orders.drop()",
		`db.orders.insertMany([{status: "open"}, {status: "sent"}])`,
	)
	t.Cleanup(func() {
		_, _ = session.RunQuery(
			context.Background(), "db.dropDatabase()", dbtest.ReadEverything, nil)
	})

	return mcp.BuildTools(mcp.ToolDeps{
		AccessDeps: mcp.AccessDeps{
			Profiles: []cfg.Profile{profile},
			Config: cfg.McpConfig{
				Profiles: []string{"shop"}, Access: cfg.McpFull, RowLimit: 100,
				Timeout: cfg.DefaultMcpTimeout,
			},
			Sessions: mcp.CreateSessions(engines.CreateAdapters()),
		},
		Asker: mcp.CreateAsker(func(string) {}),
		Plans: mcp.CreatePlanTokens(),
	})
}

// A read has nothing to take back, so it carries no undo note. A server without
// transactions would otherwise explain the undo of every statement it ran.
func TestAMongoReadCarriesNoUndoNote(t *testing.T) {
	tools := buildMongoTools(t)

	answered := runTool(t, tools, "run_query", map[string]any{
		"profile": "shop", "sql": `db.orders.find({status: "open"})`,
	})
	if held, present := answered["undo_reason"]; present {
		t.Errorf("the read answered the undo note %v", held)
	}
	if answered["error"] != nil {
		t.Fatalf("the read answered %v", answered["error"])
	}
}

// A write on a server without transactions keeps no undo, and the note says why.
func TestAMongoWriteSaysWhyItKeptNoUndo(t *testing.T) {
	tools := buildMongoTools(t)

	answered := runTool(t, tools, "run_query", map[string]any{
		"profile": "shop",
		"sql":     `db.orders.updateOne({status: "open"}, {$set: {status: "sent"}})`,
	})
	if answered["error"] != nil {
		t.Fatalf("the write answered %v", answered["error"])
	}
	if answered["undo_reason"] == nil {
		t.Error("the write answered no undo note")
	}
}
