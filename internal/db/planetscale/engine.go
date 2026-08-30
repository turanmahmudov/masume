package planetscale

import (
	"github.com/turanmahmudov/masume/internal/core"
	"github.com/turanmahmudov/masume/internal/db/mysql"
)

// Support is everything known about PlanetScale before a connection exists. It speaks
// the mysql protocol, so it takes that dialect and language.
var Support = mysql.BuildSupport(core.EnginePlanetscale)
