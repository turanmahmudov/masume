package cli

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

// Parse masume run arguments. internal/headless runs the statements and returns an exit code.

// runUsage is what `masume run --help` writes.
const runUsage = `masume run - run statements and write results

usage:
  masume run [TARGET] STATEMENT
  masume run [TARGET] -e FILE

  TARGET                 a supported URL, keyword connection string, or existing SQLite file
  -p, --profile NAME     a user or project profile; without a target or profile, use $DATABASE_URL
  -e, --execute FILE     read the statement from a file, or from - for stdin
  -f, --format FORMAT    table (the default), csv, json or markdown
  -l, --limit ROWS       positive output cap per statement, including statements with SQL limits
      --param NAME=VALUE string value for :NAME; repeat for each parameter
      --explain          write a JSON plan; run eligible queries for measurements
  -h, --help             print this help and exit

exit codes:
  0 run completed, possibly with capped output
  1 statement, parameter, plan or output failure; empty input; incomplete write results
  2 argument, input file, password or connection failure
  3 the profile is read-only and the statement writes

Without --limit, reads use page_size unless the statement has its own limit.
Headless writes have no confirmation, write plan or undo. Use explicit transactions for atomic batches.
Exit 1 does not prove a write failed. Do not automatically retry writes.`

// runInvocation is the parsed headless request.
type runInvocation struct {
	target      string
	profileName string
	// The file the statement is read from. A single hyphen means stdin.
	statementFile string
	statement     string
	format        headless.Format
	// Row limit. Zero uses the profile page size unless the statement has a limit.
	rowLimit int
	params   map[string]any
	explain  bool
	help     bool
}

// runShortFlagNames are the short flags of masume run that take a value, and the long flag of each.
var runShortFlagNames = map[string]string{
	"-p": "--profile", "-e": "--execute", "-l": "--limit", "-f": "--format",
}

// readFlagText returns the value written after the equals sign of a flag.
func readFlagText(argument, prefix string) (string, error) {
	written := strings.TrimSpace(strings.TrimPrefix(argument, prefix))
	if written == "" {
		return "", failArgument(strings.TrimSuffix(prefix, "=") + " requires a value")
	}
	return written, nil
}

// readFlagValue returns the value that follows a flag, and the index it was read from.
func readFlagValue(argv []string, at int, name string) (string, int, error) {
	if at+1 >= len(argv) {
		return "", at, failArgument(name + " requires a value")
	}
	return argv[at+1], at + 1, nil
}

// parseRunParameter parses one NAME=VALUE pair with a lowercase parameter name.
func parseRunParameter(written string, into map[string]any) error {
	name, value, cut := strings.Cut(written, "=")
	if !cut || strings.TrimSpace(name) == "" {
		return failArgument("--param requires NAME=VALUE; invalid parameter: " + written)
	}
	into[strings.ToLower(strings.TrimSpace(name))] = value
	return nil
}

// parseRunArguments parses masume run arguments. With two positional arguments, the first is the connection target.
func parseRunArguments(argv []string) (runInvocation, error) {
	held := runInvocation{format: headless.FormatTable, params: map[string]any{}}
	positional := []string{}

	for at := 0; at < len(argv); at++ {
		argument := expandShortFlag(argv[at], runShortFlagNames)
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
				"unknown masume run option: " + argument)
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
		return 0, failArgument("--limit requires a positive integer; invalid value: " + written)
	}
	return rows, nil
}

// readRunFormat parses the value of `--format`.
func readRunFormat(written string) (headless.Format, error) {
	format, known := headless.FindFormat(written)
	if !known {
		return "", failArgument(
			"--format must be one of " + headless.FormatNames() + "; invalid value: " + written)
	}
	return format, nil
}

// finishRunInvocation validates positional arguments and the statement source.
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
				"-e cannot be combined with a statement argument: " + positional[1])
		}
		held.target, held.statement = positional[0], positional[1]
	default:
		return runInvocation{}, failArgument(
			"masume run accepts at most two positional arguments; received " +
				strconv.Itoa(len(positional)))
	}

	if held.target != "" && held.profileName != "" {
		return runInvocation{}, failArgument(
			"use either --profile or a connection target, not both")
	}
	if held.statement == "" && held.statementFile == "" {
		return runInvocation{}, failArgument("masume run requires a statement or -e FILE")
	}
	return held, nil
}

// readStatementText reads the statement from an argument, a file, or stdin.
func readStatementText(held runInvocation, stdin io.Reader) (string, error) {
	if held.statementFile == "" {
		return held.statement, nil
	}
	if held.statementFile == "-" {
		written, err := io.ReadAll(stdin)
		if err != nil {
			return "", fmt.Errorf("cannot read the statement from stdin: %w", err)
		}
		return string(written), nil
	}
	written, err := os.ReadFile(held.statementFile)
	if err != nil {
		return "", fmt.Errorf("cannot read the statement file: %w", err)
	}
	return string(written), nil
}

// resolveRunProfile resolves a configured profile, a connection target, or $DATABASE_URL.
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
			"masume run requires a connection: a target, --profile, or $DATABASE_URL")
	}
	return *start, nil
}

// runHeadless runs `masume run` and returns the exit code of the process.
func runHeadless(argv []string) int {
	held, err := parseRunArguments(argv)
	if err != nil {
		fmt.Fprintln(os.Stderr, "masume: "+err.Error())
		fmt.Fprintln(os.Stderr, "run masume run --help for usage")
		return 2
	}
	if held.help {
		fmt.Println(runUsage)
		return 0
	}

	loaded := cfg.LoadConfigForWorkingDirectory(cfg.ResolveConfigPath())
	for _, problem := range loaded.Project.Problems {
		fmt.Fprintln(os.Stderr, "masume: "+problem)
	}
	// Report invalid profiles before resolving the requested profile.
	for _, problem := range loaded.Problems {
		fmt.Fprintln(os.Stderr, "masume: "+problem.Describe())
	}
	for _, warning := range loaded.Warnings {
		fmt.Fprintln(os.Stderr, "masume: "+warning.DescribeWarning())
	}
	profile, err := resolveRunProfile(held, loaded.Profiles, os.Getenv)
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
