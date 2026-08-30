package supabase

import (
	"github.com/turanmahmudov/masume/internal/core"
	"github.com/turanmahmudov/masume/internal/db/postgres"
)

// Support is everything known about Supabase before a connection exists. It speaks
// the postgres protocol, so it takes that dialect and language.
var Support = postgres.BuildSupport(core.EngineSupabase)
