package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"

	"github.com/turanmahmudov/masume/internal/cfg"
	"github.com/turanmahmudov/masume/internal/db/engines"
	"github.com/turanmahmudov/masume/internal/headless"
)

// Reads `masume run`: the connection, the statement and the format. The run itself is in
// internal/headless, which draws nothing and returns the exit code.

// runUsage is what `masume run --help` writes.
const runUsage = `masume run - run one statement and write the result

usage:
  masume run [TARGET] STATEMENT
  masume run [TARGET] -e FILE

  TARGET                 a URL, a connection string, or a database file. With none,
                         --profile or $DATABASE_URL names the connection
  -p, --profile NAME     a profile of the config file
  -e, --execute FILE     read the statement from a file, or from - for stdin
  -f, --format FORMAT    table (the default), csv, json or markdown
  -l, --limit ROWS       how many rows to return. Without it, one page of the
                         profile, or every row a statement with its own limit asks for
      --param NAME=VALUE bind :NAME in the statement. Repeat for each one
      --explain          write the plan as JSON instead of running the statement
  -h, --help             write this and exit

exit codes:
  0 every statement ran
  1 the server refused a statement, or a parameter has no value
  2 the connection could not be opened
  3 the profile is read-only and the statement writes`

// runInvocation is what `masume run` was asked to do.
type runInvocation struct {
	target      string
	profileName string
	// The file the statement is read from. A single hyphen means stdin.
	statementFile string
	statement     string
	format        headless.Format
	// How many rows to return. Zero reads one page of the profile.
	rowLimit int
	params   map[string]any
	explain  bool
	help     bool
}

// readFlagText returns the value written after the equals sign of a flag.
func readFlagText(argument, prefix string) (string, error) {
	written := strings.TrimSpace(strings.TrimPrefix(argument, prefix))
	if written == "" {
		return "", failArgument(strings.TrimSuffix(prefix, "=") + " names nothing")
	}
	return written, nil
}

// readFlagValue returns the value that follows a flag, and the index it was read from.
func readFlagValue(argv []string, at int, name string) (string, int, error) {
	if at+1 >= len(argv) {
		return "", at, failArgument(name + " needs a value")
	}
	return argv[at+1], at + 1, nil
}

// parseRunParameter reads one `NAME=VALUE` pair into the values of the statement, with the
// name in lower case.
func parseRunParameter(written string, into map[string]any) error {
	name, value, cut := strings.Cut(written, "=")
	if !cut || strings.TrimSpace(name) == "" {
		return failArgument("--param takes NAME=VALUE, and " + written + " is not one")
	}
	into[strings.ToLower(strings.TrimSpace(name))] = value
	return nil
}

// parseRunArguments reads the arguments of `masume run`. The first positional is the
// connection where a second one follows it.
func parseRunArguments(argv []string) (runInvocation, error) {
	held := runInvocation{format: headless.FormatTable, params: map[string]any{}}
	positional := []string{}

	for at := 0; at < len(argv); at++ {
		argument := argv[at]
		var value string
		var err error

		switch {
		case argument == "--help" || argument == "-h":
			held.help = true
		case argument == "--explain":
			held.explain = true
		case argument == "--profile" || argument == "-p":
			if value, at, err = readFlagValue(argv, at, argument); err != nil {
				return runInvocation{}, err
			}
			held.profileName = value
		case strings.HasPrefix(argument, "--profile="):
			if held.profileName, err = readFlagText(
				argument, "--profile="); err != nil {
				return runInvocation{}, err
			}
		case argument == "--execute" || argument == "-e":
			if value, at, err = readFlagValue(argv, at, argument); err != nil {
				return runInvocation{}, err
			}
			held.statementFile = value
		case strings.HasPrefix(argument, "--execute="):
			if held.statementFile, err = readFlagText(
				argument, "--execute="); err != nil {
				return runInvocation{}, err
			}
		case argument == "--limit" || argument == "-l":
			if value, at, err = readFlagValue(argv, at, argument); err != nil {
				return runInvocation{}, err
			}
			if held.rowLimit, err = readRunLimit(value); err != nil {
				return runInvocation{}, err
			}
		case strings.HasPrefix(argument, "--limit="):
			if held.rowLimit, err = readRunLimit(
				strings.TrimPrefix(argument, "--limit=")); err != nil {
				return runInvocation{}, err
			}
		case argument == "--format" || argument == "-f":
			if value, at, err = readFlagValue(argv, at, argument); err != nil {
				return runInvocation{}, err
			}
			if held.format, err = readRunFormat(value); err != nil {
				return runInvocation{}, err
			}
		case strings.HasPrefix(argument, "--format="):
			if held.format, err = readRunFormat(
				strings.TrimPrefix(argument, "--format=")); err != nil {
				return runInvocation{}, err
			}
		case argument == "--param":
			if value, at, err = readFlagValue(argv, at, argument); err != nil {
				return runInvocation{}, err
			}
			if err = parseRunParameter(value, held.params); err != nil {
				return runInvocation{}, err
			}
		case strings.HasPrefix(argument, "--param="):
			if err = parseRunParameter(
				strings.TrimPrefix(argument, "--param="), held.params); err != nil {
				return runInvocation{}, err
			}
		case strings.HasPrefix(argument, "-") && argument != "-":
			return runInvocation{}, failArgument(
				argument + " is not an argument masume run reads")
		default:
			positional = append(positional, argument)
		}
	}
	if held.help {
		return held, nil
	}
	return finishRunInvocation(held, positional)
}

// readRunLimit parses the value of `--limit`.
func readRunLimit(written string) (int, error) {
	rows, err := strconv.Atoi(strings.TrimSpace(written))
	if err != nil || rows < 1 {
		return 0, failArgument("--limit must be a positive number of rows, and not " + written)
	}
	return rows, nil
}

// readRunFormat parses the value of `--format`.
func readRunFormat(written string) (headless.Format, error) {
	format, known := headless.FindFormat(written)
	if !known {
		return "", failArgument(
			"--format must be one of " + headless.FormatNames() + ", and not " + written)
	}
	return format, nil
}

// finishRunInvocation places the positional arguments and reports a run that names no
// statement or two connections.
func finishRunInvocation(held runInvocation, positional []string) (runInvocation, error) {
	switch len(positional) {
	case 0:
	case 1:
		if held.statementFile != "" {
			held.target = positional[0]
		} else {
			held.statement = positional[0]
		}
	case 2:
		if held.statementFile != "" {
			return runInvocation{}, failArgument(
				"-e reads the statement, so " + positional[1] + " is a second one")
		}
		held.target, held.statement = positional[0], positional[1]
	default:
		return runInvocation{}, failArgument(
			"masume run reads one connection and one statement, and " +
				strconv.Itoa(len(positional)) + " arguments are more than that")
	}

	if held.target != "" && held.profileName != "" {
		return runInvocation{}, failArgument(
			"--profile and a connection target are two ways to name one connection")
	}
	if held.statement == "" && held.statementFile == "" {
		return runInvocation{}, failArgument("masume run needs a statement, or -e to read one")
	}
	return held, nil
}

// readStatementText returns the statement of the run: the one on the command line, or the
// text of the file, or what stdin holds.
func readStatementText(held runInvocation, stdin io.Reader) (string, error) {
	if held.statementFile == "" {
		return held.statement, nil
	}
	if held.statementFile == "-" {
		written, err := io.ReadAll(stdin)
		if err != nil {
			return "", fmt.Errorf("the statement could not be read from stdin: %w", err)
		}
		return string(written), nil
	}
	written, err := os.ReadFile(held.statementFile)
	if err != nil {
		return "", fmt.Errorf("the statement could not be read: %w", err)
	}
	return string(written), nil
}

// resolveRunProfile returns the connection the run opens: the profile of the config file,
// the target of the command line, or the one $DATABASE_URL names.
func resolveRunProfile(
	held runInvocation, profiles []cfg.Profile, environment func(string) string,
) (cfg.Profile, error) {
	_, start, err := resolveStartProfile(
		invocation{target: held.target, profileName: held.profileName},
		profiles, environment)
	if err != nil {
		return cfg.Profile{}, err
	}
	if start == nil {
		return cfg.Profile{}, failArgument(
			"masume run needs a connection: a target, --profile, or $DATABASE_URL")
	}
	return *start, nil
}

// runHeadless runs `masume run` and returns the exit code of the process.
func runHeadless(argv []string) int {
	held, err := parseRunArguments(argv)
	if err != nil {
		fmt.Fprintln(os.Stderr, "masume: "+err.Error())
		fmt.Fprintln(os.Stderr, "run masume run --help to see the arguments it reads")
		return 2
	}
	if held.help {
		fmt.Println(runUsage)
		return 0
	}

	loaded := cfg.LoadConfig(cfg.ResolveConfigPath())
	profile, err := resolveRunProfile(held, loaded.Profiles, readEnvironment)
	if err != nil {
		fmt.Fprintln(os.Stderr, "masume: "+err.Error())
		return 2
	}

	written, err := readStatementText(held, os.Stdin)
	if err != nil {
		fmt.Fprintln(os.Stderr, "masume: "+err.Error())
		return 2
	}

	password, err := cfg.ResolveProfilePassword(profile)
	if err != nil {
		fmt.Fprintln(os.Stderr, "masume: "+err.Error())
		return headless.CodeConnection
	}

	return headless.Run(context.Background(), engines.CreateAdapters(), headless.Options{
		Profile: profile, Password: password, Statement: written,
		Format: held.format, RowLimit: held.rowLimit,
		Params: held.params, Explain: held.explain,
		Out: os.Stdout, Err: os.Stderr,
	})
}
