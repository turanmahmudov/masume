package ai

import "github.com/turanmahmudov/masume/internal/cfg"

// Kept apart from the client, because a screen asks this before any reply.

// FindAPIKey returns the key of this provider, and whether the config carries one.
func FindAPIKey(settings cfg.AiProviderSettings) (string, bool) {
	key := cfg.FindConfiguredValue(settings.APIKey, settings.APIKeyEnv)
	return key, key != ""
}

// HasCredentials is true where the config has what this provider needs.
func HasCredentials(config cfg.AiConfig, id cfg.AiProviderID) bool {
	_, held := FindAPIKey(config.Providers[id])
	return held
}

// DescribeMissingKey writes what to do for a provider with no key.
func DescribeMissingKey(config cfg.AiConfig, id cfg.AiProviderID) string {
	settings := config.Providers[id]
	table := "[ai.providers." + string(id) + "]"
	if settings.APIKeyEnv != "" {
		return settings.APIKeyEnv + " carries no key; set it, or write api_key under " + table
	}
	return "write api_key under " + table + " in the config file, or api_key_env naming the " +
		"variable that carries it"
}
