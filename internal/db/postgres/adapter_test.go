package postgres

import (
	"context"
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
