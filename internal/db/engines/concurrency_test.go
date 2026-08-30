package engines

import (
	"context"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/turanmahmudov/masume/internal/cfg"
	"github.com/turanmahmudov/masume/internal/core"
	"github.com/turanmahmudov/masume/internal/db"
)

// The screens read on their own goroutines, so several calls can reach one session at once.
// A driver that speaks a single socket answers `conn busy` or drops the connection where
// that is not held back, so this asks every profile of a config file the same questions at
// the same time and reports any call that did not answer. Run it with `-race` as well: the
// goroutine that draws reads the state of the transaction while a statement records it.

// readSleepStatement answers a statement that keeps the connection for a moment, so the
// calls really do overlap.
func readSleepStatement(engine core.Engine) string {
	switch core.ResolveEngineInfo(engine).Family {
	case core.FamilyPostgres:
		return "select pg_sleep(0.3), 1 as n"
	case core.FamilyMysql:
		return "select sleep(0.3) as slept, 1 as n"
	case core.FamilySqlite:
		return "select 1 as n"
	}
	return ""
}

// TestSessionsAnswerConcurrentCalls opens every profile of the config the environment names
// and asks each session several questions at once.
func TestSessionsAnswerConcurrentCalls(t *testing.T) {
	configPath := os.Getenv("MASUME_PROBE_CONFIG")
	if configPath == "" {
		t.Skip("MASUME_PROBE_CONFIG names the config file whose profiles are asked")
	}
	loaded := cfg.LoadConfig(configPath)
	adapters := CreateAdapters()
	opened := 0

	for _, profile := range loaded.Profiles {
		sleeping := readSleepStatement(profile.Engine)
		if sleeping == "" {
			continue
		}
		password, _ := cfg.ResolveProfilePassword(profile)

		ctx, stop := context.WithTimeout(context.Background(), 10*time.Second)
		session, err := adapters.Open(ctx, profile, password)
		stop()
		if err != nil {
			t.Logf("%s did not open: %v", profile.Name, err)
			continue
		}
		opened++
		t.Run(profile.Name, func(t *testing.T) {
			defer func() { _ = session.Close() }()
			askAtOnce(t, session, sleeping)
			askInsideTransaction(t, session, sleeping)
		})
	}

	if opened == 0 {
		t.Skip("no profile of the config file opened")
	}
}

// askAtOnce sends every call of one session at the same time, reads the state of the
// transaction while they run, and reports each call that failed.
func askAtOnce(t *testing.T, session db.Session, sleeping string) {
	t.Helper()
	group := sync.WaitGroup{}
	problems := make(chan string, 32)

	ask := func(name string, work func(ctx context.Context) error) {
		group.Go(func() {
			ctx, stop := context.WithTimeout(context.Background(), 60*time.Second)
			defer stop()
			if err := work(ctx); err != nil {
				problems <- name + ": " + err.Error()
			}
		})
	}

	stopReading := readStateWhileTheCallsRun(session)

	for range 4 {
		ask("run", func(ctx context.Context) error {
			_, err := session.RunQuery(ctx, sleeping, 100, nil)
			return err
		})
	}
	ask("ping", session.Ping)
	ask("tables", func(ctx context.Context) error {
		_, err := session.ListTables(ctx)
		return err
	})
	ask("objects", func(ctx context.Context) error {
		_, err := session.ListSchemaObjects(ctx)
		return err
	})
	ask("check", func(ctx context.Context) error {
		session.CheckStatement(ctx, sleeping)
		return nil
	})

	group.Wait()
	stopReading()
	close(problems)
	for problem := range problems {
		t.Errorf("a call that ran beside the others failed: %s", problem)
	}
}

// askInsideTransaction runs a statement the server refuses inside an open transaction, so
// the state is recorded while it is read. Everything runs on the one connection there.
func askInsideTransaction(t *testing.T, session db.Session, sleeping string) {
	t.Helper()
	ctx, stop := context.WithTimeout(context.Background(), 60*time.Second)
	defer stop()

	if err := session.BeginTransaction(ctx); err != nil {
		t.Logf("this server opened no transaction: %v", err)
		return
	}
	stopReading := readStateWhileTheCallsRun(session)

	group := sync.WaitGroup{}
	for range 4 {
		group.Go(func() {
			_, _ = session.RunQuery(ctx, "select nosuchcolumn", 10, nil)
			_, _ = session.RunQuery(ctx, sleeping, 10, nil)
			_ = session.Ping(ctx)
		})
	}
	group.Wait()
	stopReading()

	if err := session.RollbackTransaction(ctx); err != nil {
		t.Errorf("the transaction did not roll back: %v", err)
	}
	if state := session.ReadTransactionState(); state != db.TransactionNone {
		t.Errorf("the transaction stayed %q after the rollback", state)
	}
}

// readStateWhileTheCallsRun keeps reading the state of the transaction, the way the frame
// does, and answers what stops it.
func readStateWhileTheCallsRun(session db.Session) func() {
	stopped := make(chan struct{})
	done := make(chan struct{})
	go func() {
		defer close(done)
		for {
			select {
			case <-stopped:
				return
			default:
			}
			_ = session.ReadTransactionState()
		}
	}()
	return func() {
		close(stopped)
		<-done
	}
}
