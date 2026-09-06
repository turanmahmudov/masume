package mysql

import (
	"context"
	"testing"

	"github.com/turanmahmudov/masume/internal/core"
	"github.com/turanmahmudov/masume/internal/db"
)

// Cancellation requires a known connection ID.
func TestCancelRunningQueryRefusesAnUnknownThread(t *testing.T) {
	session := &mysqlSession{SessionFacts: db.SessionFacts{
		Support: db.EngineSupport{EngineInfo: core.EngineInfo{
			Capabilities: core.Capabilities{CancelsRunningQuery: true},
		}},
	}}

	stopped, err := session.CancelRunningQuery(context.Background())
	if stopped {
		t.Error("a cancel with no thread id reported that it stopped one")
	}
	if err == nil {
		t.Fatal("a cancel with no thread id answered no error")
	}
	if described := db.DescribeError(err); described !=
		"cannot cancel the statement: the connection ID is unavailable" {
		t.Errorf("the cancel describes as %q", described)
	}
}
