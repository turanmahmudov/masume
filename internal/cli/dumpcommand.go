package cli

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/turanmahmudov/masume/internal/cfg"
	"github.com/turanmahmudov/masume/internal/db"
	"github.com/turanmahmudov/masume/internal/db/engines"
	"github.com/turanmahmudov/masume/internal/dump"
	"github.com/turanmahmudov/masume/internal/headless"
)

// `masume dump` writes a schema as SQL and `masume restore` runs such a file back into a
// server. internal/headless opens the connection and returns an exit code.

// dumpUsage is what `masume dump --help` writes.
const dumpUsage = `masume dump - write a schema as SQL

usage:
  masume dump [TARGET] FILE
  masume dump [TARGET] -

  TARGET                 a supported URL, keyword connection string, or existing SQLite file
  FILE                   the file the dump is written to; a single - writes to stdout
  -p, --profile NAME     a user or project profile; without a target or profile, use $DATABASE_URL
  -s, --schema NAME      the schema to dump; without it, the default schema of the connection
  -t, --table NAME       one table of the schema; repeat for more, and the objects are left out
  -c, --content WHAT     schema and rows (the default), schema only, or rows only
      --drop             put a DROP statement in front of each definition
  -h, --help             print this help and exit

exit codes:
  0 the dump was written
  1 a read, a definition or the file failed
  2 argument, password or connection failure

The dump holds the types, sequences and functions of the schema, then its tables, then the
views over them, then its triggers. Every table stands after the tables its foreign keys name.
Roles, grants and owners are not written.`

// restoreUsage is what `masume restore --help` writes.
const restoreUsage = `masume restore - run the statements of a SQL file

usage:
  masume restore [TARGET] FILE
  masume restore [TARGET] -

  TARGET                 a supported URL, keyword connection string, or existing SQLite file
  FILE                   the file the statements are read from; a single - reads stdin
  -p, --profile NAME     a user or project profile; without a target or profile, use $DATABASE_URL
  -h, --help             print this help and exit

exit codes:
  0 every statement ran
  1 a statement failed; the statements before it stand
  2 argument, input file, password or connection failure
  3 the profile is read-only

Every statement runs on its own, in file order, with no wrapping transaction. A run without a
screen has no write confirmation, no write plan and no undo.`

// dumpInvocation is the parsed dump or restore request.
type dumpInvocation struct {
	target      string
	profileName string
	path        string
	schema      string
	tables      []string
	content     dump.Content
	drops       bool
	help        bool
}

// dumpShortFlagNames are the short flags that take a value, and the long flag of each.
var dumpShortFlagNames = map[string]string{
	"-p": "--profile", "-s": "--schema", "-t": "--table", "-c": "--content",
}

// parseDumpArguments parses the arguments of `masume dump`. A restore reads the same flags
// apart from the ones that shape a dump.
func parseDumpArguments(argv []string, command string) (dumpInvocation, error) {
	held := dumpInvocation{content: dump.ContentAll}
	positional := []string{}

	for at := 0; at < len(argv); at++ {
		argument := expandShortFlag(argv[at], dumpShortFlagNames)
		var value string
		var err error

		switch {
		case argument == "--help" || argument == "-h":
			held.help = true
		case argument == "--profile" || argument == "-p":
			if value, at, err = readFlagValue(argv, at, argument); err != nil {
				return dumpInvocation{}, err
			}
			held.profileName = value
		case strings.HasPrefix(argument, "--profile="):
			if held.profileName, err = readFlagText(argument, "--profile="); err != nil {
				return dumpInvocation{}, err
			}
		case argument == "--schema" || argument == "-s":
			if value, at, err = readFlagValue(argv, at, argument); err != nil {
				return dumpInvocation{}, err
			}
			held.schema = value
		case strings.HasPrefix(argument, "--schema="):
			if held.schema, err = readFlagText(argument, "--schema="); err != nil {
				return dumpInvocation{}, err
			}
		case argument == "--table" || argument == "-t":
			if value, at, err = readFlagValue(argv, at, argument); err != nil {
				return dumpInvocation{}, err
			}
			held.tables = append(held.tables, value)
		case strings.HasPrefix(argument, "--table="):
			if value, err = readFlagText(argument, "--table="); err != nil {
				return dumpInvocation{}, err
			}
			held.tables = append(held.tables, value)
		case argument == "--content" || argument == "-c":
			if value, at, err = readFlagValue(argv, at, argument); err != nil {
				return dumpInvocation{}, err
			}
			if held.content, err = readDumpContent(value); err != nil {
				return dumpInvocation{}, err
			}
		case strings.HasPrefix(argument, "--content="):
			if held.content, err = readDumpContent(
				strings.TrimPrefix(argument, "--content=")); err != nil {
				return dumpInvocation{}, err
			}
		case argument == "--drop":
			held.drops = true
		case strings.HasPrefix(argument, "-") && argument != "-":
			return dumpInvocation{}, failArgument(
				"unknown masume " + command + " option: " + argument)
		default:
			positional = append(positional, argument)
		}
	}
	if held.help {
		return held, nil
	}
	return finishDumpInvocation(held, positional, command)
}

// readDumpContent parses the value of `--content`.
func readDumpContent(written string) (dump.Content, error) {
	content, known := dump.FindContentNamed(written)
	if !known {
		return "", failArgument(`--content must be one of "` +
			string(dump.ContentAll) + `", "` + string(dump.ContentSchema) + `" or "` +
			string(dump.ContentRows) + `"; invalid value: ` + written)
	}
	return content, nil
}

// finishDumpInvocation reads the positional arguments: the file, and the target in front of
// it.
func finishDumpInvocation(
	held dumpInvocation, positional []string, command string,
) (dumpInvocation, error) {
	switch len(positional) {
	case 1:
		held.path = positional[0]
	case 2:
		held.target, held.path = positional[0], positional[1]
	case 0:
		return dumpInvocation{}, failArgument("masume " + command + " requires a file")
	default:
		return dumpInvocation{}, failArgument(
			"masume " + command + " accepts at most two positional arguments; received " +
				strconv.Itoa(len(positional)))
	}
	if held.target != "" && held.profileName != "" {
		return dumpInvocation{}, failArgument(
			"use either --profile or a connection target, not both")
	}
	return held, nil
}

// buildDumpTables returns the tables the command named, read as `schema.table` or as a name
// in the schema of the dump.
func buildDumpTables(held dumpInvocation) []db.TableRef {
	tables := make([]db.TableRef, 0, len(held.tables))
	for _, written := range held.tables {
		table := db.TableRef{Schema: held.schema, Kind: db.RelationTable}
		if schema, name, cut := strings.Cut(strings.TrimSpace(written), "."); cut {
			table.Schema, table.Name = schema, name
		} else {
			table.Name = strings.TrimSpace(written)
		}
		tables = append(tables, table)
	}
	return tables
}

// resolveDumpProfile resolves a configured profile, a connection target, or $DATABASE_URL.
func resolveDumpProfile(
	held dumpInvocation, profiles []cfg.Profile, command string,
) (cfg.Profile, error) {
	_, start, err := resolveStartProfile(
		invocation{target: held.target, profileName: held.profileName},
		profiles, os.Getenv)
	if err != nil {
		return cfg.Profile{}, err
	}
	if start == nil {
		return cfg.Profile{}, failArgument("masume " + command +
			" requires a connection: a target, --profile, or $DATABASE_URL")
	}
	return *start, nil
}

// openDumpProfile reads the config file and answers the profile of the command.
func openDumpProfile(held dumpInvocation, command string) (cfg.Profile, string, int) {
	loaded := cfg.LoadConfigForWorkingDirectory(cfg.ResolveConfigPath())
	for _, problem := range loaded.Project.Problems {
		fmt.Fprintln(os.Stderr, "masume: "+problem)
	}
	for _, problem := range loaded.Problems {
		fmt.Fprintln(os.Stderr, "masume: "+problem.Describe())
	}
	for _, warning := range loaded.Warnings {
		fmt.Fprintln(os.Stderr, "masume: "+warning.DescribeWarning())
	}

	profile, err := resolveDumpProfile(held, loaded.Profiles, command)
	if err != nil {
		fmt.Fprintln(os.Stderr, "masume: "+err.Error())
		return cfg.Profile{}, "", 2
	}
	password, err := cfg.ResolveProfilePassword(profile)
	if err != nil {
		fmt.Fprintln(os.Stderr, "masume: "+err.Error())
		return cfg.Profile{}, "", headless.CodeConnection
	}
	return profile, password, headless.CodeOK
}

// runDumpCommand runs `masume dump` and returns the exit code of the process.
func runDumpCommand(argv []string) int {
	held, err := parseDumpArguments(argv, "dump")
	if err != nil {
		fmt.Fprintln(os.Stderr, "masume: "+err.Error())
		fmt.Fprintln(os.Stderr, "run masume dump --help for usage")
		return 2
	}
	if held.help {
		fmt.Println(dumpUsage)
		return 0
	}

	profile, password, code := openDumpProfile(held, "dump")
	if code != headless.CodeOK {
		return code
	}
	return headless.RunDump(context.Background(), engines.CreateAdapters(),
		headless.DumpOptions{
			Options: headless.Options{
				Profile: profile, Password: password,
				Out: os.Stdout, Err: os.Stderr,
			},
			Path: held.path,
			Dump: dump.Options{
				Schema: held.schema, Tables: buildDumpTables(held),
				Content: held.content, DropsFirst: held.drops,
			},
		})
}

// runRestoreCommand runs `masume restore` and returns the exit code of the process.
func runRestoreCommand(argv []string) int {
	held, err := parseDumpArguments(argv, "restore")
	if err != nil {
		fmt.Fprintln(os.Stderr, "masume: "+err.Error())
		fmt.Fprintln(os.Stderr, "run masume restore --help for usage")
		return 2
	}
	if held.help {
		fmt.Println(restoreUsage)
		return 0
	}
	if len(held.tables) > 0 || held.schema != "" || held.drops {
		fmt.Fprintln(os.Stderr,
			"masume: masume restore takes no --schema, --table or --drop")
		return 2
	}

	profile, password, code := openDumpProfile(held, "restore")
	if code != headless.CodeOK {
		return code
	}
	return headless.RunRestore(context.Background(), engines.CreateAdapters(),
		headless.RestoreOptions{
			Options: headless.Options{
				Profile: profile, Password: password,
				Out: os.Stdout, Err: os.Stderr,
			},
			Path: held.path, In: os.Stdin,
		})
}
