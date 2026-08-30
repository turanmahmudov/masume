package cfg

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/BurntSushi/toml"

	"github.com/turanmahmudov/masume/internal/core"
)

// LoadedConfig is everything one read of the config file gives.
type LoadedConfig struct {
	ParsedProfiles
	Settings UISettings
	Keys     KeySettings
	Ai       AiConfig
	Mcp      McpConfig
	// The themes the user wrote, which replace the shipped ones of the same name.
	Themes        []ThemeDocument
	ThemeProblems []string
}

// ResolveConfigPath returns where the config file is.
func ResolveConfigPath() string {
	configHome := os.Getenv("XDG_CONFIG_HOME")
	if configHome == "" {
		configHome = filepath.Join(core.HomeDirectory(), ".config")
	}
	return filepath.Join(configHome, "masume", "config.toml")
}

// ResolveThemesPath returns where the themes of the user are, one file per theme,
// beside the config.
func ResolveThemesPath(configPath string) string {
	return filepath.Join(filepath.Dir(configPath), "themes")
}

// ReadDocument reads one TOML file into a table.
func ReadDocument(path string) (Table, error) {
	text, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return DecodeDocument(string(text))
}

// DecodeDocument reads TOML text into a table.
func DecodeDocument(text string) (Table, error) {
	document := map[string]any{}
	if _, err := toml.Decode(text, &document); err != nil {
		return nil, err
	}
	return Table(document), nil
}

// ReadThemeDocuments reads the themes of the user, and everything in their files
// that could not be read. It is empty if the directory does not exist, which is
// the usual case.
func ReadThemeDocuments(themesPath string) ([]ThemeDocument, []string) {
	documents := []ThemeDocument{}
	problems := []string{}

	entries, err := os.ReadDir(themesPath)
	if err != nil {
		return documents, problems
	}

	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".toml") {
			names = append(names, entry.Name())
		}
	}
	slices.Sort(names)

	for _, name := range names {
		themeName := strings.TrimSuffix(name, ".toml")
		document, readErr := ReadDocument(filepath.Join(themesPath, name))
		if readErr != nil {
			problems = append(problems, fmt.Sprintf("theme %q: %v", themeName, readErr))
			continue
		}
		parsed, found := ParseThemeDocument(document, themeName)
		documents = append(documents, parsed)
		for _, problem := range found {
			problems = append(problems, fmt.Sprintf("theme %q: %s", themeName, problem))
		}
	}
	return documents, problems
}

// buildDefaultConfig returns every default, with the reason the file gave nothing.
// The themes still stand.
func buildDefaultConfig(path, reason string, themes []ThemeDocument, themeProblems []string) LoadedConfig {
	return LoadedConfig{
		Problems:      []ProfileProblem{{Name: path, Reason: reason}},
		Settings:      DefaultUISettings(),
		Keys:          DefaultKeySettings(),
		Ai:            DefaultAiConfig(),
		Mcp:           DefaultMcpConfig(),
		Themes:        themes,
		ThemeProblems: themeProblems,
	}
}

// describeConfigFault says why the config file gave nothing. A file that is not there is the
// first run of the client. A file that is there and cannot be read loses every profile, and
// so does one that is not TOML, and the user has to be told which of the three it is.
func describeConfigFault(err error) string {
	if errors.Is(err, fs.ErrNotExist) {
		return "config file not found"
	}
	// The read returns a path error, and the decode returns a fault in the text.
	if _, ok := errors.AsType[*fs.PathError](err); ok {
		return fmt.Sprintf("config file cannot be read: %v", err)
	}
	return fmt.Sprintf("invalid TOML: %v", err)
}

// LoadConfig reads the file once, because the profiles and the settings share it.
func LoadConfig(path string) LoadedConfig {
	// The themes are read whatever the config file says, because the palette can
	// choose a theme the config never named.
	themes, themeProblems := ReadThemeDocuments(ResolveThemesPath(path))

	document, err := ReadDocument(path)
	if err != nil {
		return buildDefaultConfig(path, describeConfigFault(err), themes, themeProblems)
	}

	return LoadedConfig{
		ParsedProfiles: ParseProfiles(document),
		Settings:       ParseUISettings(document),
		Keys:           ParseKeySettings(document),
		Ai:             ParseAiConfig(document),
		Mcp:            ParseMcpConfig(document),
		Themes:         themes,
		ThemeProblems:  themeProblems,
	}
}
