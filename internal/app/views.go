// Package app holds the state of the client: which screen is active, the open
// connections, the tabs of each and the result of every run. Nothing here draws.
package app

import (
	"time"

	"github.com/turanmahmudov/masume/internal/db"
	"github.com/turanmahmudov/masume/internal/query"
)

// ResultView is one way of looking at a result.
type ResultView string

// The views a tab can offer.
const (
	ViewData ResultView = "data"
	// ViewTree opens the rows as documents: a value that holds fields or elements is
	// opened into them rather than cut to the width of a column.
	ViewTree        ResultView = "tree"
	ViewFields      ResultView = "fields"
	ViewStatistics  ResultView = "statistics"
	ViewColumns     ResultView = "columns"
	ViewIndexes     ResultView = "indexes"
	ViewConstraints ResultView = "constraints"
	ViewDDL         ResultView = "ddl"
	ViewPlan        ResultView = "plan"
)

// DefaultView is the view a tab opens on, and falls back to.
const DefaultView = ViewData

// TabKind says what a tab is bound to.
type TabKind string

// The three kinds of tab.
const (
	// TabTable is bound to one relation, so it can describe it.
	TabTable TabKind = "table"
	// TabObject shows the definition of one schema object. Nothing runs for it.
	TabObject TabKind = "object"
	// TabQuery is bound to the text in its editor.
	TabQuery TabKind = "query"
)

// The views each kind of tab offers, before the plan is left out.
var (
	tableViews = []ResultView{
		ViewData, ViewTree, ViewColumns, ViewIndexes, ViewConstraints, ViewDDL, ViewPlan,
	}
	queryViews = []ResultView{ViewData, ViewTree, ViewFields, ViewPlan}
	// A schema object is shown by its definition only.
	objectViews = []ResultView{ViewDDL}
	// The message shown for a statement with no result set.
	outcomeViews = []ResultView{ViewStatistics, ViewPlan}
)

// ListOfferedViews returns the views this kind of tab offers, before the plan is left out.
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

// ViewDataKind says what the pane draws in place of the grid.
type ViewDataKind string

// The kinds of thing a view can be drawing.
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

// Statistic is one line about what a statement did, for the statistics view.
type Statistic struct {
	Label string
	Value string
	// True for the line that reports what the statement changed.
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
	// When the read began, so the wheel of the wait can say how long it has run.
	StartedAt time.Time
}
