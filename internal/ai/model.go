package ai

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/turanmahmudov/masume/internal/cfg"
)

// The interface every provider implements. Each provider builds its own request and reads
// its own stream, because the two protocols are different in every part except the
// content.

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
	// Arguments is the input in the exact form the model sent, so the next request can
	// send it back unchanged.
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
	// Returns holds the results of those calls, sent in the next turn.
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
	// CachedInputTokens is the part of the input the provider read from its cache, at ten
	// percent of the price. It is included in the input count and is not a separate
	// total.
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
	// Describe returns the provider and the model, in the form used by the panel and the
	// log.
	Describe() string
	// Stream sends one request and reports every event. It returns at the end of the
	// stream.
	Stream(ctx context.Context, request Request, onEvent func(Event)) (Answer, error)
}

// ResolveVersionedBaseURL adds `/v1` to a base URL that has no version, because neither
// provider adds it to a URL from the config.
func ResolveVersionedBaseURL(baseURL string) string {
	stripped := strings.TrimRight(baseURL, "/")
	if strings.HasSuffix(strings.ToLower(stripped), "/v1") {
		return stripped
	}
	return stripped + "/v1"
}

// OpenModel returns the model of the active provider, or the reason it cannot be opened.
// cacheKey groups the requests that share a prefix. A provider that marks its own blocks
// ignores it.
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
