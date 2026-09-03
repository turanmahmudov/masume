package ai_test

import (
	"strings"
	"testing"

	"github.com/turanmahmudov/masume/internal/ai"
	"github.com/turanmahmudov/masume/internal/cfg"
)

func TestFindAPIKeyPrefersTheWrittenKey(t *testing.T) {
	key, found := ai.FindAPIKey(cfg.AiProviderSettings{APIKey: "sk-written", APIKeyEnv: "MISSING"})
	if !found || key != "sk-written" {
		t.Errorf("the key reads %q, found=%v", key, found)
	}
}

func TestFindAPIKeyReadsTheEnvironment(t *testing.T) {
	t.Setenv("MASUME_TEST_AI_KEY", "sk-env")
	key, found := ai.FindAPIKey(cfg.AiProviderSettings{APIKeyEnv: "MASUME_TEST_AI_KEY"})
	if !found || key != "sk-env" {
		t.Errorf("the key reads %q, found=%v", key, found)
	}
}

func TestHasCredentialsFollowsTheProvider(t *testing.T) {
	config := cfg.AiConfig{Providers: map[cfg.AiProviderID]cfg.AiProviderSettings{
		cfg.ProviderOpenai: {APIKey: "sk-openai"},
	}}
	if !ai.HasCredentials(config, cfg.ProviderOpenai) {
		t.Error("a provider with a key reports none")
	}
	if ai.HasCredentials(config, cfg.ProviderAnthropic) {
		t.Error("a provider with no key reports one")
	}
}

func TestDescribeMissingKeyNamesTheTableAndTheVariable(t *testing.T) {
	config := cfg.AiConfig{Providers: map[cfg.AiProviderID]cfg.AiProviderSettings{
		cfg.ProviderOpenai:    {},
		cfg.ProviderAnthropic: {APIKeyEnv: "ANTHROPIC_API_KEY"},
	}}
	written := ai.DescribeMissingKey(config, cfg.ProviderOpenai)
	if !strings.Contains(written, "[ai.providers.openai]") {
		t.Errorf("the message does not name the table: %q", written)
	}
	held := ai.DescribeMissingKey(config, cfg.ProviderAnthropic)
	if !strings.Contains(held, "ANTHROPIC_API_KEY") {
		t.Errorf("the message does not name the variable: %q", held)
	}
}

// A provider without a key must give an error here, with a message that says what to
// configure. A model opened with an empty key sends the request and gets a 401.
func TestOpenModelRefusesAProviderWithNoKey(t *testing.T) {
	config := cfg.AiConfig{Providers: map[cfg.AiProviderID]cfg.AiProviderSettings{
		cfg.ProviderOpenai: {Model: "gpt-5"},
	}}
	held, err := ai.OpenModel(config, cfg.ProviderOpenai, "masume/shop")
	if held != nil {
		t.Fatal("a provider with no key opened a model")
	}
	if err == nil || !strings.Contains(err.Error(), "[ai.providers.openai]") {
		t.Errorf("the failure reads %v, wanted what DescribeMissingKey writes", err)
	}
}

func TestOpenModelOpensTheProviderWithAKey(t *testing.T) {
	config := cfg.AiConfig{Providers: map[cfg.AiProviderID]cfg.AiProviderSettings{
		cfg.ProviderAnthropic: {Model: "claude-opus-5", APIKey: "sk-written"},
	}}
	held, err := ai.OpenModel(config, cfg.ProviderAnthropic, "masume/shop")
	if err != nil {
		t.Fatalf("a provider with a key failed to open: %v", err)
	}
	if held.Describe() != "anthropic/claude-opus-5" {
		t.Errorf("the model describes itself as %q", held.Describe())
	}
}
