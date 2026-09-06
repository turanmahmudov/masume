package mcp

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/turanmahmudov/masume/internal/cfg"
)

// captureStderr returns what the call wrote to the error stream.
func captureStderr(t *testing.T, call func()) string {
	t.Helper()
	read, write, err := os.Pipe()
	if err != nil {
		t.Fatalf("cannot open a pipe: %v", err)
	}
	held := os.Stderr
	os.Stderr = write
	defer func() { os.Stderr = held }()

	written := make(chan string, 1)
	go func() {
		text, _ := io.ReadAll(read)
		written <- string(text)
	}()
	call()
	_ = write.Close()
	return <-written
}

// Every line the server writes about the config carries the prefix of the server, so a
// client log reads as one stream.
func TestRunServerPrefixesEveryConfigLine(t *testing.T) {
	home := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", home)
	if err := os.MkdirAll(filepath.Join(home, "masume"), 0o700); err != nil {
		t.Fatalf("cannot make the config directory: %v", err)
	}
	written := "[profile.bad]\nengine = \"postgres\"\n"
	if err := os.WriteFile(
		filepath.Join(home, "masume", "config.toml"), []byte(written), 0o600); err != nil {
		t.Fatalf("cannot write the config file: %v", err)
	}

	reported := captureStderr(t, func() {
		RunServer([]string{"--mcp", "--check"}, "test")
	})
	lines := strings.Split(strings.TrimSpace(reported), "\n")
	if len(lines) < 2 {
		t.Fatalf("the server reported %q, wanted the skipped profile and the result", reported)
	}
	for _, line := range lines {
		if !strings.HasPrefix(line, serverName+" mcp: ") {
			t.Errorf("the line %q carries no prefix", line)
		}
	}
}

// A first run of the server has no config file, and the starter file gives the user
// something to edit.
func TestRunServerWritesTheStarterConfig(t *testing.T) {
	home := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", home)

	if code := RunServer([]string{"--mcp", "--check"}, "test"); code != 1 {
		t.Errorf("the check answered %d, wanted 1 for a config that opens no profile", code)
	}

	written, err := os.ReadFile(filepath.Join(home, "masume", "config.toml"))
	if err != nil {
		t.Fatalf("the starter config was not written: %v", err)
	}
	if string(written) != string(cfg.StarterConfig()) {
		t.Error("the file written is not the starter config")
	}
}
