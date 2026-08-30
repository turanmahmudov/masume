// Package mcp serves the same profiles and the same reads to an agent over the Model Context
// Protocol. Only the protocol writes to standard output.
package mcp

import (
	"slices"

	"github.com/turanmahmudov/masume/internal/cfg"
	"github.com/turanmahmudov/masume/internal/query/statement"
)

// What an agent may run, decided in one place. The allowlist decides first, the profile says
// what it wants, and the access mode of the profile limits both.

// riskNeeds names the lowest level that allows a statement of each risk.
var riskNeeds = map[statement.WriteRisk]cfg.McpAccess{
	statement.RiskNone:     cfg.McpReadOnly,
	statement.RiskWrite:    cfg.McpReadWrite,
	statement.RiskDelete:   cfg.McpFull,
	statement.RiskEveryRow: cfg.McpFull,
}

// ResolveProfileAccess returns what this profile is open for. A profile the file does not
// name is closed, a profile can only lower the `[mcp]` level and never raise it, and a
// read-only profile stays read-only whatever the settings say.
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

// namesProfile is true where the config file opened this profile to an agent.
func namesProfile(config cfg.McpConfig, name string) bool {
	return slices.Contains(config.Profiles, name)
}

// FindAccessRefusal returns nothing where the statement may run, and the message to answer
// with where it may not.
func FindAccessRefusal(access cfg.McpAccess, risk statement.WriteRisk) string {
	needed := riskNeeds[risk]
	if cfg.ResolveLowerAccess(access, needed) == needed {
		return ""
	}
	return "this connection is open to MCP as " + string(access) +
		", and the statement " + statement.DescribeRisk(risk, 1) +
		"; only " + string(needed) + " or above may run it"
}

// FindClosedReason returns why this profile is closed to an agent, or nothing where it is
// open. Three settings can close it, so the message names the right one.
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

// DescribeNoOpenProfiles returns why no profile is open, which is not always a name missing
// under `profiles`.
func DescribeNoOpenProfiles(config cfg.McpConfig) string {
	if len(config.Profiles) == 0 {
		return "no profile is open to MCP; name one under [mcp] profiles in the config file"
	}
	if config.Access == cfg.McpOff {
		return `[mcp] access is "off", so no profile is open; raise it to read-only or above`
	}
	return `every profile named under [mcp] profiles sets mcp = "off" of its own`
}
