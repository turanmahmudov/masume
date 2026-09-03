package ai

import (
	"context"
	"strconv"

	"github.com/turanmahmudov/masume/internal/core"
)

// One question and its answer: the client sends the question, runs the calls the model
// requests, and sends the results back, until the model writes a reply or reaches the step
// limit.

// MaxToolSteps is the number of calls after which a run must answer with the data it has.
const MaxToolSteps = 25

// RunHooks are the callbacks the caller gets during a run.
type RunHooks struct {
	// StartTextBlock reports that a block of text follows an earlier block.
	StartTextBlock func()
	AppendText     func(delta string)
	// StartToolStep reports the call that starts, and FinishToolStep reports its end.
	StartToolStep  func(label string)
	FinishToolStep func()
	// CallTool runs one call of the catalogue and returns its result as JSON text.
	CallTool func(ctx context.Context, name string, input map[string]any) string
	LogEvent func(message string)
}

// RunResult is the result of a whole run.
type RunResult struct {
	// ReceivedChars is the number of characters received, so an empty reply can be
	// reported.
	ReceivedChars int
	FinishReason  string
	Usage         Usage
}

// RunChat sends the question, runs the calls the model requests, and returns after the
// model writes a reply.
func RunChat(
	ctx context.Context, model Model, request Request, hooks RunHooks,
) (RunResult, error) {
	result := RunResult{FinishReason: FinishUnknown}
	messages := append([]Message{}, request.Messages...)

	for range MaxToolSteps {
		asked := request
		asked.Messages = messages

		answer, err := model.Stream(ctx, asked, func(event Event) {
			// A run the caller stopped writes nothing more. The stop cancels ctx.
			if ctx.Err() != nil {
				return
			}
			switch event.Kind {
			case EventTextStart:
				hooks.StartTextBlock()
			case EventTextDelta:
				result.ReceivedChars += len([]rune(event.Text))
				hooks.AppendText(event.Text)
			}
		})
		result.Usage = addUsage(result.Usage, answer.Usage)
		if err != nil {
			return result, err
		}
		result.FinishReason = answer.FinishReason
		if len(answer.Calls) == 0 || ctx.Err() != nil {
			return result, nil
		}

		answers := make([]ToolAnswer, 0, len(answer.Calls))
		for _, call := range answer.Calls {
			answers = append(answers, runOneCall(ctx, call, hooks))
			if ctx.Err() != nil {
				return result, nil
			}
		}
		messages = append(messages,
			Message{Role: RoleAssistant, Text: answer.Text, Calls: answer.Calls},
			Message{Role: RoleUser, Answers: answers})
	}
	return result, nil
}

// runOneCall runs one call and reports it as a step of the reply.
func runOneCall(ctx context.Context, call ToolCall, hooks RunHooks) ToolAnswer {
	hooks.StartToolStep(DescribeToolActivity(call.Name, call.Input))
	hooks.LogEvent("> tool " + call.Name + " " + core.CutForLog(call.Arguments))

	output := hooks.CallTool(ctx, call.Name, call.Input)
	hooks.FinishToolStep()
	hooks.LogEvent("< tool " + call.Name + " " + core.CutForLog(output))
	return ToolAnswer{CallID: call.ID, Name: call.Name, Output: output}
}

// addUsage adds the token count of a step to the total of the run.
func addUsage(total, step Usage) Usage {
	return Usage{
		InputTokens:       total.InputTokens + step.InputTokens,
		OutputTokens:      total.OutputTokens + step.OutputTokens,
		CachedInputTokens: total.CachedInputTokens + step.CachedInputTokens,
	}
}

// FindEmptyReplyProblem returns the reason a run wrote no reply, and an empty string if it
// wrote one.
func FindEmptyReplyProblem(received int, finishReason string) string {
	if received > 0 {
		return ""
	}
	// A run that stops at a tool call without text reached the step limit.
	if finishReason == FinishToolCalls {
		return "ran out of turns after " + strconv.Itoa(MaxToolSteps) + " tool calls, before " +
			"it could answer; try asking again, or narrow the question"
	}
	// No error and no text: the model stopped early, often at a content filter.
	if finishReason != FinishStop {
		return "the model answered nothing (" + finishReason + ")"
	}
	return ""
}
