package query

// The data every tier reads about a result: what a column is, what a statement
// binds, how a relation points at another, and what a plan holds. The engines build
// these and the panes draw them, so they sit below both.

// ResultColumn is one column of a result.
type ResultColumn struct {
	Name     string
	DataType string
}

// BoundStatement is a statement with the values it binds.
type BoundStatement struct {
	SQL         string
	Params      []any
	Description string
}

// ForeignKey is a foreign key of a table.
type ForeignKey struct {
	Name          string
	Columns       []string
	TargetSchema  string
	TargetTable   string
	TargetColumns []string
}

// ForeignKeyTarget is the column a foreign key points at.
type ForeignKeyTarget struct {
	Schema string
	Table  string
	Column string
}

// PlanNode is one step of a query plan.
type PlanNode struct {
	Label            string
	Detail           string
	EstimatedRows    float64
	HasEstimatedRows bool
	ActualRows       float64
	HasActualRows    bool
	// The time of this node and everything under it, over every loop.
	TotalMs    float64
	HasTotalMs bool
	// The time of this node alone.
	SelfMs    float64
	HasSelfMs bool
	Children  []PlanNode
}

// QueryPlan is a query plan, as the server explained it.
type QueryPlan struct {
	Root           PlanNode
	Raw            string
	PlanningMs     float64
	HasPlanningMs  bool
	ExecutionMs    float64
	HasExecutionMs bool
	Analyzed       bool
	Measurable     bool
}
