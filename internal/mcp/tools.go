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
const profileField = "The connection profile from list_profiles."

// runQueryToolName is the tool the plan token belongs to.
const runQueryToolName = "run_query"

// planTokenField is the run_query token description for clients without confirmation dialogs.
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
	// Asker requests confirmation through the MCP client when required.
	Asker *Asker
	// Plans is the token store for agent-confirmed writes.
	Plans *PlanTokens
	// RecordQuery writes the statement into the history the screens read.
	RecordQuery func(entry hist.HistoryEntry)
}

// BuildTools returns profile discovery and database tools.
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
		Description: "List connections available through MCP: the `profile` name, engine, " +
			"server, and access level. Call this before any other tool.",
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
			// Remove server arguments before tool validation.
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
			// Allow concurrent calls during database operations.
			releaseReader(ctx)
			connection, err := OpenNamedConnection(ctx, deps.AccessDeps, profile)
			if err != nil {
				return nil, err
			}
			// Bind only the requested tool.
			return definition.Call(ctx, agent.ToolDeps{
				Session: connection.Session,
				Tables:  connection.Tables,
				Runner:  buildRunner(deps, profile, connection, token),
			}, asked), nil
		},
	}
}

// buildRunner configures write planning, confirmation, timeouts, and history.
func buildRunner(
	deps ToolDeps, profile cfg.Profile, connection *Connection, token string,
) agent.StatementRunner {
	session := connection.Session
	// Keep the approved undo plan until execution.
	held := &plannedWrite{}
	runner := agent.StatementRunner{
		RowLimit: deps.Config.RowLimit,
		AskToRun: func(
			ctx context.Context, risk statement.WriteRisk, statements []string,
		) agent.RunPermission {
			permission, undo := askAgentToRun(
				ctx, deps, profile, connection, token, risk, statements)
			held.undo = undo
			held.writes = risk != statement.RiskNone
			return permission
		},
		MeasureWrite: func(ctx context.Context, sql string) (agent.MeasuredWrite, bool) {
			return measureForAgent(ctx, deps, profile, connection, sql)
		},
		RunStatement: func(
			ctx context.Context, sql string, rowLimit int,
		) (agent.StatementAnswer, error) {
			if !held.writes {
				return runRead(ctx, session, deps, sql, rowLimit)
			}
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

// plannedWrite is the approved undo plan and statement write classification.
type plannedWrite struct {
	undo writeplan.UndoPlan
	// writes is false for statements classified as read-only.
	writes bool
}

// runRead runs a statement classified as read-only without undo capture.
func runRead(
	ctx context.Context, session db.Session, deps ToolDeps, sql string, rowLimit int,
) (agent.StatementAnswer, error) {
	result, err := agent.RunStatementWithin(ctx, session, deps.Config.Timeout,
		func(limited context.Context) (db.QueryResult, error) {
			return session.RunQuery(limited, sql, rowLimit, nil)
		})
	if err != nil {
		return agent.StatementAnswer{}, err
	}
	return agent.StatementAnswer{Result: result}, nil
}

// runWriteWithUndo runs a statement with transactional undo capture when available.
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

	// Tokens apply only when the client cannot show a required confirmation dialog.
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
		Refusal: "confirmation was not received; the statement did not run",
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

// describeUnaskableRefusal describes a required confirmation that the client cannot request.
func describeUnaskableRefusal(profile cfg.Profile, risk statement.WriteRisk) string {
	opening := fmt.Sprintf("%q requires confirmation for a statement that %s; this client cannot show a confirmation dialog",
		profile.Name, statement.DescribeRisk(risk, 1))
	if profile.ConfirmWrites == cfg.ConfirmAgent {
		return opening + "; call plan_write, show the plan to the user, and obtain their approval. " +
			"Then send the returned token as `plan_token`"
	}
	return opening + `; confirm_writes = "off" on the profile permits execution without confirmation`
}

// measureForAgent measures a write and issues a token when agent confirmation is enabled without client dialogs.
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
	// Issue tokens only for agent confirmation without client dialogs.
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

// buildAgentWritePlan measures one write for confirmation and undo capture.
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
