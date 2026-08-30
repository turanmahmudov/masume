package timescale

import (
	"github.com/turanmahmudov/masume/internal/core"
	"github.com/turanmahmudov/masume/internal/db/postgres"
)

// Support is everything known about TimescaleDB before a connection exists. It speaks
// the postgres protocol, so it takes that dialect and language.
var Support = postgres.BuildSupport(core.EngineTimescale)
