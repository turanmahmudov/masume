package cfg

import (
	"time"

	"github.com/turanmahmudov/masume/internal/core"
)

// McpConfig holds everything under `[mcp]`: the profiles an agent may reach, and
// what it may run.
type McpConfig struct {
	// The profiles an agent may reach. None until the file names one.
	Profiles []string
	Access   McpAccess
	// How many rows one read returns, which is also the most a caller may ask for.
	RowLimit int
	// How long a statement may take before it is cancelled.
	Timeout time.Duration
}

// The defaults an agent is held to where the file names nothing.
const (
	DefaultMcpRowLimit = 500
	DefaultMcpTimeout  = 30 * time.Second
)

// DefaultMcpConfig holds the level an agent starts at.
func DefaultMcpConfig() McpConfig {
	return McpConfig{Access: McpReadOnly, RowLimit: DefaultMcpRowLimit, Timeout: DefaultMcpTimeout}
}

// FindMcpAccess reads this text as a level.
func FindMcpAccess(written string) (McpAccess, bool) {
	return core.FindAllowed(McpAccessLevels, written)
}

// ResolveLowerAccess returns the lower of two levels, because each setting is an
// upper limit.
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

// ParseMcpConfig reads `[mcp]`. A wrong setting falls back to the default, and does
// not stop the server, which would leave the agent without a client.
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
