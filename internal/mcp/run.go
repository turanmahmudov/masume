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

// MCP startup diagnostics use stderr. stdout is reserved for protocol messages.

const serverName = "masume"

// RunServer serves an MCP client until the stream closes and returns an exit code.
func RunServer(argv []string, version string) int {
	configPath := cfg.ResolveConfigPath()
	if _, err := cfg.EnsureConfigFile(configPath); err != nil {
		fmt.Fprintf(os.Stderr, "%s mcp: config: %s\n", serverName, err.Error())
	}

	loaded := cfg.LoadConfigForWorkingDirectory(configPath)
	for _, problem := range loaded.Project.Problems {
		reportConfigLine(problem)
	}
	for _, problem := range loaded.Problems {
		reportConfigLine(problem.Describe())
	}
	for _, warning := range loaded.Warnings {
		reportConfigLine(warning.DescribeWarning())
	}

	scoped, check, argumentErr := ReadServerArguments(argv)
	if argumentErr != nil {
		fmt.Fprintf(os.Stderr, "%s mcp: %s\n", serverName, argumentErr.Error())
		fmt.Fprintln(os.Stderr, "run masume --help for usage")
		return 2
	}
	if scoped != "" && !namesProfile(loaded.Mcp, scoped) {
		fmt.Fprintf(os.Stderr, "%s mcp: profile %q is not listed under [mcp] profiles\n",
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

// serveClient processes one MCP client stream.
func serveClient(ctx context.Context, deps AccessDeps, version string) int {
	// A history failure does not stop startup.
	history, err := hist.Open(hist.DefaultPath())
	if err != nil {
		fmt.Fprintln(os.Stderr, serverName+" mcp: history: "+err.Error())
	}
	defer func() { _ = history.Close() }()

	asker := CreateAsker(LogEvent)
	tools := BuildTools(ToolDeps{
		AccessDeps: deps,
		Asker:      asker,
		Plans:      CreatePlanTokens(),
		// Record agent statements in the shared query history.
		RecordQuery: func(entry hist.HistoryEntry) { _ = history.Record(entry) },
	})

	// Serialize responses and confirmation requests on stdout.
	writing := sync.Mutex{}
	writeLine := func(line string) {
		writing.Lock()
		defer writing.Unlock()
		// Log output failures. Input EOF stops the server.
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

// ReadServerArguments parses the optional profile and check mode. Unknown arguments are errors.
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
				return "", false, errors.New("--profile requires a profile name")
			}
			at++
			scoped = argv[at]
		case strings.HasPrefix(argument, "--profile="):
			scoped = strings.TrimPrefix(argument, "--profile=")
			if scoped == "" {
				return "", false, errors.New("--profile requires a profile name")
			}
		default:
			return "", false, errors.New("unknown MCP argument: " + argument)
		}
	}
	return scoped, check, nil
}

// describeServing returns the profiles of the server, in the form used by `list_profiles`.
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

// reportConfigLine writes a prefixed configuration diagnostic to stderr.
func reportConfigLine(written string) {
	fmt.Fprintf(os.Stderr, "%s mcp: %s\n", serverName, written)
}

// describeStartedProfiles returns the first line of the log of the server.
func describeStartedProfiles(deps AccessDeps) string {
	if deps.ScopedProfile != "" {
		return deps.ScopedProfile
	}
	return strings.Join(deps.Config.Profiles, ", ")
}

// reportStart reports enabled profiles and the log path.
func reportStart(deps AccessDeps) {
	fmt.Fprintf(os.Stderr, "%s mcp: %s\n%s mcp: calls are logged to %s\n",
		serverName, describeServing(deps), serverName, ResolveLogPath())
}

// reportCheck checks enabled profiles and reports connection results.
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
