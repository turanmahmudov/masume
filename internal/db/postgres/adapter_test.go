package postgres

import (
	"context"
	"strings"
	"testing"

	"github.com/turanmahmudov/masume/internal/core"
	"github.com/turanmahmudov/masume/internal/db"
)

// A server that never named this connection leaves the backend id at zero. Cancelling id
// zero would reach whichever connection the server gave that id, so it is refused.
func TestCancelRunningQueryRefusesAnUnknownBackend(t *testing.T) {
	session := &postgresSession{SessionFacts: db.SessionFacts{
		Support: db.EngineSupport{EngineInfo: core.EngineInfo{
			Capabilities: core.Capabilities{CancelsRunningQuery: true},
		}},
	}}

	stopped, err := session.CancelRunningQuery(context.Background())
	if stopped {
		t.Error("a cancel with no backend id reported that it stopped one")
	}
	if err == nil {
		t.Fatal("a cancel with no backend id answered no error")
	}
	if described := db.DescribeError(err); described !=
		"the server did not name this connection, so its statement cannot be cancelled" {
		t.Errorf("the cancel describes as %q", described)
	}
}

// Redshift documents the catalogs of PostgreSQL 8.0, which hold no pg_extension, so the
// statement that opens a connection asks that server for less.
func TestBuildIdentityStatementAsksOnlyTheServersThatHoldTheCatalog(t *testing.T) {
	for _, one := range []struct {
		name    string
		flavour Flavour
		asks    bool
	}{
		{"standard", FlavourStandard, true},
		{"cockroach", FlavourCockroach, true},
		{"redshift", FlavourRedshift, false},
	} {
		written := buildIdentityStatement(one.flavour)
		if strings.Contains(written, "pg_extension") != one.asks {
			t.Errorf("%s reads %q, wanted the catalog asked: %v", one.name, written, one.asks)
		}
	}
}
