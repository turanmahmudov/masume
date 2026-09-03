package core

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLogFileAppendWritesEveryLineInOrder(t *testing.T) {
	path := filepath.Join(t.TempDir(), "under", "one.log")
	log := NewLogFile(path)
	log.Append("first")
	log.Append("second")

	written, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("the log was not written: %v", err)
	}
	lines := strings.Split(strings.TrimRight(string(written), "\n"), "\n")
	if len(lines) != 2 {
		t.Fatalf("the log holds %v", lines)
	}
	for at, wanted := range []string{"first", "second"} {
		if !strings.HasSuffix(lines[at], " "+wanted) {
			t.Errorf("line %d reads %q, wanted it to end with %q", at+1, lines[at], wanted)
		}
	}
	// Every line starts with the write time, as a UTC time of day.
	if len(lines[0]) < 25 || lines[0][10] != 'T' || lines[0][23] != 'Z' {
		t.Errorf("the line opens with %q, wanted a time", lines[0][:24])
	}
}

func TestLogFileAppendRollsTheFileAtItsCap(t *testing.T) {
	path := filepath.Join(t.TempDir(), "two.log")
	if err := os.WriteFile(path, []byte(strings.Repeat("x", maxLogBytes)), 0o644); err != nil {
		t.Fatalf("the log was not seeded: %v", err)
	}
	NewLogFile(path).Append("after the cap")

	rolled, err := os.ReadFile(path + ".1")
	if err != nil {
		t.Fatalf("the old log was not kept: %v", err)
	}
	if len(rolled) != maxLogBytes {
		t.Errorf("the old log holds %d bytes", len(rolled))
	}
	written, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("the new log was not written: %v", err)
	}
	if !strings.HasSuffix(strings.TrimRight(string(written), "\n"), " after the cap") {
		t.Errorf("the new log reads %q", string(written))
	}
}

// A log holds every statement and every row a tool returned, so no other user of the
// machine can be allowed to read it.
func TestLogFileAppendWritesForTheOwnerAlone(t *testing.T) {
	path := filepath.Join(t.TempDir(), "one.log")
	NewLogFile(path).Append("first")

	found, err := os.Stat(path)
	if err != nil {
		t.Fatalf("the log was not written: %v", err)
	}
	if held := found.Mode().Perm(); held != 0o600 {
		t.Errorf("the log is written %o, wanted 600", held)
	}
}

// A log file from an older build is readable by everyone. The first write restricts the
// permissions.
func TestLogFileAppendNarrowsALogItFinds(t *testing.T) {
	path := filepath.Join(t.TempDir(), "old.log")
	if err := os.WriteFile(path, []byte("older\n"), 0o644); err != nil {
		t.Fatalf("cannot write the log: %v", err)
	}
	NewLogFile(path).Append("first")

	found, err := os.Stat(path)
	if err != nil {
		t.Fatalf("the log was not read back: %v", err)
	}
	if held := found.Mode().Perm(); held != 0o600 {
		t.Errorf("the log is written %o, wanted 600", held)
	}
}

func TestCutForLog(t *testing.T) {
	if cut := CutForLog("short"); cut != "short" {
		t.Errorf("a short line was cut to %q", cut)
	}
	long := strings.Repeat("é", maxLoggedRunes+10)
	cut := CutForLog(long)
	if len([]rune(cut)) != maxLoggedRunes+1 || !strings.HasSuffix(cut, "…") {
		t.Errorf("a long line was cut to %d characters", len([]rune(cut)))
	}
}
