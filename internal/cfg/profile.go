package cfg

import (
	"fmt"
	"os"
	"slices"
	"strings"
	"time"

	"github.com/turanmahmudov/masume/internal/core"
	"github.com/turanmahmudov/masume/internal/secret"
)

// AuthMode is the password source: memory, a command, a store, the system keyring, or a prompt.
type AuthMode string

// The sources of a password.
const (
	AuthPassword AuthMode = "password"
	AuthCommand  AuthMode = "command"
	AuthPrompt   AuthMode = "prompt"
	// AuthKeyring is the system keyring password source, with a prompt for a missing password.
	AuthKeyring AuthMode = "keyring"
	// AuthSecret reads one reference out of a store the user declared under `[secret]`.
	AuthSecret AuthMode = "secret"
)

// AuthModes lists the modes a profile can use.
var AuthModes = []AuthMode{
	AuthPassword, AuthCommand, AuthPrompt, AuthKeyring, AuthSecret,
}

// Environment is the environment of a connection.
type Environment string

// The three environments a profile can use.
const (
	EnvironmentDev  Environment = "dev"
	EnvironmentTest Environment = "test"
	EnvironmentProd Environment = "prod"
)

// Environments lists the environments a profile can use.
var Environments = []Environment{EnvironmentDev, EnvironmentTest, EnvironmentProd}

// AccessMode is the connection write permission.
type AccessMode string

// The two access modes a profile can use.
const (
	AccessReadOnly AccessMode = "read-only"
	AccessWrite    AccessMode = "write"
)

// AccessModes lists the modes a profile can use.
var AccessModes = []AccessMode{AccessReadOnly, AccessWrite}

// ConfirmWrites is the confirmation level for statements.
type ConfirmWrites string

// The four levels of confirmation.
const (
	ConfirmOff    ConfirmWrites = "off"
	ConfirmDelete ConfirmWrites = "delete"
	ConfirmWrite  ConfirmWrites = "write"
	// ConfirmAgent requires confirmation for every write and allows an agent to submit the user response.
	ConfirmAgent ConfirmWrites = "agent"
)

// ConfirmModes lists the levels a profile can use.
var ConfirmModes = []ConfirmWrites{ConfirmOff, ConfirmDelete, ConfirmWrite, ConfirmAgent}

// WritePlan is the write preview level before confirmation.
type WritePlan string

// The three levels of measurement.
const (
	PlanOff WritePlan = "off"
	// PlanCount is the preview of affected rows and relations.
	PlanCount WritePlan = "count"
	// PlanUndo includes the original rows for undo.
	PlanUndo WritePlan = "undo"
)

// WritePlanModes lists the levels a profile can use.
var WritePlanModes = []WritePlan{PlanOff, PlanCount, PlanUndo}

// McpAccess is the set of operations an agent can run on a connection.
type McpAccess string

// The levels an agent can get, the lowest first.
const (
	McpOff       McpAccess = "off"
	McpReadOnly  McpAccess = "read-only"
	McpReadWrite McpAccess = "read-write"
	McpFull      McpAccess = "full"
	// McpUnset means the profile sets no level, so the `[mcp]` level applies.
	McpUnset McpAccess = ""
)

// McpAccessLevels lists the levels, the lowest first.
var McpAccessLevels = []McpAccess{McpOff, McpReadOnly, McpReadWrite, McpFull}

// The defaults a profile uses when it sets no value.
const (
	// DefaultCommandTimeout is the time a pre-connect command has to open its port.
	DefaultCommandTimeout = 10 * time.Second
	// DefaultPageSize is the number of rows one read returns.
	DefaultPageSize = 200
	// DefaultKeepalive is the time between two checks that the server responds.
	DefaultKeepalive = 30 * time.Second
	// DefaultUndoRows is the maximum row count for undo. Larger writes run without undo.
	DefaultUndoRows = 1000
)

// passwordInFileReason is the warning for an ignored password in a config file.
const passwordInFileReason = "passwords in files are ignored; enter the password and select " +
	"\"remember in the keyring\", or set password_env, password_command or a [secret] store"

// Profile is one connection as the config file defines it.
type Profile struct {
	Name   string
	Engine core.Engine
	// Empty for an engine that opens a file and not a server.
	Host string
	Port int
	// The database on the server, or the path of the SQLite file.
	Database    string
	User        string
	Auth        AuthMode
	Environment Environment
	AccessMode  AccessMode
	// The password from a command-line target or container. Config files never read or write this value.
	Password    string
	PasswordEnv string
	// A command that prints the password, for example a lookup in a secret store.
	PasswordCommand string
	// The `[secret]` store that holds the password, and the reference inside it.
	Secret    string
	SecretRef string
	// The command built from the secret store and reference. This value is absent from the config file.
	SecretCommand string
	SSLMode       core.SSLMode
	Autocommit    bool
	ConfirmWrites ConfirmWrites
	WritePlan     WritePlan
	// The maximum row count for undo.
	UndoRows int
	// A command to run before the connection, for example a tunnel.
	Command string
	// The port the command must open before the client connects.
	WaitForPort    int
	CommandTimeout time.Duration
	// Rows per read. This is the step size through a table, not a maximum.
	PageSize int
	// The time between two checks that the server responds. Zero disables the check.
	Keepalive time.Duration
	// The statement time limit. Zero uses the server limit.
	StatementTimeout time.Duration
	Description      string
	// Instructions for the chat on this connection, for example a naming rule.
	AiInstructions string
	// The operations an agent can run here over MCP. Unset keeps the `[mcp]` level.
	McpAccess McpAccess
	// True for a profile from the user config file.
	InConfigFile bool
	// The source project file path, or empty for other profiles.
	ProjectFile string
}

// IsInAFile is true for a profile from a user config or project file.
func (profile Profile) IsInAFile() bool {
	return profile.InConfigFile || profile.ProjectFile != ""
}

// ProfileProblem is a profile or secret store error.
type ProfileProblem struct {
	Name   string
	Reason string
}

// secretProblemPrefix marks a problem of a `[secret]` store rather than of a profile.
const secretProblemPrefix = "secret."

// FileProblemPrefix is the prefix for errors that prevent loading the whole config file.
const FileProblemPrefix = "file."

// Describe returns one error line with the file, store, or profile name.
func (problem ProfileProblem) Describe() string {
	// File errors start with the reason, followed by the path.
	if name, isFile := strings.CutPrefix(problem.Name, FileProblemPrefix); isFile {
		return fmt.Sprintf("%s · %s", problem.Reason, name)
	}
	if name, isStore := strings.CutPrefix(problem.Name, secretProblemPrefix); isStore {
		return fmt.Sprintf("skipped secret store %q: %s", name, problem.Reason)
	}
	return fmt.Sprintf("skipped profile %q: %s", problem.Name, problem.Reason)
}

// DescribeWarning returns a warning as one line. The profile it names was read and is used.
func (problem ProfileProblem) DescribeWarning() string {
	return fmt.Sprintf("profile %q: %s", problem.Name, problem.Reason)
}

// ParsedProfiles is the set of loaded profiles, stores, errors, and warnings.
type ParsedProfiles struct {
	Profiles []Profile
	Problems []ProfileProblem
	// Warnings are ignored keys in loaded profiles. Problems are skipped profiles or stores.
	Warnings []ProfileProblem
	// The secret stores the user declared under `[secret]`.
	Secrets []SecretSource
}

// resolveDefaultConfirmWrites returns the environment confirmation level.
func resolveDefaultConfirmWrites(environment Environment) ConfirmWrites {
	switch environment {
	case EnvironmentProd:
		return ConfirmWrite
	case EnvironmentTest:
		return ConfirmDelete
	}
	return ConfirmOff
}

// resolveDefaultWritePlan uses the environment, as the confirmation does.
func resolveDefaultWritePlan(environment Environment) WritePlan {
	switch environment {
	case EnvironmentProd:
		return PlanUndo
	case EnvironmentTest:
		return PlanCount
	}
	return PlanOff
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

// readCount reads an integer of zero or more. Zero disables the feature.
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

// readSSLMode returns the mode of the profile, or the default of its engine.
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
	if _, namesStore := FindString(source, "secret"); namesStore {
		defaultAuth = AuthSecret
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
	writePlan, err := resolveOneOf(
		source, "write_plan", WritePlanModes, resolveDefaultWritePlan(environment))
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

	database := ""
	if core.NeedsDatabase(engine) {
		if database, err = readRequiredString(source, "database"); err != nil {
			return Profile{}, err
		}
	} else {
		database, _ = FindString(source, "database")
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
	undoRows, hasUndoRows, err := readCount(source, "undo_rows")
	if err != nil {
		return Profile{}, err
	}
	if !hasUndoRows {
		undoRows = DefaultUndoRows
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

	passwordEnv, _ := FindString(source, "password_env")
	secretName, _ := FindString(source, "secret")
	secretRef, _ := FindString(source, "secret_ref")
	command, _ := FindString(source, "command")
	description, _ := FindString(source, "description")
	aiInstructions, _ := FindString(source, "ai_instructions")

	return Profile{
		Name: name, Engine: engine, Host: host, Port: port, Database: database, User: user,
		Auth: auth, Environment: environment, AccessMode: accessMode,
		PasswordEnv: passwordEnv, PasswordCommand: passwordCommand,
		Secret: secretName, SecretRef: secretRef,
		SSLMode: sslMode, Autocommit: autocommit, ConfirmWrites: confirmWrites,
		WritePlan: writePlan, UndoRows: undoRows,
		Command: command, WaitForPort: waitForPort, CommandTimeout: commandTimeout,
		PageSize: pageSize, Keepalive: keepalive, Description: description,
		StatementTimeout: time.Duration(timeoutMilliseconds) * time.Millisecond,
		AiInstructions:   aiInstructions, McpAccess: mcpAccess, InConfigFile: true,
	}, nil
}

// ParseProfiles reads `[profile]` and `[secret]`, resolves store commands, and reports skipped entries.
func ParseProfiles(document Table) ParsedProfiles {
	sources, sourceProblems := ParseSecretSources(document)
	written, present := FindSection(document, "profile")
	if !present {
		return ParsedProfiles{Secrets: sources, Problems: nameSecretProblems(sourceProblems)}
	}

	parsed := ParsedProfiles{Secrets: sources, Problems: nameSecretProblems(sourceProblems)}
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
		if _, holdsPassword := FindString(source, "password"); holdsPassword {
			parsed.Warnings = append(parsed.Warnings, ProfileProblem{
				Name: name, Reason: passwordInFileReason})
		}
		if profile.Auth == AuthSecret {
			command, storeErr := resolveSecretCommand(profile, sources)
			if storeErr != nil {
				parsed.Problems = append(parsed.Problems,
					ProfileProblem{Name: name, Reason: storeErr.Error()})
				continue
			}
			profile.SecretCommand = command
		}
		parsed.Profiles = append(parsed.Profiles, profile)
	}
	return parsed
}

// nameSecretProblems adds the secret store prefix to each error.
func nameSecretProblems(problems []ProfileProblem) []ProfileProblem {
	named := make([]ProfileProblem, 0, len(problems))
	for _, problem := range problems {
		named = append(named, ProfileProblem{
			Name: secretProblemPrefix + problem.Name, Reason: problem.Reason})
	}
	return named
}

// DescribeProfileTarget returns the database file path or server address.
func DescribeProfileTarget(profile Profile) string {
	if core.OpensFile(profile.Engine) {
		return profile.Database
	}
	if profile.Database == "" {
		return fmt.Sprintf("%s@%s:%d", profile.User, profile.Host, profile.Port)
	}
	return fmt.Sprintf("%s@%s:%d/%s", profile.User, profile.Host, profile.Port, profile.Database)
}

// FindConfiguredValue returns the direct value, or the value of the named environment
// variable. The direct value has priority.
func FindConfiguredValue(written, variableName string) string {
	if written != "" {
		return written
	}
	if variableName == "" {
		return ""
	}
	return os.Getenv(variableName)
}

// FindStoredPassword returns the password the profile already holds. It does not ask the
// user.
func FindStoredPassword(profile Profile) string {
	if profile.Auth != AuthPassword {
		return ""
	}
	return FindConfiguredValue(profile.Password, profile.PasswordEnv)
}

// NeedsPasswordPrompt is true if the user is the only source of the password.
func NeedsPasswordPrompt(profile Profile) bool {
	// A file has no password, whatever the profile sets.
	if core.OpensFile(profile.Engine) {
		return false
	}
	// Prompt mode always requests a password for a server connection.
	if profile.Auth == AuthPrompt {
		return true
	}
	// A server that connects without a password is not asked.
	if !core.ResolveEngineInfo(profile.Engine).NeedsPassword {
		return false
	}
	// MongoDB authentication requires a username. Profiles without a user omit authentication.
	if profile.User == "" {
		return false
	}
	// A command and a store both answer without the user.
	if profile.Auth == AuthCommand || profile.Auth == AuthSecret {
		return false
	}
	// Missing or inaccessible keyring passwords require a prompt.
	if profile.Auth == AuthKeyring {
		password, found, err := secret.FindPassword(profile.Name)
		return err != nil || !found || password == ""
	}
	return FindStoredPassword(profile) == ""
}
