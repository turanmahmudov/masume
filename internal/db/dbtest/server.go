// Package dbtest opens the servers an integration test reads. It knows nothing of how a
// server was started: it takes a connection URL out of the environment and opens it the way
// the client opens any other connection.
//
// One variable per engine, each a URL:
//
//	MASUME_TEST_POSTGRES=postgres://postgres:secret@127.0.0.1:55432/shop
//	MASUME_TEST_MYSQL=mysql://root:secret@127.0.0.1:55306/shop
//	MASUME_TEST_MONGO=mongodb://127.0.0.1:55017/shop
//	MASUME_TEST_MONGO_AUTH=mongodb://root:secret@127.0.0.1:55018/shop
//	MASUME_TEST_MONGO_RS=mongodb://127.0.0.1:55020/shop
//
// A variable that is not set skips the tests that need it, so `go test` on a machine with no
// servers still passes. Where a skip would hide a server that should have been there, set
// MASUME_TEST_REQUIRE=1 and a missing variable fails instead. A build server sets it, so a
// run never goes green because every test was skipped.
package dbtest

import (
	"context"
	"net/url"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/turanmahmudov/masume/internal/cfg"
	"github.com/turanmahmudov/masume/internal/core"
	"github.com/turanmahmudov/masume/internal/db"
	"github.com/turanmahmudov/masume/internal/db/engines"
)

// requireVariable names the variable that turns a skip into a failure.
const requireVariable = "MASUME_TEST_REQUIRE"

// openTimeout is how long one attempt at reaching a server may take.
const openTimeout = 30 * time.Second

// ReadEverything is a row limit above every row these tests write. The port has no value
// for "no limit": a read asks for one row more than the limit, to tell a full page from the
// last one.
const ReadEverything = 100

// Target is one server an integration test can read.
type Target struct {
	// Variable is the name of the environment variable that carries the URL.
	Variable string
	Engine   core.Engine
	// DefaultPort is used where the URL names none.
	DefaultPort int
}

// The servers the integration tests read. Every other engine speaks one of these
// protocols and shares its adapter, so its own differences are unit-tested instead.
// MongoDB is named twice, because a server with authentication turned on returns a
// connection differently from one without it.
var (
	Postgres = Target{Variable: "MASUME_TEST_POSTGRES", Engine: core.EnginePostgres, DefaultPort: 5432}
	MySQL    = Target{Variable: "MASUME_TEST_MYSQL", Engine: core.EngineMysql, DefaultPort: 3306}
	Mongo    = Target{Variable: "MASUME_TEST_MONGO", Engine: core.EngineMongo, DefaultPort: 27017}
	// MongoAuth is the same engine on a server that authenticates every command.
	MongoAuth = Target{
		Variable: "MASUME_TEST_MONGO_AUTH", Engine: core.EngineMongo, DefaultPort: 27017,
	}
	// MongoReplicaSet is the same engine on a deployment that holds a transaction.
	MongoReplicaSet = Target{
		Variable: "MASUME_TEST_MONGO_RS", Engine: core.EngineMongo, DefaultPort: 27017,
	}
)

// Open returns a session on the server this target names. It skips the test where the
// variable is not set, or fails where a skip is not allowed.
func Open(t *testing.T, target Target) db.Session {
	t.Helper()

	written := os.Getenv(target.Variable)
	if written == "" {
		if os.Getenv(requireVariable) != "" {
			t.Fatalf("%s is not set and %s says a server is required; "+
				"start the servers in compose.yaml first", target.Variable, requireVariable)
		}
		t.Skipf("%s is not set, so there is no %s to read", target.Variable, target.Engine)
	}

	profile, password, err := buildProfile(target, written)
	if err != nil {
		t.Fatalf("%s does not read as a URL: %v", target.Variable, err)
	}

	ctx, stop := context.WithTimeout(context.Background(), openTimeout)
	defer stop()
	session, err := engines.CreateAdapters().Open(ctx, profile, password)
	if err != nil {
		t.Fatalf("cannot reach the %s named by %s: %v", target.Engine, target.Variable, err)
	}
	t.Cleanup(func() { _ = session.Close() })
	return session
}

// BuildProfile reads the URL of a target into a profile and the password beside it, for
// a test that opens a connection itself rather than through Open.
func BuildProfile(t *testing.T, target Target) (cfg.Profile, string) {
	t.Helper()
	written := os.Getenv(target.Variable)
	if written == "" {
		t.Skipf("%s is not set, so there is no %s to read", target.Variable, target.Engine)
	}
	profile, password, err := buildProfile(target, written)
	if err != nil {
		t.Fatalf("%s does not read as a URL: %v", target.Variable, err)
	}
	return profile, password
}

// buildProfile reads a URL into the profile the adapter opens. The password is answered
// beside it, because a profile carries what the config file holds and not a secret from a
// URL.
func buildProfile(target Target, written string) (cfg.Profile, string, error) {
	held, err := url.Parse(written)
	if err != nil {
		return cfg.Profile{}, "", err
	}

	port := target.DefaultPort
	if named := held.Port(); named != "" {
		if read, convertErr := strconv.Atoi(named); convertErr == nil {
			port = read
		}
	}
	password, _ := held.User.Password()

	return cfg.Profile{
		Name:        "integration",
		Engine:      target.Engine,
		Host:        held.Hostname(),
		Port:        port,
		Database:    strings.TrimPrefix(held.Path, "/"),
		User:        held.User.Username(),
		AccessMode:  cfg.AccessWrite,
		PageSize:    ReadEverything,
		SSLMode:     core.SSLDisable,
		Autocommit:  true,
		Environment: cfg.EnvironmentTest,
	}, password, nil
}

// RunStatements runs each statement in turn and stops at the first one the server refuses.
// It is how a test lays out the schema it reads.
func RunStatements(t *testing.T, session db.Session, statements ...string) {
	t.Helper()
	for _, statement := range statements {
		if strings.TrimSpace(statement) == "" {
			continue
		}
		if _, err := session.RunQuery(
			context.Background(), statement, ReadEverything, nil); err != nil {
			t.Fatalf("cannot run %q: %v", firstLine(statement), err)
		}
	}
}

// firstLine returns the opening line of a statement, for a message about a whole batch.
func firstLine(statement string) string {
	trimmed := strings.TrimSpace(statement)
	if before, _, ok := strings.Cut(trimmed, "\n"); ok {
		return before + " …"
	}
	return trimmed
}
