package mcp

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"

	"github.com/turanmahmudov/masume/internal/cfg"
	"github.com/turanmahmudov/masume/internal/db/engines"
	"github.com/turanmahmudov/masume/internal/hist"
)

// The client without a screen: the same profiles and the same reads, served to an agent over
// the Model Context Protocol. Only the protocol writes to standard output, so every message of
// this file goes to standard error.

const serverName = "masume"

// RunServer serves an agent until the client closes its end, and returns the code the process
// exits with.
func RunServer(argv []string, version string) int {
	loaded := cfg.LoadConfig(cfg.ResolveConfigPath())
	for _, problem := range loaded.Problems {
		fmt.Fprintf(os.Stderr, "skipped profile %q: %s\n", problem.Name, problem.Reason)
	}

	scoped, check, argumentErr := ReadServerArguments(argv)
	if argumentErr != nil {
		fmt.Fprintf(os.Stderr, "%s mcp: %s\n", serverName, argumentErr.Error())
		fmt.Fprintln(os.Stderr, "run masume --help to see the arguments it reads")
		return 2
	}
	if scoped != "" && !namesProfile(loaded.Mcp, scoped) {
		fmt.Fprintf(os.Stderr, "%s mcp: profile %q is not named under [mcp] profiles\n",
			serverName, scoped)
		return 1
	}

	sessions := CreateSessions(engines.CreateAdapters())
	defer sessions.CloseAll()
	deps := AccessDeps{
		Profiles: loaded.Profiles, Config: loaded.Mcp,
		Sessions: sessions, ScopedProfile: scoped,
	}

	ctx := context.Background()
	if check {
		return reportCheck(ctx, deps)
	}
	return serveClient(ctx, deps, version)
}

// serveClient returns every message of one client, from the first to the closed stream.
func serveClient(ctx context.Context, deps AccessDeps, version string) int {
	// The history file is opened whatever it returns: a server that cannot write its history
	// still opens every connection.
	history, err := hist.Open(hist.DefaultPath())
	if err != nil {
		fmt.Fprintln(os.Stderr, serverName+" mcp: history: "+err.Error())
	}
	defer func() { _ = history.Close() }()

	asker := CreateAsker(LogEvent)
	tools := BuildTools(ToolDeps{
		AccessDeps: deps,
		Asker:      asker,
		// The statements of the agent go into the same history the screens read, so the user
		// can see later what ran.
		RecordQuery: func(entry hist.HistoryEntry) { _ = history.Record(entry) },
	})

	// One line at a time, because a question to the user is written beside an answer.
	writing := sync.Mutex{}
	writeLine := func(line string) {
		writing.Lock()
		defer writing.Unlock()
		// A write that fails means the client closed its end. The read of the next message
		// ends the server, so this only records why.
		if _, err := fmt.Fprintln(os.Stdout, line); err != nil {
			LogEvent("! cannot write to standard output: " + err.Error())
		}
	}
	asker.AttachWriter(func(message any) { writeLine(buildJSONLine(message)) })

	reportStart(deps)
	LogEvent("= started " + describeStartedProfiles(deps))

	ServeOverStdio(ctx, CreateResponder(ResponderDeps{
		Tools:    tools,
		Info:     ServerInfo{Name: serverName, Version: version},
		Asker:    asker,
		LogEvent: LogEvent,
	}), os.Stdin, writeLine)
	return 0
}

// ReadServerArguments returns the profile the server was started for and whether it was
// asked to check and exit. An argument it cannot read is a fault, not a default: a mistyped
// `--profile` would otherwise open every profile the config file names.
func ReadServerArguments(argv []string) (string, bool, error) {
	scoped, check := "", false
	for at := 0; at < len(argv); at++ {
		argument := argv[at]
		switch {
		case argument == "--mcp":
		case argument == "--check":
			check = true
		case argument == "--profile":
			if at+1 >= len(argv) || strings.HasPrefix(argv[at+1], "-") {
				return "", false, errors.New("--profile needs the name of a profile")
			}
			at++
			scoped = argv[at]
		case strings.HasPrefix(argument, "--profile="):
			scoped = strings.TrimPrefix(argument, "--profile=")
			if scoped == "" {
				return "", false, errors.New("--profile= names no profile")
			}
		default:
			return "", false, errors.New(argument + " is not an argument this command reads")
		}
	}
	return scoped, check, nil
}

// describeServing writes what the server serves, read the way `list_profiles` reads it.
func describeServing(deps AccessDeps) string {
	if deps.ScopedProfile != "" {
		profile, found := findProfileNamed(deps.Profiles, deps.ScopedProfile)
		if !found {
			return fmt.Sprintf("profile %q is not in the config file", deps.ScopedProfile)
		}
		if closed := FindClosedReason(deps.Config, profile); closed != "" {
			return closed
		}
		return fmt.Sprintf("serving profile %q", deps.ScopedProfile)
	}

	open := ListOpenProfiles(deps)
	if len(open) == 0 {
		return DescribeNoOpenProfiles(deps.Config)
	}
	names := make([]string, 0, len(open))
	for _, profile := range open {
		names = append(names, profile.Name)
	}
	return "serving " + strings.Join(names, ", ")
}

// describeStartedProfiles names what the log of the server opens with.
func describeStartedProfiles(deps AccessDeps) string {
	if deps.ScopedProfile != "" {
		return deps.ScopedProfile
	}
	return strings.Join(deps.Config.Profiles, ", ")
}

// reportStart writes what the server opened, so the log of a client shows what the config
// found.
func reportStart(deps AccessDeps) {
	fmt.Fprintf(os.Stderr, "%s mcp: %s\n%s mcp: calls are logged to %s\n",
		serverName, describeServing(deps), serverName, ResolveLogPath())
}

// reportCheck connects to every open profile once and reports, without waiting for a client.
func reportCheck(ctx context.Context, deps AccessDeps) int {
	checks := CheckOpenProfiles(ctx, deps)
	if len(checks) == 0 {
		fmt.Fprintf(os.Stderr, "%s mcp: %s\n", serverName, DescribeNoOpenProfiles(deps.Config))
		return 1
	}

	failed := 0
	for _, check := range checks {
		fmt.Fprintln(os.Stderr, DescribeCheck(check))
		if check.Problem != "" {
			failed = 1
		}
	}
	return failed
}
