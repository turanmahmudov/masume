package ai

import (
	"context"
	"strconv"

	"github.com/turanmahmudov/masume/internal/core"
)

// One question, answered: the model is asked, what it asks for is run, and it is asked again
// with the answers, until it writes a reply or runs out of turns.

// MaxToolSteps is the number of calls after which a run must answer with what it has.
const MaxToolSteps = 25

// RunHooks is what the caller is told while a run happens.
type RunHooks struct {
	// StartTextBlock says a block of text follows an earlier one.
	StartTextBlock func()
	AppendText     func(delta string)
	// StartToolStep names the call that runs now, and FinishToolStep keeps it as done.
	StartToolStep  func(label string)
	FinishToolStep func()
	// CallTool runs one call of the catalogue and returns what it said, as JSON text.
	CallTool func(ctx context.Context, name string, input map[string]any) string
	LogEvent func(message string)
}

// RunResult is what a whole run answered.
type RunResult struct {
	// ReceivedChars is how much text arrived, so a reply that said nothing can be reported.
	ReceivedChars int
	FinishReason  string
	Usage         Usage
}

// RunChat asks the model, runs what it asks for, and returns once it wrote a reply.
func RunChat(
	ctx context.Context, model Model, request Request, hooks RunHooks,
) (RunResult, error) {
	result := RunResult{FinishReason: FinishUnknown}
	messages := append([]Message{}, request.Messages...)

	for range MaxToolSteps {
		asked := request
		asked.Messages = messages

		answer, err := model.Stream(ctx, asked, func(event Event) {
			// A run the caller stopped writes nothing more, and the stop is the end of ctx.
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

// runOneCall runs one call, and reports it as a step of the reply.
func runOneCall(ctx context.Context, call ToolCall, hooks RunHooks) ToolAnswer {
	hooks.StartToolStep(DescribeToolActivity(call.Name, call.Input))
	hooks.LogEvent("> tool " + call.Name + " " + core.CutForLog(call.Arguments))

	output := hooks.CallTool(ctx, call.Name, call.Input)
	hooks.FinishToolStep()
	hooks.LogEvent("< tool " + call.Name + " " + core.CutForLog(output))
	return ToolAnswer{CallID: call.ID, Name: call.Name, Output: output}
}

// addUsage sums what a step spent into the total of the run.
func addUsage(total, step Usage) Usage {
	return Usage{
		InputTokens:       total.InputTokens + step.InputTokens,
		OutputTokens:      total.OutputTokens + step.OutputTokens,
		CachedInputTokens: total.CachedInputTokens + step.CachedInputTokens,
	}
}

// FindEmptyReplyProblem returns why a run wrote nothing, or nothing where it wrote a reply.
func FindEmptyReplyProblem(received int, finishReason string) string {
	if received > 0 {
		return ""
	}
	// A run that stops on a tool call and says nothing ran out of steps.
	if finishReason == FinishToolCalls {
		return "ran out of turns after " + strconv.Itoa(MaxToolSteps) + " tool calls, before " +
			"it could answer; try asking again, or narrow the question"
	}
	// No error and no text: the model stopped short, often on a content filter.
	if finishReason != FinishStop {
		return "the model answered nothing (" + finishReason + ")"
	}
	return ""
}
