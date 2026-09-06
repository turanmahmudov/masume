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

// Password sources include memory, environment variables, commands, and the system keyring.

// passwordCommandTimeout is the password command time limit.
const passwordCommandTimeout = 30 * time.Second

// readFirstLine returns the first output line without its line ending.
func readFirstLine(output string) string {
	line := output
	if before, _, ok := strings.Cut(output, "\n"); ok {
		line = before
	}
	return strings.TrimSuffix(line, "\r")
}

// runPasswordCommand returns the password command output. The source is the command label in errors.
func runPasswordCommand(source, name, written string) (string, error) {
	ctx, stop := context.WithTimeout(context.Background(), passwordCommandTimeout)
	defer stop()

	// The command has no terminal input.
	command := exec.CommandContext(ctx, "sh", "-c", written)
	command.Stdin = nil
	printed, err := command.Output()

	if ctx.Err() != nil {
		return "", fmt.Errorf("the %s for %s exceeded the %.0fs time limit: %s",
			source, name, passwordCommandTimeout.Seconds(), written)
	}
	if err != nil {
		reason := written
		if reported, is := err.(*exec.ExitError); is {
			if said := readFirstLine(strings.TrimSpace(string(reported.Stderr))); said != "" {
				reason = said
			}
			return "", fmt.Errorf("the %s for %s failed with exit code %d: %s",
				source, name, reported.ExitCode(), reason)
		}
		return "", fmt.Errorf("the %s for %s failed: %w", source, name, err)
	}

	password := readFirstLine(string(printed))
	if password == "" {
		return "", fmt.Errorf("the %s for %s returned an empty password: %s", source, name, written)
	}
	return password, nil
}

// ResolveProfilePassword returns the profile password, or an empty string if a prompt is necessary.
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
