package cfg

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/turanmahmudov/masume/internal/core"
)

// Writes one table into the text of the config file and leaves every other line as it was,
// so the comments and the layout of the reader survive a write. A rewrite of the whole
// document would drop both.

// ConfigFileError marks a config file that cannot be read. Nothing is written over one.
type ConfigFileError struct{ Reason string }

func (err ConfigFileError) Error() string { return err.Reason }

// maxHeaderDepth is how deep a `[a.b.c]` header may nest before this stops reading it.
const maxHeaderDepth = 8

// readHeaderPath returns the key path of a `[table]` header line, and nothing where the line
// is not one.
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

// readAssignmentKey returns the key one line sets, and nothing for a blank line or a comment.
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

// writeTomlValue writes one value as TOML reads it back. The form writes text, whole
// numbers and flags alone, and any other kind would be written as text and read back as
// something else, so it stops the client where it is written rather than in the file.
func writeTomlValue(value any) string {
	switch held := value.(type) {
	case string:
		return strconv.Quote(held)
	case int:
		return strconv.Itoa(held)
	case bool:
		return strconv.FormatBool(held)
	}
	panic(fmt.Sprintf("a profile holds a %T, which no config file can be written from", value))
}

// buildProfileKeys returns the keys the form writes, the values of the ones that hold a
// value, and every key the form owns. A file engine writes no host, port or user, because
// those would not read back as a profile.
func buildProfileKeys(profile Profile) ([]string, map[string]any, map[string]bool) {
	written := map[string]any{
		"engine":   string(profile.Engine),
		"database": profile.Database,
	}
	if !core.OpensFile(profile.Engine) {
		written["host"] = profile.Host
		written["port"] = profile.Port
		written["user"] = profile.User
	}
	managed := map[string]bool{}
	for key := range written {
		managed[key] = true
	}
	// A key with no value is left out rather than written empty. The reader of the file
	// takes the default for a key that is not there, and refuses one that is there and
	// names nothing, so an empty value would write a file that cannot be read back. The
	// key stays managed, so the line of a value the form cleared is taken out.
	for key, value := range map[string]string{
		"auth":             string(profile.Auth),
		"env":              string(profile.Environment),
		"mode":             string(profile.AccessMode),
		"confirm_writes":   string(profile.ConfirmWrites),
		"password":         profile.Password,
		"password_env":     profile.PasswordEnv,
		"password_command": profile.PasswordCommand,
		"sslmode":          string(profile.SSLMode),
		"description":      profile.Description,
		"ai_instructions":  profile.AiInstructions,
	} {
		managed[key] = true
		if value != "" {
			written[key] = value
		}
	}

	// The order the file reads best in, not the order of a map.
	order := []string{
		"engine", "host", "port", "database", "user", "auth",
		"password", "password_env", "password_command",
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

// findBlockEnd returns where the block of a header ends: at the next header, or at the end of
// the file.
func findBlockEnd(lines []string, from int) int {
	for at := from; at < len(lines); at++ {
		if _, isHeader := readHeaderPath(lines[at]); isHeader {
			return at
		}
	}
	return len(lines)
}

// findWrittenEnd returns the same end, without the lines that lead into the next header: the
// blank lines between the two blocks, and a comment written above the header that follows.
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

// writeProfileBlock writes the profile into the text, and keeps every line outside its block
// exactly as it was.
func writeProfileBlock(text string, profile Profile) string {
	order, values, managed := buildProfileKeys(profile)

	lines := strings.Split(text, "\n")
	start := findProfileHeaderLine(lines, profile.Name)

	written := make([]string, 0, len(order))
	for _, key := range order {
		written = append(written, key+" = "+writeTomlValue(values[key]))
	}

	if start == -1 {
		// A table the file does not name yet goes after everything it holds.
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

	// An untouched key keeps its line, and with it any comment written on it.
	for _, line := range lines[start+1 : end] {
		key, isAssignment := readAssignmentKey(line)
		if !isAssignment {
			kept = append(kept, line)
			continue
		}
		// A setting the form never showed stays exactly as it was. Only the keys the form
		// owns are written again, so editing a connection never takes the page size or the
		// keepalive out of the file.
		if !managed[key] {
			kept = append(kept, line)
			continue
		}
		// A key the form owns and cleared is dropped, so clearing a password takes its
		// line out instead of leaving the old one behind.
		if _, holdsValue := values[key]; !holdsValue {
			continue
		}
		// A key already written is dropped, so a file that named one twice ends with one.
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

// findProfileHeaderLine returns the line the block of this profile opens on, and -1 where
// the text holds no block of that name.
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

// renameProfileBlock writes a new name over the header of a block and leaves every line
// under it as it was. A rename that took the block out and wrote a new one would lose the
// settings the form does not show, `mcp` and `page_size` among them, and every comment.
func renameProfileBlock(text, from, to string) string {
	lines := strings.Split(text, "\n")
	start := findProfileHeaderLine(lines, from)
	if start == -1 {
		return text
	}
	lines[start] = "[profile." + quoteHeaderName(to) + "]"
	return strings.Join(lines, "\n")
}

// removeProfileBlock takes the block of one profile out of the text.
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

// quoteHeaderName writes a profile name in a header, and quotes one the reader would not read
// back as written.
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

// readConfigText returns the text of the config file, and an empty text where there is none.
func readConfigText(path string) (string, error) {
	held, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", err
	}
	// A file that does not parse is left as it is, so no write can go over one.
	if _, decodeErr := DecodeDocument(string(held)); decodeErr != nil {
		return "", ConfigFileError{Reason: fmt.Sprintf(
			"%s is not valid TOML, so it was left as it is: %v", path, decodeErr)}
	}
	return string(held), nil
}

// writeConfigText writes the file, and creates the directory it belongs in. The text is
// written beside the file and moved over it, so a write that fails part way leaves the file
// it replaces whole. A config file cut short would lose every profile in it.
func writeConfigText(path, text string) error {
	// The directory may hold passwords.
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
	// The write reaches the disk before the move, so a machine that stops here comes back
	// to the old file and never to an empty one.
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

// SaveProfileToFile writes the profile into the config file. A rename takes the old block
// out first.
func SaveProfileToFile(profile Profile, replacing, path string) error {
	text, err := readConfigText(path)
	if err != nil {
		return err
	}
	if replacing != "" && replacing != profile.Name {
		// The block is renamed where it can be, so the settings the form does not show
		// and the comments written beside them all stay. Where the file already holds a
		// block of the new name, the old one goes and the settings are written into the
		// block that stays, because two blocks of one name are no file.
		if findProfileHeaderLine(strings.Split(text, "\n"), profile.Name) == -1 {
			text = renameProfileBlock(text, replacing, profile.Name)
		} else {
			text = removeProfileBlock(text, replacing)
		}
	}
	written := writeProfileBlock(text, profile)
	if _, decodeErr := DecodeDocument(written); decodeErr != nil {
		return ConfigFileError{Reason: fmt.Sprintf(
			"the profile did not write back as valid TOML, so %s was left as it is", path)}
	}
	return writeConfigText(path, written)
}

// RemoveProfileFromFile takes the profile out of the config file.
func RemoveProfileFromFile(name, path string) error {
	text, err := readConfigText(path)
	if err != nil {
		return err
	}
	return writeConfigText(path, removeProfileBlock(text, name))
}

// SaveTheme writes the chosen theme under `[ui] theme`, and keeps the rest of the file.
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
