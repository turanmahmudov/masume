package cli

import (
	"errors"
	"strings"

	"github.com/turanmahmudov/masume/internal/cfg"
	"github.com/turanmahmudov/masume/internal/detect"
)

// Startup arguments select a connection target or a configured profile.

// databaseURLVariable is read when the command is given no target of its own.
const databaseURLVariable = "DATABASE_URL"

// clientShortFlagNames are the short flags of the client that take a value, and the long flag of each.
var clientShortFlagNames = map[string]string{"-p": "--profile"}

// expandShortFlag expands a short flag with an attached value, such as -p=shop.
func expandShortFlag(argument string, shortFlagNames map[string]string) string {
	name, value, attached := strings.Cut(argument, "=")
	long, isShort := shortFlagNames[name]
	if !attached || !isShort {
		return argument
	}
	return long + "=" + value
}

// argumentError is an invalid argument. The exit code is 2.
type argumentError struct{ reason string }

func (err argumentError) Error() string { return err.reason }

func failArgument(reason string) error { return argumentError{reason: reason} }

// invocation is the parsed startup request.
type invocation struct {
	target      string
	profileName string
	detect      bool
}

// parseArguments reads the arguments of the client.
func parseArguments(argv []string) (invocation, error) {
	held := invocation{}
	for at := 0; at < len(argv); at++ {
		argument := expandShortFlag(argv[at], clientShortFlagNames)
		switch {
		case argument == "--profile" || argument == "-p":
			if at+1 >= len(argv) || strings.HasPrefix(argv[at+1], "-") {
				return invocation{}, failArgument(argument + " requires a profile name")
			}
			at++
			held.profileName = argv[at]
		case strings.HasPrefix(argument, "--profile="):
			held.profileName = strings.TrimPrefix(argument, "--profile=")
			if held.profileName == "" {
				return invocation{}, failArgument("--profile requires a profile name")
			}
		case argument == "--detect":
			held.detect = true
		case strings.HasPrefix(argument, "-"):
			return invocation{}, failArgument(
				"unknown option: " + argument)
		case strings.TrimSpace(argument) == "":
			return invocation{}, failArgument("arguments cannot be empty")
		default:
			if held.target != "" {
				return invocation{}, failArgument(
					"only one connection target is allowed; extra target: " + argument)
			}
			held.target = argument
		}
	}
	if held.target != "" && held.profileName != "" {
		return invocation{}, failArgument(
			"use either --profile or a connection target, not both")
	}
	if held.detect && (held.target != "" || held.profileName != "") {
		return invocation{}, failArgument(
			"--detect cannot be combined with --profile or a connection target")
	}
	return held, nil
}

// resolveStartProfile returns the available profiles and the startup profile, including any command-line target.
func resolveStartProfile(
	held invocation, profiles []cfg.Profile, environment func(string) string,
) ([]cfg.Profile, *cfg.Profile, error) {
	if held.detect {
		listed, err := listDetectedProfiles(profiles)
		return listed, nil, err
	}
	if held.profileName != "" {
		for _, profile := range profiles {
			if profile.Name == held.profileName {
				return profiles, &profile, nil
			}
		}
		return nil, nil, failArgument(
			"profile " + held.profileName +
				" was not found in the config or project file")
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

// listDetectedProfiles returns detected container databases before configured profiles.
func listDetectedProfiles(profiles []cfg.Profile) ([]cfg.Profile, error) {
	found, err := detect.BuildContainerProfiles()
	if err != nil {
		return nil, err
	}
	if len(found) == 0 {
		return nil, errors.New("no container databases detected on this machine")
	}

	listed := []cfg.Profile{}
	for _, profile := range found {
		profile.Name = cfg.ResolveUniqueProfileName(
			append(append([]cfg.Profile{}, listed...), profiles...), profile.Name)
		listed = append(listed, profile)
	}
	return append(listed, profiles...), nil
}
