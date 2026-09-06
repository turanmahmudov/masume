package query

import "strings"

// Shared result columns, bound statements, foreign keys, and query plans.

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
	// The action after a referenced row is deleted, or empty when unknown.
	DeleteRule DeleteRule
}

// DeleteRule is the foreign key action after deletion of a referenced row.
type DeleteRule string

// The rules a server can hold. NoAction and Restrict both refuse the delete.
const (
	DeleteRuleUnknown    DeleteRule = ""
	DeleteRuleNoAction   DeleteRule = "no action"
	DeleteRuleRestrict   DeleteRule = "restrict"
	DeleteRuleCascade    DeleteRule = "cascade"
	DeleteRuleSetNull    DeleteRule = "set null"
	DeleteRuleSetDefault DeleteRule = "set default"
)

// ReachesRows is true for foreign key actions that modify referencing rows.
func (rule DeleteRule) ReachesRows() bool {
	return rule == DeleteRuleCascade ||
		rule == DeleteRuleSetNull || rule == DeleteRuleSetDefault
}

var deleteRules = []DeleteRule{
	DeleteRuleNoAction, DeleteRuleRestrict,
	DeleteRuleCascade, DeleteRuleSetNull, DeleteRuleSetDefault,
}

// ParseDeleteRule parses a catalog action or returns DeleteRuleUnknown.
func ParseDeleteRule(written string) DeleteRule {
	lowered := strings.ToLower(strings.TrimSpace(written))
	for _, rule := range deleteRules {
		if string(rule) == lowered {
			return rule
		}
	}
	return DeleteRuleUnknown
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
