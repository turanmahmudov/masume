package mcp

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/turanmahmudov/masume/internal/cfg"
	"github.com/turanmahmudov/masume/internal/core"
	"github.com/turanmahmudov/masume/internal/db"
	"github.com/turanmahmudov/masume/internal/db/engines"
)

// hangingAdapter never answers, like a server behind a firewall that drops the packet.
type hangingAdapter struct {
	entered chan struct{}
	release chan struct{}
}

func (adapter hangingAdapter) Connect(
	_ context.Context, _ cfg.Profile, _ string,
) (db.Session, error) {
	close(adapter.entered)
	<-adapter.release
	return nil, db.NewDatabaseError("the server never answered")
}

// A call that waits on the attempt of another call must stop with its own context. Without
// this one connection that hangs would block every later call to the same profile.
func TestOpenConnectionEndsAWaiterWithTheContextOfItsCall(t *testing.T) {
	adapter := hangingAdapter{entered: make(chan struct{}), release: make(chan struct{})}
	defer close(adapter.release)
	sessions := CreateSessions(engines.Adapters{core.EnginePostgres: adapter})
	profile := cfg.Profile{Name: "shop", Engine: core.EnginePostgres}

	go func() { _, _ = sessions.OpenConnection(context.Background(), profile) }()
	<-adapter.entered

	ctx, stop := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer stop()
	answered := make(chan error, 1)
	go func() {
		_, err := sessions.OpenConnection(ctx, profile)
		answered <- err
	}()

	select {
	case err := <-answered:
		if !errors.Is(err, context.DeadlineExceeded) {
			t.Errorf("the waiter ended with %v, wanted the end of its own context", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("the waiter is still held by an attempt that hangs")
	}
}
