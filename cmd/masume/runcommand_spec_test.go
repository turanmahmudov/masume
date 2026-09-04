package main

import (
	"os"
	"strings"
	"testing"

	"github.com/turanmahmudov/masume/internal/cfg"
	"github.com/turanmahmudov/masume/internal/core"
	"github.com/turanmahmudov/masume/internal/headless"
)

// The statement is the only positional argument of a run, and the connection stands before
// it where there are two.
func TestParseRunArgumentsPlacesThePositionalArguments(t *testing.T) {
	for _, one := range []struct {
		argv      []string
		target    string
		statement string
	}{
		{[]string{"select 1"}, "", "select 1"},
		{[]string{"./notes.db", "select 1"}, "./notes.db", "select 1"},
		{[]string{"postgres://ada@host/shop", "select 1"}, "postgres://ada@host/shop", "select 1"},
	} {
		held, err := parseRunArguments(one.argv)
		if err != nil {
			t.Errorf("%v does not read: %v", one.argv, err)
			continue
		}
		if held.target != one.target {
			t.Errorf("%v opens %q, wanted %q", one.argv, held.target, one.target)
		}
		if held.statement != one.statement {
			t.Errorf("%v runs %q, wanted %q", one.argv, held.statement, one.statement)
		}
	}
}

// Every flag has a long and a short form, and a value that follows it or one written with
// an equals sign, so a script writes it either way.
func TestParseRunArgumentsReadsEveryFlag(t *testing.T) {
	for _, argv := range [][]string{
		{"-p", "shop", "-f", "json", "-e", "daily.sql", "-l", "500",
			"--param", "day=2026-09-02", "--explain"},
		{"--profile=shop", "--format=json", "--execute=daily.sql", "--limit=500",
			"--param=day=2026-09-02", "--explain"},
	} {
		held, err := parseRunArguments(argv)
		if err != nil {
			t.Errorf("%v does not read: %v", argv, err)
			continue
		}
		if held.profileName != "shop" {
			t.Errorf("%v opens profile %q, wanted shop", argv, held.profileName)
		}
		if held.format != headless.FormatJSON {
			t.Errorf("%v writes %q, wanted json", argv, held.format)
		}
		if held.statementFile != "daily.sql" {
			t.Errorf("%v reads %q, wanted daily.sql", argv, held.statementFile)
		}
		if held.params["day"] != "2026-09-02" {
			t.Errorf("%v binds %v, wanted the day", argv, held.params)
		}
		if held.rowLimit != 500 {
			t.Errorf("%v returns %d rows, wanted 500", argv, held.rowLimit)
		}
		if !held.explain {
			t.Errorf("%v does not ask for the plan", argv)
		}
	}
}

// A run without a format writes a table, which is the format a person reads.
func TestParseRunArgumentsWritesATableByDefault(t *testing.T) {
	held, err := parseRunArguments([]string{"select 1"})
	if err != nil {
		t.Fatalf("the arguments do not read: %v", err)
	}
	if held.format != headless.FormatTable {
		t.Errorf("the format reads %q, wanted table", held.format)
	}
	if held.rowLimit != 0 {
		t.Errorf("the limit reads %d, wanted the page of the profile", held.rowLimit)
	}
}

// A parameter can be repeated, because a statement can hold several marks.
func TestParseRunArgumentsReadsEveryParameter(t *testing.T) {
	held, err := parseRunArguments([]string{
		"--param", "day=2026-09-02", "--param", "status=paid", "select 1"})
	if err != nil {
		t.Fatalf("the arguments do not read: %v", err)
	}
	if len(held.params) != 2 || held.params["status"] != "paid" {
		t.Errorf("the run binds %v, wanted both parameters", held.params)
	}
}

// A value with an equals sign in it belongs to the parameter, because a filter can hold one.
func TestParseRunArgumentsKeepsAnEqualsSignInAValue(t *testing.T) {
	held, err := parseRunArguments([]string{"--param", "filter=a=b", "select 1"})
	if err != nil {
		t.Fatalf("the arguments do not read: %v", err)
	}
	if held.params["filter"] != "a=b" {
		t.Errorf("the run binds %v, wanted the whole value", held.params)
	}
}

// A file and a statement on the command line are two statements, and a run takes one. Every
// other fault in the arguments is reported the same way.
func TestParseRunArgumentsReportsWhatItCannotRead(t *testing.T) {
	for _, argv := range [][]string{
		{},
		{"-p", "shop"},
		{"-f", "parquet", "select 1"},
		{"--format=parquet", "select 1"},
		{"-p", "shop", "./notes.db", "select 1"},
		{"-e", "daily.sql", "./notes.db", "select 1"},
		{"./notes.db", "select 1", "select 2"},
		{"--param", "day", "select 1"},
		{"--param", "=2026", "select 1"},
		{"-l", "0", "select 1"},
		{"-l", "-5", "select 1"},
		{"-l", "many", "select 1"},
		{"--limit=0", "select 1"},
		// A flag with nothing after its equals sign names nothing.
		{"--profile=", "select 1"},
		{"--execute="},
		{"--format=", "select 1"},
		{"-l"},
		{"-x", "select 1"},
		{"-f"},
		{"-e"},
	} {
		if _, err := parseRunArguments(argv); err == nil {
			t.Errorf("%v was read as a run", argv)
		}
	}
}

// The help is written for a run that asks for it, and it names no statement.
func TestParseRunArgumentsReadsTheHelp(t *testing.T) {
	held, err := parseRunArguments([]string{"--help"})
	if err != nil {
		t.Fatalf("--help does not read: %v", err)
	}
	if !held.help {
		t.Error("--help does not ask for the help")
	}
	if !strings.Contains(runUsage, "--explain") {
		t.Error("the help does not name every argument a run reads")
	}
}

// The statement comes from the command line, from a file, or from stdin, because a job
// writes it in whichever of the three suits it.
func TestReadStatementTextReadsEverySource(t *testing.T) {
	written, err := readStatementText(runInvocation{statement: "select 1"}, nil)
	if err != nil || written != "select 1" {
		t.Errorf("the command line reads %q, %v", written, err)
	}

	path := t.TempDir() + "/daily.sql"
	if err := writeFileForTest(path, "select 2"); err != nil {
		t.Fatalf("the file cannot be written: %v", err)
	}
	written, err = readStatementText(runInvocation{statementFile: path}, nil)
	if err != nil || written != "select 2" {
		t.Errorf("the file reads %q, %v", written, err)
	}

	written, err = readStatementText(
		runInvocation{statementFile: "-"}, strings.NewReader("select 3"))
	if err != nil || written != "select 3" {
		t.Errorf("stdin reads %q, %v", written, err)
	}

	if _, err = readStatementText(
		runInvocation{statementFile: path + ".gone"}, nil); err == nil {
		t.Error("a file that is not there was read")
	}
}

// The connection of a run comes from the same three places as the one of the client, so a
// profile, a target and the variable of the shell all work.
func TestResolveRunProfileReadsEverySource(t *testing.T) {
	profiles := []cfg.Profile{{Name: "shop", Database: "shop", InConfigFile: true}}

	profile, err := resolveRunProfile(
		runInvocation{profileName: "shop"}, profiles, noEnvironment)
	if err != nil || profile.Name != "shop" {
		t.Errorf("the profile reads %v, %v", profile.Name, err)
	}

	profile, err = resolveRunProfile(
		runInvocation{target: "postgres://ada@host/ledger"}, profiles, noEnvironment)
	if err != nil || profile.Database != "ledger" || profile.Engine != core.EnginePostgres {
		t.Errorf("the target reads %v, %v", profile, err)
	}

	profile, err = resolveRunProfile(runInvocation{}, profiles,
		buildEnvironment(databaseURLVariable, "postgres://ada@host/from-the-shell"))
	if err != nil || profile.Database != "from-the-shell" {
		t.Errorf("the variable reads %v, %v", profile, err)
	}
}

// A run with no connection anywhere must be reported, because it has nothing to open.
func TestResolveRunProfileReportsARunWithoutAConnection(t *testing.T) {
	if _, err := resolveRunProfile(runInvocation{}, nil, noEnvironment); err == nil {
		t.Fatal("a run without a connection was accepted")
	}
	if _, err := resolveRunProfile(
		runInvocation{profileName: "gone"}, nil, noEnvironment); err == nil {
		t.Fatal("a profile that is not there was accepted")
	}
}

// writeFileForTest writes one file, so a test reads a statement from it.
func writeFileForTest(path, written string) error {
	return os.WriteFile(path, []byte(written), 0o600)
}
