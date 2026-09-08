// Package detect finds local database containers through Docker or Podman without opening database connections.
package detect

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/turanmahmudov/masume/internal/cfg"
	"github.com/turanmahmudov/masume/internal/core"
)

// scanTimeout is the time the container tool has to answer.
const scanTimeout = 10 * time.Second

// containerTools are the container tools in search order.
var containerTools = []string{"docker", "podman"}

// localHost is the address of a container that publishes a port on every interface.
const localHost = "127.0.0.1"

// everyInterface is the set of wildcard addresses for published ports.
var everyInterface = []string{"", "0.0.0.0", "::", "[::]"}

// imageEngines is the ordered set of image names and database engines.
var imageEngines = []struct {
	part   string
	engine core.Engine
}{
	{"timescaledb", core.EngineTimescale},
	{"timescale", core.EngineTimescale},
	{"cockroach", core.EngineCockroach},
	{"supabase", core.EngineSupabase},
	{"mariadb", core.EngineMariadb},
	{"tidb", core.EngineTidb},
	{"percona", core.EngineMysql},
	{"mongodb", core.EngineMongo},
	{"azure-sql-edge", core.EngineSqlserver},
	{"mongo", core.EngineMongo},
	{"mysql", core.EngineMysql},
	{"postgres", core.EnginePostgres},
	{"postgis", core.EnginePostgres},
	{"pgvector", core.EnginePostgres},
}

// container is the supported subset of a container inspection response.
type container struct {
	Name   string
	Config struct {
		Image  string
		Env    []string
		Labels map[string]string
	}
	NetworkSettings struct {
		Ports map[string][]struct {
			HostIP   string `json:"HostIp"`
			HostPort string
		}
	}
}

// findContainerTool returns the tool that reads the containers of this machine.
func findContainerTool() (string, error) {
	for _, name := range containerTools {
		if _, err := exec.LookPath(name); err == nil {
			return name, nil
		}
	}
	return "", errors.New(
		"container detection requires docker or podman on PATH")
}

// runTool runs a container command and returns its output.
func runTool(ctx context.Context, tool string, arguments ...string) ([]byte, error) {
	command := exec.CommandContext(ctx, tool, arguments...)
	command.Stdin = nil
	written, err := command.Output()

	if ctx.Err() != nil {
		return nil, fmt.Errorf("%s %s exceeded the %.0fs time limit",
			tool, arguments[0], scanTimeout.Seconds())
	}
	if reported, is := errors.AsType[*exec.ExitError](err); is {
		said := strings.TrimSpace(string(reported.Stderr))
		if line, _, cut := strings.Cut(said, "\n"); cut {
			said = line
		}
		if said == "" {
			said = fmt.Sprintf("exit code %d", reported.ExitCode())
		}
		return nil, fmt.Errorf("%s %s failed: %s", tool, arguments[0], said)
	}
	if err != nil {
		return nil, fmt.Errorf("%s %s failed: %w", tool, arguments[0], err)
	}
	return written, nil
}

// readInspectedContainers returns inspection output for running containers.
func readInspectedContainers(ctx context.Context, tool string) ([]byte, error) {
	listed, err := runTool(ctx, tool, "ps", "--quiet", "--no-trunc")
	if err != nil {
		return nil, err
	}

	identifiers := strings.Fields(string(listed))
	if len(identifiers) == 0 {
		return nil, nil
	}
	return runTool(ctx, tool, append([]string{"inspect"}, identifiers...)...)
}

// findImageEngine matches the image organization and repository without the registry, tag, or digest.
func findImageEngine(image string) (core.Engine, bool) {
	name := strings.ToLower(image)
	if at := strings.LastIndex(name, "@"); at != -1 {
		name = name[:at]
	}
	if at := strings.LastIndex(name, ":"); at > strings.LastIndex(name, "/") {
		name = name[:at]
	}
	// Client images such as mongo-express and mysql-workbench are excluded.
	repository, organisation := name, ""
	if at := strings.LastIndex(repository, "/"); at != -1 {
		organisation, repository = repository[:at], repository[at+1:]
		if at := strings.LastIndex(organisation, "/"); at != -1 {
			organisation = organisation[at+1:]
		}
	}
	// The image of a Supabase server is `supabase/postgres` and is not plain PostgreSQL.
	if engine, known := imageOrganisations[organisation]; known {
		return engine, true
	}
	for _, held := range imageEngines {
		if repository == held.part || holdsImageVariant(repository, held.part) {
			return held.engine, true
		}
	}
	return "", false
}

// imageOrganisations give the engine of every image an organisation ships.
var imageOrganisations = map[string]core.Engine{
	// The image of a SQL Server is `mcr.microsoft.com/mssql/server`.
	"mssql":       core.EngineSqlserver,
	"supabase":    core.EngineSupabase,
	"timescale":   core.EngineTimescale,
	"cockroachdb": core.EngineCockroach,
}

// imageVariants are the supported database image suffixes.
var imageVariants = []string{"-alpine", "-ha", "-server", "-community-server", "-ee", "-ce"}

// holdsImageVariant is true for a supported database image variant.
func holdsImageVariant(repository, part string) bool {
	rest, cut := strings.CutPrefix(repository, part)
	if !cut || rest == "" {
		return false
	}
	if rest[0] >= '0' && rest[0] <= '9' {
		return true
	}
	return slices.Contains(imageVariants, rest)
}

// findPublishedPort returns the address on this machine that reaches the port of the
// container.
func findPublishedPort(held container, containerPort int) (string, int, bool) {
	published, mapped := held.NetworkSettings.Ports[strconv.Itoa(containerPort)+"/tcp"]
	if !mapped {
		return "", 0, false
	}
	// A loopback or wildcard binding takes priority over other addresses.
	fallbackHost, fallbackPort, hasFallback := "", 0, false
	for _, binding := range published {
		port, err := strconv.Atoi(binding.HostPort)
		if err != nil || port <= 0 {
			continue
		}
		host := binding.HostIP
		for _, every := range everyInterface {
			if host == every {
				host = localHost
			}
		}
		if host == localHost {
			return host, port, true
		}
		if !hasFallback {
			fallbackHost, fallbackPort, hasFallback = host, port, true
		}
	}
	return fallbackHost, fallbackPort, hasFallback
}

// readEnvironment returns the environment of the container as a table.
func readEnvironment(held container) map[string]string {
	environment := map[string]string{}
	for _, written := range held.Config.Env {
		if name, value, cut := strings.Cut(written, "="); cut {
			environment[name] = value
		}
	}
	return environment
}

// findFirstValue returns the value of the first name the environment sets.
func findFirstValue(environment map[string]string, names ...string) string {
	for _, name := range names {
		if value := environment[name]; value != "" {
			return value
		}
	}
	return ""
}

// applyPostgresEnvironment fills the user, the database and the password of a PostgreSQL
// image. The entrypoint of the image uses `postgres` for a variable that is not set.
func applyPostgresEnvironment(profile *cfg.Profile, environment map[string]string) {
	profile.User = findFirstValue(environment, "POSTGRES_USER", "PGUSER")
	if profile.User == "" {
		profile.User = "postgres"
	}
	profile.Database = findFirstValue(environment, "POSTGRES_DB", "PGDATABASE")
	if profile.Database == "" {
		profile.Database = profile.User
	}
	profile.Password = findFirstValue(environment, "POSTGRES_PASSWORD", "PGPASSWORD")
}

// applyCockroachEnvironment fills a CockroachDB image, which reads none of the PostgreSQL
// variables and starts with one user and one database of its own.
func applyCockroachEnvironment(profile *cfg.Profile, environment map[string]string) {
	profile.User = findFirstValue(environment, "COCKROACH_USER")
	if profile.User == "" {
		profile.User = "root"
	}
	profile.Database = findFirstValue(environment, "COCKROACH_DATABASE")
	if profile.Database == "" {
		profile.Database = "defaultdb"
	}
	profile.Password = findFirstValue(environment, "COCKROACH_PASSWORD")
}

// applyMysqlEnvironment fills a MySQL or MariaDB image. A named user has a password of its
// own; without one the connection is the root user.
func applyMysqlEnvironment(profile *cfg.Profile, environment map[string]string) {
	profile.User = findFirstValue(environment, "MYSQL_USER", "MARIADB_USER")
	if profile.User != "" {
		profile.Password = findFirstValue(environment, "MYSQL_PASSWORD", "MARIADB_PASSWORD")
	} else {
		profile.User = "root"
		profile.Password = findFirstValue(
			environment, "MYSQL_ROOT_PASSWORD", "MARIADB_ROOT_PASSWORD")
	}
	profile.Database = findFirstValue(environment, "MYSQL_DATABASE", "MARIADB_DATABASE")
	if profile.Database == "" {
		profile.Database = "mysql"
	}
}

// applyMongoEnvironment fills a MongoDB image. A server that was started without a user has
// authentication off and refuses a connection that sends one.
func applyMongoEnvironment(profile *cfg.Profile, environment map[string]string) {
	profile.User = findFirstValue(environment, "MONGO_INITDB_ROOT_USERNAME")
	profile.Password = findFirstValue(environment, "MONGO_INITDB_ROOT_PASSWORD")
	profile.Database = findFirstValue(environment, "MONGO_INITDB_DATABASE")
	if profile.Database == "" {
		profile.Database = "admin"
	}
}

// applySqlserverEnvironment fills a SQL Server image. The image starts with one
// administrator, and the password of that user is the only one the container is given.
func applySqlserverEnvironment(profile *cfg.Profile, environment map[string]string) {
	profile.User = "sa"
	profile.Password = findFirstValue(environment, "MSSQL_SA_PASSWORD", "SA_PASSWORD")
	profile.Database = "master"
}

// resolveContainerSSLMode permits non-TLS connections for local containers.
func resolveContainerSSLMode(engine core.Engine) core.SSLMode {
	mode := core.ResolveEngineInfo(engine).DefaultSSLMode
	switch core.ResolveSSLPolicy(mode) {
	case core.PolicyEncryptOnly, core.PolicyVerifyCa, core.PolicyVerifyFull:
		return core.SSLPrefer
	}
	return mode
}

// describeContainer returns the description of the profile: the image, and the compose
// project of the container when it has one.
func describeContainer(held container, name string) string {
	said := fmt.Sprintf("%s in container %s", held.Config.Image, name)
	if project := held.Config.Labels["com.docker.compose.project"]; project != "" {
		said += ", compose project " + project
	}
	return said
}

// buildContainerProfile returns a profile for a supported database container with a published port.
func buildContainerProfile(held container) (cfg.Profile, bool) {
	engine, known := findImageEngine(held.Config.Image)
	if !known {
		return cfg.Profile{}, false
	}
	host, port, published := findPublishedPort(held, core.ResolveDefaultPort(engine))
	if !published {
		return cfg.Profile{}, false
	}

	name := strings.TrimPrefix(held.Name, "/")
	if strings.TrimSpace(name) == "" {
		return cfg.Profile{}, false
	}
	profile := cfg.Profile{
		Name: name, Engine: engine, Host: host, Port: port,
		Auth: cfg.AuthPassword, Environment: cfg.EnvironmentDev,
		AccessMode: cfg.AccessWrite, SSLMode: resolveContainerSSLMode(engine),
		Autocommit: true, ConfirmWrites: cfg.ConfirmOff, WritePlan: cfg.PlanOff,
		UndoRows:       cfg.DefaultUndoRows,
		CommandTimeout: cfg.DefaultCommandTimeout, PageSize: cfg.DefaultPageSize,
		Keepalive: cfg.DefaultKeepalive, Description: describeContainer(held, name),
	}

	environment := readEnvironment(held)
	switch core.ResolveEngineInfo(engine).Family {
	case core.FamilyPostgres:
		if engine == core.EngineCockroach {
			applyCockroachEnvironment(&profile, environment)
		} else {
			applyPostgresEnvironment(&profile, environment)
		}
	case core.FamilyMysql:
		applyMysqlEnvironment(&profile, environment)
	case core.FamilyMongo:
		applyMongoEnvironment(&profile, environment)
	case core.FamilySqlserver:
		applySqlserverEnvironment(&profile, environment)
	}
	return profile, true
}

// BuildProfilesFromInspection returns one connection per database in the answer of
// `docker inspect`.
func BuildProfilesFromInspection(written []byte) ([]cfg.Profile, error) {
	found := []cfg.Profile{}
	if len(written) == 0 {
		return found, nil
	}

	containers := []container{}
	if err := json.Unmarshal(written, &containers); err != nil {
		return nil, fmt.Errorf("invalid container inspection JSON: %w", err)
	}

	for _, held := range containers {
		profile, holds := buildContainerProfile(held)
		if !holds {
			continue
		}
		profile.Name = cfg.ResolveUniqueProfileName(found, profile.Name)
		found = append(found, profile)
	}
	return found, nil
}

// BuildContainerProfiles returns one connection per database that runs in a container on
// this machine.
func BuildContainerProfiles() ([]cfg.Profile, error) {
	tool, err := findContainerTool()
	if err != nil {
		return nil, err
	}

	ctx, stop := context.WithTimeout(context.Background(), scanTimeout)
	defer stop()

	written, err := readInspectedContainers(ctx, tool)
	if err != nil {
		return nil, err
	}
	return BuildProfilesFromInspection(written)
}
