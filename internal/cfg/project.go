package cfg

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/turanmahmudov/masume/internal/core"
)

// Project files contain shared connection profiles and queries. Personal settings and secret access settings are excluded.

// ProjectFileName is the file masume looks for from the working directory upward.
const ProjectFileName = ".masume.toml"

// projectSections are the supported project file sections.
var projectSections = []string{"profile", "query"}

// ProjectQuery is one statement a project file holds under a name.
type ProjectQuery struct {
	Name string
	SQL  string
	// The query description, displayed in place of SQL text.
	Description string
	// The profiles the statement is offered on. An empty list offers it on every one.
	Profiles []string
}

// MatchesProfile is true if the query is available for the profile.
func (query ProjectQuery) MatchesProfile(name string) bool {
	if len(query.Profiles) == 0 {
		return true
	}
	return slices.Contains(query.Profiles, name)
}

// ProjectConfig is the loaded project configuration.
type ProjectConfig struct {
	// The project file path, or empty if absent.
	Path     string
	Profiles []Profile
	Queries  []ProjectQuery
	// Errors and warnings, each with the project file path.
	Problems []string
}

// FindProjectFile returns the nearest project file, walking from the directory to the root
// of the file system.
func FindProjectFile(directory string) (string, bool) {
	at, err := filepath.Abs(directory)
	if err != nil {
		return "", false
	}
	for {
		path := filepath.Join(at, ProjectFileName)
		if info, statErr := os.Stat(path); statErr == nil && !info.IsDir() {
			return path, true
		}
		parent := filepath.Dir(at)
		if parent == at {
			return "", false
		}
		at = parent
	}
}

// LoadProjectConfig reads a project file. An unreadable file returns an error message and no profiles or queries.
func LoadProjectConfig(path string) ProjectConfig {
	found := ProjectConfig{Path: path}
	document, err := ReadDocument(path)
	if err != nil {
		found.Problems = []string{path + ": " + describeConfigFault(err)}
		return found
	}

	found.Profiles, found.Problems = parseProjectProfiles(document, path)
	queries, queryProblems := parseProjectQueries(document, path)
	found.Queries = queries
	found.Problems = append(found.Problems, queryProblems...)
	found.Problems = append(found.Problems, reportIgnoredKeys(document, path)...)
	return found
}

// refusedProjectKeys are forbidden project settings and their operations. All config files ignore the password key.
var refusedProjectKeys = map[string]string{
	"command":          "shell commands are forbidden in project files",
	"password_command": "shell commands are forbidden in project files",
	"password_env":     "environment password access is forbidden in project files",
	"secret":           "secret store access is forbidden in project files",
	"secret_ref":       "secret store access is forbidden in project files",
}

// findRefusedProjectKeys returns the keys of one profile a project file must not set, sorted.
func findRefusedProjectKeys(source Table) []string {
	found := []string{}
	for key := range source {
		if _, refused := refusedProjectKeys[key]; refused {
			found = append(found, key)
		}
	}
	slices.Sort(found)
	return found
}

// parseProjectProfiles reads project profiles and rejects profiles with forbidden settings.
func parseProjectProfiles(document Table, path string) ([]Profile, []string) {
	written, _ := FindSection(document, "profile")

	// Forbidden settings are checked before profile validation.
	problems := []string{}
	refusedNames := map[string]bool{}
	names := make([]string, 0, len(written))
	for name := range written {
		names = append(names, name)
	}
	slices.Sort(names)
	for _, name := range names {
		source, isTable := FindTable(written[name])
		if !isTable {
			continue
		}
		for _, key := range findRefusedProjectKeys(source) {
			refusedNames[name] = true
			problems = append(problems, fmt.Sprintf(
				"%s: skipped profile %q: forbidden setting %q; %s. "+
					"Use auth = \"prompt\" or auth = \"keyring\"",
				path, name, key, refusedProjectKeys[key]))
		}
	}

	parsed := ParseProfiles(document)
	for _, problem := range parsed.Problems {
		if refusedNames[problem.Name] {
			continue
		}
		problems = append(problems, fmt.Sprintf(
			"%s: skipped profile %q: %s", path, problem.Name, problem.Reason))
	}
	for _, warning := range parsed.Warnings {
		if refusedNames[warning.Name] {
			continue
		}
		problems = append(problems, fmt.Sprintf(
			"%s: profile %q: %s", path, warning.Name, warning.Reason))
	}

	kept := make([]Profile, 0, len(parsed.Profiles))
	for _, profile := range parsed.Profiles {
		if refusedNames[profile.Name] {
			continue
		}
		profile.ProjectFile = path
		profile.Database = resolveProjectDatabasePath(profile, path)
		kept = append(kept, profile)
	}
	return kept, problems
}

// resolveProjectDatabasePath resolves relative database paths against the project file directory.
func resolveProjectDatabasePath(profile Profile, path string) string {
	if !core.OpensFile(profile.Engine) {
		return profile.Database
	}
	if profile.Database == memoryDatabase || filepath.IsAbs(profile.Database) ||
		strings.HasPrefix(profile.Database, "~") {
		return profile.Database
	}
	return filepath.Join(filepath.Dir(path), profile.Database)
}

// parseProjectQueries reads the `[query]` section. A statement that cannot be read is
// reported and skipped, as a profile is.
func parseProjectQueries(document Table, path string) ([]ProjectQuery, []string) {
	written, present := FindSection(document, "query")
	if !present {
		return nil, nil
	}

	names := make([]string, 0, len(written))
	for name := range written {
		names = append(names, name)
	}
	slices.Sort(names)

	queries := make([]ProjectQuery, 0, len(names))
	problems := []string{}
	for _, name := range names {
		source, isTable := FindTable(written[name])
		if !isTable {
			problems = append(problems, fmt.Sprintf(
				"%s: skipped query %q: entry is not a table", path, name))
			continue
		}
		statement, holdsSQL := FindString(source, "sql")
		if !holdsSQL {
			problems = append(problems, fmt.Sprintf(
				"%s: skipped query %q: %q must be a non-empty string", path, name, "sql"))
			continue
		}
		description, _ := FindString(source, "description")
		profiles, _ := FindStringList(source, "profiles")
		queries = append(queries, ProjectQuery{
			Name: name, SQL: statement, Description: description, Profiles: profiles,
		})
	}
	return queries, problems
}

// reportIgnoredKeys reports unsupported top-level project settings.
func reportIgnoredKeys(document Table, path string) []string {
	names := make([]string, 0, len(document))
	for name := range document {
		if !slices.Contains(projectSections, name) {
			names = append(names, name)
		}
	}
	slices.Sort(names)

	problems := make([]string, 0, len(names))
	for _, name := range names {
		problems = append(problems, fmt.Sprintf(
			"%s: %q is unsupported in project files; use the user "+
				"config file", path, name))
	}
	return problems
}

// AddProjectProfiles merges user and project profiles. User profiles replace project profiles with the same name.
func AddProjectProfiles(user, project []Profile) []Profile {
	merged := make([]Profile, 0, len(user)+len(project))
	merged = append(merged, user...)
	for _, profile := range project {
		if slices.ContainsFunc(user, func(held Profile) bool {
			return held.Name == profile.Name
		}) {
			continue
		}
		merged = append(merged, profile)
	}
	slices.SortStableFunc(merged, func(left, right Profile) int {
		return strings.Compare(left.Name, right.Name)
	})
	return merged
}

// FindProjectQueries returns the statements of the project file the profile of that name can
// use.
func FindProjectQueries(project ProjectConfig, profileName string) []ProjectQuery {
	kept := make([]ProjectQuery, 0, len(project.Queries))
	for _, query := range project.Queries {
		if query.MatchesProfile(profileName) {
			kept = append(kept, query)
		}
	}
	return kept
}
