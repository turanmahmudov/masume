package mcp

import (
	"context"
	"fmt"

	"github.com/turanmahmudov/masume/internal/agent"
	"github.com/turanmahmudov/masume/internal/cfg"
	"github.com/turanmahmudov/masume/internal/db"
	"github.com/turanmahmudov/masume/internal/hist"
	"github.com/turanmahmudov/masume/internal/query/statement"
)

// Everything an agent may call: the profiles, and the tools of one connection.

// profileField describes the argument the server adds to every tool of a connection.
const profileField = "The name of the connection to work on, as list_profiles reports it."

// Tool is one tool as the protocol describes it, with its input schema.
type Tool struct {
	Name        string
	Description string
	InputSchema map[string]any
	Call        func(ctx context.Context, input map[string]any) (any, error)
}

// ToolDeps is what the tools of the server reach.
type ToolDeps struct {
	AccessDeps
	// Asker asks the user through the client of the agent before a costly write.
	Asker *Asker
	// RecordQuery writes what ran into the history the screens read.
	RecordQuery func(entry hist.HistoryEntry)
}

// BuildTools returns everything an agent may call: the profiles, and the tools of one
// connection.
func BuildTools(deps ToolDeps) []Tool {
	tools := []Tool{buildListProfilesTool(deps)}
	for _, definition := range agent.Definitions() {
		tools = append(tools, buildConnectionTool(deps, definition))
	}
	return tools
}

func buildListProfilesTool(deps ToolDeps) Tool {
	return Tool{
		Name: "list_profiles",
		Description: "List the database connections this client opens to an agent: the name " +
			"to pass as `profile`, the engine, the server it reaches, and what may be run on " +
			"it. Call this first, before any other tool.",
		InputSchema: agent.BuildEmptySchema(),
		Call: func(_ context.Context, _ map[string]any) (any, error) {
			open := ListOpenProfiles(deps.AccessDeps)
			if len(open) == 0 {
				return map[string]any{
					"profiles": []any{}, "note": DescribeNoOpenProfiles(deps.Config),
				}, nil
			}
			described := make([]map[string]any, 0, len(open))
			for _, profile := range open {
				described = append(described, describeProfileForAgent(deps.Config, profile))
			}
			return map[string]any{"profiles": described}, nil
		},
	}
}

// describeProfileForAgent writes one connection as the agent reads it.
func describeProfileForAgent(config cfg.McpConfig, profile cfg.Profile) map[string]any {
	var description any
	if profile.Description != "" {
		description = profile.Description
	}
	described := map[string]any{
		"name":        profile.Name,
		"engine":      string(profile.Engine),
		"target":      cfg.DescribeProfileTarget(profile),
		"database":    profile.Database,
		"environment": string(profile.Environment),
		"access":      string(ResolveProfileAccess(config, profile)),
		"description": description,
	}
	if unreachable := FindUnreachableReason(profile); unreachable != "" {
		described["unreachable"] = unreachable
	}
	return described
}

// buildConnectionTool binds one tool of the chat to the connection a call names.
func buildConnectionTool(deps ToolDeps, definition agent.ToolDefinition) Tool {
	schema := definition.InputSchema
	if deps.ScopedProfile == "" {
		schema = agent.ExtendSchema(schema, "profile", profileField)
	}

	return Tool{
		Name: definition.Name, Description: definition.Description, InputSchema: schema,
		Call: func(ctx context.Context, input map[string]any) (any, error) {
			// `profile` is an argument of the server, not of the tool, and the schema of the
			// tool refuses an unknown field. So it is read and removed.
			asked := map[string]any{}
			for name, value := range input {
				if name != "profile" {
					asked[name] = value
				}
			}
			profile, err := GetNamedProfile(deps.AccessDeps, input["profile"])
			if err != nil {
				return nil, err
			}
			// From here the call reaches a server, and the next call may start beside it.
			releaseReader(ctx)
			connection, err := OpenNamedConnection(ctx, deps.AccessDeps, profile)
			if err != nil {
				return nil, err
			}
			// Only this tool for this connection. Building the whole catalog would create
			// eight tools nobody calls.
			return definition.Call(ctx, agent.ToolDeps{
				Session: connection.Session,
				Tables:  connection.Tables,
				Runner:  buildRunner(deps, profile, connection.Session),
			}, asked), nil
		},
	}
}

type runnableSession interface {
	db.QueryRunner
	agent.StoppableSession
}

// buildRunner returns how a statement runs for an agent: confirmed, timed, and stored.
func buildRunner(
	deps ToolDeps, profile cfg.Profile, session runnableSession,
) agent.StatementRunner {
	runner := agent.StatementRunner{
		RowLimit: deps.Config.RowLimit,
		AskToRun: func(
			ctx context.Context, risk statement.WriteRisk, statements []string,
		) string {
			return findRunRefusal(ctx, deps, profile, risk, statements)
		},
		RunStatement: func(
			ctx context.Context, sql string, rowLimit int,
		) (db.QueryResult, error) {
			return agent.RunStatementWithin(ctx, session, deps.Config.Timeout,
				func(running context.Context) (db.QueryResult, error) {
					return session.RunQuery(running, sql, rowLimit, nil)
				})
		},
	}
	if deps.RecordQuery != nil {
		runner.ReportRun = func(report agent.StatementReport) {
			deps.RecordQuery(hist.HistoryEntry{
				ProfileName:  profile.Name,
				SQL:          report.SQL,
				RanAt:        report.RanAt,
				Elapsed:      report.Elapsed,
				RowCount:     report.RowCount,
				HasRowCount:  report.HasRowCount,
				ErrorMessage: report.ErrorMessage,
			})
		}
	}
	return runner
}

// findRunRefusal returns whether a statement may run: first the access level of the
// connection, then the `confirm_writes` setting of the profile, asked the way the screens ask
// it. The client of the agent shows the question if it can. If it cannot, the statement does
// not run.
func findRunRefusal(
	ctx context.Context, deps ToolDeps, profile cfg.Profile,
	risk statement.WriteRisk, statements []string,
) string {
	access := ResolveProfileAccess(deps.Config, profile)
	if refusal := FindAccessRefusal(access, risk); refusal != "" {
		return refusal
	}
	if !db.NeedsConfirmation(profile.ConfirmWrites, risk) {
		return ""
	}

	if !deps.Asker.CanAsk() {
		return fmt.Sprintf("%q confirms a statement that %s, and this client cannot ask you; "+
			`set confirm_writes = "off" on the profile to run it unasked`,
			profile.Name, statement.DescribeRisk(risk, 1))
	}

	question := statement.BuildConfirmation(
		profile.Name, string(profile.Environment), risk, statements)
	if deps.Asker.AskConfirmation(ctx, question.Title, question.Body) {
		return ""
	}
	return "you were asked to confirm this statement, and did not; nothing ran"
}
