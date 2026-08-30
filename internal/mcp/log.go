package mcp

import (
	"sync"

	"github.com/turanmahmudov/masume/internal/core"
)

// ResolveLogPath returns where the calls of an agent are written, so the user can read them
// later.
func ResolveLogPath() string {
	return core.ResolveStatePath("mcp.log")
}

var serverLog = sync.OnceValue(func() *core.LogFile { return core.NewLogFile(ResolveLogPath()) })

// LogEvent writes one line of the log of this server.
func LogEvent(message string) {
	serverLog().Append(message)
}
