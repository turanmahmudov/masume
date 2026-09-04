package mcp

import (
	"context"
	"fmt"
	"strings"

	"github.com/turanmahmudov/masume/internal/agent"
	"github.com/turanmahmudov/masume/internal/cfg"
	"github.com/turanmahmudov/masume/internal/db"
	"github.com/turanmahmudov/masume/internal/hist"
	"github.com/turanmahmudov/masume/internal/query/language"
	"github.com/turanmahmudov/masume/internal/query/statement"
	"github.com/turanmahmudov/masume/internal/writeplan"
)

// Every call an agent can make: the profiles, and the tools of one connection.

// profileField is the argument the server adds to every tool of a connection.
const profileField = "The name of the connection to work on, as list_profiles reports it."

// runQueryToolName is the tool the plan token belongs to.
const runQueryToolName = "run_query"

// planTokenField is the argument the server adds to run_query, for a client that cannot
// show a question of its own.
const planTokenField = "The `token` of a plan_write answer for this exact statement. " +
	"Send it only after the user read the plan and agreed to run the statement."

// Tool is one tool in the form of the protocol, with its input schema.
type Tool struct {
	Name        string
	Description string
	InputSchema map[string]any
	Call        func(ctx context.Context, input map[string]any) (any, error)
}

// ToolDeps holds the resources the tools of the server use.
type ToolDeps struct {
	AccessDeps
	// Asker asks the user through the client of the agent before an expensive write.
	Asker *Asker
	// Plans holds the tokens of the writes an agent measured, for a client that cannot be
	// asked and leaves the agent to ask the user itself.
	Plans *PlanTokens
	// RecordQuery writes the statement into the history the screens read.
	RecordQuery func(entry hist.HistoryEntry)
}

// BuildTools returns every call an agent can make: the profiles, and the tools of one
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

// describeProfileForAgent returns one connection in the form the agent reads.
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

// buildConnectionTool binds one tool of the chat to the connection of the call.
func buildConnectionTool(deps ToolDeps, definition agent.ToolDefinition) Tool {
	schema := definition.InputSchema
	if definition.Name == runQueryToolName {
		schema = agent.ExtendSchemaOptionally(schema, "plan_token", planTokenField)
	}
	if deps.ScopedProfile == "" {
		schema = agent.ExtendSchema(schema, "profile", profileField)
	}

	return Tool{
		Name: definition.Name, Description: definition.Description, InputSchema: schema,
		Call: func(ctx context.Context, input map[string]any) (any, error) {
			// `profile` and `plan_token` are arguments of the server and not of the
			// tool, and the schema of the tool rejects an unknown field. So they are
			// read and removed.
			asked := map[string]any{}
			for name, value := range input {
				if name != "profile" && name != "plan_token" {
					asked[name] = value
				}
			}
			token, _ := input["plan_token"].(string)
			profile, err := GetNamedProfile(deps.AccessDeps, input["profile"])
			if err != nil {
				return nil, err
			}
			// From here the call reaches a server, and the next call can start in
			// parallel.
			releaseReader(ctx)
			connection, err := OpenNamedConnection(ctx, deps.AccessDeps, profile)
			if err != nil {
				return nil, err
			}
			// Only this tool for this connection. The whole catalogue would build
			// eight tools that nobody calls.
			return definition.Call(ctx, agent.ToolDeps{
				Session: connection.Session,
				Tables:  connection.Tables,
				Runner:  buildRunner(deps, profile, connection, token),
			}, asked), nil
		},
	}
}

// buildRunner returns the runner of a statement for an agent: with a plan of what a write
// does, a confirmation, a time limit, and a history entry.
func buildRunner(
	deps ToolDeps, profile cfg.Profile, connection *Connection, token string,
) agent.StatementRunner {
	session := connection.Session
	// The undo of the write the user allowed, carried from the question to the run that
	// reads it inside the transaction of the write.
	held := &plannedWrite{}
	runner := agent.StatementRunner{
		RowLimit: deps.Config.RowLimit,
		AskToRun: func(
			ctx context.Context, risk statement.WriteRisk, statements []string,
		) agent.RunPermission {
			permission, undo := askAgentToRun(
				ctx, deps, profile, connection, token, risk, statements)
			held.undo = undo
			return permission
		},
		MeasureWrite: func(ctx context.Context, sql string) (agent.MeasuredWrite, bool) {
			return measureForAgent(ctx, deps, profile, connection, sql)
		},
		RunStatement: func(
			ctx context.Context, sql string, rowLimit int,
		) (agent.StatementAnswer, error) {
			return runWriteWithUndo(ctx, session, held.undo, func(
				running context.Context,
			) (db.QueryResult, error) {
				return agent.RunStatementWithin(running, session, deps.Config.Timeout,
					func(limited context.Context) (db.QueryResult, error) {
						return session.RunQuery(limited, sql, rowLimit, nil)
					})
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

// askAgentToRun decides whether a statement can run: first the access level of the
// connection, then the plan of what the write does, and then the `confirm_writes` setting of
// the profile, with the same question the screens use. The client of the agent shows the
// question if it can. If it cannot, the statement does not run.
// plannedWrite carries the undo of one write from the question to the run.
type plannedWrite struct{ undo writeplan.UndoPlan }

// runWriteWithUndo runs the statement, and reads its undo inside the same transaction where
// the plan of the write keeps one.
func runWriteWithUndo(
	ctx context.Context, session db.Session, plan writeplan.UndoPlan,
	run func(context.Context) (db.QueryResult, error),
) (agent.StatementAnswer, error) {
	result, undo, err := writeplan.RunWithUndo(ctx, session, plan, run)
	if err != nil {
		return agent.StatementAnswer{}, err
	}
	return agent.StatementAnswer{
		Result: result, Undo: undo.Display, UndoReason: undo.Reason,
	}, nil
}

func askAgentToRun(
	ctx context.Context, deps ToolDeps, profile cfg.Profile, connection *Connection,
	token string, risk statement.WriteRisk, statements []string,
) (agent.RunPermission, writeplan.UndoPlan) {
	access := ResolveProfileAccess(deps.Config, profile)
	if refusal := FindAccessRefusal(access, risk); refusal != "" {
		return agent.RunPermission{Refusal: refusal}, writeplan.UndoPlan{}
	}

	plan, measured := buildAgentWritePlan(ctx, profile, connection, risk, statements)
	allowed := agent.RunPermission{}
	if !db.NeedsConfirmation(profile.ConfirmWrites, risk) {
		return allowed, plan.Undo
	}

	// A client that can be asked is always asked. A token is what an agent brings back
	// where there is no other way to reach the user, and never a way around the question.
	if !deps.Asker.CanAsk() {
		if takesPlanToken(deps, profile, token, statements) {
			return allowed, plan.Undo
		}
		return agent.RunPermission{
			Refusal: describeUnaskableRefusal(profile, risk),
		}, writeplan.UndoPlan{}
	}

	question := statement.BuildConfirmation(
		profile.Name, string(profile.Environment), risk, statements)
	if measured {
		question.Body += "\n\n" + strings.Join(writeplan.DescribeLines(plan), "\n")
	}
	if deps.Asker.AskConfirmation(ctx, question.Title, question.Body) {
		return allowed, plan.Undo
	}
	return agent.RunPermission{
		Refusal: "you were asked to confirm this statement, and did not; nothing ran",
	}, writeplan.UndoPlan{}
}

// takesPlanToken is true where this write carries the token of a plan the user read.
func takesPlanToken(
	deps ToolDeps, profile cfg.Profile, token string, statements []string,
) bool {
	if profile.ConfirmWrites != cfg.ConfirmAgent || len(statements) != 1 {
		return false
	}
	if !deps.Plans.Take(token, profile.Name, statements[0]) {
		return false
	}
	LogEvent("= plan token taken for " + profile.Name)
	return true
}

// describeUnaskableRefusal says why a write did not run on a client that cannot show a
// question, and what to do about it.
func describeUnaskableRefusal(profile cfg.Profile, risk statement.WriteRisk) string {
	opening := fmt.Sprintf("%q confirms a statement that %s, and this client cannot ask you",
		profile.Name, statement.DescribeRisk(risk, 1))
	if profile.ConfirmWrites == cfg.ConfirmAgent {
		return opening + "; call plan_write for this statement, show the plan to the user, " +
			"and send the token it answers with as `plan_token`"
	}
	return opening + `; set confirm_writes = "off" on the profile to run it unasked`
}

// measureForAgent measures one write for the plan_write tool, and issues the token that
// runs it where the client of the agent cannot be asked.
func measureForAgent(
	ctx context.Context, deps ToolDeps, profile cfg.Profile,
	connection *Connection, sql string,
) (agent.MeasuredWrite, bool) {
	statements := connection.Session.Language().SplitStatements(sql)
	risk := language.ResolveBatchRisk(statements, connection.Session.Language())
	plan, measured := buildAgentWritePlan(ctx, profile, connection, risk, statements)
	if !measured {
		return agent.MeasuredWrite{}, false
	}

	written := describeMeasuredPlan(plan)
	// A token is issued only where the client cannot be asked and the profile lets an
	// agent carry the answer. A client that shows a dialog gets the dialog.
	if profile.ConfirmWrites == cfg.ConfirmAgent && !deps.Asker.CanAsk() {
		written.Token = deps.Plans.Issue(profile.Name, statements[0])
	}
	return written, true
}

// describeMeasuredPlan returns the plan in the form a tool answers with.
func describeMeasuredPlan(plan writeplan.Plan) agent.MeasuredWrite {
	written := agent.MeasuredWrite{
		Lines: writeplan.DescribeLines(plan),
		Table: plan.Table.Schema + "." + plan.Table.Name,
		Rows:  plan.Rows, HasRows: plan.HasRows,
		Total: plan.Total, HasTotal: plan.HasTotal,
		Columns:    plan.Columns,
		UndoRows:   plan.Undo.Rows,
		UndoReason: plan.Undo.Reason,
	}
	for _, cascade := range plan.Cascades {
		written.Cascades = append(written.Cascades, writeplan.DescribeCascade(cascade))
	}
	for _, blocker := range plan.Blockers {
		written.Blocked = append(written.Blocked, writeplan.DescribeBlocker(blocker))
	}
	return written
}

// buildAgentWritePlan measures the write the agent asked to run, so the person who answers
// the question reads what it lands on and the agent is handed the undo.
func buildAgentWritePlan(
	ctx context.Context, profile cfg.Profile, connection *Connection,
	risk statement.WriteRisk, statements []string,
) (writeplan.Plan, bool) {
	if !writeplan.Measures(profile, connection.Session.Capabilities(),
		risk, len(statements)) {
		return writeplan.Plan{}, false
	}
	return writeplan.Build(ctx, connection.Session, writeplan.Request{
		SQL: statements[0], Tables: connection.Tables(), Mode: profile.WritePlan,
		UndoRows: profile.UndoRows,
		InTransaction: connection.Session.ReadTransactionState() ==
			db.TransactionOpen,
	})
}
