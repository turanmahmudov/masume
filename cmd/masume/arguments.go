package main

import (
	"os"
	"strings"

	"github.com/turanmahmudov/masume/internal/cfg"
)

// Reads what the command was given: a connection target, or the name of a profile of the
// config file. The client opens that connection as it starts.

// databaseURLVariable is read when the command is given no target of its own.
const databaseURLVariable = "DATABASE_URL"

// argumentError is a mistake in what the command was given. It exits with 2.
type argumentError struct{ reason string }

func (err argumentError) Error() string { return err.reason }

func failArgument(reason string) error { return argumentError{reason: reason} }

// invocation is what the command line asked for.
type invocation struct {
	target      string
	profileName string
}

// parseArguments reads the arguments of the client.
func parseArguments(argv []string) (invocation, error) {
	held := invocation{}
	for at := 0; at < len(argv); at++ {
		argument := argv[at]
		switch {
		case argument == "--profile" || argument == "-p":
			if at+1 >= len(argv) || strings.HasPrefix(argv[at+1], "-") {
				return invocation{}, failArgument(argument + " needs the name of a profile")
			}
			at++
			held.profileName = argv[at]
		case strings.HasPrefix(argument, "--profile="):
			held.profileName = strings.TrimPrefix(argument, "--profile=")
			if held.profileName == "" {
				return invocation{}, failArgument("--profile= names no profile")
			}
		case strings.HasPrefix(argument, "-"):
			return invocation{}, failArgument(
				argument + " is not an argument this command reads")
		case strings.TrimSpace(argument) == "":
			return invocation{}, failArgument("an argument of this command cannot be empty")
		default:
			if held.target != "" {
				return invocation{}, failArgument(
					"this command opens one connection, and " + argument + " is a second")
			}
			held.target = argument
		}
	}
	if held.target != "" && held.profileName != "" {
		return invocation{}, failArgument(
			"--profile and a connection target are two ways to name one connection")
	}
	return held, nil
}

// resolveStartProfile returns the profiles the picker lists and the one the client opens as
// it starts. A target of the command line is added to the list.
func resolveStartProfile(
	held invocation, profiles []cfg.Profile, environment func(string) string,
) ([]cfg.Profile, *cfg.Profile, error) {
	if held.profileName != "" {
		for _, profile := range profiles {
			if profile.Name == held.profileName {
				return profiles, &profile, nil
			}
		}
		return nil, nil, failArgument(
			"profile " + held.profileName + " is not one the config file has")
	}

	target := held.target
	if target == "" {
		target = strings.TrimSpace(environment(databaseURLVariable))
	}
	if target == "" {
		return profiles, nil, nil
	}

	built, err := cfg.BuildProfileFromTarget(target)
	if err != nil {
		return nil, nil, failArgument(err.Error())
	}
	built.Name = cfg.ResolveUniqueProfileName(profiles, built.Name)
	return append([]cfg.Profile{built}, profiles...), &built, nil
}

// readEnvironment is the source resolveStartProfile reads outside the tests.
func readEnvironment(name string) string { return os.Getenv(name) }
