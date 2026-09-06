// Package ai provides chat prompts, provider clients, and tool execution.
package ai

import (
	"sync"

	"github.com/turanmahmudov/masume/internal/core"
)

// ResolveLogPath returns the path of the chat traffic log, to read with `tail -f`.
func ResolveLogPath() string {
	return core.ResolveStatePath("ai-chat.log")
}

var chatLog = sync.OnceValue(func() *core.LogFile { return core.NewLogFile(ResolveLogPath()) })

// LogEvent writes one line to the chat log.
func LogEvent(message string) {
	chatLog().Append(message)
}
