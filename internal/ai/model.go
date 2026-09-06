package ai

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/turanmahmudov/masume/internal/cfg"
)

// Shared provider requests, responses, and streaming interface.

// The roles a turn of the request can have.
const (
	RoleUser      = "user"
	RoleAssistant = "assistant"
)

// ToolCall is one call the model requested.
type ToolCall struct {
	// ID is the identifier the provider gave this call. The answer uses it as its key.
	ID    string
	Name  string
	Input map[string]any
	// Arguments is the original input text for subsequent provider requests.
	Arguments string
}

// ToolAnswer is the result of one call.
type ToolAnswer struct {
	CallID string
	Name   string
	// Output is the result as JSON text, which is the form the model reads.
	Output string
}

// Message is one turn of the request.
type Message struct {
	Role string
	Text string
	// Calls holds the calls an assistant turn requested.
	Calls []ToolCall
	// Answers is the tool results sent in the next turn.
	Answers []ToolAnswer
}

// ToolSchema is one tool in the form sent to a provider.
type ToolSchema struct {
	Name        string
	Description string
	InputSchema map[string]any
}

// Request is one question with everything the model needs to answer it.
type Request struct {
	System   string
	Messages []Message
	Tools    []ToolSchema
}

// The kinds of event that arrive while a model answers.
const (
	// EventTextStart starts a block of text that follows an earlier block.
	EventTextStart = "text-start"
	EventTextDelta = "text-delta"
)

// Event is one event of the stream.
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

// Usage is the token count of one request.
type Usage struct {
	InputTokens  int
	OutputTokens int
	// CachedInputTokens is the cached portion of InputTokens.
	CachedInputTokens int
}

// Answer is the result of one request, after the end of the stream.
type Answer struct {
	Text         string
	Calls        []ToolCall
	FinishReason string
	Usage        Usage
}

// Model is one provider, configured for one model.
type Model interface {
	// Describe returns the provider and model for display and logging.
	Describe() string
	// Stream sends a request and reports events until the stream ends.
	Stream(ctx context.Context, request Request, onEvent func(Event)) (Answer, error)
}

// ResolveVersionedBaseURL appends /v1 unless the URL already ends with /v1.
func ResolveVersionedBaseURL(baseURL string) string {
	stripped := strings.TrimRight(baseURL, "/")
	if strings.HasSuffix(strings.ToLower(stripped), "/v1") {
		return stripped
	}
	return stripped + "/v1"
}

// OpenModel configures a provider model. cacheKey is the OpenAI cache grouping key.
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

// DescribeActiveModel returns the provider and the model the chat sends to.
func DescribeActiveModel(config cfg.AiConfig, id cfg.AiProviderID) string {
	return string(id) + "/" + config.Providers[id].Model
}
