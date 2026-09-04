package query

import (
	"regexp"
	"strings"

	"github.com/turanmahmudov/masume/internal/core"
	"github.com/turanmahmudov/masume/internal/query/syntax"
)

// QualifiedName is a relation named by schema and name.
type QualifiedName struct {
	Schema string
	Name   string
}

// Dialect says where one server writes SQL differently from another. Each family
// has one, and the servers that speak its protocol share it.
type Dialect struct {
	// Servers that share a dialect share the name: CockroachDB and Redshift use `postgres`.
	Engine core.Engine
	// How this server reads a comment and a string, which the lexer needs.
	Syntax syntax.SyntaxFlavour
	// The word for the group of a relation, in a message to the user.
	SchemaWord string
	// StatementLanguage names what a statement of this server is written in, for a
	// message to the user and for the chat.
	StatementLanguage string
	// FenceTag is how a fenced block of that language is opened in a reply of a model.
	FenceTag string
	// StatementExample shows the shape of one statement, for a server whose statements
	// are no SQL a model already knows how to write. It is empty for SQL.
	StatementExample string
	// StatementHint is the shape of one statement in a few cells, which an empty editor
	// draws to say what it takes.
	StatementHint string

	// QuoteIdentifier writes the name so the server reads it back exactly.
	QuoteIdentifier func(name string) string
	// BuildPlaceholder writes a bind placeholder, counted from one.
	BuildPlaceholder func(position int) string
	// CountExpression counts the rows of a read.
	CountExpression string
	// RowLockClause holds the rows a read returns until the transaction ends, so a write
	// that follows the read finds them as they were. A server that locks the whole
	// database for a write leaves it empty.
	RowLockClause string
	// QuoteTextLiteral writes a value as the server would read it, for a person to see.
	QuoteTextLiteral func(text string) string
	// CanCompareType is true if the server can compare this type with `=`.
	CanCompareType func(dataType string) bool
	// IdentityColumn is the column a new table numbers its rows with.
	IdentityColumn string
	// ColumnTypes give the type this server writes for each kind of value, so a table
	// made for a data file is written in the types of its own server.
	ColumnTypes map[core.ColumnKind]string
	// BindLimit is how many placeholders one statement of this server may hold. A server
	// that leaves it unset takes the limit of a sixteen bit count, which is what the
	// PostgreSQL and MySQL protocols hold.
	BindLimit int
	// DropSchema, DropTrigger and DropRoutine write the statement that removes one
	// object. Each takes the dialect, so it can quote the names it writes.
	DropSchema  func(dialect *Dialect, schema string) string
	DropTrigger func(dialect *Dialect, schema, name, table string) string
	DropRoutine func(dialect *Dialect, schema, name, identity string) string
	// NamesWithoutQuotes is true for a name this server reads back as written. A
	// server that leaves it out takes the SQL rule: a plain word that is no keyword.
	NamesWithoutQuotes func(name string) bool
}

// plainIdentifier matches a name a server accepts without quotes.
var plainIdentifier = regexp.MustCompile(`^[a-z_][a-z0-9_$]*$`)

// QuoteIdentifierIfNeeded quotes only a name the server would not read back as written.
func (dialect *Dialect) QuoteIdentifierIfNeeded(name string) string {
	if dialect.NamesWithoutQuotes != nil && dialect.NamesWithoutQuotes(name) {
		return name
	}
	if plainIdentifier.MatchString(name) && !syntax.IsKeyword(name) {
		return name
	}
	return dialect.QuoteIdentifier(name)
}

// defaultBindLimit is how many placeholders a statement may hold on a server that names no
// limit of its own. Both wire protocols count them in a sixteen bit field.
const defaultBindLimit = 65535

// ResolveBindLimit returns how many placeholders one statement of this server may hold.
func (dialect *Dialect) ResolveBindLimit() int {
	if dialect.BindLimit > 0 {
		return dialect.BindLimit
	}
	return defaultBindLimit
}

// BuildColumnType returns the type this server writes for that kind of value.
func (dialect *Dialect) BuildColumnType(kind core.ColumnKind) string {
	if written, known := dialect.ColumnTypes[kind]; known {
		return written
	}
	return dialect.ColumnTypes[core.KindText]
}

// BuildQualifiedName writes a relation with its schema.
func (dialect *Dialect) BuildQualifiedName(target QualifiedName) string {
	return dialect.QuoteIdentifier(target.Schema) + "." + dialect.QuoteIdentifier(target.Name)
}

// BuildDropSchema writes the statement that removes a schema.
func (dialect *Dialect) BuildDropSchema(schema string) string {
	return dialect.DropSchema(dialect, schema)
}

// BuildDropTrigger writes the statement that removes a trigger.
func (dialect *Dialect) BuildDropTrigger(schema, name, table string) string {
	return dialect.DropTrigger(dialect, schema, name, table)
}

// BuildDropRoutine writes the statement that removes a function or a procedure.
func (dialect *Dialect) BuildDropRoutine(schema, name, identity string) string {
	return dialect.DropRoutine(dialect, schema, name, identity)
}

// ReadBaseType returns the type name without its modifier or array suffix.
func ReadBaseType(dataType string) string {
	base := strings.ToLower(strings.TrimSpace(stripModifier(dataType)))
	if before, ok := strings.CutSuffix(base, "[]"); ok {
		return strings.TrimSpace(before)
	}
	return base
}

// typeKinds give the kind of value each base type holds. A type that is in none of these
// holds text, which is what every remaining type is read and written as.
var typeKinds = map[string]core.ColumnKind{
	"smallint": core.KindInteger, "integer": core.KindInteger, "int": core.KindInteger,
	"bigint": core.KindInteger, "int2": core.KindInteger, "int4": core.KindInteger,
	"int8": core.KindInteger, "tinyint": core.KindInteger, "mediumint": core.KindInteger,
	"serial": core.KindInteger, "bigserial": core.KindInteger, "smallserial": core.KindInteger,

	"numeric": core.KindNumber, "decimal": core.KindNumber, "real": core.KindNumber,
	"double": core.KindNumber, "double precision": core.KindNumber,
	"float": core.KindNumber, "float4": core.KindNumber, "float8": core.KindNumber,
	"money": core.KindNumber,

	"boolean": core.KindBoolean, "bool": core.KindBoolean,

	"timestamp": core.KindTimestamp, "timestamptz": core.KindTimestamp,
	"timestamp with time zone":    core.KindTimestamp,
	"timestamp without time zone": core.KindTimestamp,
	"date":                        core.KindTimestamp, "datetime": core.KindTimestamp,
}

// ReadTypeKind returns the kind of value a column of that type holds, so a value read from
// a file can be checked against the column it is mapped to.
func ReadTypeKind(dataType string) core.ColumnKind {
	if kind, known := typeKinds[ReadBaseType(dataType)]; known {
		return kind
	}
	return core.KindText
}

var modifierGroup = regexp.MustCompile(`\(.*\)`)

func stripModifier(dataType string) string {
	return modifierGroup.ReplaceAllString(dataType, "")
}

// BoundValues numbers the placeholders of a statement, so no caller counts them itself.
type BoundValues struct {
	dialect *Dialect
	first   int
	Params  []any
}

// NewBoundValues starts a set of bind values, numbered from firstParamIndex.
func NewBoundValues(dialect *Dialect, firstParamIndex int) *BoundValues {
	return &BoundValues{dialect: dialect, first: firstParamIndex}
}

// Bind adds a value and returns the placeholder the statement writes for it.
func (bound *BoundValues) Bind(value any) string {
	bound.Params = append(bound.Params, value)
	return bound.dialect.BuildPlaceholder(bound.first + len(bound.Params) - 1)
}
