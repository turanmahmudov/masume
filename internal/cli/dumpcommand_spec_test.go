package cli

import (
	"strings"
	"testing"

	"github.com/turanmahmudov/masume/internal/dump"
)

func TestParseDumpArgumentsReadsTheFileAndTheSettings(t *testing.T) {
	held, err := parseDumpArguments([]string{
		"postgres://you@host/shop", "shop.sql",
		"--schema", "public", "-t", "orders", "--table=audit.log",
		"--content", "schema only", "--drop",
	}, "dump")
	if err != nil {
		t.Fatal(err)
	}
	if held.target != "postgres://you@host/shop" || held.path != "shop.sql" {
		t.Fatalf("target %q and file %q", held.target, held.path)
	}
	if held.schema != "public" || held.content != dump.ContentSchema || !held.drops {
		t.Fatalf("settings: %+v", held)
	}

	tables := buildDumpTables(held)
	if len(tables) != 2 {
		t.Fatalf("tables: %v, want both", tables)
	}
	if tables[0].Schema != "public" || tables[0].Name != "orders" {
		t.Errorf("table 0: %+v, want the schema of the dump", tables[0])
	}
	if tables[1].Schema != "audit" || tables[1].Name != "log" {
		t.Errorf("table 1: %+v, want the schema the name holds", tables[1])
	}
}

func TestParseDumpArgumentsRefusesWhatItCannotRun(t *testing.T) {
	for _, held := range []struct {
		name   string
		argv   []string
		reason string
	}{
		{"no file", []string{"--schema", "public"}, "requires a file"},
		{"a profile and a target", []string{"-p", "shop", "postgres://host/db", "shop.sql"},
			"not both"},
		{"three arguments", []string{"a", "b", "c"}, "at most two"},
		{"an unknown option", []string{"--rows", "shop.sql"}, "unknown"},
		{"an unknown content", []string{"--content", "everything", "shop.sql"},
			"--content must be one of"},
		{"a flag with no value", []string{"--schema"}, "requires a value"},
	} {
		t.Run(held.name, func(t *testing.T) {
			_, err := parseDumpArguments(held.argv, "dump")
			if err == nil {
				t.Fatalf("%v was taken", held.argv)
			}
			if !strings.Contains(err.Error(), held.reason) {
				t.Errorf("error %q, want %q in it", err, held.reason)
			}
		})
	}
}

func TestParseDumpArgumentsTakesTheHelpFlagAlone(t *testing.T) {
	held, err := parseDumpArguments([]string{"--help"}, "dump")
	if err != nil || !held.help {
		t.Fatalf("help: %v, error: %v", held.help, err)
	}
}

func TestParseDumpArgumentsDefaultsToEveryPart(t *testing.T) {
	held, err := parseDumpArguments([]string{"shop.sql"}, "dump")
	if err != nil {
		t.Fatal(err)
	}
	if held.content != dump.ContentAll || held.drops {
		t.Fatalf("settings: %+v, want the whole schema and no drop", held)
	}
	if held.path != "shop.sql" || held.target != "" {
		t.Fatalf("file %q and target %q", held.path, held.target)
	}
}

// The usage text names every flag the parser reads, so the help and the parser cannot drift
// apart.
func TestDumpUsageNamesEveryFlag(t *testing.T) {
	for _, flag := range []string{
		"--profile", "--schema", "--table", "--content", "--drop", "--help",
	} {
		if !strings.Contains(dumpUsage, flag) {
			t.Errorf("masume dump --help names no %s", flag)
		}
	}
	for _, flag := range []string{"--profile", "--help"} {
		if !strings.Contains(restoreUsage, flag) {
			t.Errorf("masume restore --help names no %s", flag)
		}
	}
	if strings.Contains(restoreUsage, "--drop") {
		t.Error("masume restore --help names a flag it refuses")
	}
}
