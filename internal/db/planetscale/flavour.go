package planetscale

import (
	"github.com/turanmahmudov/masume/internal/db/mysql"
)

// Flavour stops no session for a client. The platform does that in its
// own interface, so nothing is sent.
var Flavour = mysql.Flavour{
	BuildExplainStatement: mysql.FlavourStandard.BuildExplainStatement,
	ReadPlan:              mysql.FlavourStandard.ReadPlan,
	ReadOnlyStatement:     mysql.FlavourStandard.ReadOnlyStatement,
	BuildKillStatement:    func(int64, bool) string { return "" },
}
