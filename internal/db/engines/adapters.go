package engines

import (
	"context"

	"github.com/turanmahmudov/masume/internal/cfg"
	"github.com/turanmahmudov/masume/internal/core"
	"github.com/turanmahmudov/masume/internal/db"
	"github.com/turanmahmudov/masume/internal/db/auroramysql"
	"github.com/turanmahmudov/masume/internal/db/mariadb"
	"github.com/turanmahmudov/masume/internal/db/mongo"
	"github.com/turanmahmudov/masume/internal/db/mysql"
	"github.com/turanmahmudov/masume/internal/db/planetscale"
	"github.com/turanmahmudov/masume/internal/db/postgres"
	"github.com/turanmahmudov/masume/internal/db/sqlite"
	"github.com/turanmahmudov/masume/internal/db/sqlserver"
	"github.com/turanmahmudov/masume/internal/db/tidb"
)

// Adapters is the map of engines to connection adapters.
type Adapters map[core.Engine]db.Adapter

// CreateAdapters builds adapters with engine-specific metadata and protocol settings.
func CreateAdapters() Adapters {
	return Adapters{
		core.EnginePostgres:  postgres.NewAdapter(postgres.Support, postgres.FlavourStandard),
		core.EngineMysql:     mysql.NewAdapter(mysql.Support, mysql.FlavourStandard),
		core.EngineSqlite:    sqlite.NewAdapter(sqlite.Support),
		core.EngineMongo:     mongo.NewAdapter(mongo.Support),
		core.EngineSqlserver: sqlserver.NewAdapter(sqlserver.Support),

		// These use the PostgreSQL protocol.
		core.EngineCockroach: postgres.NewAdapter(ResolveSupport(core.EngineCockroach), postgres.FlavourCockroach),
		core.EngineTimescale: postgres.NewAdapter(ResolveSupport(core.EngineTimescale), postgres.FlavourStandard),
		core.EngineRedshift:  postgres.NewAdapter(ResolveSupport(core.EngineRedshift), postgres.FlavourRedshift),
		core.EngineNeon:      postgres.NewAdapter(ResolveSupport(core.EngineNeon), postgres.FlavourStandard),
		core.EngineSupabase:  postgres.NewAdapter(ResolveSupport(core.EngineSupabase), postgres.FlavourStandard),

		// These use the MySQL protocol.
		core.EngineMariadb:     mysql.NewAdapter(ResolveSupport(core.EngineMariadb), mariadb.Flavour),
		core.EngineTidb:        mysql.NewAdapter(ResolveSupport(core.EngineTidb), tidb.Flavour),
		core.EnginePlanetscale: mysql.NewAdapter(ResolveSupport(core.EnginePlanetscale), planetscale.Flavour),
		core.EngineAuroraMysql: mysql.NewAdapter(ResolveSupport(core.EngineAuroraMysql), auroramysql.Flavour),
	}
}

// Open returns the session of that profile, through the adapter of its engine.
func (adapters Adapters) Open(
	ctx context.Context, profile cfg.Profile, password string,
) (db.Session, error) {
	adapter, known := adapters[profile.Engine]
	if !known {
		return nil, db.NewDatabaseError("no driver is registered for engine %s", profile.Engine)
	}
	session, err := adapter.Connect(ctx, profile, password)
	if err != nil {
		return nil, err
	}
	// Session wrappers apply reconnection, time limits, and read-only checks.
	return db.MakeReadOnly(
		db.MakeTimeLimited(db.MakeReconnectable(session, adapter, password))), nil
}
