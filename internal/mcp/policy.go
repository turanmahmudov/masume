// Package mcp provides database tools through the Model Context Protocol. stdout is reserved for protocol messages.
package mcp

import (
	"slices"

	"github.com/turanmahmudov/masume/internal/cfg"
	"github.com/turanmahmudov/masume/internal/query/statement"
)

// MCP access combines the profile allowlist, global access, profile access, and read-only mode.

// riskNeeds gives the lowest level that allows a statement of each risk.
var riskNeeds = map[statement.WriteRisk]cfg.McpAccess{
	statement.RiskNone:     cfg.McpReadOnly,
	statement.RiskWrite:    cfg.McpReadWrite,
	statement.RiskDelete:   cfg.McpFull,
	statement.RiskEveryRow: cfg.McpFull,
}

// ResolveProfileAccess returns the lowest applicable access level. Profiles outside the allowlist have no MCP access.
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

// FindAccessRefusal returns an access error or an empty string when allowed.
func FindAccessRefusal(access cfg.McpAccess, risk statement.WriteRisk) string {
	needed := riskNeeds[risk]
	if cfg.ResolveLowerAccess(access, needed) == needed {
		return ""
	}
	return "MCP access is " + string(access) +
		"; the statement " + statement.DescribeRisk(risk, 1) +
		" and requires " + string(needed) + " access or higher"
}

// FindClosedReason returns the setting that disables MCP access, or an empty string when enabled.
func FindClosedReason(config cfg.McpConfig, profile cfg.Profile) string {
	if !namesProfile(config, profile.Name) {
		return `"` + profile.Name + `" is not enabled for MCP; add it under [mcp] profiles ` +
			"in the config file"
	}
	if profile.McpAccess == cfg.McpOff {
		return `"` + profile.Name + `" has mcp = "off"; MCP access requires ` +
			"read-only or higher"
	}
	if config.Access == cfg.McpOff {
		return `[mcp] access is "off"; MCP access requires read-only or higher`
	}
	return ""
}

// DescribeNoOpenProfiles describes disabled or unavailable MCP profiles.
func DescribeNoOpenProfiles(config cfg.McpConfig) string {
	if len(config.Profiles) == 0 {
		return "no profiles are enabled for MCP; add a profile under [mcp] profiles in the config file"
	}
	if config.Access == cfg.McpOff {
		return `[mcp] access is "off"; MCP access requires read-only or higher`
	}
	return `no listed MCP profiles are available; check profile names and profile mcp settings`
}
