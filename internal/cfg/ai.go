package cfg

import (
	"strings"
	"time"

	"github.com/turanmahmudov/masume/internal/core"
)

// AiProviderID names one provider the chat can use. Each one has its own SDK.
type AiProviderID string

// The providers the chat can use.
const (
	ProviderAnthropic AiProviderID = "anthropic"
	ProviderOpenai    AiProviderID = "openai"
)

// AiProviderIDs lists the providers a config file may name.
var AiProviderIDs = []AiProviderID{ProviderAnthropic, ProviderOpenai}

// describeAiProviderIDs writes the providers a config file may name, so a report says what
// to write instead.
func describeAiProviderIDs() string {
	written := make([]string, 0, len(AiProviderIDs))
	for _, id := range AiProviderIDs {
		written = append(written, string(id))
	}
	return "The providers are " + strings.Join(written, " and ") + "."
}

// AiProviderSettings holds the model, the key and the base URL of a proxy. The key
// and the URL can name an environment variable.
type AiProviderSettings struct {
	Model      string
	APIKey     string
	APIKeyEnv  string
	BaseURL    string
	BaseURLEnv string
}

// AiConfig holds everything under `[ai]`: the providers configured, and the one
// that starts active.
type AiConfig struct {
	// Enabled turns every AI feature on or off. With it off the chat cannot be opened, no
	// AI action is bound, and nothing about AI is drawn or offered anywhere.
	Enabled         bool
	DefaultProvider AiProviderID
	Providers       map[AiProviderID]AiProviderSettings
	// How long a statement of the chat may take before it is cancelled.
	StatementTimeout time.Duration
	// The settings under `[ai]` that name a provider this client does not have. A name
	// that is not there is reported rather than dropped, so a misspelling is found
	// instead of read as silence.
	Problems []string
}

// DefaultAiStatementTimeout is how long a statement of the chat may take. The MCP
// server has the same default, because nobody watches a statement a model asked for.
const DefaultAiStatementTimeout = 30 * time.Second

// DefaultAiConfig holds the settings the chat opens with.
func DefaultAiConfig() AiConfig {
	return AiConfig{
		Enabled:          true,
		DefaultProvider:  ProviderAnthropic,
		StatementTimeout: DefaultAiStatementTimeout,
		Providers: map[AiProviderID]AiProviderSettings{
			ProviderAnthropic: {Model: "claude-opus-5"},
			ProviderOpenai:    {Model: "gpt-5"},
		},
	}
}

// parseProviderSettings reads the table of one provider over its default.
func parseProviderSettings(table Table, fallback AiProviderSettings) AiProviderSettings {
	if table == nil {
		return fallback
	}
	settings := fallback
	if written, present := FindString(table, "model"); present {
		settings.Model = written
	}
	if written, present := FindString(table, "api_key"); present {
		settings.APIKey = written
	}
	if written, present := FindString(table, "api_key_env"); present {
		settings.APIKeyEnv = written
	}
	if written, present := FindString(table, "base_url"); present {
		settings.BaseURL = written
	}
	if written, present := FindString(table, "base_url_env"); present {
		settings.BaseURLEnv = written
	}
	return settings
}

// ParseAiConfig reads `[ai]`. A wrong setting falls back to the default.
func ParseAiConfig(document Table) AiConfig {
	config := DefaultAiConfig()
	ai, present := FindSection(document, "ai")
	if !present {
		return config
	}

	if enabled, named := FindBool(ai, "enabled"); named {
		config.Enabled = enabled
	}

	providerTables, _ := FindTable(ai["providers"])
	for _, id := range AiProviderIDs {
		table, _ := FindTable(providerTables[string(id)])
		config.Providers[id] = parseProviderSettings(table, config.Providers[id])
	}
	for _, name := range sortedKeys(providerTables) {
		if _, known := core.FindAllowed(AiProviderIDs, name); !known {
			config.Problems = append(config.Problems,
				"ai.providers: \""+name+"\" is not a provider there is, so it is not read. "+
					describeAiProviderIDs())
		}
	}

	if written, named := FindString(ai, "default_provider"); named {
		id, known := core.FindAllowed(AiProviderIDs, written)
		if known {
			config.DefaultProvider = id
		} else {
			config.Problems = append(config.Problems,
				"ai.default_provider: \""+written+"\" is not a provider there is, so "+
					string(config.DefaultProvider)+" is used. "+describeAiProviderIDs())
		}
	}
	if milliseconds, named := FindPositiveInteger(ai, "statement_timeout_ms"); named {
		config.StatementTimeout = time.Duration(milliseconds) * time.Millisecond
	}
	return config
}
