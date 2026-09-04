package cfg

import (
	"fmt"
	"slices"
	"strings"
)

// The secret stores of the user, declared once under `[secret.NAME]` and named by a profile.
// One store is a command that prints one secret. masume knows no tool: `op`, `vault`, `sops`,
// `pass` and a script of the user all fit the same shape.

// secretReferenceMark is what a store command holds where the reference of the profile goes.
const secretReferenceMark = "{{ref}}"

// SecretSource is one store the user declared.
type SecretSource struct {
	Name string
	// The command that prints the secret. It holds secretReferenceMark one time or more.
	Command string
}

// ParseSecretSources reads the `[secret]` section. A store that cannot be read is reported
// and skipped, so one bad entry does not stop the app.
func ParseSecretSources(document Table) ([]SecretSource, []ProfileProblem) {
	written, present := FindSection(document, "secret")
	if !present {
		return nil, nil
	}

	names := make([]string, 0, len(written))
	for name := range written {
		names = append(names, name)
	}
	slices.Sort(names)

	sources := make([]SecretSource, 0, len(names))
	problems := []ProfileProblem{}
	for _, name := range names {
		source, isTable := FindTable(written[name])
		if !isTable {
			problems = append(problems, ProfileProblem{
				Name: name, Reason: "entry is not a table"})
			continue
		}
		command, holdsCommand := FindString(source, "command")
		if !holdsCommand {
			problems = append(problems, ProfileProblem{
				Name:   name,
				Reason: fmt.Sprintf("%q must be a non-empty string", "command")})
			continue
		}
		if !strings.Contains(command, secretReferenceMark) {
			problems = append(problems, ProfileProblem{
				Name: name, Reason: fmt.Sprintf(
					"%q must hold %s, which is where the reference of a profile goes",
					"command", secretReferenceMark)})
			continue
		}
		sources = append(sources, SecretSource{Name: name, Command: command})
	}
	return sources, problems
}

// FindSecretSource returns the store of that name.
func FindSecretSource(sources []SecretSource, name string) (SecretSource, bool) {
	for _, source := range sources {
		if source.Name == name {
			return source, true
		}
	}
	return SecretSource{}, false
}

// ListSecretSourceNames returns the names of the stores, for a form to offer.
func ListSecretSourceNames(sources []SecretSource) []string {
	names := make([]string, 0, len(sources))
	for _, source := range sources {
		names = append(names, source.Name)
	}
	return names
}

// quoteForShell returns the value as one argument of a shell command. A reference is data,
// so it is quoted rather than pasted in: a reference with a blank or a quote in it must not
// become a second argument or a second command.
func quoteForShell(value string) string {
	return "'" + strings.ReplaceAll(value, "'", `'\''`) + "'"
}

// BuildSecretCommand returns the command that reads one reference out of the store.
func BuildSecretCommand(source SecretSource, reference string) string {
	return strings.ReplaceAll(source.Command, secretReferenceMark, quoteForShell(reference))
}

// ApplySecretCommand returns the profile with the command of its store built into it. A
// profile the form made carries the name and the reference only, so the command is built
// here as well as when the file is read.
func ApplySecretCommand(profile Profile, sources []SecretSource) (Profile, error) {
	if profile.Auth != AuthSecret {
		profile.SecretCommand = ""
		return profile, nil
	}
	command, err := resolveSecretCommand(profile, sources)
	if err != nil {
		return profile, err
	}
	profile.SecretCommand = command
	return profile, nil
}

// resolveSecretCommand returns the command the profile reads its password with, and the
// reason it has none.
func resolveSecretCommand(profile Profile, sources []SecretSource) (string, error) {
	if profile.Secret == "" {
		return "", failProfile("%q must name a [secret] store when %q is secret",
			"secret", "auth")
	}
	if profile.SecretRef == "" {
		return "", failProfile("%q must be set when %q is secret", "secret_ref", "auth")
	}
	source, found := FindSecretSource(sources, profile.Secret)
	if !found {
		return "", failProfile("there is no [secret.%s] store in the config file",
			profile.Secret)
	}
	return BuildSecretCommand(source, profile.SecretRef), nil
}
