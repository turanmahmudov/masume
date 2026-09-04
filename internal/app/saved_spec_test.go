package app_test

import (
	"strings"
	"testing"
	"time"

	"github.com/turanmahmudov/masume/internal/app"
	"github.com/turanmahmudov/masume/internal/cfg"
	"github.com/turanmahmudov/masume/internal/hist"
)

// describeSavedRows returns one line per row, so a case states the list it wants rather than
// a field at a time.
func describeSavedRows(rows []app.SavedRow) string {
	said := make([]string, 0, len(rows))
	for _, row := range rows {
		mark := "mine"
		if row.IsFromProject() {
			mark = "project"
		}
		said = append(said, row.Name+":"+mark)
	}
	return strings.Join(said, " ")
}

func TestBuildSavedRowsMergesBothSourcesByName(t *testing.T) {
	saved := []hist.SavedQuery{{Name: "my-check", SQL: "select 1", SavedAt: time.Now()}}
	project := cfg.ProjectConfig{
		Path: "/repo/.masume.toml",
		Queries: []cfg.ProjectQuery{
			{Name: "recent-orders", SQL: "select 2", Description: "the newest orders"},
			{Name: "audit", SQL: "select 3"},
		},
	}

	rows := app.BuildSavedRows(saved, project, "dev")
	if got := describeSavedRows(rows); got != "audit:project my-check:mine recent-orders:project" {
		t.Fatalf("the card holds %q", got)
	}
	if rows[0].ProjectFile != project.Path {
		t.Errorf("a project row names %q as its file", rows[0].ProjectFile)
	}
	if rows[2].Description != "the newest orders" {
		t.Errorf("the description reads %q", rows[2].Description)
	}
}

// A statement the user saved replaces a project statement of the same name, so a personal
// statement always wins over a committed one.
func TestBuildSavedRowsKeepsTheStatementOfTheUser(t *testing.T) {
	saved := []hist.SavedQuery{{Name: "audit", SQL: "select mine"}}
	project := cfg.ProjectConfig{
		Path:    "/repo/.masume.toml",
		Queries: []cfg.ProjectQuery{{Name: "audit", SQL: "select theirs"}},
	}

	rows := app.BuildSavedRows(saved, project, "dev")
	if len(rows) != 1 {
		t.Fatalf("the card holds %q, wanted one row", describeSavedRows(rows))
	}
	if rows[0].SQL != "select mine" || rows[0].IsFromProject() {
		t.Errorf("the card holds the project statement %q", rows[0].SQL)
	}
}

// A statement the file offers on named profiles only does not reach another connection.
func TestBuildSavedRowsKeepsTheQueriesOfThisProfile(t *testing.T) {
	project := cfg.ProjectConfig{
		Path: "/repo/.masume.toml",
		Queries: []cfg.ProjectQuery{
			{Name: "dev-only", SQL: "select 1", Profiles: []string{"dev"}},
			{Name: "everywhere", SQL: "select 2"},
		},
	}

	rows := app.BuildSavedRows(nil, project, "prod")
	if got := describeSavedRows(rows); got != "everywhere:project" {
		t.Fatalf("the card holds %q", got)
	}
}

// A user without a project file sees the statements they saved and nothing else.
func TestBuildSavedRowsWithoutAProjectFile(t *testing.T) {
	saved := []hist.SavedQuery{{Name: "nightly", SQL: "select 1"}}

	rows := app.BuildSavedRows(saved, cfg.ProjectConfig{}, "dev")
	if got := describeSavedRows(rows); got != "nightly:mine" {
		t.Fatalf("the card holds %q", got)
	}
}
