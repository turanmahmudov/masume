// Package engines is the one place that knows every engine: what each one supports,
// the driver that opens it, and how a read or a staged edit is composed for it.
package engines

import (
	"github.com/turanmahmudov/masume/internal/core"
	"github.com/turanmahmudov/masume/internal/db"
	"github.com/turanmahmudov/masume/internal/db/auroramysql"
	"github.com/turanmahmudov/masume/internal/db/cockroach"
	"github.com/turanmahmudov/masume/internal/db/mariadb"
	"github.com/turanmahmudov/masume/internal/db/mongo"
	"github.com/turanmahmudov/masume/internal/db/mysql"
	"github.com/turanmahmudov/masume/internal/db/neon"
	"github.com/turanmahmudov/masume/internal/db/planetscale"
	"github.com/turanmahmudov/masume/internal/db/postgres"
	"github.com/turanmahmudov/masume/internal/db/redshift"
	"github.com/turanmahmudov/masume/internal/db/sqlite"
	"github.com/turanmahmudov/masume/internal/db/supabase"
	"github.com/turanmahmudov/masume/internal/db/tidb"
	"github.com/turanmahmudov/masume/internal/db/timescale"
)

// support holds one entry per engine. An engine missing here cannot be opened.
var support = map[core.Engine]db.EngineSupport{
	core.EnginePostgres: postgres.Support,
	core.EngineMysql:    mysql.Support,
	core.EngineSqlite:   sqlite.Support,
	core.EngineMongo:    mongo.Support,

	core.EngineCockroach: cockroach.Support,
	core.EngineTimescale: timescale.Support,
	core.EngineRedshift:  redshift.Support,
	core.EngineNeon:      neon.Support,
	core.EngineSupabase:  supabase.Support,

	core.EngineMariadb:     mariadb.Support,
	core.EngineTidb:        tidb.Support,
	core.EnginePlanetscale: planetscale.Support,
	core.EngineAuroraMysql: auroramysql.Support,
}

// ResolveSupport returns everything known about that engine before a connection exists.
func ResolveSupport(engine core.Engine) db.EngineSupport {
	return support[engine]
}
