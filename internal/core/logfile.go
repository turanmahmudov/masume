package core

import (
	"os"
	"path/filepath"
	"sync"
	"time"
)

// A log of the traffic of this client, to read with `tail -f` while it runs. Each log names
// its own file, and all of them are written the same way.

// maxLogBytes is the size above which a log is rolled: the old file becomes `<name>.1` and a
// new one starts. A log holds every statement and every answer, so a client left open for
// weeks would fill the disk.
const maxLogBytes = 2_000_000

type LogFile struct {
	path string
	// guard is held while a line is written, so two lines of one file stay in the order they
	// were written.
	guard sync.Mutex
	// size is what this process last wrote, so the file is not measured again for every
	// line.
	size     int64
	measured bool
}

func NewLogFile(path string) *LogFile {
	return &LogFile{path: path}
}

// Append writes one line, with the time it was written. A failed write is dropped, because a
// log must not break the work it records. The file is opened for the owner alone, because a
// log holds the rows a statement returned.
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
			// A file an older build left behind is readable by everyone, so it is
			// narrowed the first time this process writes to it.
			if found.Mode().Perm() != 0o600 {
				_ = os.Chmod(log.path, 0o600)
			}
		}
		log.measured = true
	}
	// Rolled before the line, so the size limit is never passed.
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

// maxLoggedRunes is the length above which a line of a log is cut.
const maxLoggedRunes = 500

// CutForLog shortens a line of a log, because one answer can hold a whole relation.
func CutForLog(text string) string {
	written := []rune(text)
	if len(written) <= maxLoggedRunes {
		return text
	}
	return string(written[:maxLoggedRunes]) + "…"
}
