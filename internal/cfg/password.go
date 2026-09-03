package cfg

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
	"time"
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

// runPasswordCommand runs the password command of a profile and returns its output.
func runPasswordCommand(name, written string) (string, error) {
	ctx, stop := context.WithTimeout(context.Background(), passwordCommandTimeout)
	defer stop()

	// Without stdin, a command that prompts in the terminal fails instead of drawing
	// over the interface.
	command := exec.CommandContext(ctx, "sh", "-c", written)
	command.Stdin = nil
	printed, err := command.Output()

	if ctx.Err() != nil {
		return "", fmt.Errorf(
			"the password command for %s did not answer within %.0fs: %s",
			name, passwordCommandTimeout.Seconds(), written)
	}
	if err != nil {
		reason := written
		if reported, is := err.(*exec.ExitError); is {
			if said := readFirstLine(strings.TrimSpace(string(reported.Stderr))); said != "" {
				reason = said
			}
			return "", fmt.Errorf("the password command for %s failed with code %d: %s",
				name, reported.ExitCode(), reason)
		}
		return "", fmt.Errorf("the password command for %s failed: %w", name, err)
	}

	password := readFirstLine(string(printed))
	if password == "" {
		return "", fmt.Errorf("the password command for %s printed nothing: %s", name, written)
	}
	return password, nil
}

// ResolveProfilePassword returns the password of the profile, or an empty value if the user
// is the only source.
func ResolveProfilePassword(profile Profile) (string, error) {
	if profile.Auth == AuthCommand && profile.PasswordCommand != "" {
		return runPasswordCommand(profile.Name, profile.PasswordCommand)
	}
	return FindStoredPassword(profile), nil
}
