package app

import (
	"hash/fnv"
	"time"

	"github.com/turanmahmudov/masume/internal/core"
	"github.com/turanmahmudov/masume/internal/present"
)

// The object tree is rebuilt from the whole catalog, which costs milliseconds on a server of
// thousands of relations. The frame is drawn after every key press and on every wake, and the
// keys of the tree read the rows as well, so the same tree was built several times for one
// press. It is built once here and kept until something it reads changes.

// treeFingerprint identifies everything BuildTree reads.
//
// The relations, the objects and the roles are replaced together, and ReadAt is set with them,
// so the moment of the read stands for all three. Everything else is small enough to read in
// full: only the rows the user opened, the relations they looked at, and what they marked.
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
	// The age beside a recent schema is written to the second, so a later moment inside one
	// second draws the same tree.
	second int64
}

// buildTreeFingerprint reads the state the tree is built from. The engine of a connection is
// settled when it opens, so it is not among it.
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

// BuildTree returns the rows of the object tree and the counts for its border. The tree is
// kept until something it is built from changes, because one key press draws one frame and
// reads the rows more than once.
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

// hashText adds one text to a running hash.
func hashText(running uint64, text string) uint64 {
	held := fnv.New64a()
	_, _ = held.Write([]byte(text))
	return running*31 + held.Sum64()
}

// hashOpenRows reads the rows the user opened. The order of a map is not settled, so each
// entry is added rather than written in turn.
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

// CatalogFingerprint identifies what the catalog holds, so a caller that keeps something it
// read from it can tell that it has to read it again.
type CatalogFingerprint struct {
	readAt  time.Time
	tables  int
	details uint64
}

// FingerprintCatalog reads the state of the catalog. The relations are replaced together and
// ReadAt is set with them, so the moment of the read stands for the whole list. The detail of
// one relation arrives on its own, so every column of every one that was read is read here:
// what a statement is checked against is the name and the type of each column.
func (connection *Connection) FingerprintCatalog() CatalogFingerprint {
	return CatalogFingerprint{
		readAt:  connection.Catalog.ReadAt,
		tables:  len(connection.Catalog.Tables),
		details: hashDetailColumns(connection.Catalog.Details),
	}
}

// hashDetailColumns reads every column of every relation whose detail was read.
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

// hashDetails reads what was read for each relation. The state of one turns from loading to
// read without the count of them changing, so the state is read as well as the name.
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

// hashFavourites reads what the user marked. A mark goes on and comes off, so the count moves
// with it, and the names are read in case two changes meet in one frame.
func hashFavourites(favourites []core.Favourite) uint64 {
	held := uint64(len(favourites))
	for _, one := range favourites {
		held += hashText(hashText(hashText(1, string(one.Kind)), one.Schema), one.Name)
	}
	return held
}

// hashRecent reads the schemas opened lately. The order is what the list is for, so it is read
// in turn rather than added.
func hashRecent(recent []core.RecentSchema) uint64 {
	held := uint64(len(recent))
	for _, one := range recent {
		held = hashText(held, one.Schema)
	}
	return held
}
