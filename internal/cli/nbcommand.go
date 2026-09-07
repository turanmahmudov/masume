package cli

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/turanmahmudov/masume/internal/cfg"
	"github.com/turanmahmudov/masume/internal/db/engines"
	"github.com/turanmahmudov/masume/internal/headless"
	"github.com/turanmahmudov/masume/internal/notebook"
)

// `masume nb run` runs a notebook file without a screen. internal/headless runs the cells
// and returns an exit code.

// notebookUsage is what `masume nb --help` writes.
const notebookUsage = `masume nb - run a notebook without a screen

usage:
  masume nb run [TARGET] FILE

  TARGET                 a supported URL, keyword connection string, or existing SQLite file
  -p, --profile NAME     a user or project profile; without a target or profile, use $DATABASE_URL
  -f, --format FORMAT    table (the default), csv, json or markdown
  -l, --limit ROWS       positive output cap per statement
      --param NAME=VALUE string value for :NAME; it replaces the value of a parameter cell
      --only CELL        run only the cell of that id; repeat for each cell
      --explain          write a JSON plan of every statement, and run none of them
      --allow-writes     run the cells that write
  -h, --help             print this help and exit

exit codes:
  0 run completed
  1 a cell failed, or a write cell ran without --allow-writes
  2 argument, input file, password or connection failure
  3 the profile is read-only and a cell writes

markdown output writes the whole notebook with the rows of every cell.
A run without a screen has no write confirmation, no write plan and no undo.`

// notebookInvocation is the parsed notebook request.
type notebookInvocation struct {
	target      string
	profileName string
	path        string
	format      headless.Format
	rowLimit    int
	params      map[string]any
	only        []string
	explain     bool
	allowWrites bool
	help        bool
}

// notebookShortFlagNames are the short flags that take a value, and the long flag of each.
var notebookShortFlagNames = map[string]string{
	"-p": "--profile", "-l": "--limit", "-f": "--format",
}

// parseNotebookArguments parses the arguments of `masume nb run`.
func parseNotebookArguments(argv []string) (notebookInvocation, error) {
	held := notebookInvocation{
		format: headless.FormatTable, params: map[string]any{},
	}
	positional := []string{}

	for at := 0; at < len(argv); at++ {
		argument := expandShortFlag(argv[at], notebookShortFlagNames)
		var value string
		var err error

		switch {
		case argument == "--help" || argument == "-h":
			held.help = true
		case argument == "--allow-writes":
			held.allowWrites = true
		case argument == "--explain":
			held.explain = true
		case argument == "--profile" || argument == "-p":
			if value, at, err = readFlagValue(argv, at, argument); err != nil {
				return notebookInvocation{}, err
			}
			held.profileName = value
		case strings.HasPrefix(argument, "--profile="):
			if held.profileName, err = readFlagText(argument, "--profile="); err != nil {
				return notebookInvocation{}, err
			}
		case argument == "--format" || argument == "-f":
			if value, at, err = readFlagValue(argv, at, argument); err != nil {
				return notebookInvocation{}, err
			}
			if held.format, err = readRunFormat(value); err != nil {
				return notebookInvocation{}, err
			}
		case strings.HasPrefix(argument, "--format="):
			if held.format, err = readRunFormat(
				strings.TrimPrefix(argument, "--format=")); err != nil {
				return notebookInvocation{}, err
			}
		case argument == "--limit" || argument == "-l":
			if value, at, err = readFlagValue(argv, at, argument); err != nil {
				return notebookInvocation{}, err
			}
			if held.rowLimit, err = readRunLimit(value); err != nil {
				return notebookInvocation{}, err
			}
		case strings.HasPrefix(argument, "--limit="):
			if held.rowLimit, err = readRunLimit(
				strings.TrimPrefix(argument, "--limit=")); err != nil {
				return notebookInvocation{}, err
			}
		case argument == "--param":
			if value, at, err = readFlagValue(argv, at, argument); err != nil {
				return notebookInvocation{}, err
			}
			if err = parseRunParameter(value, held.params); err != nil {
				return notebookInvocation{}, err
			}
		case strings.HasPrefix(argument, "--param="):
			if err = parseRunParameter(
				strings.TrimPrefix(argument, "--param="), held.params); err != nil {
				return notebookInvocation{}, err
			}
		case argument == "--only":
			if value, at, err = readFlagValue(argv, at, argument); err != nil {
				return notebookInvocation{}, err
			}
			held.only = append(held.only, value)
		case strings.HasPrefix(argument, "--only="):
			if value, err = readFlagText(argument, "--only="); err != nil {
				return notebookInvocation{}, err
			}
			held.only = append(held.only, value)
		case strings.HasPrefix(argument, "-") && argument != "-":
			return notebookInvocation{}, failArgument(
				"unknown masume nb option: " + argument)
		default:
			positional = append(positional, argument)
		}
	}
	if held.help {
		return held, nil
	}
	return finishNotebookInvocation(held, positional)
}

// finishNotebookInvocation checks the positional arguments of `masume nb run`.
func finishNotebookInvocation(
	held notebookInvocation, positional []string,
) (notebookInvocation, error) {
	switch len(positional) {
	case 1:
		held.path = positional[0]
	case 2:
		held.target, held.path = positional[0], positional[1]
	case 0:
		return notebookInvocation{}, failArgument("masume nb run requires a notebook file")
	default:
		return notebookInvocation{}, failArgument(
			"masume nb run accepts at most two positional arguments; received " +
				strconv.Itoa(len(positional)))
	}
	if held.target != "" && held.profileName != "" {
		return notebookInvocation{}, failArgument(
			"use either --profile or a connection target, not both")
	}
	return held, nil
}

// runNotebookCommand runs `masume nb` and returns the exit code of the process.
func runNotebookCommand(argv []string) int {
	if len(argv) > 0 && (argv[0] == "--help" || argv[0] == "-h") {
		fmt.Println(notebookUsage)
		return 0
	}
	if len(argv) == 0 || argv[0] != "run" {
		fmt.Fprintln(os.Stderr, "masume: masume nb takes one command: run")
		fmt.Fprintln(os.Stderr, "run masume nb --help for usage")
		return 2
	}

	held, err := parseNotebookArguments(argv[1:])
	if err != nil {
		fmt.Fprintln(os.Stderr, "masume: "+err.Error())
		fmt.Fprintln(os.Stderr, "run masume nb --help for usage")
		return 2
	}
	if held.help {
		fmt.Println(notebookUsage)
		return 0
	}

	loaded := cfg.LoadConfigForWorkingDirectory(cfg.ResolveConfigPath())
	for _, problem := range loaded.Project.Problems {
		fmt.Fprintln(os.Stderr, "masume: "+problem)
	}
	for _, problem := range loaded.Problems {
		fmt.Fprintln(os.Stderr, "masume: "+problem.Describe())
	}

	book, err := notebook.Read(held.path)
	if err != nil {
		fmt.Fprintln(os.Stderr, "masume: cannot read the notebook: "+err.Error())
		return 2
	}
	for _, problem := range book.Problems {
		fmt.Fprintln(os.Stderr, "masume: "+problem)
	}
	if len(book.Cells) == 0 {
		fmt.Fprintln(os.Stderr, "masume: this notebook holds no cell")
		return headless.CodeStatement
	}

	profile, err := resolveNotebookProfile(held, book, loaded.Profiles)
	if err != nil {
		fmt.Fprintln(os.Stderr, "masume: "+err.Error())
		return 2
	}

	password, err := cfg.ResolveProfilePassword(profile)
	if err != nil {
		fmt.Fprintln(os.Stderr, "masume: "+err.Error())
		return headless.CodeConnection
	}

	return headless.RunNotebook(
		context.Background(), engines.CreateAdapters(), headless.NotebookOptions{
			Options: headless.Options{
				Profile: profile, Password: password, Format: held.format,
				RowLimit: held.rowLimit, Params: held.params, Explain: held.explain,
				Out: os.Stdout, Err: os.Stderr,
			},
			Notebook: book, Only: held.only, AllowWrites: held.allowWrites,
		})
}

// resolveNotebookProfile resolves the profile of the run. The profiles of the front matter
// are a suggestion, so a named profile that is not among them is reported and used.
func resolveNotebookProfile(
	held notebookInvocation, book notebook.Notebook, profiles []cfg.Profile,
) (cfg.Profile, error) {
	_, start, err := resolveStartProfile(
		invocation{target: held.target, profileName: held.profileName},
		profiles, os.Getenv)
	if err != nil {
		return cfg.Profile{}, err
	}
	if start == nil {
		return cfg.Profile{}, failArgument(
			"masume nb run requires a connection: a target, --profile, or $DATABASE_URL")
	}
	if !holdsProfile(book.Profiles, start.Name) {
		fmt.Fprintln(os.Stderr, "masume: this notebook names the profiles "+
			strings.Join(book.Profiles, ", ")+" and the run uses "+start.Name)
	}
	return *start, nil
}

// holdsProfile is true where the notebook offers itself on that profile. A notebook that
// names none is offered on every profile.
func holdsProfile(named []string, name string) bool {
	if len(named) == 0 {
		return true
	}
	for _, held := range named {
		if held == name {
			return true
		}
	}
	return false
}
