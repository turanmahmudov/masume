package core_test

import (
	"path/filepath"
	"testing"

	"github.com/turanmahmudov/masume/internal/core"
)

func TestExpandHomePathExpandsALeadingTilde(t *testing.T) {
	home := core.HomeDirectory()
	if home == "" {
		t.Skip("this process has no home directory")
	}
	if core.ExpandHomePath("~") != home {
		t.Errorf("~ expands to %q, wanted the home directory", core.ExpandHomePath("~"))
	}
	held := core.ExpandHomePath("~/masume/history")
	want := filepath.Join(home, "masume", "history")
	if held != want {
		t.Errorf("~/masume/history expands to %q, wanted %q", held, want)
	}
	if core.ExpandHomePath("/tmp/shop.db") != "/tmp/shop.db" {
		t.Error("an absolute path was rewritten")
	}
}

func TestResolveStatePathUsesXDGStateHome(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", "/tmp/state")
	held := core.ResolveStatePath("ai-chat.log")
	want := filepath.Join("/tmp/state", "masume", "ai-chat.log")
	if held != want {
		t.Errorf("the path reads %q, wanted %q", held, want)
	}
}

func TestResolveStatePathFallsBackBesideTheHomeDirectory(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", "")
	home := core.HomeDirectory()
	if home == "" {
		t.Skip("this process has no home directory")
	}
	held := core.ResolveStatePath("history.json")
	want := filepath.Join(home, ".local", "state", "masume", "history.json")
	if held != want {
		t.Errorf("the path reads %q, wanted %q", held, want)
	}
}
