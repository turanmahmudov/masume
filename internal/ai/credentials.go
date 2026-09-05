package ai

import "github.com/turanmahmudov/masume/internal/cfg"

// This is separate from the client, because a screen asks these questions before the first
// request.

// FindAPIKey returns the key of this provider, and whether the config has one.
func FindAPIKey(settings cfg.AiProviderSettings) (string, bool) {
	key := cfg.FindConfiguredValue(settings.APIKey, settings.APIKeyEnv)
	return key, key != ""
}

// HasCredentials is true if the config has the credentials this provider needs.
func HasCredentials(config cfg.AiConfig, id cfg.AiProviderID) bool {
	_, held := FindAPIKey(config.Providers[id])
	return held
}

// DescribeMissingKey returns the instructions for a provider without a key.
func DescribeMissingKey(config cfg.AiConfig, id cfg.AiProviderID) string {
	settings := config.Providers[id]
	table := "[ai.providers." + string(id) + "]"
	if settings.APIKeyEnv != "" {
		return "no API key: " + settings.APIKeyEnv + " is empty. Set it, or set api_key " +
			"under " + table + " in the config file."
	}
	return "no API key: set api_key under " + table + " in the config file, or set " +
		"api_key_env to the name of the environment variable that holds the key."
}
