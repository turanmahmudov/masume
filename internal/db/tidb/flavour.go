package tidb

import (
	"github.com/turanmahmudov/masume/internal/db"
	"github.com/turanmahmudov/masume/internal/db/mysql"
)

// Flavour writes a plan as rows, and it has no read-only session. The server
// takes the statement and does nothing under it, so a profile that asks for one is
// refused rather than opened on a promise the server would not keep.
var Flavour = mysql.Flavour{
	BuildExplainStatement: func(sql string, analyze bool) string {
		if analyze {
			return "explain analyze " + sql
		}
		return "explain " + sql
	},
	ReadPlan: func(result db.QueryResult, analyzed bool) (db.QueryPlan, bool) {
		rows, order := mysql.ReadNamedPlanRows(result)
		return ReadPlan(rows, order, analyzed)
	},
	ReadOnlyStatement:  "",
	BuildKillStatement: mysql.BuildKillStatement,
}
