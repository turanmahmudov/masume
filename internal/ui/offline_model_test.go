package ui

import (
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/turanmahmudov/masume/internal/app"
	"github.com/turanmahmudov/masume/internal/cfg"
	"github.com/turanmahmudov/masume/internal/core"
	"github.com/turanmahmudov/masume/internal/db"
	"github.com/turanmahmudov/masume/internal/db/postgres"
	"github.com/turanmahmudov/masume/internal/query"
	"github.com/turanmahmudov/masume/internal/query/language"
)

// offlineSession answers only the parts of a session a frame reads, so a frame can be drawn
// with no server behind it.
type offlineSession struct {
	db.Session
	profile      cfg.Profile
	capabilities core.Capabilities
	// The dialect of the server, so a test can stand this session in for one that has no
	// SQL. The PostgreSQL dialect answers where none is named.
	dialect *query.Dialect
}

func (session *offlineSession) Describe() db.SessionDescriptor {
	return db.SessionDescriptor{Profile: session.profile}
}

func (session *offlineSession) Dialect() *query.Dialect {
	if session.dialect != nil {
		return session.dialect
	}
	return postgres.Dialect
}

func (session *offlineSession) Language() language.Language {
	return language.SQL
}

func (session *offlineSession) Capabilities() core.Capabilities {
	return session.capabilities
}

func (session *offlineSession) Composer() db.Composer {
	return db.NewSQLComposer(session.Dialect())
}

func (session *offlineSession) ReadTransactionState() db.TransactionState {
	return db.TransactionNone
}

// buildOfflineModel answers a model on one connection with one tab, drawn at this size.
func buildOfflineModel(t *testing.T, width, height int) *Model {
	t.Helper()
	return buildOfflineModelFor(t, width, height)
}

// buildOfflineModelFor answers the same model for a benchmark as for a test.
func buildOfflineModelFor(reporter testing.TB, width, height int) *Model {
	reporter.Helper()
	model := NewModel(loadedConfigForTest("tokyonight"), nil, nil, nil)
	held, _ := model.Update(tea.WindowSizeMsg{Width: width, Height: height})
	model = held.(*Model)

	session := &offlineSession{
		profile: cfg.Profile{Name: "offline", Engine: "postgres"},
		// A real server of this engine can order a read, so the header reads the sort
		// the statement itself writes and the frame pays for that read.
		capabilities: core.Capabilities{SortsRead: true},
	}
	connection := app.NewConnection(session, nil, true)
	model.connections.open(connection)
	model.screen = ScreenWorking
	return model
}
