package main

import (
	"errors"
	"testing"

	"github.com/turanmahmudov/masume/internal/cfg"
	"github.com/turanmahmudov/masume/internal/core"
)

// noEnvironment is the reader used by a test that must not see $DATABASE_URL of the machine
// the test runs on.
func noEnvironment(string) string { return "" }

// buildEnvironment returns a reader of one variable, so a test states what the shell holds.
func buildEnvironment(name, value string) func(string) string {
	return func(asked string) string {
		if asked == name {
			return value
		}
		return ""
	}
}

// The command reads one connection target and the name of a profile, so both forms of
// opening a connection reach the client.
func TestParseArgumentsReadsATargetAndAProfileName(t *testing.T) {
	for _, one := range []struct {
		argv        []string
		target      string
		profileName string
	}{
		{nil, "", ""},
		{[]string{"postgres://ada@host/shop"}, "postgres://ada@host/shop", ""},
		{[]string{"./notes.db"}, "./notes.db", ""},
		{[]string{"--profile", "shop-prod"}, "", "shop-prod"},
		{[]string{"--profile=shop-prod"}, "", "shop-prod"},
		{[]string{"-p", "shop-prod"}, "", "shop-prod"},
	} {
		held, err := parseArguments(one.argv)
		if err != nil {
			t.Errorf("%v does not read: %v", one.argv, err)
			continue
		}
		if held.target != one.target {
			t.Errorf("%v opens target %q, wanted %q", one.argv, held.target, one.target)
		}
		if held.profileName != one.profileName {
			t.Errorf("%v opens profile %q, wanted %q",
				one.argv, held.profileName, one.profileName)
		}
	}
}

// A scan of the containers of this machine finds the connections itself, so it takes
// nothing else.
func TestParseArgumentsReadsTheDetectFlag(t *testing.T) {
	held, err := parseArguments([]string{"--detect"})
	if err != nil {
		t.Fatalf("--detect does not read: %v", err)
	}
	if !held.detect {
		t.Error("--detect does not ask for a scan")
	}
	if held.target != "" || held.profileName != "" {
		t.Error("--detect named a connection of its own")
	}
}

// An argument the command does not read must be reported. A misspelled flag that is taken
// for a connection target would open the picker and look as if it was accepted.
func TestParseArgumentsReportsWhatItCannotRead(t *testing.T) {
	for _, argv := range [][]string{
		{"--proflie=shop"},
		{"--detekt"},
		{"-x"},
		{"--profile"},
		{"--profile="},
		{"--profile", "--mcp"},
		{"postgres://ada@host/shop", "./notes.db"},
		{"--profile=shop", "postgres://ada@host/shop"},
		{"--detect", "postgres://ada@host/shop"},
		{"--detect", "--profile=shop"},
		{"", "./notes.db"},
		{"  "},
	} {
		if _, err := parseArguments(argv); err == nil {
			t.Errorf("%v was read as an invocation", argv)
		}
	}
}

// A name of a profile must open that profile of the config file and list the same profiles
// as before, because nothing was added.
func TestResolveStartProfileOpensTheNamedProfile(t *testing.T) {
	profiles := []cfg.Profile{{Name: "shop"}, {Name: "shop-prod"}}

	listed, start, err := resolveStartProfile(
		invocation{profileName: "shop-prod"}, profiles, noEnvironment)
	if err != nil {
		t.Fatalf("the profile does not open: %v", err)
	}
	if start == nil || start.Name != "shop-prod" {
		t.Fatalf("the client opens %v, wanted shop-prod", start)
	}
	if len(listed) != len(profiles) {
		t.Errorf("the picker lists %d profiles, wanted %d", len(listed), len(profiles))
	}
}

// A name that is in no profile must be reported before a screen opens, so the user reads it
// on the terminal they are looking at.
func TestResolveStartProfileReportsANameItCannotFind(t *testing.T) {
	_, _, err := resolveStartProfile(
		invocation{profileName: "staging"}, []cfg.Profile{{Name: "shop"}}, noEnvironment)
	if err == nil {
		t.Fatal("a profile that is not there was opened")
	}
	if _, isArgument := errors.AsType[argumentError](err); !isArgument {
		t.Errorf("the error is %T, wanted one that exits with 2", err)
	}
}

// A target must be added to the profiles the picker lists, so the connection it opened has
// a row, and the connection form can save it to the config file.
func TestResolveStartProfileListsTheTargetItOpened(t *testing.T) {
	profiles := []cfg.Profile{{Name: "ledger"}}

	listed, start, err := resolveStartProfile(
		invocation{target: "postgres://ada@db.internal/shop"}, profiles, noEnvironment)
	if err != nil {
		t.Fatalf("the target does not open: %v", err)
	}
	if start == nil {
		t.Fatal("the client opens no connection")
	}
	if start.Engine != core.EnginePostgres || start.Database != "shop" {
		t.Errorf("the client opens %s/%s, wanted postgres/shop", start.Engine, start.Database)
	}
	if len(listed) != 2 || listed[0].Name != start.Name {
		t.Fatalf("the picker lists %d profiles, wanted the target first of two", len(listed))
	}
	if listed[1].Name != "ledger" {
		t.Errorf("the picker lost the profile %q of the config file", "ledger")
	}
}

// A target that would take the name of a profile of the config file must be renamed, so the
// two rows of the picker can be told apart.
func TestResolveStartProfileKeepsTheNameOfTheConfigFile(t *testing.T) {
	profiles := []cfg.Profile{{Name: "shop", Host: "db.example.com"}}

	listed, start, err := resolveStartProfile(
		invocation{target: "postgres://ada@127.0.0.1/shop"}, profiles, noEnvironment)
	if err != nil {
		t.Fatalf("the target does not open: %v", err)
	}
	if start.Name != "shop-2" {
		t.Errorf("the target is named %q, wanted shop-2", start.Name)
	}
	if listed[1].Name != "shop" || listed[1].Host != "db.example.com" {
		t.Errorf("the profile of the config file reads %v, wanted the one it held", listed[1])
	}
}

// A target the client cannot read must be reported and must exit with 2.
func TestResolveStartProfileReportsATargetItCannotRead(t *testing.T) {
	_, _, err := resolveStartProfile(invocation{target: "ftp://host/shop"}, nil, noEnvironment)
	if err == nil {
		t.Fatal("a target that cannot be read was opened")
	}
	if _, isArgument := errors.AsType[argumentError](err); !isArgument {
		t.Errorf("the error is %T, wanted one that exits with 2", err)
	}
}

// A shell that exports a connection string must open it, which is how a client in a project
// directory opens the database of that project.
func TestResolveStartProfileOpensTheDatabaseURLOfTheShell(t *testing.T) {
	_, start, err := resolveStartProfile(invocation{}, nil,
		buildEnvironment(databaseURLVariable, "postgres://ada@db.internal/shop"))
	if err != nil {
		t.Fatalf("the variable does not open: %v", err)
	}
	if start == nil || start.Database != "shop" {
		t.Fatalf("the client opens %v, wanted the database of the variable", start)
	}
}

// A target on the command line is what the user asked for now, so it must win over a
// variable the shell exported earlier.
func TestResolveStartProfilePrefersTheTargetOverTheVariable(t *testing.T) {
	_, start, err := resolveStartProfile(invocation{target: "postgres://ada@host/ledger"}, nil,
		buildEnvironment(databaseURLVariable, "postgres://ada@db.internal/shop"))
	if err != nil {
		t.Fatalf("the target does not open: %v", err)
	}
	if start == nil || start.Database != "ledger" {
		t.Fatalf("the client opens %v, wanted the database of the argument", start)
	}
}

// With no target and no variable, the client opens the picker and nothing else.
func TestResolveStartProfileOpensNothingWithoutATarget(t *testing.T) {
	profiles := []cfg.Profile{{Name: "shop"}}

	listed, start, err := resolveStartProfile(invocation{}, profiles, noEnvironment)
	if err != nil {
		t.Fatalf("the client does not start: %v", err)
	}
	if start != nil {
		t.Errorf("the client opens %q, wanted the picker", start.Name)
	}
	if len(listed) != 1 || listed[0].Name != "shop" {
		t.Errorf("the picker lists %v, wanted the profiles of the config file", listed)
	}
}
