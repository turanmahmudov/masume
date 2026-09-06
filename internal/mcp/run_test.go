package mcp

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/turanmahmudov/masume/internal/cfg"
)

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
