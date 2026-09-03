package cfg_test

import (
	"strings"
	"testing"

	"github.com/turanmahmudov/masume/internal/cfg"
)

// readAiConfig reads the chat settings from a config file.
func readAiConfig(t *testing.T, body string) cfg.AiConfig {
	t.Helper()
	document, err := cfg.DecodeDocument(body)
	if err != nil {
		t.Fatalf("the config text does not read: %v", err)
	}
	return cfg.ParseAiConfig(document)
}

// An unknown provider is reported. Without the report the chat starts with the default
// provider and the user does not see the spelling error.
func TestParseAiConfigReportsAProviderItDoesNotHave(t *testing.T) {
	for _, held := range []struct {
		name string
		body string
		says string
	}{
		{
			"the provider that starts active",
			"[ai]\ndefault_provider = \"anthropik\"\n",
			"anthropik",
		},
		{
			"a table of settings",
			"[ai.providers.anthropik]\nmodel = \"a-model\"\n",
			"anthropik",
		},
	} {
		t.Run(held.name, func(t *testing.T) {
			config := readAiConfig(t, held.body)
			if len(config.Problems) != 1 {
				t.Fatalf("the read reports %v, wanted one problem", config.Problems)
			}
			if !strings.Contains(config.Problems[0], held.says) {
				t.Errorf("the problem reads %q, wanted the name it does not have",
					config.Problems[0])
			}
		})
	}
}

// A known provider is read and reports no problem.
func TestParseAiConfigReadsAProviderItHas(t *testing.T) {
	config := readAiConfig(t,
		"[ai]\ndefault_provider = \"openai\"\n[ai.providers.openai]\nmodel = \"a-model\"\n")
	if len(config.Problems) != 0 {
		t.Errorf("the read reports %v, wanted nothing", config.Problems)
	}
	if config.DefaultProvider != cfg.ProviderOpenai {
		t.Errorf("the provider reads %q, wanted openai", config.DefaultProvider)
	}
	if config.Providers[cfg.ProviderOpenai].Model != "a-model" {
		t.Errorf("the model reads %q", config.Providers[cfg.ProviderOpenai].Model)
	}
}
