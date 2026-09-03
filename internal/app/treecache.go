package app

import (
	"hash/fnv"
	"time"

	"github.com/turanmahmudov/masume/internal/core"
	"github.com/turanmahmudov/masume/internal/present"
)

// The object tree is built from the whole catalog, which costs milliseconds on a server with
// thousands of tables. The frame is drawn after every key press and at every wake, and the
// keys of the tree also read the rows, so the same tree was built several times for one key
// press. This file builds it one time and keeps it until an input changes.

// treeFingerprint identifies every input of BuildTree.
//
// The tables, the objects and the roles are replaced together, and ReadAt is set with them, so
// the read time identifies all three. Every other input is small enough to hash completely:
// the open rows, the tables the user looked at, and the marks.
type treeFingerprint struct {
	readAt      time.Time
	tables      int
	objects     int
	roles       int
	details     uint64
	expanded    uint64
	favourites  uint64
	recent      uint64
	filter      string
	filterScope string
	hideSystem  bool
	// The age of a recent schema is shown in seconds, so two times in the same second
	// give the same tree.
	second int64
}

// buildTreeFingerprint returns the fingerprint of the tree inputs. The engine of a connection
// is fixed at the open, so it is not part of the fingerprint.
func (connection *Connection) buildTreeFingerprint(now time.Time) treeFingerprint {
	return treeFingerprint{
		readAt:      connection.Catalog.ReadAt,
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

// BuildTree returns the rows of the object tree and the counts of its border. The tree is
// cached until an input changes, because one key press draws one frame and reads the rows
// several times.
func (connection *Connection) BuildTree(now time.Time) present.TreeResult {
	held := connection.buildTreeFingerprint(now)
	if connection.treeBuilt && connection.treeAt == held {
		return connection.treeResult
	}

	result := present.BuildTree(present.TreeInput{
		Tables: connection.Catalog.Tables, Objects: connection.Catalog.Objects,
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

// hashOpenRows hashes the open rows. A map has no fixed order, so each entry is added to the
// hash and the order is not used.
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

// CatalogFingerprint identifies the content of the catalog, so a caller that caches a result
// of the catalog knows when to build it again.
type CatalogFingerprint struct {
	readAt  time.Time
	tables  int
	details uint64
}

// FingerprintCatalog returns the fingerprint of the catalog. The tables are replaced together
// and ReadAt is set with them, so the read time identifies the whole list. The detail of one
// table arrives separately, so this function hashes every column of every table that was
// read: a statement is checked against the name and the type of each column.
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

// hashDetails hashes the read state of each table. A state changes from loading to read
// without a change of the count, so the hash contains the state and the name.
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

// hashFavourites hashes the marks of the user. A mark is added and removed, so the count
// changes with it. The names are hashed as well, in case two changes happen in one frame.
func hashFavourites(favourites []core.Favourite) uint64 {
	held := uint64(len(favourites))
	for _, one := range favourites {
		held += hashText(hashText(hashText(1, string(one.Kind)), one.Schema), one.Name)
	}
	return held
}

// hashRecent hashes the recently opened schemas. The order is the purpose of the list, so the
// hash uses the order.
func hashRecent(recent []core.RecentSchema) uint64 {
	held := uint64(len(recent))
	for _, one := range recent {
		held = hashText(held, one.Schema)
	}
	return held
}
