package auroramysql

import (
	"strconv"

	"github.com/turanmahmudov/masume/internal/db/mysql"
)

// Flavour refuses KILL, because on Aurora and RDS the administrator is the
// platform and not the client. The server offers two procedures instead.
var Flavour = mysql.Flavour{
	BuildExplainStatement: mysql.FlavourStandard.BuildExplainStatement,
	ReadPlan:              mysql.FlavourStandard.ReadPlan,
	ReadOnlyStatement:     mysql.FlavourStandard.ReadOnlyStatement,
	BuildKillStatement: func(pid int64, terminate bool) string {
		if terminate {
			return "call mysql.rds_kill(" + strconv.FormatInt(pid, 10) + ")"
		}
		return "call mysql.rds_kill_query(" + strconv.FormatInt(pid, 10) + ")"
	},
}
