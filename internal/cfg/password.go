package cfg

import (
	"context"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"github.com/turanmahmudov/masume/internal/secret"
)

// The password of a profile: the value in the file, the value of an environment variable,
// or the output of a command. A command that fails is reported, so a locked keyring does not
// become a login error.

// passwordCommandTimeout is the time a locked keyring has to ask the user to unlock it.
const passwordCommandTimeout = 30 * time.Second

// readFirstLine returns the first line of the output of a command, so a store that prints
// more than one line still works.
func readFirstLine(output string) string {
	line := output
	if before, _, ok := strings.Cut(output, "\n"); ok {
		line = before
	}
	return strings.TrimSuffix(line, "\r")
}

// runPasswordCommand runs the command a profile reads its password with and returns its
// output. The source is what the command is called in a report: the password command of the
// profile, or the store it names.
func runPasswordCommand(source, name, written string) (string, error) {
	ctx, stop := context.WithTimeout(context.Background(), passwordCommandTimeout)
	defer stop()

	// Without stdin, a command that prompts in the terminal fails instead of drawing
	// over the interface.
	command := exec.CommandContext(ctx, "sh", "-c", written)
	command.Stdin = nil
	printed, err := command.Output()

	if ctx.Err() != nil {
		return "", fmt.Errorf("the %s for %s did not answer within %.0fs: %s",
			source, name, passwordCommandTimeout.Seconds(), written)
	}
	if err != nil {
		reason := written
		if reported, is := err.(*exec.ExitError); is {
			if said := readFirstLine(strings.TrimSpace(string(reported.Stderr))); said != "" {
				reason = said
			}
			return "", fmt.Errorf("the %s for %s failed with code %d: %s",
				source, name, reported.ExitCode(), reason)
		}
		return "", fmt.Errorf("the %s for %s failed: %w", source, name, err)
	}

	password := readFirstLine(string(printed))
	if password == "" {
		return "", fmt.Errorf("the %s for %s printed nothing: %s", source, name, written)
	}
	return password, nil
}

// ResolveProfilePassword returns the password of the profile, or an empty value if the user
// is the only source. A keyring that holds nothing for the profile answers with an empty
// value as well, because the first connection of such a profile is typed.
func ResolveProfilePassword(profile Profile) (string, error) {
	switch profile.Auth {
	case AuthCommand:
		if profile.PasswordCommand != "" {
			return runPasswordCommand("password command", profile.Name, profile.PasswordCommand)
		}
	case AuthSecret:
		if profile.SecretCommand != "" {
			return runPasswordCommand(
				"secret store "+strconv.Quote(profile.Secret), profile.Name,
				profile.SecretCommand)
		}
	case AuthKeyring:
		password, found, err := secret.FindPassword(profile.Name)
		if err != nil {
			return "", err
		}
		if found {
			return password, nil
		}
		return "", nil
	}
	return FindStoredPassword(profile), nil
}
