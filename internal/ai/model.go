package ai

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/turanmahmudov/masume/internal/cfg"
)

// What every provider returns for. Each provider writes its own request and reads its own
// stream, because the two protocols differ in everything but what they carry.

// The roles a turn of the request can have.
const (
	RoleUser      = "user"
	RoleAssistant = "assistant"
)

// ToolCall is one call a model asked for.
type ToolCall struct {
	// ID is what the provider called this call, which the answer is keyed by.
	ID    string
	Name  string
	Input map[string]any
	// Arguments is the input as the model wrote it, kept so the next request sends it back
	// exactly as it came.
	Arguments string
}

// ToolAnswer is what one call answered.
type ToolAnswer struct {
	CallID string
	Name   string
	// Output is the answer as JSON text, which is what a model reads.
	Output string
}

// Message is one turn of the request.
type Message struct {
	Role string
	Text string
	// Calls holds the calls an assistant turn asked for.
	Calls []ToolCall
	// Returns holds what those calls answered, sent as the turn after them.
	Answers []ToolAnswer
}

// ToolSchema is one tool as a provider is told about it.
type ToolSchema struct {
	Name        string
	Description string
	InputSchema map[string]any
}

// Request is one question, with everything the model needs to answer it.
type Request struct {
	System   string
	Messages []Message
	Tools    []ToolSchema
}

// The kinds of thing that arrive while a model returns.
const (
	// EventTextStart opens a block of text, which follows the text of an earlier block.
	EventTextStart = "text-start"
	EventTextDelta = "text-delta"
)

// Event is one thing that arrived.
type Event struct {
	Kind string
	Text string
}

// The reasons a model stops.
const (
	FinishStop          = "stop"
	FinishToolCalls     = "tool-calls"
	FinishLength        = "length"
	FinishContentFilter = "content-filter"
	FinishOther         = "other"
	FinishUnknown       = "unknown"
)

// Usage is what one request spent.
type Usage struct {
	InputTokens  int
	OutputTokens int
	// CachedInputTokens is the part of the input the provider read from its cache, at a tenth
	// of the price. It is a part of the input count, not a sum beside it.
	CachedInputTokens int
}

// Answer is what one request answered, once the stream ended.
type Answer struct {
	Text         string
	Calls        []ToolCall
	FinishReason string
	Usage        Usage
}

// Model is one provider, opened for one model.
type Model interface {
	// Describe writes the provider and the model, as the panel and the log name them.
	Describe() string
	// Stream sends one request and reports what arrives. It returns once the stream ends.
	Stream(ctx context.Context, request Request, onEvent func(Event)) (Answer, error)
}

// ResolveVersionedBaseURL adds `/v1` where a base URL has none, because neither provider adds
// it to a URL the config gave.
func ResolveVersionedBaseURL(baseURL string) string {
	stripped := strings.TrimRight(baseURL, "/")
	if strings.HasSuffix(strings.ToLower(stripped), "/v1") {
		return stripped
	}
	return stripped + "/v1"
}

// OpenModel returns the model of the active provider, and why it cannot be opened. The
// cacheKey says which requests share a prefix; a provider that marks its own blocks ignores it.
func OpenModel(config cfg.AiConfig, id cfg.AiProviderID, cacheKey string) (Model, error) {
	if id != cfg.ProviderAnthropic && id != cfg.ProviderOpenai {
		return nil, fmt.Errorf("no provider named %q", string(id))
	}

	settings := config.Providers[id]
	apiKey, held := FindAPIKey(settings)
	if !held {
		return nil, errors.New(DescribeMissingKey(config, id))
	}
	baseURL := cfg.FindConfiguredValue(settings.BaseURL, settings.BaseURLEnv)
	if baseURL != "" {
		baseURL = ResolveVersionedBaseURL(baseURL)
	}

	if id == cfg.ProviderAnthropic {
		return openAnthropicModel(settings.Model, apiKey, baseURL), nil
	}
	return openOpenaiModel(settings.Model, apiKey, baseURL, cacheKey), nil
}

// DescribeActiveModel writes the provider and the model the chat sends to.
func DescribeActiveModel(config cfg.AiConfig, id cfg.AiProviderID) string {
	return string(id) + "/" + config.Providers[id].Model
}
