package mariadb

import (
	"github.com/turanmahmudov/masume/internal/db"
	"github.com/turanmahmudov/masume/internal/db/mysql"
)

// Flavour has neither the plan tree of MySQL nor `explain analyze`. It
// writes JSON, and it measures a plan with its own keyword.
var Flavour = mysql.Flavour{
	BuildExplainStatement: func(sql string, analyze bool) string {
		if analyze {
			return "analyze format=json " + sql
		}
		return "explain format=json " + sql
	},
	ReadPlan: func(result db.QueryResult, analyzed bool) (db.QueryPlan, bool) {
		return ReadPlan(mysql.ReadFirstCell(result), analyzed)
	},
	ReadOnlyStatement:  "set session transaction read only",
	BuildKillStatement: mysql.BuildKillStatement,
}
