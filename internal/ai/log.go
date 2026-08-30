// Package ai holds what the chat of one connection needs: the prompt a model is told, the
// providers it can be asked through, and the run that returns one question.
package ai

import (
	"sync"

	"github.com/turanmahmudov/masume/internal/core"
)

// ResolveLogPath returns where the traffic of the chat is written, to read with `tail -f`.
func ResolveLogPath() string {
	return core.ResolveStatePath("ai-chat.log")
}

var chatLog = sync.OnceValue(func() *core.LogFile { return core.NewLogFile(ResolveLogPath()) })

// LogEvent writes one line of the log of the chat.
func LogEvent(message string) {
	chatLog().Append(message)
}
