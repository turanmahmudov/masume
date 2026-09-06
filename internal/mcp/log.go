package mcp

import (
	"sync"

	"github.com/turanmahmudov/masume/internal/core"
)

// ResolveLogPath returns the MCP log path.
func ResolveLogPath() string {
	return core.ResolveStatePath("mcp.log")
}

var serverLog = sync.OnceValue(func() *core.LogFile { return core.NewLogFile(ResolveLogPath()) })

// LogEvent writes one line to the log of this server.
func LogEvent(message string) {
	serverLog().Append(message)
}
