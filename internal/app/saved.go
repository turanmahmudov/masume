package app

import (
	"slices"
	"strings"
	"time"

	"github.com/turanmahmudov/masume/internal/cfg"
	"github.com/turanmahmudov/masume/internal/hist"
)

// SavedRow is one row of the saved statements: one the user kept under a name in the history
// file, or one the project file holds for the whole team.
type SavedRow struct {
	Name string
	SQL  string
	// When the user saved it. Zero for a statement of the project file.
	SavedAt time.Time
	// The path of the project file that holds it. Empty for one the user saved.
	ProjectFile string
	// What the statement answers, shown instead of its text where the file says.
	Description string
}

// IsFromProject is true for a statement the project file holds, which the user cannot
// delete.
func (row SavedRow) IsFromProject() bool {
	return row.ProjectFile != ""
}

// BuildSavedRows returns the statements the user saved on this profile with the ones the
// project file offers on it, sorted by name. A statement the user saved replaces a project
// statement of the same name, so a personal statement always wins over a committed one.
func BuildSavedRows(
	saved []hist.SavedQuery, project cfg.ProjectConfig, profileName string,
) []SavedRow {
	queries := cfg.FindProjectQueries(project, profileName)
	rows := make([]SavedRow, 0, len(saved)+len(queries))
	for _, held := range saved {
		rows = append(rows, SavedRow{Name: held.Name, SQL: held.SQL, SavedAt: held.SavedAt})
	}
	for _, query := range queries {
		if slices.ContainsFunc(saved, func(held hist.SavedQuery) bool {
			return held.Name == query.Name
		}) {
			continue
		}
		rows = append(rows, SavedRow{
			Name: query.Name, SQL: query.SQL, Description: query.Description,
			ProjectFile: project.Path,
		})
	}
	slices.SortStableFunc(rows, func(left, right SavedRow) int {
		return strings.Compare(left.Name, right.Name)
	})
	return rows
}
