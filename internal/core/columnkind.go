package core

// Column kinds are inferred from import values. Each dialect maps kinds to database types.

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

// widerKinds is the set of compatible type pairs. Other mixed types use text.
var widerKinds = map[ColumnKind]map[ColumnKind]ColumnKind{
	KindInteger: {KindNumber: KindNumber},
	KindNumber:  {KindInteger: KindNumber},
}

// ResolveWiderKind returns a common type, with text as the fallback.
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
