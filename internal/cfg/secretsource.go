package cfg

import (
	"fmt"
	"slices"
	"strings"
)

// A `[secret.NAME]` store is a command that prints one secret.

// secretReferenceMark is the placeholder for the profile secret reference.
const secretReferenceMark = "{{ref}}"

// SecretSource is one store the user declared.
type SecretSource struct {
	Name string
	// The command that prints the secret, with at least one secretReferenceMark placeholder.
	Command string
}

// ParseSecretSources reads `[secret]` and reports invalid stores without loading them.
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
		if err := validateSecretCommand(command); err != nil {
			problems = append(problems, ProfileProblem{
				Name: name, Reason: err.Error()})
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

// quoteForShell quotes a value as one literal shell argument.
func quoteForShell(value string) string {
	return "'" + strings.ReplaceAll(value, "'", `'\''`) + "'"
}

func validateSecretCommand(command string) error {
	invalid := fmt.Errorf("command requires standalone unquoted %s arguments; use literal arguments, quoted flags and optional pipelines", secretReferenceMark)
	var quote byte
	inWord, hasCommand, hasReference := false, false, false
	for at := 0; at < len(command); at++ {
		character := command[at]
		if strings.ContainsRune("\x00\r\n", rune(character)) {
			return invalid
		}
		if strings.HasPrefix(command[at:], secretReferenceMark) {
			end := at + len(secretReferenceMark)
			if quote != 0 || inWord || !hasCommand ||
				(end < len(command) && command[end] != ' ' && command[end] != '\t') {
				return invalid
			}
			hasReference, inWord = true, true
			at = end - 1
			continue
		}
		if quote == '\'' {
			if character == quote {
				quote = 0
			}
			continue
		}
		if strings.ContainsRune("$`\\", rune(character)) {
			return invalid
		}
		if quote == '"' {
			if character == quote {
				quote = 0
			}
			continue
		}
		switch character {
		case '\'', '"':
			quote = character
			inWord = true
		case ' ', '\t':
			hasCommand = hasCommand || inWord
			inWord = false
		case '|':
			if !hasCommand && !inWord {
				return invalid
			}
			hasCommand, inWord = false, false
		case ';', '&', '<', '>', '(', ')', '{', '}', '#':
			return invalid
		default:
			inWord = true
		}
	}
	if quote != 0 || !hasReference || (!hasCommand && !inWord) {
		return invalid
	}
	return nil
}

// BuildSecretCommand returns the command that reads one reference out of the store.
func BuildSecretCommand(source SecretSource, reference string) (string, error) {
	if err := validateSecretCommand(source.Command); err != nil {
		return "", err
	}
	return strings.ReplaceAll(source.Command, secretReferenceMark, quoteForShell(reference)), nil
}

// ApplySecretCommand builds the profile command from the store and secret reference.
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

// resolveSecretCommand returns the password command or a store configuration error.
func resolveSecretCommand(profile Profile, sources []SecretSource) (string, error) {
	if profile.Secret == "" {
		return "", failProfile("%q must be a [secret] store name when %q is secret",
			"secret", "auth")
	}
	if profile.SecretRef == "" {
		return "", failProfile("%q must be set when %q is secret", "secret_ref", "auth")
	}
	source, found := FindSecretSource(sources, profile.Secret)
	if !found {
		return "", failProfile("[secret.%s] store is missing from the config file",
			profile.Secret)
	}
	return BuildSecretCommand(source, profile.SecretRef)
}
