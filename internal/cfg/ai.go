package cfg

import (
	"strings"
	"time"

	"github.com/turanmahmudov/masume/internal/core"
)

// AiProviderID is the name of one provider the AI chat can use. Each one has its own SDK.
type AiProviderID string

// The providers the AI chat can use.
const (
	ProviderAnthropic AiProviderID = "anthropic"
	ProviderOpenai    AiProviderID = "openai"
)

// AiProviderIDs lists the providers a config file can use.
var AiProviderIDs = []AiProviderID{ProviderAnthropic, ProviderOpenai}

// describeAiProviderIDs returns the supported provider names.
func describeAiProviderIDs() string {
	written := make([]string, 0, len(AiProviderIDs))
	for _, id := range AiProviderIDs {
		written = append(written, string(id))
	}
	return "The providers are " + strings.Join(written, " and ") + "."
}

// AiProviderSettings is the model, API key, and provider address configuration.
type AiProviderSettings struct {
	Model      string
	APIKey     string
	APIKeyEnv  string
	BaseURL    string
	BaseURLEnv string
}

// AiConfig is the configuration under `[ai]`.
type AiConfig struct {
	// Enabled is the switch for the AI chat, its actions, and its interface elements.
	Enabled         bool
	DefaultProvider AiProviderID
	Providers       map[AiProviderID]AiProviderSettings
	// The time one AI chat statement can run before it is cancelled.
	StatementTimeout time.Duration
	// Unsupported provider names under `[ai]`.
	Problems []string
}

// DefaultAiStatementTimeout is the default time limit for AI chat statements.
const DefaultAiStatementTimeout = 30 * time.Second

// DefaultAiConfig returns the default AI chat settings.
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

// parseProviderSettings reads the table of one provider and applies it over the default.
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

// ParseAiConfig reads `[ai]`. An invalid setting uses the default.
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
				"ai.providers: unsupported provider \""+name+"\". Skipping this provider. "+
					describeAiProviderIDs())
		}
	}

	if written, named := FindString(ai, "default_provider"); named {
		id, known := core.FindAllowed(AiProviderIDs, written)
		if known {
			config.DefaultProvider = id
		} else {
			config.Problems = append(config.Problems,
				"ai.default_provider: unsupported provider \""+written+"\". Using "+
					string(config.DefaultProvider)+". "+describeAiProviderIDs())
		}
	}
	if milliseconds, named := FindPositiveInteger(ai, "statement_timeout_ms"); named {
		config.StatementTimeout = time.Duration(milliseconds) * time.Millisecond
	}
	return config
}
