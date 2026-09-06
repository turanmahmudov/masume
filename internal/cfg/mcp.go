package cfg

import (
	"time"

	"github.com/turanmahmudov/masume/internal/core"
)

// McpConfig is the profile access and limit configuration under `[mcp]`.
type McpConfig struct {
	// The profiles an agent can connect to. It is empty until the file lists one.
	Profiles []string
	Access   McpAccess
	// The default and maximum rows per read.
	RowLimit int
	// The time a statement can run before it is cancelled.
	Timeout time.Duration
}

// The limits applied to an agent when the file sets none.
const (
	DefaultMcpRowLimit = 500
	DefaultMcpTimeout  = 30 * time.Second
)

// DefaultMcpConfig returns the default MCP access and limits.
func DefaultMcpConfig() McpConfig {
	return McpConfig{Access: McpReadOnly, RowLimit: DefaultMcpRowLimit, Timeout: DefaultMcpTimeout}
}

// FindMcpAccess parses the text as an access level.
func FindMcpAccess(written string) (McpAccess, bool) {
	return core.FindAllowed(McpAccessLevels, written)
}

// ResolveLowerAccess returns the lower access level.
func ResolveLowerAccess(left, right McpAccess) McpAccess {
	if indexOfAccess(left) <= indexOfAccess(right) {
		return left
	}
	return right
}

func indexOfAccess(level McpAccess) int {
	for at, candidate := range McpAccessLevels {
		if candidate == level {
			return at
		}
	}
	return 0
}

// ParseMcpConfig reads `[mcp]` with defaults for invalid settings.
func ParseMcpConfig(document Table) McpConfig {
	config := DefaultMcpConfig()
	mcp, present := FindSection(document, "mcp")
	if !present {
		return config
	}

	if profiles, isList := FindStringList(mcp, "profiles"); isList {
		config.Profiles = profiles
	}
	if written, isText := mcp["access"].(string); isText {
		if level, known := FindMcpAccess(written); known {
			config.Access = level
		}
	}
	if limit, named := FindPositiveInteger(mcp, "row_limit"); named {
		config.RowLimit = limit
	}
	if milliseconds, named := FindPositiveInteger(mcp, "timeout_ms"); named {
		config.Timeout = time.Duration(milliseconds) * time.Millisecond
	}
	return config
}
