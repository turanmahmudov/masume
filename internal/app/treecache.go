package app

import (
	"hash/fnv"
	"time"

	"github.com/turanmahmudov/masume/internal/core"
	"github.com/turanmahmudov/masume/internal/present"
)

// Cache the object tree until its input fingerprint changes.

// treeFingerprint is the cache key for tree inputs. Catalog replacement updates ReadAt; detail and view changes use hashes.
type treeFingerprint struct {
	readAt      time.Time
	tables      int
	objects     int
	roles       int
	details     uint64
	schemas     int
	expanded    uint64
	favourites  uint64
	recent      uint64
	filter      string
	filterScope string
	hideSystem  bool
	// The display time rounded to seconds.
	second int64
}

// buildTreeFingerprint builds a cache key for mutable tree inputs.
func (connection *Connection) buildTreeFingerprint(now time.Time) treeFingerprint {
	return treeFingerprint{
		readAt:      connection.Catalog.ReadAt,
		schemas:     len(connection.Catalog.Schemas),
		tables:      len(connection.Catalog.Tables),
		objects:     len(connection.Catalog.Objects),
		roles:       len(connection.Catalog.Roles),
		details:     hashDetails(connection.Catalog.Details),
		expanded:    hashOpenRows(connection.Tree.Expanded),
		favourites:  hashFavourites(connection.Marks.Favourites),
		recent:      hashRecent(connection.Marks.Recent),
		filter:      connection.Tree.Filter,
		filterScope: connection.Tree.FilterScope,
		hideSystem:  connection.Tree.HideSystemSchemas,
		second:      now.Unix(),
	}
}

// BuildTree returns cached tree rows and counts until the input fingerprint changes.
func (connection *Connection) BuildTree(now time.Time) present.TreeResult {
	held := connection.buildTreeFingerprint(now)
	if connection.treeBuilt && connection.treeAt == held {
		return connection.treeResult
	}

	result := present.BuildTree(present.TreeInput{
		Schemas: connection.Catalog.Schemas,
		Tables:  connection.Catalog.Tables, Objects: connection.Catalog.Objects,
		Roles: connection.Catalog.Roles, Details: connection.Catalog.Details,
		Favourites: connection.Marks.Favourites, Recent: connection.Marks.Recent,
		Engine: connection.Profile().Engine, HideSystemSchemas: connection.Tree.HideSystemSchemas,
		Expanded: connection.Tree.Expanded, Filter: connection.Tree.Filter,
		FilterScope: connection.Tree.FilterScope, Now: now,
	})

	connection.treeAt, connection.treeResult, connection.treeBuilt = held, result, true
	return result
}

// hashText adds one text to the hash.
func hashText(running uint64, text string) uint64 {
	held := fnv.New64a()
	_, _ = held.Write([]byte(text))
	return running*31 + held.Sum64()
}

// hashOpenRows hashes expanded row IDs independently of map iteration order.
func hashOpenRows(open map[string]bool) uint64 {
	held := uint64(len(open))
	for id, isOpen := range open {
		if !isOpen {
			continue
		}
		held += hashText(1, id)
	}
	return held
}

// CatalogFingerprint is the cache key for loaded tables and column details.
type CatalogFingerprint struct {
	readAt  time.Time
	tables  int
	details uint64
}

// FingerprintCatalog combines the catalog timestamp, table count, and loaded column hashes.
func (connection *Connection) FingerprintCatalog() CatalogFingerprint {
	return CatalogFingerprint{
		readAt:  connection.Catalog.ReadAt,
		tables:  len(connection.Catalog.Tables),
		details: hashDetailColumns(connection.Catalog.Details),
	}
}

// hashDetailColumns hashes every column of every table whose detail was read.
func hashDetailColumns(details map[string]present.TableDetailState) uint64 {
	held := uint64(len(details))
	for id, state := range details {
		one := hashText(1, id)
		one = hashText(one, string(state.Kind))
		one = hashText(one, state.Message)
		for _, column := range state.Detail.Columns {
			one = hashText(one, column.Name)
			one = hashText(one, column.DataType)
			if column.IsPrimaryKey {
				one = one*31 + 1
			}
		}
		held += one
	}
	return held
}

// hashDetails hashes table detail states, errors, and column and foreign key counts.
func hashDetails(details map[string]present.TableDetailState) uint64 {
	held := uint64(len(details))
	for id, state := range details {
		one := hashText(1, id)
		one = hashText(one, string(state.Kind))
		one = hashText(one, state.Message)
		one = one*31 + uint64(len(state.Detail.Columns))
		one = one*31 + uint64(len(state.Detail.ForeignKeys))
		held += one
	}
	return held
}

// hashFavourites hashes favourite kinds, schemas, and names.
func hashFavourites(favourites []core.Favourite) uint64 {
	held := uint64(len(favourites))
	for _, one := range favourites {
		held += hashText(hashText(hashText(1, string(one.Kind)), one.Schema), one.Name)
	}
	return held
}

// hashRecent hashes recent schema names in list order.
func hashRecent(recent []core.RecentSchema) uint64 {
	held := uint64(len(recent))
	for _, one := range recent {
		held = hashText(held, one.Schema)
	}
	return held
}
