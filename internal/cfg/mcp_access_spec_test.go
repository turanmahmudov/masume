package cfg_test

import (
	"testing"

	"github.com/turanmahmudov/masume/internal/cfg"
)

func TestResolveLowerAccessPicksTheStricterLevel(t *testing.T) {
	if held := cfg.ResolveLowerAccess(cfg.McpFull, cfg.McpReadOnly); held != cfg.McpReadOnly {
		t.Errorf("full and read-only resolve to %q", held)
	}
	if held := cfg.ResolveLowerAccess(cfg.McpReadOnly, cfg.McpFull); held != cfg.McpReadOnly {
		t.Errorf("the order of the arguments changed the level: %q", held)
	}
	if held := cfg.ResolveLowerAccess(cfg.McpOff, cfg.McpReadWrite); held != cfg.McpOff {
		t.Errorf("off and read-write resolve to %q", held)
	}
	if held := cfg.ResolveLowerAccess(cfg.McpReadWrite, cfg.McpReadWrite); held != cfg.McpReadWrite {
		t.Errorf("two equal levels resolve to %q", held)
	}
}

func TestFindMcpAccessReadsALevel(t *testing.T) {
	level, found := cfg.FindMcpAccess("read-write")
	if !found || level != cfg.McpReadWrite {
		t.Errorf("%q, found=%v", level, found)
	}
	if _, found := cfg.FindMcpAccess("write"); found {
		t.Error("an unknown level was read")
	}
}
