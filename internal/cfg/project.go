package cfg

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/turanmahmudov/masume/internal/core"
)

// The project file: the connections and the statements a team commits next to its code. It
// holds no setting of the person who opens it, and nothing that reaches a secret, because it
// is a file in a repository that everybody reads and anybody can change.

// ProjectFileName is the file masume looks for from the working directory upward.
const ProjectFileName = ".masume.toml"

// projectSections are the sections a project file provides. Every other key belongs to the
// user alone: a repository must not set the keys, the theme, the interface or the AI
// provider of the person who opens it, and it must not name the profiles an agent reaches.
var projectSections = []string{"profile", "query"}

// ProjectQuery is one statement a project file holds under a name.
type ProjectQuery struct {
	Name string
	SQL  string
	// What the statement answers, shown instead of its text.
	Description string
	// The profiles the statement is offered on. An empty list offers it on every one.
	Profiles []string
}

// MatchesProfile is true where the statement is offered on the profile of that name.
func (query ProjectQuery) MatchesProfile(name string) bool {
	if len(query.Profiles) == 0 {
		return true
	}
	return slices.Contains(query.Profiles, name)
}

// ProjectConfig is what one project file provides.
type ProjectConfig struct {
	// The path of the file. Empty where the walk found none.
	Path     string
	Profiles []Profile
	Queries  []ProjectQuery
	// What the file could not provide, one line each, ready to show. Each line names the
	// path, because the user did not choose this file on the command line.
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

// LoadProjectConfig reads one project file. A file that cannot be read reports the reason
// and provides nothing, so a broken file in a repository does not stop the client.
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

// refusedProjectKeys are the profile keys a project file must not set, with the reason. The
// file is committed, so anyone who can open a pull request can set them. `command` and
// `password_command` run a shell on connect, and would be arbitrary code from a repository.
// The secret keys read a secret of the store of the user, and a profile names the server that
// secret is then sent to. A `password` in any file is ignored everywhere, so it needs no entry
// here.
var refusedProjectKeys = map[string]string{
	"command":          "it runs a shell command on connect",
	"password_command": "it runs a shell command on connect",
	"password_env":     "it reads a variable of your shell",
	"secret":           "it reads a secret of your own store",
	"secret_ref":       "it reads a secret of your own store",
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

// parseProjectProfiles reads the `[profile]` section of a project file and refuses every
// profile that sets a key of refusedProjectKeys. A profile of a repository says where the
// server is; it never says how to reach a secret.
func parseProjectProfiles(document Table, path string) ([]Profile, []string) {
	written, _ := FindSection(document, "profile")

	// The refused keys are read from the file itself, before a profile is built from it. A
	// profile that reaches for a secret is refused for that reason and for no other, so the
	// report says what is wrong rather than what the missing half of it broke.
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
				"%s: skipped profile %q: a project file cannot set %q, because %s. "+
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

// resolveProjectDatabasePath returns the database file of a profile against the directory of
// the project file. A committed `./notes.db` means the file of the repository, whichever
// directory of it masume was started in.
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

// reportIgnoredKeys names the top-level keys of a project file the client does not read, so
// a setting written in the wrong file is not lost in silence.
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
			"%s: %q is not read from a project file; write it in the config file of "+
				"the user instead", path, name))
	}
	return problems
}

// AddProjectProfiles returns the profiles of the user with the ones of the project it does
// not name. A profile of the user replaces the project profile of the same name, so a
// personal setting always wins over a committed one.
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
