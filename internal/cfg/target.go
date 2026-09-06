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

// Connection targets are URLs, keyword DSNs, or database file paths.

// CommandLineProfileName is the fallback profile name for a target without a database name.
const CommandLineProfileName = "command-line"

// defaultTargetHost is the fallback host for a URL such as `postgres:///shop`.
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

// keywordAliases are the profile keys for DSN keywords.
var keywordAliases = map[string]string{
	"host": "host", "hostaddr": "host", "port": "port",
	"dbname": "database", "database": "database",
	"user": "user", "password": "password", "sslmode": "sslmode",
	// `engine` is a masume extension to PostgreSQL connection keywords.
	"engine": "engine",
}

// highestPort is the largest port a server can listen on.
const highestPort = 65535

// readPortNumber reads a port of a target.
func readPortNumber(written string) (int, error) {
	port, err := strconv.Atoi(written)
	if err != nil || port <= 0 || port > highestPort {
		return 0, failTarget("invalid port %q; use an integer from 1 to 65535", written)
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
		WritePlan:      resolveDefaultWritePlan(EnvironmentDev),
		UndoRows:       DefaultUndoRows,
		CommandTimeout: DefaultCommandTimeout, PageSize: DefaultPageSize,
		Keepalive: DefaultKeepalive,
	}
}

// BuildProfileFromTarget parses a URL, keyword DSN, or database path into an unsaved profile.
func BuildProfileFromTarget(text string) (Profile, error) {
	trimmed := strings.TrimSpace(text)
	if trimmed == "" {
		return Profile{}, failTarget("the connection target is empty")
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

// describeKnownSchemes returns the supported URL schemes.
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
	case core.FamilyMongo:
		return "admin"
	}
	return ""
}

// readTargetSSLMode returns the URL SSL mode or the engine default.
func readTargetSSLMode(parsed *url.URL, engine core.Engine) (core.SSLMode, error) {
	written := ""
	for _, key := range sslKeys {
		if held := parsed.Query().Get(key); held != "" {
			written = held
			break
		}
	}
	if written == "" {
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
			"invalid connection URL; escape special characters in passwords: " +
				"use %%25 for %% and %%2F for /")
	}

	engine, known := urlSchemes[strings.ToLower(parsed.Scheme)]
	if !known {
		return Profile{}, failTarget("unsupported URL scheme %q; use one of %s",
			parsed.Scheme, describeKnownSchemes())
	}

	built := buildTargetProfile(engine)
	// Connection hosts use IPv6 addresses without brackets.
	built.Host = strings.Trim(parsed.Hostname(), "[]")
	if strings.Contains(built.Host, ",") {
		return Profile{}, failTarget(
			"multiple hosts are unsupported; use one host in the URL")
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
	// The database path has one segment.
	if strings.Contains(built.Database, "/") {
		return Profile{}, failTarget(
			"invalid database path %q; use one path segment", built.Database)
	}
	if built.Database == "" {
		built.Database = resolveDefaultDatabase(engine, built.User)
	}
	if built.Database == "" {
		return Profile{}, failTarget("the URL database is missing")
	}

	if built.SSLMode, err = readTargetSSLMode(parsed, engine); err != nil {
		return Profile{}, err
	}
	return built, nil
}

// holdsKeywordPairs is true for `key=value` pairs with a supported first keyword.
func holdsKeywordPairs(text string) bool {
	pairs, err := splitKeywordPairs(text)
	if err != nil || len(pairs) == 0 {
		return false
	}
	_, known := keywordAliases[strings.ToLower(pairs[0][0])]
	return known
}

// splitKeywordPairs parses DSN pairs. Single-quoted values allow spaces and backslash escapes.
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
			return nil, failTarget("missing = after keyword %q", key.String())
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
			return nil, failTarget("missing closing quote in the connection string")
		}
		pairs = append(pairs, [2]string{key.String(), value.String()})
	}
	return pairs, nil
}

// readKeywordEngine returns the connection string engine or the default engine.
func readKeywordEngine(pairs [][2]string) (core.Engine, error) {
	for _, pair := range pairs {
		if !strings.EqualFold(pair[0], "engine") {
			continue
		}
		engine, known := core.FindEngine(pair[1])
		if !known {
			return "", failTarget("unsupported engine %q", pair[1])
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

	// The engine provides the default port and SSL mode.
	engine, err := readKeywordEngine(pairs)
	if err != nil {
		return Profile{}, err
	}

	built := buildTargetProfile(engine)
	for _, pair := range pairs {
		key, known := keywordAliases[strings.ToLower(pair[0])]
		if !known {
			return Profile{}, failTarget(
				"unsupported connection keyword %q", pair[0])
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
		return Profile{}, failTarget("the connection string database is missing")
	}
	return built, nil
}

// holdsDatabaseFile is true for a SQLite header, a database extension, or :memory:.
func holdsDatabaseFile(path string) bool {
	if path == memoryDatabase {
		return true
	}
	if slices.Contains(databaseFileExtensions, strings.ToLower(filepath.Ext(path))) {
		return true
	}
	// Files without a database extension require a SQLite header.
	return holdsSqliteHeader(core.ExpandHomePath(path))
}

// sqliteHeader is the SQLite file signature, including the final zero byte.
var sqliteHeader = []byte("SQLite format 3\x00")

// holdsSqliteHeader is true if the file starts with the SQLite signature.
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
			"unrecognized target %s; use a URL, keyword DSN, or SQLite database path", path)
	}
	built := buildTargetProfile(core.EngineSqlite)
	built.Database = path
	return built, nil
}

// buildTargetProfileName returns the database name or the file name without its extension.
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

// ResolveUniqueProfileName returns an unused profile name.
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
