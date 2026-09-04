package core

// The kind of value a column holds, which is what a data file is read as before a server
// is involved. A file carries text, so the kind of each column is read from the values in
// it, and each dialect writes its own type name for the kind.

// ColumnKind is the kind of value a column holds.
type ColumnKind string

// The kinds a column of a data file is read as.
const (
	KindText      ColumnKind = "text"
	KindInteger   ColumnKind = "integer"
	KindNumber    ColumnKind = "number"
	KindBoolean   ColumnKind = "boolean"
	KindTimestamp ColumnKind = "timestamp"
)

// widerKinds give the kind that holds both of two kinds. Only the pairs that share one are
// here: any other pair is held by text alone.
var widerKinds = map[ColumnKind]map[ColumnKind]ColumnKind{
	KindInteger: {KindNumber: KindNumber},
	KindNumber:  {KindInteger: KindNumber},
}

// ResolveWiderKind returns the kind that holds the values of both kinds. Text holds every
// value, so it is the answer for two kinds that share nothing else.
func ResolveWiderKind(left, right ColumnKind) ColumnKind {
	if left == right {
		return left
	}
	if left == "" {
		return right
	}
	if right == "" {
		return left
	}
	if wider, shared := widerKinds[left][right]; shared {
		return wider
	}
	return KindText
}
