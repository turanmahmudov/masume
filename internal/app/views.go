// Package app holds the state of the client: the active screen, the open connections, the
// tabs of each connection, and the result of every run. Nothing in this package draws.
package app

import (
	"time"

	"github.com/turanmahmudov/masume/internal/db"
	"github.com/turanmahmudov/masume/internal/query"
)

// ResultView is one display mode of a result.
type ResultView string

// The views a tab can have.
const (
	ViewData ResultView = "data"
	// ViewTree shows the rows as documents: a value with fields or elements is expanded
	// and not truncated to the column width.
	ViewTree        ResultView = "tree"
	ViewFields      ResultView = "fields"
	ViewStatistics  ResultView = "statistics"
	ViewColumns     ResultView = "columns"
	ViewIndexes     ResultView = "indexes"
	ViewConstraints ResultView = "constraints"
	ViewDDL         ResultView = "ddl"
	ViewPlan        ResultView = "plan"
)

// DefaultView is the view a tab starts with and falls back to.
const DefaultView = ViewData

// TabKind is the binding of a tab.
type TabKind string

// The three kinds of tab.
const (
	// TabTable is bound to one table, so it can describe that table.
	TabTable TabKind = "table"
	// TabObject shows the definition of one schema object. It runs no statement.
	TabObject TabKind = "object"
	// TabQuery is bound to the text in its editor.
	TabQuery TabKind = "query"
)

// The views of each kind of tab, before the plan is removed.
var (
	tableViews = []ResultView{
		ViewData, ViewTree, ViewColumns, ViewIndexes, ViewConstraints, ViewDDL, ViewPlan,
	}
	queryViews = []ResultView{ViewData, ViewTree, ViewFields, ViewPlan}
	// A schema object has the definition view only.
	objectViews = []ResultView{ViewDDL}
	// The message shown for a statement without a result set.
	outcomeViews = []ResultView{ViewStatistics, ViewPlan}
)

// ListOfferedViews returns the views of this kind of tab, before the plan is removed.
func ListOfferedViews(kind TabKind, hasResultSet bool) []ResultView {
	switch kind {
	case TabTable:
		return tableViews
	case TabObject:
		return objectViews
	}
	if hasResultSet {
		return queryViews
	}
	return outcomeViews
}

// ViewDataKind is the content the pane draws in place of the grid.
type ViewDataKind string

// The kinds of content a view can draw.
const (
	DataIdle          ViewDataKind = "idle"
	DataTree          ViewDataKind = "tree"
	DataLoading       ViewDataKind = "loading"
	DataFailed        ViewDataKind = "failed"
	DataColumns       ViewDataKind = "columns"
	DataResultColumns ViewDataKind = "resultColumns"
	DataIndexes       ViewDataKind = "indexes"
	DataConstraints   ViewDataKind = "constraints"
	DataDDL           ViewDataKind = "ddl"
	DataStatistics    ViewDataKind = "statistics"
	DataPlan          ViewDataKind = "plan"
	DataGrid          ViewDataKind = "grid"
)

// Statistic is one line of the statistics view about the result of a statement.
type Statistic struct {
	Label string
	Value string
	// True for the line that reports the changes of the statement.
	Leading bool
}

// PaneContent is the data one view of the pane draws.
type PaneContent struct {
	Kind          ViewDataKind
	Reason        string
	Message       string
	Columns       []db.ColumnDetail
	ResultColumns []db.ResultColumn
	Indexes       []db.IndexDetail
	Constraints   []db.ConstraintDetail
	Lines         []string
	Statistics    []Statistic
	Plan          query.QueryPlan
	// The start time of the read, so the wait indicator can show the elapsed time.
	StartedAt time.Time
}
