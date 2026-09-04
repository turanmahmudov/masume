package cfg

import (
	"bytes"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	"github.com/turanmahmudov/masume/internal/core"
)

// Reads one connection target given on the command line, so the client opens a database
// without a profile in the config file first. Three forms are read: a URL, a keyword DSN,
// and the path of a database file.

// CommandLineProfileName is the name of a profile built from a target that names no
// database to take a name from.
const CommandLineProfileName = "command-line"

// defaultTargetHost is the host of a URL that names none, for example `postgres:///shop`.
const defaultTargetHost = "127.0.0.1"

// TargetError is a connection target the client cannot read.
type TargetError struct{ Reason string }

func (err TargetError) Error() string { return err.Reason }

func failTarget(format string, parts ...any) error {
	return TargetError{Reason: fmt.Sprintf(format, parts...)}
}

// databaseFileExtensions are the names a SQLite file is recognised by.
var databaseFileExtensions = []string{".db", ".db3", ".sqlite", ".sqlite3"}

// memoryDatabase is the SQLite database that is never written to a file.
const memoryDatabase = ":memory:"

// keywordAliases give the profile key each keyword of a DSN sets.
var keywordAliases = map[string]string{
	"host": "host", "hostaddr": "host", "port": "port",
	"dbname": "database", "database": "database",
	"user": "user", "password": "password", "sslmode": "sslmode",
	// `engine` is not a keyword of a PostgreSQL connection string. The keywords carry
	// nothing about which server they are for.
	"engine": "engine",
}

// highestPort is the largest port a server can listen on.
const highestPort = 65535

// readPortNumber reads a port of a target.
func readPortNumber(written string) (int, error) {
	port, err := strconv.Atoi(written)
	if err != nil || port <= 0 || port > highestPort {
		return 0, failTarget("%q is not a port", written)
	}
	return port, nil
}

// buildTargetProfile returns the profile a target starts from.
func buildTargetProfile(engine core.Engine) Profile {
	return Profile{
		Engine: engine, Port: core.ResolveDefaultPort(engine),
		Auth: AuthPassword, Environment: EnvironmentDev, AccessMode: AccessWrite,
		SSLMode: core.ResolveEngineInfo(engine).DefaultSSLMode, Autocommit: true,
		ConfirmWrites:  resolveDefaultConfirmWrites(EnvironmentDev),
		CommandTimeout: DefaultCommandTimeout, PageSize: DefaultPageSize,
		Keepalive: DefaultKeepalive,
	}
}

// BuildProfileFromTarget reads one connection target: a URL, a keyword DSN, or the path of
// a database file. The profile it returns is not in the config file.
func BuildProfileFromTarget(text string) (Profile, error) {
	trimmed := strings.TrimSpace(text)
	if trimmed == "" {
		return Profile{}, failTarget("a connection target cannot be empty")
	}

	var built Profile
	var err error
	switch {
	case strings.Contains(trimmed, "://"):
		built, err = buildProfileFromURL(trimmed)
	case holdsKeywordPairs(trimmed):
		built, err = buildProfileFromKeywords(trimmed)
	default:
		built, err = buildProfileFromFilePath(trimmed)
	}
	if err != nil {
		return Profile{}, err
	}

	built.Name = buildTargetProfileName(built)
	return built, nil
}

// describeKnownSchemes returns the URL schemes a target can use, for the error of one that
// uses another.
func describeKnownSchemes() string {
	names := make([]string, 0, len(urlSchemes))
	for scheme := range urlSchemes {
		names = append(names, scheme)
	}
	slices.Sort(names)
	return strings.Join(names, ", ")
}

// resolveDefaultDatabase returns the database a URL without one connects to.
func resolveDefaultDatabase(engine core.Engine, user string) string {
	switch core.ResolveEngineInfo(engine).Family {
	case core.FamilyPostgres:
		return user
	case core.FamilyRedis:
		return "0"
	case core.FamilyMongo:
		return "admin"
	}
	return ""
}

// readTargetSSLMode returns the SSL mode of a URL: the one its query names, the one its
// scheme implies, or the default of its engine.
func readTargetSSLMode(parsed *url.URL, engine core.Engine) (core.SSLMode, error) {
	written := ""
	for _, key := range sslKeys {
		if held := parsed.Query().Get(key); held != "" {
			written = held
			break
		}
	}
	if written == "" {
		if implied, asks := tlsSchemes[strings.ToLower(parsed.Scheme)]; asks {
			return implied, nil
		}
		return core.ResolveEngineInfo(engine).DefaultSSLMode, nil
	}
	mode, known := core.FindSSLMode(written)
	if !known {
		return core.SSLUnset, failTarget(
			"sslmode %q is not one of %s", written, core.SSLModeNames())
	}
	return mode, nil
}

func buildProfileFromURL(text string) (Profile, error) {
	parsed, err := url.Parse(text)
	if err != nil {
		return Profile{}, failTarget(
			"the connection URL cannot be read; a password in it has to be escaped, " +
				"so a %% is written %%25 and a / is written %%2F")
	}

	engine, known := urlSchemes[strings.ToLower(parsed.Scheme)]
	if !known {
		return Profile{}, failTarget("%q is not a scheme this command reads, which are %s",
			parsed.Scheme, describeKnownSchemes())
	}

	built := buildTargetProfile(engine)
	// A URL puts an IPv6 address in brackets. Every other reader needs it without them.
	built.Host = strings.Trim(parsed.Hostname(), "[]")
	if strings.Contains(built.Host, ",") {
		return Profile{}, failTarget(
			"the URL names more than one host, and this client opens one")
	}
	if built.Host == "" {
		built.Host = defaultTargetHost
	}
	if written := parsed.Port(); written != "" {
		port, portErr := readPortNumber(written)
		if portErr != nil {
			return Profile{}, portErr
		}
		built.Port = port
	}
	if parsed.User != nil {
		built.User = parsed.User.Username()
		if password, set := parsed.User.Password(); set {
			built.Password = password
		}
	}

	built.Database = strings.TrimPrefix(parsed.Path, "/")
	// A URL names one database, so a path of several parts is a URL of something else.
	if strings.Contains(built.Database, "/") {
		return Profile{}, failTarget(
			"%q names more than one database", built.Database)
	}
	if built.Database == "" {
		built.Database = resolveDefaultDatabase(engine, built.User)
	}
	if built.Database == "" {
		return Profile{}, failTarget("the URL names no database")
	}

	if built.SSLMode, err = readTargetSSLMode(parsed, engine); err != nil {
		return Profile{}, err
	}
	return built, nil
}

// holdsKeywordPairs is true for a target written as `key=value` pairs, whose first pair
// always names a keyword this command knows.
func holdsKeywordPairs(text string) bool {
	pairs, err := splitKeywordPairs(text)
	if err != nil || len(pairs) == 0 {
		return false
	}
	_, known := keywordAliases[strings.ToLower(pairs[0][0])]
	return known
}

// splitKeywordPairs splits a DSN into its pairs. A value can be quoted with single quotes
// and can hold a space, and a backslash escapes the character after it.
func splitKeywordPairs(text string) ([][2]string, error) {
	pairs := [][2]string{}
	runes := []rune(text)
	at := 0
	for at < len(runes) {
		for at < len(runes) && runes[at] == ' ' {
			at++
		}
		if at >= len(runes) {
			break
		}

		key := strings.Builder{}
		for at < len(runes) && runes[at] != '=' && runes[at] != ' ' {
			key.WriteRune(runes[at])
			at++
		}
		if at >= len(runes) || runes[at] != '=' {
			return nil, failTarget("%q sets no value", key.String())
		}
		at++

		value := strings.Builder{}
		quoted := at < len(runes) && runes[at] == '\''
		if quoted {
			at++
		}
		for at < len(runes) {
			if quoted && runes[at] == '\\' && at+1 < len(runes) {
				at++
				value.WriteRune(runes[at])
				at++
				continue
			}
			if quoted && runes[at] == '\'' {
				at++
				quoted = false
				break
			}
			if !quoted && runes[at] == ' ' {
				break
			}
			value.WriteRune(runes[at])
			at++
		}
		if quoted {
			return nil, failTarget("a quoted value is not closed")
		}
		pairs = append(pairs, [2]string{key.String(), value.String()})
	}
	return pairs, nil
}

// readKeywordEngine returns the engine a connection string names, or the default where it
// names none.
func readKeywordEngine(pairs [][2]string) (core.Engine, error) {
	for _, pair := range pairs {
		if !strings.EqualFold(pair[0], "engine") {
			continue
		}
		engine, known := core.FindEngine(pair[1])
		if !known {
			return "", failTarget("%q is not an engine this client opens", pair[1])
		}
		return engine, nil
	}
	return core.DefaultEngine, nil
}

// buildProfileFromKeywords reads a `key=value` connection string.
func buildProfileFromKeywords(text string) (Profile, error) {
	pairs, err := splitKeywordPairs(text)
	if err != nil {
		return Profile{}, err
	}

	// The port and the SSL mode take the default of the engine, so it is read first.
	engine, err := readKeywordEngine(pairs)
	if err != nil {
		return Profile{}, err
	}

	built := buildTargetProfile(engine)
	for _, pair := range pairs {
		key, known := keywordAliases[strings.ToLower(pair[0])]
		if !known {
			return Profile{}, failTarget(
				"%q is not a keyword this command reads", pair[0])
		}
		value := pair[1]
		switch key {
		case "engine":
			continue
		case "host":
			built.Host = value
		case "port":
			port, portErr := readPortNumber(value)
			if portErr != nil {
				return Profile{}, portErr
			}
			built.Port = port
		case "database":
			built.Database = value
		case "user":
			built.User = value
		case "password":
			built.Password = value
		case "sslmode":
			mode, isMode := core.FindSSLMode(value)
			if !isMode {
				return Profile{}, failTarget(
					"sslmode %q is not one of %s", value, core.SSLModeNames())
			}
			built.SSLMode = mode
		}
	}

	if built.Host == "" {
		built.Host = defaultTargetHost
	}
	if built.Database == "" {
		built.Database = built.User
	}
	if built.Database == "" {
		return Profile{}, failTarget("the connection string names no database")
	}
	return built, nil
}

// holdsDatabaseFile is true for a path the client opens as a SQLite database: a file that
// is there already, or a name with an extension a database file uses.
func holdsDatabaseFile(path string) bool {
	if path == memoryDatabase {
		return true
	}
	if slices.Contains(databaseFileExtensions, strings.ToLower(filepath.Ext(path))) {
		return true
	}
	// Without this `masume README.md` opens a document as a database.
	return holdsSqliteHeader(core.ExpandHomePath(path))
}

// sqliteHeader opens every SQLite file, including the zero byte at its end.
var sqliteHeader = []byte("SQLite format 3\x00")

// holdsSqliteHeader is true where the file opens with the mark every SQLite file carries.
func holdsSqliteHeader(path string) bool {
	held, err := os.Open(path)
	if err != nil {
		return false
	}
	defer func() { _ = held.Close() }()

	read := make([]byte, len(sqliteHeader))
	if _, err := io.ReadFull(held, read); err != nil {
		return false
	}
	return bytes.Equal(read, sqliteHeader)
}

func buildProfileFromFilePath(path string) (Profile, error) {
	if !holdsDatabaseFile(path) {
		return Profile{}, failTarget(
			"%s is not a URL, a connection string, or a database file that is there", path)
	}
	built := buildTargetProfile(core.EngineSqlite)
	built.Database = path
	return built, nil
}

// buildTargetProfileName returns the name the picker lists the target under: the name of
// the database, or the name of the file without its extension.
func buildTargetProfileName(profile Profile) string {
	if !core.OpensFile(profile.Engine) {
		if profile.Database == "" {
			return CommandLineProfileName
		}
		return profile.Database
	}
	if profile.Database == memoryDatabase {
		return "memory"
	}
	base := filepath.Base(profile.Database)
	name := strings.TrimSuffix(base, filepath.Ext(base))
	if name == "" || name == "." || name == string(filepath.Separator) {
		return CommandLineProfileName
	}
	return name
}

// ResolveUniqueProfileName returns a name no profile in the list holds.
func ResolveUniqueProfileName(profiles []Profile, wanted string) string {
	held := func(name string) bool {
		return slices.ContainsFunc(profiles, func(profile Profile) bool {
			return profile.Name == name
		})
	}
	if !held(wanted) {
		return wanted
	}
	for suffix := 2; ; suffix++ {
		candidate := wanted + "-" + strconv.Itoa(suffix)
		if !held(candidate) {
			return candidate
		}
	}
}
