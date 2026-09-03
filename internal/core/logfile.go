package core

import (
	"os"
	"path/filepath"
	"sync"
	"time"
)

// A log of the traffic of this client, to read with `tail -f` while it runs. Each log has
// its own file, and all logs use the same format.

// maxLogBytes is the size at which a log is rotated: the old file becomes `<name>.1` and a
// new file starts. A log records every statement and every result, so a client that runs
// for weeks would fill the disk.
const maxLogBytes = 2_000_000

type LogFile struct {
	path string
	// guard is locked while a line is written, so the lines of one file keep their order.
	guard sync.Mutex
	// size is the size after the last write of this process, so the file is not measured
	// again for every line.
	size     int64
	measured bool
}

func NewLogFile(path string) *LogFile {
	return &LogFile{path: path}
}

// Append writes one line with a timestamp. A write error is ignored, because a log must
// not stop the work it records. The file permissions allow the owner only, because a log
// contains the rows a statement returned.
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
			// A file from an older build is readable by everyone. The first write of
			// this process restricts the permissions.
			if found.Mode().Perm() != 0o600 {
				_ = os.Chmod(log.path, 0o600)
			}
		}
		log.measured = true
	}
	// Rotate before the write, so the size limit is never exceeded.
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

// CutForLog truncates a log line, because one result can contain a whole table.
func CutForLog(text string) string {
	written := []rune(text)
	if len(written) <= maxLoggedRunes {
		return text
	}
	return string(written[:maxLoggedRunes]) + "…"
}
