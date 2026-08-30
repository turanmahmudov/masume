package mcp

import (
	"context"
	"fmt"

	"github.com/turanmahmudov/masume/internal/cfg"
	"github.com/turanmahmudov/masume/internal/core"
)

// Refusal is a call the server refused: a closed profile, or a statement above its level.
type Refusal struct {
	Reason string
}

func (refusal *Refusal) Error() string {
	return refusal.Reason
}

// refuse builds the refusal of one call.
func refuse(format string, parts ...any) error {
	return &Refusal{Reason: fmt.Sprintf(format, parts...)}
}

// AccessDeps is what every entry point needs to reach a connection.
type AccessDeps struct {
	Profiles []cfg.Profile
	Config   cfg.McpConfig
	Sessions *Sessions
	// ScopedProfile is the profile the server was started for, so a caller names none.
	ScopedProfile string
}

// ListOpenProfiles returns the profiles the config opened to an agent, in the order of the
// picker.
func ListOpenProfiles(deps AccessDeps) []cfg.Profile {
	open := []cfg.Profile{}
	for _, profile := range deps.Profiles {
		if ResolveProfileAccess(deps.Config, profile) != cfg.McpOff {
			open = append(open, profile)
		}
	}
	return open
}

// GetNamedProfile returns the profile a call names, or the one the server was started for.
func GetNamedProfile(deps AccessDeps, named any) (cfg.Profile, error) {
	written, isText := named.(string)
	asked, given := deps.ScopedProfile, deps.ScopedProfile != ""
	if !given {
		asked, given = written, isText
	}
	if !given {
		return cfg.Profile{}, refuse(
			"name the profile to work on; call list_profiles to see them")
	}
	if deps.ScopedProfile != "" && isText && written != deps.ScopedProfile {
		return cfg.Profile{}, refuse(
			"this server was started for %q alone, so it cannot reach %q",
			deps.ScopedProfile, written)
	}

	profile, found := findProfileNamed(deps.Profiles, asked)
	if !found {
		return cfg.Profile{}, refuse(
			"no profile named %q; call list_profiles to see them", asked)
	}
	if closed := FindClosedReason(deps.Config, profile); closed != "" {
		return cfg.Profile{}, refuse("%s", closed)
	}
	return profile, nil
}

// applyAccessMode returns the profile this server connects with. An agent that may only
// read gets a read-only connection, so the write a statement does not show in its words is
// refused as well, such as a SELECT of a function that writes. An engine that cannot be
// opened read-only is left as it is, because refusing to connect would serve nothing.
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

// findProfileNamed returns the profile of this name, and whether the config holds one.
func findProfileNamed(profiles []cfg.Profile, name string) (cfg.Profile, bool) {
	for _, profile := range profiles {
		if profile.Name == name {
			return profile, true
		}
	}
	return cfg.Profile{}, false
}

// OpenNamedConnection returns the connection of this profile, opened on the first call.
func OpenNamedConnection(
	ctx context.Context, deps AccessDeps, profile cfg.Profile,
) (*Connection, error) {
	connection, err := deps.Sessions.OpenConnection(ctx, applyAccessMode(deps.Config, profile))
	if err != nil {
		return nil, err
	}
	// A stale list is read again, so a table created during the session is found. A read
	// that fails leaves the older list in place, which the log names so it can be seen.
	if err := connection.RefreshTables(ctx); err != nil {
		LogEvent("! the relations of " + profile.Name + " were not read again: " + err.Error())
	}
	return connection, nil
}
