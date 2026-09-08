package mcp

import (
	"context"
	"fmt"

	"github.com/turanmahmudov/masume/internal/cfg"
	"github.com/turanmahmudov/masume/internal/core"
)

// Refusal is an access error for a profile or statement.
type Refusal struct {
	Reason string
}

func (refusal *Refusal) Error() string {
	return refusal.Reason
}

// refuse returns the refusal of one call.
func refuse(format string, parts ...any) error {
	return &Refusal{Reason: fmt.Sprintf(format, parts...)}
}

// AccessDeps holds what every entry point needs to reach a connection.
type AccessDeps struct {
	Profiles []cfg.Profile
	Config   cfg.McpConfig
	Sessions *Sessions
	// ScopedProfile is the only enabled profile for a scoped server.
	ScopedProfile string
}

// ListOpenProfiles returns enabled profiles in picker order, restricted to the server scope.
func ListOpenProfiles(deps AccessDeps) []cfg.Profile {
	open := []cfg.Profile{}
	for _, profile := range deps.Profiles {
		if deps.ScopedProfile != "" && profile.Name != deps.ScopedProfile {
			continue
		}
		if ResolveProfileAccess(deps.Config, profile) != cfg.McpOff {
			open = append(open, profile)
		}
	}
	return open
}

// GetNamedProfile resolves the requested or scoped profile and checks access.
func GetNamedProfile(deps AccessDeps, named any) (cfg.Profile, error) {
	written, isText := named.(string)
	if named != nil && !isText && deps.ScopedProfile == "" {
		return cfg.Profile{}, refuse("profile: expected a string")
	}
	asked, given := deps.ScopedProfile, deps.ScopedProfile != ""
	if !given {
		asked, given = written, isText
	}
	if !given {
		return cfg.Profile{}, refuse(
			"profile is required; call list_profiles for available profiles")
	}
	if deps.ScopedProfile != "" && isText && written != deps.ScopedProfile {
		return cfg.Profile{}, refuse(
			"this server only permits profile %q; requested %q",
			deps.ScopedProfile, written)
	}

	profile, found := findProfileNamed(deps.Profiles, asked)
	if !found {
		return cfg.Profile{}, refuse(
			"no profile named %q; call list_profiles for available profiles", asked)
	}
	if closed := FindClosedReason(deps.Config, profile); closed != "" {
		return cfg.Profile{}, refuse("%s", closed)
	}
	return profile, nil
}

// applyAccessMode requests a read-only connection for MCP read-only access when the engine supports read-only mode.
func applyAccessMode(config cfg.McpConfig, profile cfg.Profile) cfg.Profile {
	if ResolveProfileAccess(config, profile) != cfg.McpReadOnly {
		return profile
	}
	if !core.ResolveEngineInfo(profile.Engine).Capabilities.TakesReadOnlyMode {
		return profile
	}
	profile.AccessMode = cfg.AccessReadOnly
	return profile
}

// findProfileNamed returns the profile with this name, and whether the config has one.
func findProfileNamed(profiles []cfg.Profile, name string) (cfg.Profile, bool) {
	for _, profile := range profiles {
		if profile.Name == name {
			return profile, true
		}
	}
	return cfg.Profile{}, false
}

// OpenNamedConnection opens or reuses a profile connection and refreshes stale table metadata.
func OpenNamedConnection(
	ctx context.Context, deps AccessDeps, profile cfg.Profile,
) (*Connection, error) {
	connection, err := deps.Sessions.OpenConnection(ctx, applyAccessMode(deps.Config, profile))
	if err != nil {
		return nil, err
	}
	// Keep cached tables after a refresh failure.
	if err := connection.RefreshTables(ctx); err != nil {
		LogEvent("! cannot refresh tables for " + profile.Name + ": " + err.Error())
	}
	return connection, nil
}
