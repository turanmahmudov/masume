package core

import (
	"os"
	"path/filepath"
	"sync"
	"time"
)

// A log of the traffic of this client, to read with `tail -f` while it runs. Each log has
// its own file, and all logs use the same format.

// maxLogBytes is the rotation threshold. The previous log is `<name>.1`.
const maxLogBytes = 2_000_000

type LogFile struct {
	path string
	// guard is locked while a line is written, so the lines of one file keep their order.
	guard sync.Mutex
	// The file size after the last write by this process.
	size     int64
	measured bool
}

func NewLogFile(path string) *LogFile {
	return &LogFile{path: path}
}

// Append writes a timestamped line to an owner-only file. Write errors are ignored.
func (log *LogFile) Append(message string) {
	line := time.Now().UTC().Format("2006-01-02T15:04:05.000Z") + " " + message + "\n"

	log.guard.Lock()
	defer log.guard.Unlock()

	if !log.measured {
		if err := os.MkdirAll(filepath.Dir(log.path), 0o700); err != nil {
			return
		}
		if found, err := os.Stat(log.path); err == nil {
			log.size = found.Size()
			// Existing files receive owner-only permissions before the first write.
			if found.Mode().Perm() != 0o600 {
				_ = os.Chmod(log.path, 0o600)
			}
		}
		log.measured = true
	}
	// Rotate before a line exceeds the size threshold.
	if log.size+int64(len(line)) > maxLogBytes {
		_ = os.Rename(log.path, log.path+".1")
		log.size = 0
	}

	file, err := os.OpenFile(log.path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return
	}
	defer func() { _ = file.Close() }()
	if _, err := file.WriteString(line); err == nil {
		log.size += int64(len(line))
	}
}

// maxLoggedRunes is the length at which a log line is truncated.
const maxLoggedRunes = 500

// CutForLog truncates text to maxLoggedRunes and adds an ellipsis.
func CutForLog(text string) string {
	written := []rune(text)
	if len(written) <= maxLoggedRunes {
		return text
	}
	return string(written[:maxLoggedRunes]) + "…"
}
