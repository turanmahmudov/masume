package ai

import (
	"context"
	"fmt"
	"strings"
)

// The Anthropic Messages client marks the final message block as an explicit cache boundary.

const (
	anthropicBaseURL = "https://api.anthropic.com/v1"
	anthropicVersion = "2023-06-01"
	// strictSchemaBeta is the beta header value for strict tool schemas.
	strictSchemaBeta = "structured-outputs-2025-11-13"
)

// Output token limits for supported model families.
const (
	leastOutputTokens   = 4096
	largeOutputTokens   = 128_000
	mediumOutputTokens  = 64_000
	smallerOutputTokens = 32_000
)

// namesOlderClaudeModel matches instant models and generations 2 and 3. Generation digits require a separator or the end of the name.
func namesOlderClaudeModel(model string) bool {
	_, after, ok := strings.Cut(model, "claude-")
	if !ok {
		return false
	}
	rest := after
	if rest == "instant" || strings.HasPrefix(rest, "instant-") {
		return true
	}

	generation := strings.TrimPrefix(rest, "v")
	for _, family := range []struct{ digit, separators string }{{"2", "-.:"}, {"3", "-."}} {
		if !strings.HasPrefix(generation, family.digit) {
			continue
		}
		tail := generation[len(family.digit):]
		return tail == "" || strings.ContainsAny(tail[:1], family.separators)
	}
	return false
}

// modelCapabilities is the output limit and schema support inferred from the model family.
type modelCapabilities struct {
	// outputLimit is the maximum tokens per response.
	outputLimit int
	// readsSchemasStrictly is true for model families with strict schema support.
	readsSchemasStrictly bool
}

// resolveModelCapabilities returns the properties of this model.
func resolveModelCapabilities(model string) modelCapabilities {
	switch {
	case strings.Contains(model, "claude-opus-5"),
		strings.Contains(model, "claude-opus-4-8"),
		strings.Contains(model, "claude-opus-4-7"),
		strings.Contains(model, "claude-fable-5"),
		strings.Contains(model, "claude-sonnet-5"),
		strings.Contains(model, "claude-sonnet-4-6"),
		strings.Contains(model, "claude-opus-4-6"):
		return modelCapabilities{largeOutputTokens, true}
	case strings.Contains(model, "claude-sonnet-4-5"),
		strings.Contains(model, "claude-opus-4-5"),
		strings.Contains(model, "claude-haiku-4-5"):
		return modelCapabilities{mediumOutputTokens, true}
	case strings.Contains(model, "claude-opus-4-1"):
		return modelCapabilities{smallerOutputTokens, true}
	case strings.Contains(model, "claude-sonnet-4-"):
		return modelCapabilities{mediumOutputTokens, false}
	case strings.Contains(model, "claude-opus-4-"):
		return modelCapabilities{smallerOutputTokens, false}
	case strings.Contains(model, "claude-3-haiku"):
		return modelCapabilities{leastOutputTokens, false}
	case namesOlderClaudeModel(model):
		return modelCapabilities{leastOutputTokens, false}
	case strings.Contains(model, "claude-"):
		return modelCapabilities{largeOutputTokens, true}
	}
	return modelCapabilities{leastOutputTokens, false}
}

// anthropicModel is one model of this provider, used for one request.
type anthropicModel struct {
	model   string
	apiKey  string
	baseURL string
}

func openAnthropicModel(model, apiKey, baseURL string) Model {
	if baseURL == "" {
		baseURL = anthropicBaseURL
	}
	return &anthropicModel{model: model, apiKey: apiKey, baseURL: baseURL}
}

func (held *anthropicModel) Describe() string {
	return "anthropic/" + held.model
}

// The blocks of a message, in the form of this protocol.
type anthropicBlock struct {
	Type string `json:"type"`
	Text string `json:"text,omitempty"`
	// The protocol requires input even for calls without arguments.
	ID    string          `json:"id,omitempty"`
	Name  string          `json:"name,omitempty"`
	Input *map[string]any `json:"input,omitempty"`
	// The result of a call, keyed by the id of that call.
	ToolUseID string `json:"tool_use_id,omitempty"`
	Content   string `json:"content,omitempty"`
	// CacheControl is the cache boundary for the request prefix.
	CacheControl *anthropicCacheControl `json:"cache_control,omitempty"`
}

type anthropicCacheControl struct {
	Type string `json:"type"`
}

type anthropicMessage struct {
	Role    string           `json:"role"`
	Content []anthropicBlock `json:"content"`
}

type anthropicTool struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	InputSchema map[string]any `json:"input_schema"`
	// EagerInputStreaming is the option for streamed tool argument fragments.
	EagerInputStreaming bool `json:"eager_input_streaming"`
}

// anthropicToolChoice lets the model decide whether to call a tool.
type anthropicToolChoice struct {
	Type string `json:"type"`
}

type anthropicRequest struct {
	Model      string               `json:"model"`
	MaxTokens  int                  `json:"max_tokens"`
	System     []anthropicBlock     `json:"system,omitempty"`
	Messages   []anthropicMessage   `json:"messages"`
	Tools      []anthropicTool      `json:"tools,omitempty"`
	ToolChoice *anthropicToolChoice `json:"tool_choice,omitempty"`
	Stream     bool                 `json:"stream"`
}

// buildAnthropicMessages builds Messages input. The protocol requires tool results in user messages.
func buildAnthropicMessages(messages []Message) []anthropicMessage {
	written := make([]anthropicMessage, 0, len(messages))
	for _, message := range messages {
		if len(message.Answers) > 0 {
			blocks := make([]anthropicBlock, 0, len(message.Answers))
			for _, answered := range message.Answers {
				blocks = append(blocks, anthropicBlock{
					Type: "tool_result", ToolUseID: answered.CallID, Content: answered.Output,
				})
			}
			written = append(written, anthropicMessage{Role: RoleUser, Content: blocks})
			continue
		}

		blocks := []anthropicBlock{}
		if message.Text != "" {
			blocks = append(blocks, anthropicBlock{Type: "text", Text: message.Text})
		}
		for _, call := range message.Calls {
			input := call.Input
			if input == nil {
				input = map[string]any{}
			}
			blocks = append(blocks, anthropicBlock{
				Type: "tool_use", ID: call.ID, Name: call.Name, Input: &input,
			})
		}
		if len(blocks) == 0 {
			continue
		}
		written = append(written, anthropicMessage{Role: message.Role, Content: blocks})
	}
	return written
}

// markLastBlock requests prefix caching through the final message block, including the tools and system prompt.
func markLastBlock(messages []anthropicMessage) {
	if len(messages) == 0 {
		return
	}
	last := &messages[len(messages)-1]
	if len(last.Content) == 0 {
		return
	}
	last.Content[len(last.Content)-1].CacheControl = &anthropicCacheControl{Type: "ephemeral"}
}

func (held *anthropicModel) buildRequest(
	request Request, capabilities modelCapabilities,
) anthropicRequest {
	tools := make([]anthropicTool, 0, len(request.Tools))
	for _, tool := range request.Tools {
		tools = append(tools, anthropicTool{
			Name: tool.Name, Description: tool.Description, InputSchema: tool.InputSchema,
			EagerInputStreaming: true,
		})
	}

	system := []anthropicBlock{}
	if request.System != "" {
		system = append(system, anthropicBlock{Type: "text", Text: request.System})
	}
	messages := buildAnthropicMessages(request.Messages)
	markLastBlock(messages)

	built := anthropicRequest{
		Model: held.model, MaxTokens: capabilities.outputLimit,
		System: system, Messages: messages, Tools: tools, Stream: true,
	}
	if len(tools) > 0 {
		built.ToolChoice = &anthropicToolChoice{Type: "auto"}
	}
	return built
}

// buildAnthropicHeaders includes the schema beta header when tools and model support are available.
func (held *anthropicModel) buildHeaders(
	request Request, capabilities modelCapabilities,
) map[string]string {
	headers := map[string]string{
		"x-api-key":         held.apiKey,
		"anthropic-version": anthropicVersion,
	}
	if len(request.Tools) > 0 && capabilities.readsSchemasStrictly {
		headers["anthropic-beta"] = strictSchemaBeta
	}
	return headers
}

// The events of the stream, in the form of this protocol.
type anthropicStreamEvent struct {
	Type         string `json:"type"`
	Index        int    `json:"index"`
	ContentBlock *struct {
		Type string `json:"type"`
		ID   string `json:"id"`
		Name string `json:"name"`
	} `json:"content_block"`
	Delta *struct {
		Type        string `json:"type"`
		Text        string `json:"text"`
		PartialJSON string `json:"partial_json"`
		StopReason  string `json:"stop_reason"`
	} `json:"delta"`
	Message *struct {
		Usage *anthropicUsage `json:"usage"`
	} `json:"message"`
	Usage *anthropicUsage `json:"usage"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error"`
}

type anthropicUsage struct {
	InputTokens         int `json:"input_tokens"`
	OutputTokens        int `json:"output_tokens"`
	CacheReadTokens     int `json:"cache_read_input_tokens"`
	CacheCreationTokens int `json:"cache_creation_input_tokens"`
}

func (held *anthropicModel) Stream(
	ctx context.Context, request Request, onEvent func(Event),
) (Answer, error) {
	capabilities := resolveModelCapabilities(held.model)
	body, err := sendJSON(ctx, held.baseURL+"/messages",
		held.buildHeaders(request, capabilities), held.buildRequest(request, capabilities))
	if err != nil {
		return Answer{}, err
	}
	defer func() { _ = body.Close() }()

	answer := Answer{FinishReason: FinishUnknown}
	text := strings.Builder{}
	open := openBlock{}

	err = readServerEvents(body, func(held serverEvent) error {
		event := anthropicStreamEvent{}
		if problem := readJSONInto(held.data, &event); problem != nil {
			LogEvent("! cannot parse anthropic event: " + problem.Error())
			return nil
		}

		switch event.Type {
		case "message_start":
			if event.Message != nil && event.Message.Usage != nil {
				answer.Usage = readAnthropicUsage(*event.Message.Usage, answer.Usage)
			}
		case "content_block_start":
			if event.ContentBlock == nil {
				return nil
			}
			open = openBlock{kind: event.ContentBlock.Type,
				callID: event.ContentBlock.ID, name: event.ContentBlock.Name}
			if open.kind == "text" {
				onEvent(Event{Kind: EventTextStart})
			}
		case "content_block_delta":
			if event.Delta == nil {
				return nil
			}
			if event.Delta.Type == "text_delta" && event.Delta.Text != "" {
				text.WriteString(event.Delta.Text)
				onEvent(Event{Kind: EventTextDelta, Text: event.Delta.Text})
			}
			if event.Delta.Type == "input_json_delta" {
				open.arguments.WriteString(event.Delta.PartialJSON)
			}
		case "content_block_stop":
			if open.kind == "tool_use" {
				answer.Calls = append(answer.Calls, readToolCall(
					open.callID, open.name, open.arguments.String()))
			}
			open = openBlock{}
		case "message_delta":
			if event.Delta != nil && event.Delta.StopReason != "" {
				answer.FinishReason = readAnthropicStopReason(event.Delta.StopReason)
			}
			if event.Usage != nil {
				answer.Usage = readAnthropicUsage(*event.Usage, answer.Usage)
			}
		case "error":
			if event.Error != nil {
				return fmt.Errorf("%s", event.Error.Message)
			}
			return fmt.Errorf("the provider reported an error")
		}
		return nil
	})
	if err != nil {
		return answer, err
	}

	answer.Text = text.String()
	return answer, nil
}

// readAnthropicUsage combines current and previous event token counts.
func readAnthropicUsage(reported anthropicUsage, kept Usage) Usage {
	// Anthropic reports uncached input, cache reads, and cache writes as separate counts.
	input := reported.InputTokens + reported.CacheReadTokens + reported.CacheCreationTokens
	if input > kept.InputTokens {
		kept.InputTokens = input
	}
	if reported.CacheReadTokens > kept.CachedInputTokens {
		kept.CachedInputTokens = reported.CacheReadTokens
	}
	if reported.OutputTokens > kept.OutputTokens {
		kept.OutputTokens = reported.OutputTokens
	}
	return kept
}

// readAnthropicStopReason maps provider stop reasons to shared finish reasons.
func readAnthropicStopReason(reported string) string {
	switch reported {
	case "end_turn", "stop_sequence":
		return FinishStop
	case "tool_use":
		return FinishToolCalls
	case "max_tokens":
		return FinishLength
	case "refusal":
		return FinishContentFilter
	}
	return FinishOther
}
