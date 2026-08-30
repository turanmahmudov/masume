package mysql

import (
	"strconv"

	"github.com/turanmahmudov/masume/internal/core"
	"github.com/turanmahmudov/masume/internal/db"
	"github.com/turanmahmudov/masume/internal/query/result"
)

// Flavour holds the parts each MySQL-protocol server does differently. They share
// the wire, `information_schema` and the SQL, so they share one adapter.
type Flavour struct {
	// MySQL returns a tree in one cell, MariaDB returns JSON, and TiDB returns one row
	// per operator.
	BuildExplainStatement func(sql string, analyze bool) string
	ReadPlan              func(answered db.QueryResult, analyzed bool) (db.QueryPlan, bool)
	// Empty if the server has no read-only session.
	ReadOnlyStatement string
	// BuildKillStatement writes nothing if a client cannot stop a session.
	BuildKillStatement func(pid int64, terminate bool) string
}

// ReadFirstCell returns the first cell of the first row, where a server writes the whole
// plan.
func ReadFirstCell(answered db.QueryResult) string {
	if len(answered.Rows) == 0 || len(answered.Rows[0]) == 0 {
		return ""
	}
	return core.FormatCell(answered.Rows[0][0], "")
}

// ReadNamedPlanRows returns the plan rows keyed by column name, for a server that writes
// one row per operator.
func ReadNamedPlanRows(answered db.QueryResult) ([]map[string]any, []string) {
	order := make([]string, 0, len(answered.Columns))
	for _, column := range answered.Columns {
		order = append(order, column.Name)
	}
	rows := make([]map[string]any, 0, len(answered.Rows))
	for _, row := range answered.Rows {
		named := map[string]any{}
		for at, name := range order {
			if at < len(row) {
				named[name] = row[at]
			}
		}
		rows = append(rows, named)
	}
	return rows, order
}

func BuildKillStatement(pid int64, terminate bool) string {
	if terminate {
		return "kill connection " + strconv.FormatInt(pid, 10)
	}
	return "kill query " + strconv.FormatInt(pid, 10)
}

// FlavourStandard is MySQL itself, which the other flavours differ from.
var FlavourStandard = Flavour{
	// A tree is the only form MySQL returns for both an estimated and a measured plan.
	BuildExplainStatement: func(sql string, analyze bool) string {
		if analyze {
			return "explain analyze " + sql
		}
		return "explain format=tree " + sql
	},
	ReadPlan: func(answered db.QueryResult, analyzed bool) (db.QueryPlan, bool) {
		return result.ParseTextPlan(ReadFirstCell(answered), analyzed, true)
	},
	ReadOnlyStatement:  "set session transaction read only",
	BuildKillStatement: BuildKillStatement,
}
