package cfg

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/turanmahmudov/masume/internal/core"
)

// Config updates replace one table and preserve lines outside that table.

// ConfigFileError is the error class for a config file that cannot be read. The client
// never writes over such a file.
type ConfigFileError struct{ Reason string }

func (err ConfigFileError) Error() string { return err.Reason }

// maxHeaderDepth is the maximum number of parts in a `[a.b.c]` header.
const maxHeaderDepth = 8

// readHeaderPath returns the key path of a `[table]` header line, and false for any other
// line.
func readHeaderPath(line string) ([]string, bool) {
	trimmed := strings.TrimSpace(line)
	if !strings.HasPrefix(trimmed, "[") || strings.HasPrefix(trimmed, "[[") ||
		!strings.HasSuffix(trimmed, "]") {
		return nil, false
	}
	inner := strings.TrimSpace(trimmed[1 : len(trimmed)-1])
	if inner == "" {
		return nil, false
	}

	parts := []string{}
	for part := range strings.SplitSeq(inner, ".") {
		name := strings.TrimSpace(part)
		name = strings.Trim(name, `"`)
		if name == "" {
			return nil, false
		}
		parts = append(parts, name)
	}
	if len(parts) > maxHeaderDepth {
		return nil, false
	}
	return parts, true
}

// readAssignmentKey returns the key a line sets, and false for a blank line or a comment.
func readAssignmentKey(line string) (string, bool) {
	trimmed := strings.TrimSpace(line)
	if trimmed == "" || strings.HasPrefix(trimmed, "#") {
		return "", false
	}
	at := strings.Index(trimmed, "=")
	if at <= 0 {
		return "", false
	}
	key := strings.TrimSpace(trimmed[:at])
	return strings.Trim(key, `"`), key != ""
}

// writeTomlValue serializes strings, integers, and booleans as TOML. Other types cause a panic.
func writeTomlValue(value any) string {
	switch held := value.(type) {
	case string:
		return strconv.Quote(held)
	case int:
		return strconv.Itoa(held)
	case bool:
		return strconv.FormatBool(held)
	}
	panic(fmt.Sprintf("unsupported profile value type %T", value))
}

// buildProfileKeys returns ordered keys, non-empty values, and managed keys. File engines omit host, port, and user.
func buildProfileKeys(profile Profile) ([]string, map[string]any, map[string]bool) {
	written := map[string]any{"engine": string(profile.Engine)}
	if !core.OpensFile(profile.Engine) {
		written["host"] = profile.Host
		written["port"] = profile.Port
		written["user"] = profile.User
	}
	managed := map[string]bool{}
	for key := range written {
		managed[key] = true
	}
	// Empty values remove managed keys. Missing keys use reader defaults.
	for key, value := range map[string]string{
		"database":       profile.Database,
		"auth":           string(profile.Auth),
		"env":            string(profile.Environment),
		"mode":           string(profile.AccessMode),
		"confirm_writes": string(profile.ConfirmWrites),
		// Saving removes any existing password key.
		"password":         "",
		"password_env":     profile.PasswordEnv,
		"password_command": profile.PasswordCommand,
		"secret":           profile.Secret,
		"secret_ref":       profile.SecretRef,
		"sslmode":          string(profile.SSLMode),
		"description":      profile.Description,
		"ai_instructions":  profile.AiInstructions,
	} {
		managed[key] = true
		if value != "" {
			written[key] = value
		}
	}

	// A fixed order that is easy to read, not the order of a map.
	order := []string{
		"engine", "host", "port", "database", "user", "auth",
		"password", "password_env", "password_command", "secret", "secret_ref",
		"env", "mode", "sslmode", "confirm_writes", "description", "ai_instructions",
	}
	kept := make([]string, 0, len(order))
	for _, key := range order {
		if _, held := written[key]; held {
			kept = append(kept, key)
		}
	}
	return kept, written, managed
}

// findBlockEnd returns the end of the block of a header: the next header, or the end of the
// file.
func findBlockEnd(lines []string, from int) int {
	for at := from; at < len(lines); at++ {
		if _, isHeader := readHeaderPath(lines[at]); isHeader {
			return at
		}
	}
	return len(lines)
}

// findWrittenEnd returns the same end without the lines that belong to the next header: the
// blank lines between the two blocks, and a comment above the next header.
func findWrittenEnd(lines []string, blockEnd, from int) int {
	leadsIntoHeader := blockEnd < len(lines)
	at := blockEnd
	for at > from {
		line := strings.TrimSpace(lines[at-1])
		if line != "" && !(leadsIntoHeader && strings.HasPrefix(line, "#")) {
			break
		}
		at--
	}
	return at
}

func matchesPath(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for at, part := range left {
		if part != right[at] {
			return false
		}
	}
	return true
}

// writeProfileBlock writes the profile into the text and keeps every line outside its block
// unchanged.
func writeProfileBlock(text string, profile Profile) string {
	order, values, managed := buildProfileKeys(profile)

	lines := strings.Split(text, "\n")
	start := findProfileHeaderLine(lines, profile.Name)
	if start == -1 {
		for _, setting := range []struct {
			key   string
			value any
			omit  bool
		}{
			{"mcp", string(profile.McpAccess), profile.McpAccess == McpUnset},
			{"write_plan", string(profile.WritePlan), profile.WritePlan == ""},
			{"undo_rows", profile.UndoRows, false},
			{"statement_timeout_ms", int(profile.StatementTimeout / time.Millisecond), false},
			{"autocommit", profile.Autocommit, false},
			{"page_size", profile.PageSize, profile.PageSize == 0},
			{"keepalive_s", int(profile.Keepalive / time.Second), false},
			{"command", profile.Command, profile.Command == ""},
			{"wait_for_port", profile.WaitForPort, profile.WaitForPort == 0},
			{"command_timeout", int(profile.CommandTimeout / time.Second), profile.CommandTimeout == 0},
		} {
			if !setting.omit {
				order = append(order, setting.key)
				values[setting.key] = setting.value
			}
		}
	}

	written := make([]string, 0, len(order))
	for _, key := range order {
		written = append(written, key+" = "+writeTomlValue(values[key]))
	}

	if start == -1 {
		// A table that is not in the file yet is added at the end.
		header := "[profile." + quoteHeaderName(profile.Name) + "]"
		block := append([]string{header}, written...)
		if strings.TrimSpace(text) == "" {
			return strings.Join(block, "\n") + "\n"
		}
		tail := text
		if !strings.HasSuffix(tail, "\n") {
			tail += "\n"
		}
		return tail + "\n" + strings.Join(block, "\n") + "\n"
	}

	end := findWrittenEnd(lines, findBlockEnd(lines, start+1), start+1)
	kept := []string{}
	pending := map[string]bool{}
	for _, key := range order {
		pending[key] = true
	}

	// Unmanaged keys and non-assignment lines remain unchanged.
	for _, line := range lines[start+1 : end] {
		key, isAssignment := readAssignmentKey(line)
		if !isAssignment {
			kept = append(kept, line)
			continue
		}
		// Settings absent from the form remain unchanged.
		if !managed[key] {
			kept = append(kept, line)
			continue
		}
		// Cleared managed keys are removed.
		if _, holdsValue := values[key]; !holdsValue {
			continue
		}
		// Repeated managed keys keep one copy.
		if !pending[key] {
			continue
		}
		delete(pending, key)
		kept = append(kept, key+" = "+writeTomlValue(values[key]))
	}
	for _, key := range order {
		if pending[key] {
			kept = append(kept, key+" = "+writeTomlValue(values[key]))
		}
	}

	rebuilt := append([]string{}, lines[:start+1]...)
	rebuilt = append(rebuilt, kept...)
	rebuilt = append(rebuilt, lines[end:]...)
	return strings.Join(rebuilt, "\n")
}

// findProfileHeaderLine returns the header line of the block of this profile, and -1 if the
// text has no block with that name.
func findProfileHeaderLine(lines []string, name string) int {
	path := []string{"profile", name}
	for at, line := range lines {
		held, isHeader := readHeaderPath(line)
		if isHeader && matchesPath(held, path) {
			return at
		}
	}
	return -1
}

// renameProfileBlock replaces the profile header name and preserves all other lines.
func renameProfileBlock(text, from, to string) string {
	lines := strings.Split(text, "\n")
	start := findProfileHeaderLine(lines, from)
	if start == -1 {
		return text
	}
	lines[start] = "[profile." + quoteHeaderName(to) + "]"
	return strings.Join(lines, "\n")
}

// removeProfileBlock deletes the block of one profile from the text.
func removeProfileBlock(text, name string) string {
	lines := strings.Split(text, "\n")
	start := findProfileHeaderLine(lines, name)
	if start == -1 {
		return text
	}
	end := findWrittenEnd(lines, findBlockEnd(lines, start+1), start+1)
	kept := append([]string{}, lines[:start]...)
	return strings.Join(append(kept, lines[end:]...), "\n")
}

// quoteHeaderName returns a profile name for a header, with quotes if the reader would
// otherwise parse it differently.
func quoteHeaderName(name string) string {
	for _, character := range name {
		isPlain := character == '-' || character == '_' ||
			(character >= 'a' && character <= 'z') ||
			(character >= 'A' && character <= 'Z') ||
			(character >= '0' && character <= '9')
		if !isPlain {
			return strconv.Quote(name)
		}
	}
	return name
}

// readConfigText returns the text of the config file, or an empty string if there is no
// file.
func readConfigText(path string) (string, error) {
	held, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", err
	}
	// Invalid TOML prevents config updates.
	if _, decodeErr := DecodeDocument(string(held)); decodeErr != nil {
		return "", ConfigFileError{Reason: fmt.Sprintf(
			"%s contains invalid TOML; the file is unchanged: %v", path, decodeErr)}
	}
	return string(held), nil
}

// writeConfigText writes and syncs a temporary file, then renames the temporary file over the config file.
func writeConfigText(path, text string) error {
	// The config directory is private to its owner.
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	file, err := os.CreateTemp(filepath.Dir(path), filepath.Base(path)+".*")
	if err != nil {
		return err
	}
	temporaryPath := file.Name()
	dropTemporary := func(reason error) error {
		_ = file.Close()
		_ = os.Remove(temporaryPath)
		return reason
	}
	if _, err := file.WriteString(text); err != nil {
		return dropTemporary(err)
	}
	// Sync completes before the rename.
	if err := file.Sync(); err != nil {
		return dropTemporary(err)
	}
	if err := file.Close(); err != nil {
		_ = os.Remove(temporaryPath)
		return err
	}
	if err := os.Rename(temporaryPath, path); err != nil {
		_ = os.Remove(temporaryPath)
		return err
	}
	return nil
}

// SaveProfileToFile adds or updates a profile and handles a changed profile name.
func SaveProfileToFile(profile Profile, replacing, path string) error {
	text, err := readConfigText(path)
	if err != nil {
		return err
	}
	if replacing != "" && replacing != profile.Name {
		// An existing target block replaces the old block. Otherwise, only the header name changes.
		if findProfileHeaderLine(strings.Split(text, "\n"), profile.Name) == -1 {
			text = renameProfileBlock(text, replacing, profile.Name)
		} else {
			text = removeProfileBlock(text, replacing)
		}
	}
	written := writeProfileBlock(text, profile)
	if _, decodeErr := DecodeDocument(written); decodeErr != nil {
		return ConfigFileError{Reason: fmt.Sprintf(
			"the generated profile is invalid TOML; %s is unchanged", path)}
	}
	return writeConfigText(path, written)
}

// RemoveProfileFromFile deletes the profile from the config file.
func RemoveProfileFromFile(name, path string) error {
	text, err := readConfigText(path)
	if err != nil {
		return err
	}
	return writeConfigText(path, removeProfileBlock(text, name))
}

// SaveTheme writes the selected theme to `[ui] theme` and keeps the rest of the file
// unchanged.
func SaveTheme(name, path string) error {
	text, err := readConfigText(path)
	if err != nil {
		return err
	}

	lines := strings.Split(text, "\n")
	start := -1
	for at, line := range lines {
		held, isHeader := readHeaderPath(line)
		if isHeader && matchesPath(held, []string{"ui"}) {
			start = at
			break
		}
	}

	written := "theme = " + strconv.Quote(name)
	if start == -1 {
		tail := text
		if strings.TrimSpace(tail) == "" {
			return writeConfigText(path, "[ui]\n"+written+"\n")
		}
		if !strings.HasSuffix(tail, "\n") {
			tail += "\n"
		}
		return writeConfigText(path, tail+"\n[ui]\n"+written+"\n")
	}

	end := findWrittenEnd(lines, findBlockEnd(lines, start+1), start+1)
	kept := []string{}
	replaced := false
	for _, line := range lines[start+1 : end] {
		key, isAssignment := readAssignmentKey(line)
		if isAssignment && key == "theme" {
			kept = append(kept, written)
			replaced = true
			continue
		}
		kept = append(kept, line)
	}
	if !replaced {
		kept = append([]string{written}, kept...)
	}

	rebuilt := append([]string{}, lines[:start+1]...)
	rebuilt = append(rebuilt, kept...)
	rebuilt = append(rebuilt, lines[end:]...)
	return writeConfigText(path, strings.Join(rebuilt, "\n"))
}
