package mcp

import (
	"context"
	"sync"
	"time"

	"github.com/turanmahmudov/masume/internal/cfg"
	"github.com/turanmahmudov/masume/internal/core"
	"github.com/turanmahmudov/masume/internal/db"
	"github.com/turanmahmudov/masume/internal/db/engines"
)

// MCP reuses one connection per profile and cannot prompt for passwords.

// FindUnreachableReason reports profiles that require an interactive password prompt.
func FindUnreachableReason(profile cfg.Profile) string {
	if !cfg.NeedsPasswordPrompt(profile) {
		return ""
	}
	return "this profile requires a password prompt, which MCP does not support. " +
		"Set password_env, password_command, or a [secret] store, or " +
		"save the password in the keyring through the interactive client"
}

// Connection is one connection of this server, with its table list.
type Connection struct {
	Session db.Session

	guard      sync.Mutex
	preConnect *cfg.PreConnectHandle
	tables     []db.TableRef
	readAt     time.Time
}

// Tables returns the tables of the connection.
func (connection *Connection) Tables() []db.TableRef {
	connection.guard.Lock()
	defer connection.guard.Unlock()
	return connection.tables
}

// RefreshTables refreshes the table catalog after its TTL expires.
func (connection *Connection) RefreshTables(ctx context.Context) error {
	connection.guard.Lock()
	stale := time.Since(connection.readAt) >= core.CatalogTTL
	connection.guard.Unlock()
	if !stale {
		return nil
	}

	tables, err := connection.Session.ListTables(ctx)
	if err != nil {
		return err
	}
	connection.guard.Lock()
	connection.tables, connection.readAt = tables, time.Now()
	connection.guard.Unlock()
	return nil
}

// stop closes the connection and the processes started for it.
func (connection *Connection) stop() {
	_ = connection.Session.Close()
	connection.preConnect.Stop()
}

// Sessions opens one connection per profile and holds it.
type Sessions struct {
	adapters engines.Adapters

	guard sync.Mutex
	// Concurrent calls share a connection attempt.
	opening map[string]*attempt
}

// openTimeout is the connection attempt timeout.
const openTimeout = 30 * time.Second

// attempt is one attempt to open a connection. Several calls can wait on it.
type attempt struct {
	done       chan struct{}
	connection *Connection
	err        error
}

// CreateSessions returns the connection store of this server.
func CreateSessions(adapters engines.Adapters) *Sessions {
	return &Sessions{adapters: adapters, opening: map[string]*attempt{}}
}

// OpenConnection returns the connection of this profile and opens it on the first call.
func (sessions *Sessions) OpenConnection(
	ctx context.Context, profile cfg.Profile,
) (*Connection, error) {
	sessions.guard.Lock()
	started, running := sessions.opening[profile.Name]
	if !running {
		started = &attempt{done: make(chan struct{})}
		sessions.opening[profile.Name] = started
	}
	sessions.guard.Unlock()

	if !running {
		go sessions.runOpenAttempt(started, profile)
	}
	select {
	case <-started.done:
		return started.connection, started.err
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// runOpenAttempt uses an independent context shared by all waiting calls.
func (sessions *Sessions) runOpenAttempt(started *attempt, profile cfg.Profile) {
	ctx, cancel := context.WithTimeout(context.Background(), openTimeout)
	defer cancel()

	started.connection, started.err = openProfileConnection(ctx, profile, sessions.adapters)
	if started.err != nil {
		// Remove failed attempts before the next call.
		sessions.guard.Lock()
		delete(sessions.opening, profile.Name)
		sessions.guard.Unlock()
	}
	close(started.done)
}

// CloseAll closes every connection of this server.
func (sessions *Sessions) CloseAll() {
	sessions.guard.Lock()
	open := make([]*attempt, 0, len(sessions.opening))
	for _, started := range sessions.opening {
		open = append(open, started)
	}
	sessions.opening = map[string]*attempt{}
	sessions.guard.Unlock()

	for _, started := range open {
		<-started.done
		if started.connection != nil {
			started.connection.stop()
		}
	}
}

// openProfileConnection opens a connection and reads its tables, closing resources on failure.
func openProfileConnection(
	ctx context.Context, profile cfg.Profile, adapters engines.Adapters,
) (*Connection, error) {
	if unreachable := FindUnreachableReason(profile); unreachable != "" {
		return nil, db.NewDatabaseError("%s", unreachable)
	}

	password, err := cfg.ResolveProfilePassword(profile)
	if err != nil {
		return nil, err
	}
	preConnect, err := cfg.StartPreConnectCommand(profile)
	if err != nil {
		return nil, err
	}
	session, err := adapters.Open(ctx, profile, password)
	if err != nil {
		preConnect.Stop()
		return nil, err
	}
	tables, err := session.ListTables(ctx)
	if err != nil {
		_ = session.Close()
		preConnect.Stop()
		return nil, err
	}
	return &Connection{
		Session: session, preConnect: preConnect, tables: tables, readAt: time.Now(),
	}, nil
}
