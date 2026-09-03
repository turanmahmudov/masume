// Package mcp gives an agent the same profiles and the same reads over the Model Context
// Protocol. Only the protocol writes to standard output.
package mcp

import (
	"slices"

	"github.com/turanmahmudov/masume/internal/cfg"
	"github.com/turanmahmudov/masume/internal/query/statement"
)

// The access level of an agent, resolved in one place. The allowlist decides first, the
// profile sets its own level, and the access mode of the profile limits both.

// riskNeeds gives the lowest level that allows a statement of each risk.
var riskNeeds = map[statement.WriteRisk]cfg.McpAccess{
	statement.RiskNone:     cfg.McpReadOnly,
	statement.RiskWrite:    cfg.McpReadWrite,
	statement.RiskDelete:   cfg.McpFull,
	statement.RiskEveryRow: cfg.McpFull,
}

// ResolveProfileAccess returns the access level of this profile. A profile that is not in
// the file is closed. A profile can lower the `[mcp]` level and never raise it. A read-only
// profile stays read-only, whatever the settings are.
func ResolveProfileAccess(config cfg.McpConfig, profile cfg.Profile) cfg.McpAccess {
	if !namesProfile(config, profile.Name) {
		return cfg.McpOff
	}
	asked := config.Access
	if profile.McpAccess != cfg.McpUnset {
		asked = cfg.ResolveLowerAccess(config.Access, profile.McpAccess)
	}
	if profile.AccessMode == cfg.AccessReadOnly {
		return cfg.ResolveLowerAccess(asked, cfg.McpReadOnly)
	}
	return asked
}

// namesProfile is true if the config file opens this profile to an agent.
func namesProfile(config cfg.McpConfig, name string) bool {
	return slices.Contains(config.Profiles, name)
}

// FindAccessRefusal returns an empty string if the statement can run, and the error message
// if it cannot.
func FindAccessRefusal(access cfg.McpAccess, risk statement.WriteRisk) string {
	needed := riskNeeds[risk]
	if cfg.ResolveLowerAccess(access, needed) == needed {
		return ""
	}
	return "this connection is open to MCP as " + string(access) +
		", and the statement " + statement.DescribeRisk(risk, 1) +
		"; only " + string(needed) + " or above may run it"
}

// FindClosedReason returns the reason this profile is closed to an agent, or an empty string
// if it is open. Three settings can close it, so the message names the correct one.
func FindClosedReason(config cfg.McpConfig, profile cfg.Profile) string {
	if !namesProfile(config, profile.Name) {
		return `"` + profile.Name + `" is not open to MCP; name it under [mcp] profiles ` +
			"in the config file"
	}
	if profile.McpAccess == cfg.McpOff {
		return `"` + profile.Name + `" sets mcp = "off" on the profile; raise it to ` +
			"read-only or above"
	}
	if config.Access == cfg.McpOff {
		return `[mcp] access is "off", so no profile is open; raise it to read-only or above`
	}
	return ""
}

// DescribeNoOpenProfiles returns the reason no profile is open. A missing name under
// `profiles` is not the only reason.
func DescribeNoOpenProfiles(config cfg.McpConfig) string {
	if len(config.Profiles) == 0 {
		return "no profile is open to MCP; name one under [mcp] profiles in the config file"
	}
	if config.Access == cfg.McpOff {
		return `[mcp] access is "off", so no profile is open; raise it to read-only or above`
	}
	return `every profile named under [mcp] profiles sets mcp = "off" of its own`
}
