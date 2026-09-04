// Command masume is a database client for the terminal. It opens PostgreSQL, MySQL, SQLite,
// Redis and MongoDB, and the servers that speak their protocols.
//
// This is the only entry point, and the only place that chooses between the two clients: the
// screen for a user, or the protocol for an agent. Neither one starts before it is chosen.
package main

import (
	"errors"
	"fmt"
	"os"
	"runtime/debug"
	"slices"

	tea "charm.land/bubbletea/v2"

	"github.com/turanmahmudov/masume/internal/cfg"
	"github.com/turanmahmudov/masume/internal/db/engines"
	"github.com/turanmahmudov/masume/internal/hist"
	"github.com/turanmahmudov/masume/internal/mcp"
	"github.com/turanmahmudov/masume/internal/ui"
)

// version is what this client reports to the client of an agent, and what `--version`
// returns. A release build stamps it with `-ldflags "-X main.version=…"`; a build from the
// tree reads the revision the toolchain records instead.
var version = "dev"

// usage is what `--help` writes. It names every argument this command reads.
const usage = `masume - a database client for the terminal

usage:
  masume                     open the client
  masume run STATEMENT       run one statement, write the result, and exit
  masume URL                 open that connection, for example postgres://you@host/shop
  masume FILE                open that SQLite file, for example ./notes.db
  masume DSN                 open that connection string, for example "host=db dbname=shop"
  masume --profile NAME      open that profile of the config file
  masume --detect            list the databases running in a container on this machine
  masume --mcp               serve the profiles to an agent over JSON-RPC on stdio
  masume --mcp --profile=NAME  serve that one profile alone
  masume --mcp --check       open every profile once, report, and exit
  masume --version           write the version and exit
  masume --help              write this and exit

Run masume run --help for the arguments of a run without a screen.

A connection given on the command line is not written to the config file. Press
Ctrl+N and then e to save it. With no target, $DATABASE_URL is opened if it is set.

The config file is read from $XDG_CONFIG_HOME/masume/config.toml, and the history
from $XDG_STATE_HOME/masume/history.sqlite. A .masume.toml found from the working
directory upward adds the connections and the queries of a project.`

func main() {
	argv := os.Args[1:]
	// A subcommand reads its own arguments, so it is taken before the flags of the client.
	if len(argv) > 0 && argv[0] == "run" {
		os.Exit(runHeadless(argv[1:]))
	}
	if slices.Contains(argv, "--help") || slices.Contains(argv, "-h") {
		fmt.Println(usage)
		return
	}
	if slices.Contains(argv, "--version") || slices.Contains(argv, "-v") {
		fmt.Println("masume " + resolveVersion())
		return
	}
	if slices.Contains(argv, "--mcp") {
		os.Exit(mcp.RunServer(argv, resolveVersion()))
	}
	held, err := parseArguments(argv)
	if err == nil {
		err = runApp(held)
	}
	if err == nil {
		return
	}
	fmt.Fprintln(os.Stderr, "masume: "+err.Error())
	if _, isArgument := errors.AsType[argumentError](err); isArgument {
		fmt.Fprintln(os.Stderr, "run masume --help to see the arguments it reads")
		os.Exit(2)
	}
	os.Exit(1)
}

// resolveVersion returns what this build calls itself. A build that was stamped keeps its
// stamp; any other reads the revision of the tree it was built from.
func resolveVersion() string {
	if version != "dev" {
		return version
	}
	info, read := debug.ReadBuildInfo()
	if !read {
		return version
	}
	revision, modified := "", false
	for _, setting := range info.Settings {
		switch setting.Key {
		case "vcs.revision":
			revision = setting.Value
		case "vcs.modified":
			modified = setting.Value == "true"
		}
	}
	if revision == "" {
		return version
	}
	if len(revision) > 12 {
		revision = revision[:12]
	}
	if modified {
		return version + "+" + revision + "-dirty"
	}
	return version + "+" + revision
}

func runApp(held invocation) error {
	configPath := cfg.ResolveConfigPath()

	// Written to stderr and kept for the app to show. Only the app reaches the user, because
	// the renderer takes the alternate screen a moment later.
	problems := []string{}

	// A first run has no config file. Writing the starter gives the user something to edit
	// and the connection form something to write into. A client that cannot write it still
	// opens every connection it was given.
	if _, err := cfg.EnsureConfigFile(configPath); err != nil {
		problems = append(problems, "config: "+err.Error())
	}

	loaded := cfg.LoadConfigForWorkingDirectory(configPath)
	problems = append(problems, loaded.Project.Problems...)

	// Read before anything is opened, while the terminal still shows the shell.
	listed, start, err := resolveStartProfile(held, loaded.Profiles, readEnvironment)
	if err != nil {
		return err
	}
	loaded.Profiles = listed

	for _, problem := range loaded.Problems {
		problems = append(problems, problem.Describe())
	}
	for _, warning := range loaded.Warnings {
		problems = append(problems, warning.DescribeWarning())
	}
	for _, problem := range loaded.ThemeProblems {
		problems = append(problems, "theme: "+problem)
	}

	// The history file is opened whatever it returns: a client that cannot write its history
	// still opens every connection.
	historyStore, historyErr := hist.Open(hist.DefaultPath())
	if historyErr != nil {
		problems = append(problems, "history: "+historyErr.Error())
	}
	defer func() { _ = historyStore.Close() }()

	for _, problem := range problems {
		fmt.Fprintln(os.Stderr, problem)
	}

	model := ui.NewModel(loaded, engines.CreateAdapters(), historyStore, problems)
	if start != nil {
		model.OpenAtStart(*start)
	}
	_, runErr := tea.NewProgram(model).Run()
	return runErr
}
