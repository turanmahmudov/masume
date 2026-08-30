package cfg

import (
	"fmt"
	"os"
	"slices"
	"strings"
	"time"

	"github.com/turanmahmudov/masume/internal/core"
)

// AuthMode says where the password comes from: the profile itself, a command, or
// the user.
type AuthMode string

// The three places a password can come from.
const (
	AuthPassword AuthMode = "password"
	AuthCommand  AuthMode = "command"
	AuthPrompt   AuthMode = "prompt"
)

// AuthModes lists the modes a profile may name.
var AuthModes = []AuthMode{AuthPassword, AuthCommand, AuthPrompt}

// Environment is the environment a connection is in.
type Environment string

// The three environments a profile may name.
const (
	EnvironmentDev  Environment = "dev"
	EnvironmentTest Environment = "test"
	EnvironmentProd Environment = "prod"
)

// Environments lists the environments a profile may name.
var Environments = []Environment{EnvironmentDev, EnvironmentTest, EnvironmentProd}

// AccessMode says whether the client may write in the server.
type AccessMode string

// The two access modes a profile may name.
const (
	AccessReadOnly AccessMode = "read-only"
	AccessWrite    AccessMode = "write"
)

// AccessModes lists the modes a profile may name.
var AccessModes = []AccessMode{AccessReadOnly, AccessWrite}

// ConfirmWrites says which statements need a yes before they run.
type ConfirmWrites string

// The three levels of confirmation.
const (
	ConfirmOff    ConfirmWrites = "off"
	ConfirmDelete ConfirmWrites = "delete"
	ConfirmWrite  ConfirmWrites = "write"
)

// ConfirmModes lists the levels a profile may name.
var ConfirmModes = []ConfirmWrites{ConfirmOff, ConfirmDelete, ConfirmWrite}

// McpAccess says what an agent may run on a connection.
type McpAccess string

// The levels an agent can be given, the lowest first.
const (
	McpOff       McpAccess = "off"
	McpReadOnly  McpAccess = "read-only"
	McpReadWrite McpAccess = "read-write"
	McpFull      McpAccess = "full"
	// McpUnset means the profile named no level, so the `[mcp]` level stands.
	McpUnset McpAccess = ""
)

// McpAccessLevels lists the levels, the lowest first.
var McpAccessLevels = []McpAccess{McpOff, McpReadOnly, McpReadWrite, McpFull}

// The defaults a profile takes where it names nothing.
const (
	// DefaultCommandTimeout is how long a pre-connect command may take to open its port.
	DefaultCommandTimeout = 10 * time.Second
	// DefaultPageSize is how many rows one read returns.
	DefaultPageSize = 200
	// DefaultKeepalive is how long between two checks that the server responds.
	DefaultKeepalive = 30 * time.Second
)

// Profile is one connection as the config file describes it.
type Profile struct {
	Name   string
	Engine core.Engine
	// Empty for an engine that opens a file instead of a server.
	Host string
	Port int
	// The database on the server, or the path of the SQLite file.
	Database    string
	User        string
	Auth        AuthMode
	Environment Environment
	AccessMode  AccessMode
	Password    string
	PasswordEnv string
	// A command that prints the password, such as a keyring lookup.
	PasswordCommand string
	SSLMode         core.SSLMode
	Autocommit      bool
	ConfirmWrites   ConfirmWrites
	// A command to run before connecting, such as a tunnel.
	Command string
	// The port the command must open before the connection is tried.
	WaitForPort    int
	CommandTimeout time.Duration
	// Rows per read. This is the step size through a relation, not a limit.
	PageSize int
	// How long between two checks that the server responds. Zero turns it off.
	Keepalive time.Duration
	// How long one statement may run before it is cancelled. Zero leaves the limit to
	// the server.
	StatementTimeout time.Duration
	Description      string
	// Instructions for the chat on this connection, such as a naming rule.
	AiInstructions string
	// What an agent may run here over MCP. Unset keeps the `[mcp]` level.
	McpAccess McpAccess
}

// ProfileProblem is a profile the file could not read, and the reason.
type ProfileProblem struct {
	Name   string
	Reason string
}

// ParsedProfiles holds the profiles read from the file and the ones that failed.
type ParsedProfiles struct {
	Profiles []Profile
	Problems []ProfileProblem
}

// resolveDefaultConfirmWrites reads the environment, which is the only measure of
// the cost of a mistake: production confirms every write and development none.
func resolveDefaultConfirmWrites(environment Environment) ConfirmWrites {
	switch environment {
	case EnvironmentProd:
		return ConfirmWrite
	case EnvironmentTest:
		return ConfirmDelete
	}
	return ConfirmOff
}

type profileError struct{ reason string }

func (err profileError) Error() string { return err.reason }

func failProfile(format string, parts ...any) error {
	return profileError{reason: fmt.Sprintf(format, parts...)}
}

func readRequiredString(source Table, key string) (string, error) {
	written, present := FindString(source, key)
	if !present {
		return "", failProfile("%q must be a non-empty string", key)
	}
	return written, nil
}

// readCount reads a whole number of zero or more, where zero turns a feature off.
func readCount(source Table, key string) (int, bool, error) {
	if _, present := source[key]; !present {
		return 0, false, nil
	}
	value, isWhole := FindInteger(source, key)
	if !isWhole || value < 0 {
		return 0, false, failProfile("%q must be a whole number of zero or more", key)
	}
	return value, true, nil
}

func readPositiveInteger(source Table, key string) (int, bool, error) {
	if _, present := source[key]; !present {
		return 0, false, nil
	}
	value, isPositive := FindPositiveInteger(source, key)
	if !isPositive {
		return 0, false, failProfile("%q must be a positive integer", key)
	}
	return value, true, nil
}

func resolveOneOf[T ~string](source Table, key string, allowed []T, fallback T) (T, error) {
	value, present := source[key]
	if !present {
		return fallback, nil
	}
	if written, isText := value.(string); isText {
		if found, known := core.FindAllowed(allowed, written); known {
			return found, nil
		}
	}
	names := make([]string, 0, len(allowed))
	for _, candidate := range allowed {
		names = append(names, string(candidate))
	}
	return fallback, failProfile("%q must be one of %s", key, strings.Join(names, ", "))
}

// readSSLMode returns the mode the profile named, or the default of its engine.
func readSSLMode(source Table, engine core.Engine) (core.SSLMode, error) {
	written, present := FindString(source, "sslmode")
	if !present {
		return core.ResolveEngineInfo(engine).DefaultSSLMode, nil
	}
	mode, known := core.FindSSLMode(written)
	if !known {
		return core.SSLUnset, failProfile("sslmode %q is not one of %s", written, core.SSLModeNames())
	}
	return mode, nil
}

func findProfileMcpAccess(source Table) (McpAccess, error) {
	return resolveOneOf(source, "mcp", McpAccessLevels, McpUnset)
}

func buildProfile(name string, source Table) (Profile, error) {
	engine, err := resolveOneOf(source, "engine", core.Engines, core.DefaultEngine)
	if err != nil {
		return Profile{}, err
	}
	environment, err := resolveOneOf(source, "env", Environments, EnvironmentDev)
	if err != nil {
		return Profile{}, err
	}

	passwordCommand, _ := FindString(source, "password_command")
	defaultAuth := AuthPassword
	if passwordCommand != "" {
		defaultAuth = AuthCommand
	}
	auth, err := resolveOneOf(source, "auth", AuthModes, defaultAuth)
	if err != nil {
		return Profile{}, err
	}
	if auth == AuthCommand && passwordCommand == "" {
		return Profile{}, failProfile("%q must be set when %q is command", "password_command", "auth")
	}

	accessMode, err := resolveOneOf(source, "mode", AccessModes, AccessWrite)
	if err != nil {
		return Profile{}, err
	}
	confirmWrites, err := resolveOneOf(
		source, "confirm_writes", ConfirmModes, resolveDefaultConfirmWrites(environment))
	if err != nil {
		return Profile{}, err
	}
	sslMode, err := readSSLMode(source, engine)
	if err != nil {
		return Profile{}, err
	}
	mcpAccess, err := findProfileMcpAccess(source)
	if err != nil {
		return Profile{}, err
	}

	database, err := readRequiredString(source, "database")
	if err != nil {
		return Profile{}, err
	}

	opensFile := core.OpensFile(engine)
	host := ""
	if opensFile {
		host, _ = FindString(source, "host")
	} else if host, err = readRequiredString(source, "host"); err != nil {
		return Profile{}, err
	}

	user := ""
	if core.NeedsUser(engine) {
		if user, err = readRequiredString(source, "user"); err != nil {
			return Profile{}, err
		}
	} else {
		user, _ = FindString(source, "user")
	}

	port, hasPort, err := readPositiveInteger(source, "port")
	if err != nil {
		return Profile{}, err
	}
	if !hasPort {
		port = core.ResolveDefaultPort(engine)
	}

	waitForPort, _, err := readPositiveInteger(source, "wait_for_port")
	if err != nil {
		return Profile{}, err
	}
	commandTimeoutSeconds, hasTimeout, err := readPositiveInteger(source, "command_timeout")
	if err != nil {
		return Profile{}, err
	}
	pageSize, hasPageSize, err := readPositiveInteger(source, "page_size")
	if err != nil {
		return Profile{}, err
	}
	keepaliveSeconds, hasKeepalive, err := readCount(source, "keepalive_s")
	if err != nil {
		return Profile{}, err
	}
	timeoutMilliseconds, _, err := readCount(source, "statement_timeout_ms")
	if err != nil {
		return Profile{}, err
	}

	commandTimeout := DefaultCommandTimeout
	if hasTimeout {
		commandTimeout = time.Duration(commandTimeoutSeconds) * time.Second
	}
	keepalive := DefaultKeepalive
	if hasKeepalive {
		keepalive = time.Duration(keepaliveSeconds) * time.Second
	}
	if !hasPageSize {
		pageSize = DefaultPageSize
	}

	autocommit := true
	if written, isFlag := FindBool(source, "autocommit"); isFlag {
		autocommit = written
	}

	password, _ := FindString(source, "password")
	passwordEnv, _ := FindString(source, "password_env")
	command, _ := FindString(source, "command")
	description, _ := FindString(source, "description")
	aiInstructions, _ := FindString(source, "ai_instructions")

	return Profile{
		Name: name, Engine: engine, Host: host, Port: port, Database: database, User: user,
		Auth: auth, Environment: environment, AccessMode: accessMode,
		Password: password, PasswordEnv: passwordEnv, PasswordCommand: passwordCommand,
		SSLMode: sslMode, Autocommit: autocommit, ConfirmWrites: confirmWrites,
		Command: command, WaitForPort: waitForPort, CommandTimeout: commandTimeout,
		PageSize: pageSize, Keepalive: keepalive, Description: description,
		StatementTimeout: time.Duration(timeoutMilliseconds) * time.Millisecond,
		AiInstructions:   aiInstructions, McpAccess: mcpAccess,
	}, nil
}

// ParseProfiles reads the `[profile]` section. A profile the client cannot read is
// reported and left out, so one wrong entry never stops the app.
func ParseProfiles(document Table) ParsedProfiles {
	written, present := FindSection(document, "profile")
	if !present {
		return ParsedProfiles{}
	}

	parsed := ParsedProfiles{}
	names := make([]string, 0, len(written))
	for name := range written {
		names = append(names, name)
	}
	slices.Sort(names)

	for _, name := range names {
		source, isTable := FindTable(written[name])
		if !isTable {
			parsed.Problems = append(parsed.Problems,
				ProfileProblem{Name: name, Reason: "entry is not a table"})
			continue
		}
		profile, err := buildProfile(name, source)
		if err != nil {
			parsed.Problems = append(parsed.Problems,
				ProfileProblem{Name: name, Reason: err.Error()})
			continue
		}
		parsed.Profiles = append(parsed.Profiles, profile)
	}
	return parsed
}

// DescribeProfileTarget writes the target of the profile in short: a file path, or
// a server.
func DescribeProfileTarget(profile Profile) string {
	if core.OpensFile(profile.Engine) {
		return profile.Database
	}
	return fmt.Sprintf("%s@%s:%d/%s", profile.User, profile.Host, profile.Port, profile.Database)
}

// FindConfiguredValue returns a value written directly, or the environment
// variable that holds it. The direct value wins.
func FindConfiguredValue(written, variableName string) string {
	if written != "" {
		return written
	}
	if variableName == "" {
		return ""
	}
	return os.Getenv(variableName)
}

// FindStoredPassword returns the password the profile already holds, without
// asking the user.
func FindStoredPassword(profile Profile) string {
	if profile.Auth != AuthPassword {
		return ""
	}
	return FindConfiguredValue(profile.Password, profile.PasswordEnv)
}

// NeedsPasswordPrompt is true where only the user can give the password.
func NeedsPasswordPrompt(profile Profile) bool {
	// A file has no password, whatever the profile says.
	if core.OpensFile(profile.Engine) {
		return false
	}
	// A profile that asks for a prompt gets one, because the server can need a
	// password the client cannot find another way.
	if profile.Auth == AuthPrompt {
		return true
	}
	// A server that connects without a password is not asked. Redis wants one only
	// if it was configured to, and then the profile holds it.
	info := core.ResolveEngineInfo(profile.Engine)
	if !info.NeedsPassword {
		return false
	}
	// A server that checks a password against a named user has nothing to check one
	// against where the profile names none. MongoDB is where this shows: its user is
	// optional, because a server with authentication turned off refuses a connection
	// that carries one.
	if !info.PasswordWithoutUser && profile.User == "" {
		return false
	}
	if profile.Auth == AuthCommand {
		return false
	}
	return FindStoredPassword(profile) == ""
}
