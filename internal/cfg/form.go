package cfg

import (
	"fmt"
	"net/url"
	"strconv"
	"strings"

	"github.com/turanmahmudov/masume/internal/core"
)

// FormField is one field of the connection form.
type FormField struct {
	Key   string
	Label string
	Value string
	// The values of a field that is a choice and not free text.
	Choices []string
}

// buildBlankProfile returns the profile a new connection starts from.
func buildBlankProfile() Profile {
	return Profile{
		Name: "new-connection", Engine: core.DefaultEngine, Host: "127.0.0.1",
		Port: core.ResolveDefaultPort(core.DefaultEngine), Auth: AuthPassword,
		Environment: EnvironmentDev, AccessMode: AccessWrite, Autocommit: true,
		ConfirmWrites: ConfirmOff, WritePlan: PlanOff, UndoRows: DefaultUndoRows,
		CommandTimeout: DefaultCommandTimeout,
		PageSize:       DefaultPageSize, Keepalive: DefaultKeepalive,
	}
}

// resolveDatabaseLabel returns the label of the field: a database name on a server, or a
// file path.
func resolveDatabaseLabel(engine core.Engine) string {
	if core.OpensFile(engine) {
		return "file"
	}
	return "database"
}

// serverFields are the fields for the address of a server and the user.
var serverFields = map[string]bool{
	"host": true, "port": true, "user": true, "auth": true,
	"passwordEnv": true, "passwordCommand": true, "sslMode": true,
	"secret": true, "secretRef": true,
}

// passwordFields give the field each auth mode reads the password from. The prompt mode and
// the keyring mode read no field: one asks the user, the other asks the operating system.
var passwordFields = map[AuthMode]map[string]bool{
	AuthPassword: {"passwordEnv": true},
	AuthCommand:  {"passwordCommand": true},
	AuthPrompt:   {},
	AuthKeyring:  {},
	AuthSecret:   {"secret": true, "secretRef": true},
}

var everyPasswordField = map[string]bool{
	"passwordEnv": true, "passwordCommand": true, "secret": true, "secretRef": true,
}

// ReadField returns the value of one field.
func ReadField(fields []FormField, key string) string {
	for _, field := range fields {
		if field.Key == key {
			return field.Value
		}
	}
	return ""
}

func listEngineNames() []string {
	names := make([]string, 0, len(core.Engines))
	for _, engine := range core.Engines {
		names = append(names, string(engine))
	}
	return names
}

func listModeNames[T ~string](allowed []T) []string {
	names := make([]string, 0, len(allowed))
	for _, held := range allowed {
		names = append(names, string(held))
	}
	return names
}

// BuildFormFields returns the fields of the form, filled from the profile under edit. The
// store names are the ones the config file declares, which the secret field offers.
func BuildFormFields(profile Profile, editing bool, secretStoreNames []string) []FormField {
	source := profile
	if !editing {
		source = buildBlankProfile()
	}
	return []FormField{
		{Key: "name", Label: "name", Value: source.Name},
		{Key: "engine", Label: "engine", Value: string(source.Engine), Choices: listEngineNames()},
		{Key: "host", Label: "host", Value: source.Host},
		{Key: "port", Label: "port", Value: strconv.Itoa(source.Port)},
		{Key: "database", Label: resolveDatabaseLabel(source.Engine), Value: source.Database},
		{Key: "user", Label: "user", Value: source.User},
		{Key: "auth", Label: "auth", Value: string(source.Auth), Choices: listModeNames(AuthModes)},
		{Key: "passwordEnv", Label: "password env", Value: source.PasswordEnv},
		{Key: "passwordCommand", Label: "password command", Value: source.PasswordCommand},
		{
			Key: "secret", Label: "secret store", Value: source.Secret,
			Choices: secretStoreNames,
		},
		{Key: "secretRef", Label: "secret ref", Value: source.SecretRef},
		{
			Key: "environment", Label: "env", Value: string(source.Environment),
			Choices: listModeNames(Environments),
		},
		{
			Key: "accessMode", Label: "mode", Value: string(source.AccessMode),
			Choices: listModeNames(AccessModes),
		},
		{Key: "sslMode", Label: "sslmode", Value: string(source.SSLMode)},
		{
			Key: "confirmWrites", Label: "confirm", Value: string(source.ConfirmWrites),
			Choices: listModeNames(ConfirmModes),
		},
		{Key: "description", Label: "description", Value: source.Description},
		{Key: "aiInstructions", Label: "ai instructions", Value: source.AiInstructions},
	}
}

// FindShownFields returns the fields the form draws. A file engine has no server, so the
// host and password fields are hidden. An auth mode reads the password from one field, so
// the other password fields are hidden. The text of a hidden field is kept and appears
// again with the mode that reads it.
func FindShownFields(fields []FormField) []FormField {
	engine, known := core.FindEngine(ReadField(fields, "engine"))
	if known && core.OpensFile(engine) {
		kept := make([]FormField, 0, len(fields))
		for _, field := range fields {
			if !serverFields[field.Key] {
				kept = append(kept, field)
			}
		}
		return kept
	}

	auth := AuthPassword
	for _, mode := range AuthModes {
		if string(mode) == ReadField(fields, "auth") {
			auth = mode
		}
	}
	read := passwordFields[auth]

	kept := make([]FormField, 0, len(fields))
	for _, field := range fields {
		if everyPasswordField[field.Key] && !read[field.Key] {
			continue
		}
		kept = append(kept, field)
	}
	return kept
}

// FormError is a form value that cannot be used to open a connection.
type FormError struct{ Reason string }

func (err FormError) Error() string { return err.Reason }

// findChoice returns the value of the field, or the fallback if the field has none.
func findChoice[T ~string](allowed []T, written string, fallback T) T {
	if found, known := core.FindAllowed(allowed, written); known {
		return found
	}
	return fallback
}

// BuildProfileFromFields converts the edited fields back into a profile and rejects an
// invalid value. The settings the form does not show are taken from the profile under edit,
// so a test connection and a save both keep the pre-connect command, the page size and the
// other settings.
func BuildProfileFromFields(fields []FormField, source Profile, editing bool) (Profile, error) {
	built := source
	if !editing {
		built = buildBlankProfile()
	}

	read := func(key string) string { return strings.TrimSpace(ReadField(fields, key)) }
	engine := core.DefaultEngine
	if named, known := core.FindEngine(read("engine")); known {
		engine = named
	}
	opensFile := core.OpensFile(engine)

	built.Name = read("name")
	built.Engine = engine
	built.Host = read("host")
	built.Database = read("database")
	built.User = read("user")
	built.Auth = findChoice(AuthModes, read("auth"), AuthPassword)
	built.Environment = findChoice(Environments, read("environment"), EnvironmentDev)
	built.AccessMode = findChoice(AccessModes, read("accessMode"), AccessWrite)
	built.ConfirmWrites = findChoice(ConfirmModes, read("confirmWrites"), ConfirmOff)
	// The form never writes a password: the file it saves to must hold none. A profile
	// that carries one in memory, from a URL or from a container, keeps it.
	built.PasswordEnv = read("passwordEnv")
	built.PasswordCommand = read("passwordCommand")
	built.Secret = read("secret")
	built.SecretRef = read("secretRef")
	built.Description = read("description")
	built.AiInstructions = read("aiInstructions")

	// A file engine uses no port, so the form does not show one.
	built.Port = core.ResolveDefaultPort(engine)
	if !opensFile {
		port, err := strconv.Atoi(read("port"))
		if err != nil || port <= 0 {
			return Profile{}, FormError{Reason: "port must be a positive integer"}
		}
		built.Port = port
	}

	written := read("sslMode")
	built.SSLMode = core.SSLUnset
	if written != "" {
		mode, known := core.FindSSLMode(written)
		if !known {
			return Profile{}, FormError{
				Reason: "sslmode must be one of " + core.SSLModeNames(),
			}
		}
		built.SSLMode = mode
	}

	if built.Name == "" {
		return Profile{}, FormError{Reason: "name cannot be empty"}
	}
	if built.Database == "" {
		if opensFile {
			return Profile{}, FormError{Reason: "the database file cannot be empty"}
		}
		return Profile{}, FormError{Reason: "database cannot be empty"}
	}
	if !opensFile && built.Host == "" {
		return Profile{}, FormError{Reason: "host cannot be empty"}
	}
	if core.NeedsUser(engine) && built.User == "" {
		return Profile{}, FormError{Reason: "user cannot be empty"}
	}
	if built.Auth == AuthSecret && (built.Secret == "" || built.SecretRef == "") {
		return Profile{}, FormError{
			Reason: "the secret store and the reference cannot be empty when auth is secret",
		}
	}
	if built.Auth == AuthCommand && built.PasswordCommand == "" {
		return Profile{}, FormError{
			Reason: "password command cannot be empty when auth is command",
		}
	}
	return built, nil
}

// ConnectionURL is a parsed connection URL, in the form fields it fills.
type ConnectionURL struct {
	Engine   core.Engine
	Host     string
	Port     int
	Database string
	User     string
	SSLMode  string
}

// urlSchemes give the engine of each scheme, including the alternative scheme names.
var urlSchemes = func() map[string]core.Engine {
	schemes := map[string]core.Engine{}
	for _, info := range core.ListEngineInfo() {
		for _, scheme := range info.URLSchemes {
			schemes[scheme] = info.Engine
		}
	}
	return schemes
}()

// sslKeys are the query keys a URL can use for the SSL setting.
var sslKeys = []string{"sslmode", "ssl-mode", "sslMode"}

// tlsSchemes are the schemes that request TLS by their name, with the mode of each one. A
// Redis client reads `rediss://` as a TLS connection that verifies the certificate, so a
// URL without a mode must not fall back to an unencrypted connection.
var tlsSchemes = map[string]core.SSLMode{"rediss": core.SSLVerifyFull}

// ParseConnectionURL reads a pasted connection string. It returns false unless the scheme,
// the host and the database are all present, because an incomplete URL means the user is
// still typing. A password in the URL is removed.
func ParseConnectionURL(text string) (ConnectionURL, bool) {
	trimmed := strings.TrimSpace(text)
	if !strings.Contains(trimmed, "://") {
		return ConnectionURL{}, false
	}
	parsed, err := url.Parse(trimmed)
	if err != nil {
		return ConnectionURL{}, false
	}

	engine, known := urlSchemes[parsed.Scheme]
	if !known {
		return ConnectionURL{}, false
	}

	// A URL puts an IPv6 address in brackets. Every other reader needs it without them.
	host := strings.Trim(parsed.Hostname(), "[]")
	database := strings.TrimPrefix(parsed.Path, "/")
	if host == "" || database == "" {
		return ConnectionURL{}, false
	}

	port := core.ResolveDefaultPort(engine)
	if written := parsed.Port(); written != "" {
		held, portErr := strconv.Atoi(written)
		if portErr != nil || held <= 0 {
			return ConnectionURL{}, false
		}
		port = held
	}

	sslMode := ""
	for _, key := range sslKeys {
		if written := parsed.Query().Get(key); written != "" {
			sslMode = written
			break
		}
	}
	if sslMode == "" {
		if implied, asks := tlsSchemes[parsed.Scheme]; asks {
			sslMode = string(implied)
		}
	}

	user := ""
	if parsed.User != nil {
		user = parsed.User.Username()
	}
	return ConnectionURL{
		Engine: engine, Host: host, Port: port, Database: database,
		User: user, SSLMode: sslMode,
	}, true
}

// writeField sets one value and keeps every other field.
func writeField(fields []FormField, key, value string) []FormField {
	written := make([]FormField, 0, len(fields))
	for _, field := range fields {
		if field.Key == key {
			field.Value = value
		}
		written = append(written, field)
	}
	return written
}

// ApplyConnectionURL fills the form from a pasted connection string. The profile name
// follows the database name only while the user has not typed a name.
func ApplyConnectionURL(fields []FormField, held ConnectionURL) []FormField {
	named := strings.TrimSpace(ReadField(fields, "name"))
	if named == "" || named == buildBlankProfile().Name {
		named = held.Database
	}

	filled := fields
	for _, written := range [][2]string{
		{"engine", string(held.Engine)}, {"host", held.Host},
		{"port", strconv.Itoa(held.Port)}, {"database", held.Database},
		{"user", held.User}, {"sslMode", held.SSLMode}, {"name", named},
	} {
		filled = writeField(filled, written[0], written[1])
	}
	return filled
}

// ApplyFieldChange sets one value and updates the fields that depend on it. A host never
// contains `://`, so a value with it is a pasted connection string.
func ApplyFieldChange(fields []FormField, key, value string) []FormField {
	if key == "host" {
		if held, parsed := ParseConnectionURL(value); parsed {
			return ApplyConnectionURL(fields, held)
		}
	}
	changed := writeField(fields, key, value)
	if key != "engine" {
		return changed
	}
	return followEngine(fields, changed, value)
}

// followEngine sets the port and the database label of the selected engine. A port the user
// typed is kept.
func followEngine(before, after []FormField, engine string) []FormField {
	wanted, known := core.FindEngine(engine)
	if !known {
		return after
	}
	previous, hadEngine := core.FindEngine(ReadField(before, "engine"))
	followsPort := hadEngine &&
		ReadField(after, "port") == strconv.Itoa(core.ResolveDefaultPort(previous))

	written := make([]FormField, 0, len(after))
	for _, field := range after {
		if field.Key == "database" {
			field.Label = resolveDatabaseLabel(wanted)
		}
		if field.Key == "port" && followsPort {
			field.Value = strconv.Itoa(core.ResolveDefaultPort(wanted))
		}
		written = append(written, field)
	}
	return written
}

// DescribeFormValue returns a value as the form draws it. An empty field shows a
// placeholder.
func DescribeFormValue(field FormField) string {
	if field.Value != "" {
		return field.Value
	}
	if len(field.Choices) > 0 {
		return fmt.Sprintf("one of %s", strings.Join(field.Choices, ", "))
	}
	return ""
}
