package core

import (
	"fmt"
	"strings"
	"time"
)

// FavouriteKind tells a marked schema from a marked relation.
type FavouriteKind string

// The two kinds of mark.
const (
	FavouriteSchema FavouriteKind = "schema"
	FavouriteTable  FavouriteKind = "table"
)

// Favourite is a schema or a table the user marked, kept per profile.
type Favourite struct {
	Kind   FavouriteKind
	Schema string
	Name   string
}

// RecentSchema is a schema the user opened, and when.
type RecentSchema struct {
	Schema    string
	VisitedAt time.Time
}

// RecentLimit is how many schemas the recent list keeps.
const RecentLimit = 5

// CatalogTTL is how long a table list is trusted before it is read again. A table
// created after the read must be found.
const CatalogTTL = time.Minute

// schemaIDPrefix is the prefix every schema row id carries.
const schemaIDPrefix = "schema:"

// BuildSchemaID names one schema, which the tree uses as the id of its row.
func BuildSchemaID(schema string) string {
	return schemaIDPrefix + schema
}

// FindSchemaOfID returns the schema this row id names, or an empty name for any
// other row.
func FindSchemaOfID(id string) string {
	if !strings.HasPrefix(id, schemaIDPrefix) {
		return ""
	}
	return strings.TrimPrefix(id, schemaIDPrefix)
}

// BuildFavouriteID names one mark, so a row the user marked is drawn marked.
func BuildFavouriteID(favourite Favourite) string {
	if favourite.Kind == FavouriteSchema {
		return "favourite:schema:" + favourite.Schema
	}
	return fmt.Sprintf("favourite:table:%s.%s", favourite.Schema, favourite.Name)
}
