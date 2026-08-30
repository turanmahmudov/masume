package ai

import (
	"context"
	"fmt"
	"strings"
)

// The Responses protocol of OpenAI. It caches a repeated prefix by itself, with no mark and no
// extra cost. It takes a key instead, which says which requests share a prefix. The key is
// held here, and not in the request, because no other protocol reads one.

const openaiBaseURL = "https://api.openai.com/v1"

// openaiModel is one model of this provider.
type openaiModel struct {
	model    string
	apiKey   string
	baseURL  string
	cacheKey string
}

func openOpenaiModel(model, apiKey, baseURL, cacheKey string) Model {
	if baseURL == "" {
		baseURL = openaiBaseURL
	}
	return &openaiModel{model: model, apiKey: apiKey, baseURL: baseURL, cacheKey: cacheKey}
}

func (held *openaiModel) Describe() string {
	return "openai/" + held.model
}

// The items of the input, as this protocol writes them. A turn is a role with parts, and a
// call and its answer are items of their own.
type openaiPart struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

type openaiItem struct {
	Type string `json:"type,omitempty"`
	Role string `json:"role,omitempty"`
	// Content is the parts of a turn, and the plain text of the one that opens the request.
	Content any `json:"content,omitempty"`
	// A call the model asked for, and the answer of that call.
	CallID    string `json:"call_id,omitempty"`
	Name      string `json:"name,omitempty"`
	Arguments string `json:"arguments,omitempty"`
	Output    string `json:"output,omitempty"`
}

type openaiTool struct {
	Type        string         `json:"type"`
	Name        string         `json:"name"`
	Description string         `json:"description"`
	Parameters  map[string]any `json:"parameters"`
}

type openaiRequest struct {
	Model          string       `json:"model"`
	Input          []openaiItem `json:"input"`
	PromptCacheKey string       `json:"prompt_cache_key,omitempty"`
	Tools          []openaiTool `json:"tools,omitempty"`
	ToolChoice     string       `json:"tool_choice,omitempty"`
	Stream         bool         `json:"stream"`
}

// The kinds of part this protocol reads and writes.
const (
	openaiInputText  = "input_text"
	openaiOutputText = "output_text"
)

// buildOpenaiInput writes the turns as this protocol carries them. The system prompt is a turn
// of its own, which this protocol calls the developer.
func buildOpenaiInput(request Request) []openaiItem {
	items := []openaiItem{}
	if request.System != "" {
		// The turn that opens the request carries its text plainly, where every turn after it
		// carries parts.
		items = append(items, openaiItem{Role: "developer", Content: request.System})
	}

	for _, message := range request.Messages {
		for _, answered := range message.Answers {
			items = append(items, openaiItem{
				Type: "function_call_output", CallID: answered.CallID, Output: answered.Output,
			})
		}
		if len(message.Answers) > 0 {
			continue
		}

		if message.Text != "" {
			part := openaiInputText
			if message.Role == RoleAssistant {
				part = openaiOutputText
			}
			items = append(items, openaiItem{
				Role: message.Role, Content: []openaiPart{{Type: part, Text: message.Text}},
			})
		}
		for _, call := range message.Calls {
			items = append(items, openaiItem{
				Type: "function_call", CallID: call.ID, Name: call.Name,
				Arguments: call.Arguments,
			})
		}
	}
	return items
}

func (held *openaiModel) buildRequest(request Request) openaiRequest {
	tools := make([]openaiTool, 0, len(request.Tools))
	for _, tool := range request.Tools {
		tools = append(tools, openaiTool{
			Type: "function", Name: tool.Name, Description: tool.Description,
			Parameters: tool.InputSchema,
		})
	}
	built := openaiRequest{
		Model: held.model, Input: buildOpenaiInput(request), Tools: tools, Stream: true,
		PromptCacheKey: held.cacheKey,
	}
	if len(tools) > 0 {
		built.ToolChoice = "auto"
	}
	return built
}

// The events of the stream, as this protocol writes them.
type openaiStreamEvent struct {
	Type  string `json:"type"`
	Delta string `json:"delta"`
	Item  *struct {
		Type      string `json:"type"`
		ID        string `json:"id"`
		CallID    string `json:"call_id"`
		Name      string `json:"name"`
		Arguments string `json:"arguments"`
	} `json:"item"`
	Part *struct {
		Type string `json:"type"`
	} `json:"part"`
	Response *struct {
		Status            string `json:"status"`
		IncompleteDetails *struct {
			Reason string `json:"reason"`
		} `json:"incomplete_details"`
		Usage *struct {
			InputTokens        int `json:"input_tokens"`
			OutputTokens       int `json:"output_tokens"`
			InputTokensDetails *struct {
				CachedTokens int `json:"cached_tokens"`
			} `json:"input_tokens_details"`
		} `json:"usage"`
		Error *struct {
			Message string `json:"message"`
		} `json:"error"`
	} `json:"response"`
	Message string `json:"message"`
}

func (held *openaiModel) Stream(
	ctx context.Context, request Request, onEvent func(Event),
) (Answer, error) {
	body, err := sendJSON(ctx, held.baseURL+"/responses", map[string]string{
		"authorization": "Bearer " + held.apiKey,
	}, held.buildRequest(request))
	if err != nil {
		return Answer{}, err
	}
	defer func() { _ = body.Close() }()

	answer := Answer{FinishReason: FinishUnknown}
	text := strings.Builder{}
	// The arguments of each call arrive a piece at a time, keyed by the call.
	building := map[string]*openBlock{}
	order := []string{}

	err = readServerEvents(body, func(held serverEvent) error {
		event := openaiStreamEvent{}
		if problem := readJSONInto(held.data, &event); problem != nil {
			LogEvent("! openai wrote an event this build cannot read: " + problem.Error())
			return nil
		}

		switch event.Type {
		case "response.content_part.added":
			if event.Part != nil && event.Part.Type == openaiOutputText {
				onEvent(Event{Kind: EventTextStart})
			}
		case "response.output_text.delta":
			if event.Delta != "" {
				text.WriteString(event.Delta)
				onEvent(Event{Kind: EventTextDelta, Text: event.Delta})
			}
		case "response.output_item.added":
			if event.Item == nil || event.Item.Type != "function_call" {
				return nil
			}
			building[event.Item.ID] = &openBlock{
				kind: "function_call", callID: event.Item.CallID, name: event.Item.Name,
			}
			order = append(order, event.Item.ID)
		case "response.function_call_arguments.delta":
			if open := findOpenCall(building, event); open != nil {
				open.arguments.WriteString(event.Delta)
			}
		case "response.output_item.done":
			if event.Item == nil || event.Item.Type != "function_call" {
				return nil
			}
			// The whole arguments come with this event, so what was built is only a fallback.
			arguments := event.Item.Arguments
			if arguments == "" {
				if open, held := building[event.Item.ID]; held {
					arguments = open.arguments.String()
				}
			}
			answer.Calls = append(answer.Calls,
				readToolCall(event.Item.CallID, event.Item.Name, arguments))
			delete(building, event.Item.ID)
		case "response.completed", "response.incomplete", "response.failed":
			if event.Response == nil {
				return nil
			}
			answer.Usage = readOpenaiUsage(event, answer.Usage)
			answer.FinishReason = readOpenaiStopReason(event, len(answer.Calls) > 0)
			if event.Response.Error != nil && event.Response.Error.Message != "" {
				return fmt.Errorf("%s", event.Response.Error.Message)
			}
		case "error":
			if event.Message != "" {
				return fmt.Errorf("%s", event.Message)
			}
			return fmt.Errorf("the provider reported an error")
		}
		return nil
	})
	if err != nil {
		return answer, err
	}

	// A call whose closing event never came is still a call the model asked for.
	for _, id := range order {
		if open, waiting := building[id]; waiting {
			answer.Calls = append(answer.Calls,
				readToolCall(open.callID, open.name, open.arguments.String()))
		}
	}
	answer.Text = text.String()
	return answer, nil
}

// findOpenCall returns the call these arguments belong to. The event names the item it is part
// of, and a stream with one call at a time names none.
func findOpenCall(building map[string]*openBlock, event openaiStreamEvent) *openBlock {
	if event.Item != nil {
		if open, held := building[event.Item.ID]; held {
			return open
		}
	}
	if len(building) != 1 {
		return nil
	}
	for _, open := range building {
		return open
	}
	return nil
}

// readOpenaiUsage keeps what the request spent.
func readOpenaiUsage(event openaiStreamEvent, kept Usage) Usage {
	if event.Response == nil || event.Response.Usage == nil {
		return kept
	}
	spent := event.Response.Usage
	kept.InputTokens = spent.InputTokens
	kept.OutputTokens = spent.OutputTokens
	if spent.InputTokensDetails != nil {
		kept.CachedInputTokens = spent.InputTokensDetails.CachedTokens
	}
	return kept
}

// readOpenaiStopReason names the reason this model stopped as the panel names it. This protocol
// reports no reason for a call, so a call that was asked for is the reason.
func readOpenaiStopReason(event openaiStreamEvent, asked bool) string {
	if event.Response == nil {
		return FinishUnknown
	}
	if event.Response.IncompleteDetails != nil {
		switch event.Response.IncompleteDetails.Reason {
		case "max_output_tokens":
			return FinishLength
		case "content_filter":
			return FinishContentFilter
		}
	}
	switch event.Response.Status {
	case "completed":
		if asked {
			return FinishToolCalls
		}
		return FinishStop
	case "incomplete":
		return FinishOther
	case "failed":
		return FinishOther
	}
	return FinishUnknown
}
