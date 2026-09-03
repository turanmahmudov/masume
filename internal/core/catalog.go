package core

import (
	"fmt"
	"strings"
	"time"
)

// FavouriteKind says whether a favourite is a schema or a table.
type FavouriteKind string

// The two kinds of favourite.
const (
	FavouriteSchema FavouriteKind = "schema"
	FavouriteTable  FavouriteKind = "table"
)

// Favourite is a schema or a table the user marked. It is stored per profile.
type Favourite struct {
	Kind   FavouriteKind
	Schema string
	Name   string
}

// RecentSchema is a schema the user opened, with the time of the visit.
type RecentSchema struct {
	Schema    string
	VisitedAt time.Time
}

// RecentLimit is the number of schemas the recent list keeps.
const RecentLimit = 5

// CatalogTTL is how long a table list stays valid before it is read again. A table
// created after the read must become visible.
const CatalogTTL = time.Minute

// schemaIDPrefix is the prefix of every schema row id.
const schemaIDPrefix = "schema:"

// BuildSchemaID returns the row id the tree uses for one schema.
func BuildSchemaID(schema string) string {
	return schemaIDPrefix + schema
}

// FindSchemaOfID returns the schema name in a row id, or an empty string for any
// other row.
func FindSchemaOfID(id string) string {
	if !strings.HasPrefix(id, schemaIDPrefix) {
		return ""
	}
	return strings.TrimPrefix(id, schemaIDPrefix)
}

// BuildFavouriteID returns the id of one favourite, so the tree can mark that row.
func BuildFavouriteID(favourite Favourite) string {
	if favourite.Kind == FavouriteSchema {
		return "favourite:schema:" + favourite.Schema
	}
	return fmt.Sprintf("favourite:table:%s.%s", favourite.Schema, favourite.Name)
}
