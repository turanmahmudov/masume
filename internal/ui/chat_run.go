package ui

import (
	"context"
	"strconv"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/turanmahmudov/masume/internal/agent"
	"github.com/turanmahmudov/masume/internal/ai"
	"github.com/turanmahmudov/masume/internal/app"
	"github.com/turanmahmudov/masume/internal/cfg"
	"github.com/turanmahmudov/masume/internal/db"
	"github.com/turanmahmudov/masume/internal/hist"
	"github.com/turanmahmudov/masume/internal/query/statement"
)

// One question of the chat, asked and answered. The run happens in a goroutine, and reports
// what arrives through a channel the screen reads one event at a time, because the state of the
// client belongs to the one goroutine that draws it.

// chatEventsMsg is everything a run reported since the screen last read it.
type chatEventsMsg struct {
	ConnectionID int
	Run          int
	Events       []app.ChatEvent
	// Source is the channel these arrived on, so the next wait reads the same run to its
	// end and never the channel of a later one.
	Source chan app.ChatEvent
}

// conversationKeptMsg returns the id the file gave the conversation, or why it kept none.
type conversationKeptMsg struct {
	ConnectionID int
	ID           int64
	Problem      string
}

// chatClosedMsg says the run ended and reported everything it had.
type chatClosedMsg struct {
	ConnectionID int
	Run          int
}

// waitForChatEvents returns everything the run has to report. It waits for the first thing
// and then takes what is already waiting, because a reply arrives a few characters at a time
// and a frame for each of them would leave the screen no time to draw anything else, so the
// wheel of the wait would stop turning.
func waitForChatEvents(id, run int, events chan app.ChatEvent) tea.Cmd {
	return func() tea.Msg {
		first, open := <-events
		if !open {
			return chatClosedMsg{ConnectionID: id, Run: run}
		}
		read := make([]app.ChatEvent, 0, app.ChatEventRoom)
		read = append(read, first)
		reported := func() tea.Msg {
			return chatEventsMsg{ConnectionID: id, Run: run, Events: read, Source: events}
		}
		for len(read) < app.ChatEventRoom {
			select {
			case next, stillOpen := <-events:
				if !stillOpen {
					// The close is reported by the wait that follows, which reads it at once.
					return reported()
				}
				read = append(read, next)
			default:
				return reported()
			}
		}
		return reported()
	}
}

// sendChatMessage asks the model, and reports why it cannot be asked.
func (model *Model) sendChatMessage(
	connection *app.Connection, tab *app.Tab, prompt string,
) (tea.Model, tea.Cmd) {
	chat := connection.Chat
	asked := strings.TrimSpace(prompt)
	if asked == "" || chat.IsStreaming() {
		return model, nil
	}

	if !ai.HasCredentials(model.ai, model.aiProvider) {
		chat.Fail(ai.DescribeMissingKey(model.ai, model.aiProvider))
		return model, nil
	}
	// The cache key names the profile, so the turns of one connection share a prefix.
	cacheKey := "masume/" + connection.Profile().Name
	held, err := ai.OpenModel(model.ai, model.aiProvider, cacheKey)
	if err != nil {
		chat.Fail(db.DescribeError(err))
		return model, nil
	}

	editor := ai.EditorContext{SQL: tab.Editor.Text, LastError: findLastRunError(tab)}
	history := chat.StartTurn(asked, ai.DescribeEditorContext(editor))

	id := model.ActiveID()
	ctx, stop := context.WithCancel(context.Background())
	run, events := chat.Begin(stop)
	ai.LogEvent("> " + held.Describe() + ": " + asked)

	go runChatReply(ctx, chatRun{
		model:    held,
		request:  model.buildChatRequest(connection, history),
		deps:     model.buildChatToolDeps(connection, run, events),
		run:      run,
		events:   events,
		provider: held.Describe(),
	})
	return model, waitForChatEvents(id, run, events)
}

// findLastRunError returns the message of the server from the last run of this tab, where it
// failed.
func findLastRunError(tab *app.Tab) string {
	state := tab.Results.State()
	if state.Kind == app.QueryFailed {
		return state.Message
	}
	return ""
}

// buildChatRequest writes what the model is told: the prompt of this connection, and the turns
// said so far.
func (model *Model) buildChatRequest(
	connection *app.Connection, history []app.ChatMessage,
) ai.Request {
	session := connection.Session
	profile := connection.Profile()
	messages := make([]ai.Message, 0, len(history))
	for _, message := range history {
		written := message.Content
		if message.Context != "" {
			written += "\n\n" + message.Context
		}
		messages = append(messages, ai.Message{Role: message.Role, Text: written})
	}

	return ai.Request{
		System: ai.BuildChatSystemPrompt(ai.ChatPromptSource{
			DialectName:   string(profile.Engine),
			Language:      readStatementLanguage(session),
			DefaultSchema: session.Describe().DefaultSchema,
			Tables:        connection.Catalog.Tables,
			Instructions:  profile.AiInstructions,
		}),
		Messages: messages,
		Tools:    ai.BuildToolSchemas(agent.Definitions()),
	}
}

// readStatementLanguage returns what a statement of this connection is written in, which
// the chat is told before anything else.
func readStatementLanguage(session db.SessionInfo) ai.StatementLanguage {
	dialect := session.Dialect()
	return ai.StatementLanguage{
		Name: dialect.StatementLanguage, FenceTag: dialect.FenceTag,
		Example: dialect.StatementExample,
	}
}

// buildChatToolDeps returns what the tools of the chat reach. The relations are read once, so
// the goroutine of the run touches nothing the screen writes.
func (model *Model) buildChatToolDeps(
	connection *app.Connection, run int, events chan app.ChatEvent,
) agent.ToolDeps {
	session := connection.Session
	tables := append([]db.TableRef{}, connection.Catalog.Tables...)
	profileName := connection.Profile().Name
	log := model.log

	return agent.ToolDeps{
		Session: session,
		Tables:  func() []db.TableRef { return tables },
		MarkTableDescribed: func(table db.TableRef, detail db.TableDetail) {
			events <- app.ChatEvent{
				Run: run, Kind: app.ChatTableRead, Table: table, Detail: detail,
			}
		},
		Runner: agent.StatementRunner{
			// A chat returns with figures, not with a page of rows.
			RowLimit: connection.Profile().PageSize,
			AskToRun: func(
				ctx context.Context, risk statement.WriteRisk, statements []string,
			) string {
				return askChatToRun(ctx, run, events, connection.Profile(), risk, statements)
			},
			// The statement gets a limit, because the panel holds the keyboard while it runs.
			RunStatement: func(
				ctx context.Context, sql string, rowLimit int,
			) (db.QueryResult, error) {
				return agent.RunStatementWithin(ctx, session, model.ai.StatementTimeout,
					func(running context.Context) (db.QueryResult, error) {
						return session.RunQuery(running, sql, rowLimit, nil)
					})
			},
			ReportRun: func(report agent.StatementReport) {
				_ = log.Record(hist.HistoryEntry{
					ProfileName: profileName, SQL: report.SQL, RanAt: report.RanAt,
					Elapsed: report.Elapsed, RowCount: report.RowCount,
					HasRowCount: report.HasRowCount, ErrorMessage: report.ErrorMessage,
				})
			},
		},
	}
}

// askChatToRun asks the user whether a statement may run, and waits for the answer. The panel
// asks it, not a card: only one overlay is drawn at a time, and a card would cover the
// conversation.
func askChatToRun(
	ctx context.Context, run int, events chan app.ChatEvent,
	profile cfg.Profile, risk statement.WriteRisk, statements []string,
) string {
	summary := statement.DescribeRisk(risk, len(statements)) + " on " + string(profile.Environment)
	ai.LogEvent("> asking to run (" + summary + "): " + strings.Join(statements, "; "))

	allowed := make(chan bool, 1)
	events <- app.ChatEvent{
		Run: run, Kind: app.ChatRunAsked, Allowed: allowed,
		Ask: app.PendingRun{Summary: summary, SQL: strings.Join(statements, ";\n")},
	}

	select {
	case confirmed := <-allowed:
		if confirmed {
			ai.LogEvent("< run allowed")
			return ""
		}
		ai.LogEvent("< run refused")
		return "the user did not allow this statement to run; nothing ran"
	case <-ctx.Done():
		return "the user did not allow this statement to run; nothing ran"
	}
}

// chatRun is everything one run of the chat needs.
type chatRun struct {
	model    ai.Model
	request  ai.Request
	deps     agent.ToolDeps
	run      int
	events   chan app.ChatEvent
	provider string
}

// runChatReply asks the model, runs what it asks for, and reports everything through the
// channel. It closes the channel when it is done, whatever happened.
func runChatReply(ctx context.Context, held chatRun) {
	defer close(held.events)

	definitions := agent.Definitions()
	result, err := ai.RunChat(ctx, held.model, held.request, ai.RunHooks{
		StartTextBlock: func() {
			held.events <- app.ChatEvent{Run: held.run, Kind: app.ChatTextStarted}
		},
		AppendText: func(delta string) {
			held.events <- app.ChatEvent{
				Run: held.run, Kind: app.ChatTextArrived, Text: delta,
			}
		},
		StartToolStep: func(label string) {
			held.events <- app.ChatEvent{
				Run: held.run, Kind: app.ChatStepStarted, Text: label,
			}
		},
		FinishToolStep: func() {
			held.events <- app.ChatEvent{Run: held.run, Kind: app.ChatStepFinished}
		},
		CallTool: func(
			callCtx context.Context, name string, input map[string]any,
		) string {
			return ai.CallToolDefinition(callCtx, definitions, held.deps, name, input)
		},
		LogEvent: ai.LogEvent,
	})

	ended := app.ChatEvent{
		Run: held.run, Kind: app.ChatEnded,
		Usage: app.ChatUsage{
			InputTokens: result.Usage.InputTokens, OutputTokens: result.Usage.OutputTokens,
			CachedInputTokens: result.Usage.CachedInputTokens,
		},
	}
	switch {
	case err != nil && ctx.Err() == nil:
		ended.Problem = db.DescribeError(err)
		ai.LogEvent("! " + ended.Problem)
	case err == nil:
		ai.LogEvent("< " + result.FinishReason + " · " +
			strconv.Itoa(result.ReceivedChars) + " chars")
		ended.Problem = ai.FindEmptyReplyProblem(result.ReceivedChars, result.FinishReason)
	}
	if usage := ended.Usage; usage.InputTokens > 0 || usage.OutputTokens > 0 {
		ai.LogEvent("< usage: " + strconv.Itoa(usage.InputTokens) + " in (" +
			strconv.Itoa(usage.CachedInputTokens) + " cached), " + strconv.Itoa(usage.OutputTokens) + " out")
	}
	held.events <- ended
}
