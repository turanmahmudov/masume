// Package detect finds the databases that run in containers on this machine, so the client
// opens one without a profile in the config file and without a URL to type.
//
// The container tool is asked and its answer is read. Nothing here connects to a database.
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

// containerTools are the tools that are asked, in this order.
var containerTools = []string{"docker", "podman"}

// localHost is the address of a container that publishes a port on every interface.
const localHost = "127.0.0.1"

// everyInterface are the addresses a published port carries when it is bound to all of them.
var everyInterface = []string{"", "0.0.0.0", "::", "[::]"}

// imageEngines give the engine of an image, by a part of its name. The list is read in
// order, so an image of a server that is built on another one is matched first:
// `supabase/postgres` and `timescale/timescaledb` both hold `postgres`.
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
	{"valkey", core.EngineRedis},
	{"redis", core.EngineRedis},
	{"mongodb", core.EngineMongo},
	{"mongo", core.EngineMongo},
	{"mysql", core.EngineMysql},
	{"postgres", core.EnginePostgres},
	{"postgis", core.EnginePostgres},
	{"pgvector", core.EnginePostgres},
}

// container is the part of the answer of the container tool that this package reads.
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
		"neither docker nor podman is on the path, so no container can be read")
}

// runTool runs one command of the container tool and returns what it wrote.
func runTool(ctx context.Context, tool string, arguments ...string) ([]byte, error) {
	command := exec.CommandContext(ctx, tool, arguments...)
	command.Stdin = nil
	written, err := command.Output()

	if ctx.Err() != nil {
		return nil, fmt.Errorf("%s %s did not answer within %.0fs",
			tool, arguments[0], scanTimeout.Seconds())
	}
	if reported, is := errors.AsType[*exec.ExitError](err); is {
		said := strings.TrimSpace(string(reported.Stderr))
		if line, _, cut := strings.Cut(said, "\n"); cut {
			said = line
		}
		if said == "" {
			said = fmt.Sprintf("code %d", reported.ExitCode())
		}
		return nil, fmt.Errorf("%s %s failed: %s", tool, arguments[0], said)
	}
	if err != nil {
		return nil, fmt.Errorf("%s %s failed: %w", tool, arguments[0], err)
	}
	return written, nil
}

// readInspectedContainers returns the containers that run on this machine, as the tool
// describes them.
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

// findImageEngine returns the engine of an image. The tag and the registry are cut first,
// so `ghcr.io/org/postgres:18-alpine` is read as `org/postgres`.
func findImageEngine(image string) (core.Engine, bool) {
	name := strings.ToLower(image)
	if at := strings.LastIndex(name, "@"); at != -1 {
		name = name[:at]
	}
	if at := strings.LastIndex(name, ":"); at > strings.LastIndex(name, "/") {
		name = name[:at]
	}
	// Matching anywhere in the name would take `mongo-express`, a web page for a MongoDB
	// server, and `mysql-workbench`, a client.
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
	"supabase":    core.EngineSupabase,
	"timescale":   core.EngineTimescale,
	"cockroachdb": core.EngineCockroach,
}

// imageVariants are the marks a server puts after its own name, so `postgres-alpine` and
// `timescaledb-ha` are the server and `mongo-express` is not.
var imageVariants = []string{"-alpine", "-ha", "-server", "-community-server", "-ee", "-ce"}

// holdsImageVariant is true where the repository is the server under a variant name.
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
	// A port published on both stacks is listed twice, so the IPv4 binding is taken first.
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

// applyRedisEnvironment fills a Redis image. The server has no user, and a password belongs
// to the server itself.
func applyRedisEnvironment(profile *cfg.Profile, environment map[string]string) {
	profile.Password = findFirstValue(environment, "REDIS_PASSWORD")
	profile.Database = "0"
}

// resolveContainerSSLMode returns the SSL mode of a container, which listens without TLS
// where the hosted service of the same engine does not.
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

// buildContainerProfile returns the connection to the database of the container, and false
// for a container that holds no database this client opens or publishes no port for it.
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
		Autocommit: true, ConfirmWrites: cfg.ConfirmOff,
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
	case core.FamilyRedis:
		applyRedisEnvironment(&profile, environment)
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
		return nil, fmt.Errorf("the container tool wrote no JSON this client reads: %w", err)
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
