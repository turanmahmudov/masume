package detect_test

import (
	"fmt"
	"testing"

	"github.com/turanmahmudov/masume/internal/cfg"
	"github.com/turanmahmudov/masume/internal/core"
	"github.com/turanmahmudov/masume/internal/detect"
)

// inspectedContainers is the answer of `docker inspect`, cut down to the fields this client
// reads. It holds the five servers of compose.yaml, a container that runs no database, and
// a database container that publishes no port.
const inspectedContainers = `[
  {
    "Name": "/masume-test-postgres-1",
    "Config": {
      "Image": "postgres:18-alpine",
      "Env": ["POSTGRES_DB=shop", "POSTGRES_PASSWORD=secret", "PG_VERSION=18.6"],
      "Labels": {"com.docker.compose.project": "masume-test"}
    },
    "NetworkSettings": {
      "Ports": {"5432/tcp": [{"HostIp": "127.0.0.1", "HostPort": "55432"}]}
    }
  },
  {
    "Name": "/masume-test-mysql-1",
    "Config": {
      "Image": "mysql:8.4",
      "Env": ["MYSQL_DATABASE=shop", "MYSQL_ROOT_PASSWORD=secret", "MYSQL_VERSION=8.4.11-1.el9"],
      "Labels": {}
    },
    "NetworkSettings": {
      "Ports": {
        "3306/tcp": [{"HostIp": "127.0.0.1", "HostPort": "55306"}],
        "33060/tcp": null
      }
    }
  },
  {
    "Name": "/masume-test-mongo-auth-1",
    "Config": {
      "Image": "mongo:8",
      "Env": [
        "MONGO_INITDB_ROOT_USERNAME=root",
        "MONGO_INITDB_ROOT_PASSWORD=secret",
        "MONGO_INITDB_DATABASE=shop",
        "MONGO_VERSION=8.2.12"
      ],
      "Labels": {}
    },
    "NetworkSettings": {
      "Ports": {"27017/tcp": [{"HostIp": "127.0.0.1", "HostPort": "55018"}]}
    }
  },
  {
    "Name": "/buildkit",
    "Config": {"Image": "moby/buildkit:buildx-stable-1", "Env": [], "Labels": {}},
    "NetworkSettings": {"Ports": {}}
  },
  {
    "Name": "/hidden-postgres",
    "Config": {"Image": "postgres:17", "Env": [], "Labels": {}},
    "NetworkSettings": {"Ports": {"5432/tcp": null}}
  }
]`

// findProfileNamed returns the connection of that name.
func findProfileNamed(profiles []cfg.Profile, name string) (cfg.Profile, bool) {
	for _, profile := range profiles {
		if profile.Name == name {
			return profile, true
		}
	}
	return cfg.Profile{}, false
}

// A scan must return one connection per database container, and nothing for a container
// that runs no database.
func TestBuildProfilesFromInspectionFindsEveryDatabaseContainer(t *testing.T) {
	found, err := detect.BuildProfilesFromInspection([]byte(inspectedContainers))
	if err != nil {
		t.Fatalf("the answer does not read: %v", err)
	}

	names := []string{}
	for _, profile := range found {
		names = append(names, profile.Name)
	}
	if len(found) != 3 {
		t.Fatalf("the scan found %v, wanted the three database containers", names)
	}
	if _, holds := findProfileNamed(found, "buildkit"); holds {
		t.Error("a container that runs no database was offered as a connection")
	}
	if _, holds := findProfileNamed(found, "hidden-postgres"); holds {
		t.Error("a database container that publishes no port was offered as a connection")
	}
}

// The connection of a PostgreSQL container must be complete, so it opens without anything
// typed. The user, the database and the password are in the environment of the container.
func TestBuildProfilesFromInspectionReadsAPostgresContainer(t *testing.T) {
	found, err := detect.BuildProfilesFromInspection([]byte(inspectedContainers))
	if err != nil {
		t.Fatalf("the answer does not read: %v", err)
	}
	profile, holds := findProfileNamed(found, "masume-test-postgres-1")
	if !holds {
		t.Fatal("the PostgreSQL container was not found")
	}

	for _, one := range []struct {
		part string
		got  any
		want any
	}{
		{"engine", profile.Engine, core.EnginePostgres},
		{"host", profile.Host, "127.0.0.1"},
		{"port", profile.Port, 55432},
		{"database", profile.Database, "shop"},
		{"user", profile.User, "postgres"},
		{"password", profile.Password, "secret"},
		{"environment", profile.Environment, cfg.EnvironmentDev},
		{"access mode", profile.AccessMode, cfg.AccessWrite},
		{"page size", profile.PageSize, cfg.DefaultPageSize},
	} {
		if one.got != one.want {
			t.Errorf("the %s reads %v, wanted %v", one.part, one.got, one.want)
		}
	}
	if cfg.NeedsPasswordPrompt(profile) {
		t.Error("a container that holds its password is asked for one")
	}
	if profile.Description == "" {
		t.Error("the connection says nothing about the container it came from")
	}
}

// Each family keeps its own names for the user, the database and the password, so each one
// must be read with the names of its own image.
func TestBuildProfilesFromInspectionReadsTheEnvironmentOfEachFamily(t *testing.T) {
	found, err := detect.BuildProfilesFromInspection([]byte(inspectedContainers))
	if err != nil {
		t.Fatalf("the answer does not read: %v", err)
	}

	for _, one := range []struct {
		name     string
		engine   core.Engine
		user     string
		database string
		password string
		port     int
	}{
		{"masume-test-mysql-1", core.EngineMysql, "root", "shop", "secret", 55306},
		{"masume-test-mongo-auth-1", core.EngineMongo, "root", "shop", "secret", 55018},
	} {
		profile, holds := findProfileNamed(found, one.name)
		if !holds {
			t.Errorf("%s was not found", one.name)
			continue
		}
		if profile.Engine != one.engine {
			t.Errorf("%s opens %s, wanted %s", one.name, profile.Engine, one.engine)
		}
		if profile.User != one.user {
			t.Errorf("%s connects as %q, wanted %q", one.name, profile.User, one.user)
		}
		if profile.Database != one.database {
			t.Errorf("%s opens %q, wanted %q", one.name, profile.Database, one.database)
		}
		if profile.Password != one.password {
			t.Errorf("%s holds the password %q, wanted %q",
				one.name, profile.Password, one.password)
		}
		if profile.Port != one.port {
			t.Errorf("%s uses port %d, wanted %d", one.name, profile.Port, one.port)
		}
	}
}

// buildInspectedImage returns the answer of `docker inspect` for one container of that
// image, which publishes the port the engine of the image listens on.
func buildInspectedImage(image string, containerPort int) []byte {
	return []byte(fmt.Sprintf(`[{
	  "Name": "/one",
	  "Config": {"Image": %q, "Env": [], "Labels": {}},
	  "NetworkSettings": {
	    "Ports": {"%d/tcp": [{"HostIp": "127.0.0.1", "HostPort": "15000"}]}
	  }
	}]`, image, containerPort))
}

// The image names the server. A server that is built on another one must be read as itself
// and not as the one under it, because the two do not hold the same catalog.
func TestBuildProfilesFromInspectionReadsTheEngineOfTheImage(t *testing.T) {
	for _, one := range []struct {
		image  string
		engine core.Engine
	}{
		{"postgres:18-alpine", core.EnginePostgres},
		{"docker.io/library/postgres", core.EnginePostgres},
		{"pgvector/pgvector:pg17", core.EnginePostgres},
		{"postgis/postgis:17-3.5", core.EnginePostgres},
		{"timescale/timescaledb:latest-pg17", core.EngineTimescale},
		{"supabase/postgres:15.8.1", core.EngineSupabase},
		{"cockroachdb/cockroach:v25.1.0", core.EngineCockroach},
		{"mysql:8.4", core.EngineMysql},
		{"percona/percona-server:8.0", core.EngineMysql},
		{"mariadb:11", core.EngineMariadb},
		{"pingcap/tidb:v8.5.0", core.EngineTidb},
		{"mongo:8", core.EngineMongo},
		{"ghcr.io/org/postgres@sha256:abc", core.EnginePostgres},
		{"postgres16", core.EnginePostgres},
		{"mongodb/mongodb-community-server:8.0", core.EngineMongo},
	} {
		written := buildInspectedImage(one.image, core.ResolveDefaultPort(one.engine))
		found, err := detect.BuildProfilesFromInspection(written)
		if err != nil {
			t.Errorf("%s does not read: %v", one.image, err)
			continue
		}
		if len(found) != 1 {
			t.Errorf("%s found %d connections, wanted one", one.image, len(found))
			continue
		}
		if found[0].Engine != one.engine {
			t.Errorf("%s opens %s, wanted %s", one.image, found[0].Engine, one.engine)
		}
		if found[0].Port != 15000 {
			t.Errorf("%s uses port %d, wanted the published one", one.image, found[0].Port)
		}
	}
}

// A hosted service accepts a TLS connection only, and its engine is set for that. The
// container of the same server listens without TLS, so the mode must not require it.
func TestBuildProfilesFromInspectionDoesNotRequireTLSOnAContainer(t *testing.T) {
	written := buildInspectedImage("supabase/postgres:15.8.1", 5432)
	found, err := detect.BuildProfilesFromInspection(written)
	if err != nil || len(found) != 1 {
		t.Fatalf("the container does not read: %v", err)
	}
	if core.ResolveEngineInfo(core.EngineSupabase).DefaultSSLMode != core.SSLRequire {
		t.Fatal("the engine of the test no longer requires TLS, so the test says nothing")
	}
	if found[0].SSLMode != core.SSLPrefer {
		t.Errorf("the ssl mode reads %q, wanted prefer", found[0].SSLMode)
	}
}

// A tool that watches a server is not the server. An image whose name holds the name of a
// server inside a longer word must not be offered as one.
func TestBuildProfilesFromInspectionOffersNoToolThatIsNotAServer(t *testing.T) {
	for _, image := range []string{
		"mongo-express:1.0",
		"mysql-workbench:8.0",
		"prom/mysqld-exporter:v0.15",
		"dpage/pgadmin4:8",
	} {
		written := buildInspectedImage(image, 5432)
		found, err := detect.BuildProfilesFromInspection(written)
		if err != nil {
			t.Errorf("%s does not read: %v", image, err)
			continue
		}
		if len(found) != 0 {
			t.Errorf("%s was offered as a server: %v", image, found[0].Engine)
		}
	}
}

// A port published on both stacks is listed twice. The binding of this machine is taken, so
// a client that reaches only one of the two still connects.
func TestBuildProfilesFromInspectionTakesTheBindingOfThisMachine(t *testing.T) {
	written := []byte(`[{
	  "Name": "/db",
	  "Config": {"Image": "postgres:18", "Env": [], "Labels": {}},
	  "NetworkSettings": {
	    "Ports": {"5432/tcp": [
	      {"HostIp": "::", "HostPort": "55432"},
	      {"HostIp": "127.0.0.1", "HostPort": "55432"}
	    ]}
	  }
	}]`)
	found, err := detect.BuildProfilesFromInspection(written)
	if err != nil || len(found) != 1 {
		t.Fatalf("the container does not read: %v", err)
	}
	if found[0].Host != "127.0.0.1" {
		t.Errorf("the host reads %q, wanted the address of this machine", found[0].Host)
	}
}

// A container with no name has nothing to list it under, so it is not offered.
func TestBuildProfilesFromInspectionOffersNoContainerWithoutAName(t *testing.T) {
	written := []byte(`[{
	  "Name": "/",
	  "Config": {"Image": "postgres:18", "Env": [], "Labels": {}},
	  "NetworkSettings": {
	    "Ports": {"5432/tcp": [{"HostIp": "127.0.0.1", "HostPort": "55432"}]}
	  }
	}]`)
	found, err := detect.BuildProfilesFromInspection(written)
	if err != nil {
		t.Fatalf("the container does not read: %v", err)
	}
	if len(found) != 0 {
		t.Errorf("a container with no name was offered as %q", found[0].Name)
	}
}

// A scan of a machine with no container must return nothing and no error, because a machine
// without containers is not a fault.
func TestBuildProfilesFromInspectionReadsAnEmptyAnswer(t *testing.T) {
	for _, written := range []string{"", "[]"} {
		found, err := detect.BuildProfilesFromInspection([]byte(written))
		if err != nil {
			t.Errorf("%q does not read: %v", written, err)
		}
		if len(found) != 0 {
			t.Errorf("%q found %d connections, wanted none", written, len(found))
		}
	}
}

// An answer this client cannot read must be reported and not read as a machine with no
// container, so a tool that changed its output is noticed.
func TestBuildProfilesFromInspectionReportsAnAnswerItCannotRead(t *testing.T) {
	if _, err := detect.BuildProfilesFromInspection([]byte("not json")); err == nil {
		t.Fatal("an answer that is not JSON was read as a machine with no container")
	}
}
