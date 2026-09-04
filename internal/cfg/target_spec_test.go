package cfg_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/turanmahmudov/masume/internal/cfg"
	"github.com/turanmahmudov/masume/internal/core"
)

// A URL on the command line must fill every part of the connection, so the client opens it
// without a profile in the config file.
func TestBuildProfileFromTargetReadsEveryPartOfAURL(t *testing.T) {
	built, err := cfg.BuildProfileFromTarget(
		"postgres://reader:hunter2@db.internal:6543/shop?sslmode=verify-full")
	if err != nil {
		t.Fatalf("the URL does not read: %v", err)
	}

	for _, one := range []struct {
		part string
		got  any
		want any
	}{
		{"engine", built.Engine, core.EnginePostgres},
		{"host", built.Host, "db.internal"},
		{"port", built.Port, 6543},
		{"database", built.Database, "shop"},
		{"user", built.User, "reader"},
		{"password", built.Password, "hunter2"},
		{"ssl mode", built.SSLMode, core.SSLVerifyFull},
		{"name", built.Name, "shop"},
	} {
		if one.got != one.want {
			t.Errorf("the %s reads %v, wanted %v", one.part, one.got, one.want)
		}
	}
}

// The settings a URL does not carry must take the defaults of a new connection, because the
// profile is never read from a file.
func TestBuildProfileFromTargetTakesTheDefaultsOfANewConnection(t *testing.T) {
	built, err := cfg.BuildProfileFromTarget("postgres://ada@127.0.0.1/shop")
	if err != nil {
		t.Fatalf("the URL does not read: %v", err)
	}

	for _, one := range []struct {
		part string
		got  any
		want any
	}{
		{"port", built.Port, core.ResolveDefaultPort(core.EnginePostgres)},
		{"environment", built.Environment, cfg.EnvironmentDev},
		{"access mode", built.AccessMode, cfg.AccessWrite},
		{"confirm writes", built.ConfirmWrites, cfg.ConfirmOff},
		{"autocommit", built.Autocommit, true},
		{"page size", built.PageSize, cfg.DefaultPageSize},
		{"keepalive", built.Keepalive, cfg.DefaultKeepalive},
		{"command timeout", built.CommandTimeout, cfg.DefaultCommandTimeout},
		{"statement timeout", built.StatementTimeout, time.Duration(0)},
	} {
		if one.got != one.want {
			t.Errorf("the %s reads %v, wanted %v", one.part, one.got, one.want)
		}
	}
}

// The scheme selects the engine, so every engine that has one can be opened by URL.
func TestBuildProfileFromTargetReadsTheEngineOfTheScheme(t *testing.T) {
	for _, one := range []struct {
		url    string
		engine core.Engine
		port   int
	}{
		{"postgres://ada@host/shop", core.EnginePostgres, 5432},
		{"postgresql://ada@host/shop", core.EnginePostgres, 5432},
		{"mysql://ada@host/shop", core.EngineMysql, 3306},
		{"mariadb://ada@host/shop", core.EngineMariadb, 3306},
		{"cockroachdb://ada@host/shop", core.EngineCockroach, 26257},
		{"redshift://ada@host/shop", core.EngineRedshift, 5439},
		{"mongodb://host/shop", core.EngineMongo, 27017},
	} {
		built, err := cfg.BuildProfileFromTarget(one.url)
		if err != nil {
			t.Errorf("%s does not read: %v", one.url, err)
			continue
		}
		if built.Engine != one.engine {
			t.Errorf("%s opens %s, wanted %s", one.url, built.Engine, one.engine)
		}
		if built.Port != one.port {
			t.Errorf("%s uses port %d, wanted %d", one.url, built.Port, one.port)
		}
	}
}

// A URL that names no database must fall back to what the server itself falls back to,
// because both forms are what a user pastes.
func TestBuildProfileFromTargetFillsTheDatabaseTheServerDefaultsTo(t *testing.T) {
	for _, one := range []struct {
		url      string
		database string
	}{
		{"postgres://ada@host", "ada"},
		{"postgres://ada@host/", "ada"},
		{"mongodb://host", "admin"},
	} {
		built, err := cfg.BuildProfileFromTarget(one.url)
		if err != nil {
			t.Errorf("%s does not read: %v", one.url, err)
			continue
		}
		if built.Database != one.database {
			t.Errorf("%s opens %q, wanted %q", one.url, built.Database, one.database)
		}
	}
}

// A MySQL server has no database of the same name as the user, so a URL without one cannot
// be opened and must be reported.
func TestBuildProfileFromTargetReportsAMysqlURLWithoutADatabase(t *testing.T) {
	if _, err := cfg.BuildProfileFromTarget("mysql://ada@host"); err == nil {
		t.Fatal("a MySQL URL without a database was read")
	}
}

// A host without a name is a URL of the form `postgres:///shop`, which the client opens on
// the local machine.
func TestBuildProfileFromTargetFillsTheLocalHost(t *testing.T) {
	built, err := cfg.BuildProfileFromTarget("postgres:///shop")
	if err != nil {
		t.Fatalf("the URL does not read: %v", err)
	}
	if built.Host != "127.0.0.1" {
		t.Errorf("the host reads %q, wanted the local host", built.Host)
	}
}

// An IPv6 address is written in brackets in a URL. Every other reader needs it without them.
func TestBuildProfileFromTargetReadsAnIPv6Host(t *testing.T) {
	built, err := cfg.BuildProfileFromTarget("postgres://ada@[::1]:5433/shop")
	if err != nil {
		t.Fatalf("the URL does not read: %v", err)
	}
	if built.Host != "::1" {
		t.Errorf("the host reads %q, wanted ::1", built.Host)
	}
	if built.Port != 5433 {
		t.Errorf("the port reads %d, wanted 5433", built.Port)
	}
}

// A target the client cannot open must be reported, because a client that opened the picker
// instead would look as if the argument was accepted.
func TestBuildProfileFromTargetReportsATargetItCannotRead(t *testing.T) {
	for _, target := range []string{
		"",
		"   ",
		"ftp://host/shop",
		"postgres://ada@host:0/shop",
		"postgres://ada@host:port/shop",
		"postgres://ada@host/shop?sslmode=maybe",
		"shop",
		"host=db dbname=shop sslmode=maybe",
		"host=db dbname='shop",
		"host=db unknown=1",
	} {
		if _, err := cfg.BuildProfileFromTarget(target); err == nil {
			t.Errorf("%q was read as a connection", target)
		}
	}
}

// A path is opened as a SQLite file, which is the form a user types for a file that is in
// the directory they are in.
func TestBuildProfileFromTargetOpensASqliteFile(t *testing.T) {
	built, err := cfg.BuildProfileFromTarget("./notes.db")
	if err != nil {
		t.Fatalf("the path does not read: %v", err)
	}
	if built.Engine != core.EngineSqlite {
		t.Errorf("the engine reads %q, wanted sqlite", built.Engine)
	}
	if built.Database != "./notes.db" {
		t.Errorf("the file reads %q, wanted ./notes.db", built.Database)
	}
	if built.Name != "notes" {
		t.Errorf("the name reads %q, wanted notes", built.Name)
	}
	if built.Host != "" || built.Port != 0 {
		t.Errorf("the file has host %q and port %d, wanted neither", built.Host, built.Port)
	}
}

// A database file with no extension is opened when it is there, because the name alone does
// not carry what it holds.
func TestBuildProfileFromTargetOpensAFileThatIsThere(t *testing.T) {
	path := filepath.Join(t.TempDir(), "ledger")
	if err := os.WriteFile(path, []byte("SQLite format 3\x00"), 0o600); err != nil {
		t.Fatalf("the file cannot be written: %v", err)
	}

	built, err := cfg.BuildProfileFromTarget(path)
	if err != nil {
		t.Fatalf("the path does not read: %v", err)
	}
	if built.Engine != core.EngineSqlite {
		t.Errorf("the engine reads %q, wanted sqlite", built.Engine)
	}
	if built.Name != "ledger" {
		t.Errorf("the name reads %q, wanted ledger", built.Name)
	}
}

// A database in memory is a SQLite file name of its own, and it holds no path to take a
// name from.
func TestBuildProfileFromTargetOpensTheDatabaseInMemory(t *testing.T) {
	built, err := cfg.BuildProfileFromTarget(":memory:")
	if err != nil {
		t.Fatalf("the target does not read: %v", err)
	}
	if built.Engine != core.EngineSqlite {
		t.Errorf("the engine reads %q, wanted sqlite", built.Engine)
	}
	if built.Name != "memory" {
		t.Errorf("the name reads %q, wanted memory", built.Name)
	}
}

// A connection string of `key=value` pairs is the other form a PostgreSQL client accepts,
// and a value in it can be quoted and hold a space.
func TestBuildProfileFromTargetReadsAKeywordConnectionString(t *testing.T) {
	built, err := cfg.BuildProfileFromTarget(
		"host=db.internal port=6543 dbname=shop user=reader password='two words' sslmode=require")
	if err != nil {
		t.Fatalf("the connection string does not read: %v", err)
	}

	for _, one := range []struct {
		part string
		got  any
		want any
	}{
		{"engine", built.Engine, core.EnginePostgres},
		{"host", built.Host, "db.internal"},
		{"port", built.Port, 6543},
		{"database", built.Database, "shop"},
		{"user", built.User, "reader"},
		{"password", built.Password, "two words"},
		{"ssl mode", built.SSLMode, core.SSLRequire},
		{"name", built.Name, "shop"},
	} {
		if one.got != one.want {
			t.Errorf("the %s reads %v, wanted %v", one.part, one.got, one.want)
		}
	}
}

// A password in the target is the password of the connection, so the client must not ask
// for one. A target without a password must be asked for.
func TestBuildProfileFromTargetAsksForAPasswordOnlyWhenItHasNone(t *testing.T) {
	withPassword, err := cfg.BuildProfileFromTarget("postgres://ada:secret@host/shop")
	if err != nil {
		t.Fatalf("the URL does not read: %v", err)
	}
	if cfg.NeedsPasswordPrompt(withPassword) {
		t.Error("a URL that carries a password is asked for one")
	}

	without, err := cfg.BuildProfileFromTarget("postgres://ada@host/shop")
	if err != nil {
		t.Fatalf("the URL does not read: %v", err)
	}
	if !cfg.NeedsPasswordPrompt(without) {
		t.Error("a URL without a password is not asked for one")
	}
}

// A target must not take the name of a profile of the config file, because the picker names
// the connection it opens and two rows with one name cannot be told apart.
func TestResolveUniqueProfileNameStepsPastTheNamesInUse(t *testing.T) {
	held := []cfg.Profile{{Name: "shop"}, {Name: "shop-2"}}

	if name := cfg.ResolveUniqueProfileName(held, "ledger"); name != "ledger" {
		t.Errorf("a free name reads %q, wanted ledger", name)
	}
	if name := cfg.ResolveUniqueProfileName(held, "shop"); name != "shop-3" {
		t.Errorf("a name in use reads %q, wanted shop-3", name)
	}
	if name := cfg.ResolveUniqueProfileName(nil, "shop"); name != "shop" {
		t.Errorf("a name with no profiles reads %q, wanted shop", name)
	}
}

// A connection opened from the command line is saved through the connection form, so the
// profile a target builds must read back from the config file as the same connection.
func TestAProfileFromATargetSavesToTheConfigFile(t *testing.T) {
	built, err := cfg.BuildProfileFromTarget(
		"postgres://reader:hunter2@db.internal:6543/shop?sslmode=verify-full")
	if err != nil {
		t.Fatalf("the URL does not read: %v", err)
	}

	path := filepath.Join(t.TempDir(), "config.toml")
	if writeErr := cfg.SaveProfileToFile(built, "", path); writeErr != nil {
		t.Fatalf("the profile does not save: %v", writeErr)
	}

	loaded := cfg.LoadConfig(path)
	if len(loaded.Problems) > 0 {
		t.Fatalf("the saved file reports %v", loaded.Problems)
	}
	if len(loaded.Profiles) != 1 {
		t.Fatalf("the saved file holds %d profiles, wanted one", len(loaded.Profiles))
	}

	held := loaded.Profiles[0]
	for _, one := range []struct {
		part string
		got  any
		want any
	}{
		{"name", held.Name, built.Name},
		{"engine", held.Engine, built.Engine},
		{"host", held.Host, built.Host},
		{"port", held.Port, built.Port},
		{"database", held.Database, built.Database},
		{"user", held.User, built.User},
		{"ssl mode", held.SSLMode, built.SSLMode},
	} {
		if one.got != one.want {
			t.Errorf("the saved %s reads %v, wanted %v", one.part, one.got, one.want)
		}
	}
	// The URL carried a password. No file masume writes holds one, so it is not in the
	// saved profile and not in the text of the file.
	if held.Password != "" {
		t.Errorf("the saved profile carries the password %q", held.Password)
	}
	written, readErr := os.ReadFile(path)
	if readErr != nil {
		t.Fatalf("the config file could not be read: %v", readErr)
	}
	if strings.Contains(string(written), "hunter2") {
		t.Errorf("the config file holds the password:\n%s", written)
	}
}

// A port is sent as two bytes, so a number above the range of one cannot be connected to.
func TestBuildProfileFromTargetRefusesAPortOutsideItsRange(t *testing.T) {
	for _, target := range []string{
		"postgres://ada@host:65536/shop",
		"postgres://ada@host:99999/shop",
		"host=db dbname=shop port=70000",
	} {
		if _, err := cfg.BuildProfileFromTarget(target); err == nil {
			t.Errorf("%q was read as a connection", target)
		}
	}
	// The largest port there is still opens.
	if _, err := cfg.BuildProfileFromTarget("postgres://ada@host:65535/shop"); err != nil {
		t.Errorf("the largest port there is does not read: %v", err)
	}
}

// A URL names one database. A path of several parts is a URL of something else, and reading
// its first part as a database would open one nobody asked for.
func TestBuildProfileFromTargetRefusesAPathOfSeveralParts(t *testing.T) {
	for _, target := range []string{
		"postgres://ada@host/shop/extra",
		"mysql://ada@host/shop/orders/1",
	} {
		if _, err := cfg.BuildProfileFromTarget(target); err == nil {
			t.Errorf("%q was read as a connection", target)
		}
	}
}

// A file with no extension of a database is opened only where it holds one. Without that
// every file in the directory is a database and a document opens as one.
func TestBuildProfileFromTargetOpensOnlyAFileThatHoldsADatabase(t *testing.T) {
	directory := t.TempDir()

	document := filepath.Join(directory, "README")
	if err := os.WriteFile(document, []byte("# not a database\n"), 0o600); err != nil {
		t.Fatalf("the file cannot be written: %v", err)
	}
	if _, err := cfg.BuildProfileFromTarget(document); err == nil {
		t.Error("a document was opened as a database")
	}

	database := filepath.Join(directory, "ledger")
	if err := os.WriteFile(
		database, []byte("SQLite format 3\x00and the rest"), 0o600); err != nil {
		t.Fatalf("the file cannot be written: %v", err)
	}
	if _, err := cfg.BuildProfileFromTarget(database); err != nil {
		t.Errorf("a database with no extension does not open: %v", err)
	}
}

// The keywords of a connection string carry nothing about which server they are for, so the
// string names its engine where it is not the default one.
func TestBuildProfileFromTargetReadsTheEngineOfAConnectionString(t *testing.T) {
	built, err := cfg.BuildProfileFromTarget(
		"engine=mysql host=db.internal dbname=shop user=root")
	if err != nil {
		t.Fatalf("the connection string does not read: %v", err)
	}
	if built.Engine != core.EngineMysql {
		t.Errorf("the engine reads %q, wanted mysql", built.Engine)
	}
	// The port it leaves out is the port of the engine it named.
	if built.Port != core.ResolveDefaultPort(core.EngineMysql) {
		t.Errorf("the port reads %d, wanted the one MySQL uses", built.Port)
	}

	if _, err := cfg.BuildProfileFromTarget(
		"engine=oracle host=db dbname=shop"); err == nil {
		t.Error("an engine this client does not open was read")
	}
}

// A backslash escapes the letter after it inside quotes alone, as libpq reads it, so a
// Windows path keeps its separators.
func TestBuildProfileFromTargetKeepsTheBackslashesOfAPath(t *testing.T) {
	built, err := cfg.BuildProfileFromTarget(`engine=sqlite dbname=C:\db\shop.db`)
	if err != nil {
		t.Fatalf("the connection string does not read: %v", err)
	}
	if built.Database != `C:\db\shop.db` {
		t.Errorf("the database reads %q, wanted the path with its backslashes", built.Database)
	}

	quoted, err := cfg.BuildProfileFromTarget(`engine=sqlite dbname='a\'b'`)
	if err != nil {
		t.Fatalf("the quoted value does not read: %v", err)
	}
	if quoted.Database != `a'b` {
		t.Errorf("the quoted value reads %q, wanted a'b", quoted.Database)
	}
}
