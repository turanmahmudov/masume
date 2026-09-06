package app

import (
	"slices"
	"strings"
	"time"

	"github.com/turanmahmudov/masume/internal/cfg"
	"github.com/turanmahmudov/masume/internal/hist"
)

// SavedRow is a saved statement from query history or the project file.
type SavedRow struct {
	Name string
	SQL  string
	// When the user saved it. Zero for a statement of the project file.
	SavedAt time.Time
	// The path of the project file that holds it. Empty for one the user saved.
	ProjectFile string
	// The optional statement description.
	Description string
}

// IsFromProject is true for a project statement, which the saved query list cannot delete.
func (row SavedRow) IsFromProject() bool {
	return row.ProjectFile != ""
}

// BuildSavedRows merges saved and project statements by name. User-saved statements replace project statements with the same name.
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
