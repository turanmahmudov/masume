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

// Dialect is the SQL generation configuration for an engine family.
type Dialect struct {
	// Servers that share a dialect share the name: CockroachDB and Redshift use `postgres`.
	Engine core.Engine
	// The lexer syntax variant.
	Syntax syntax.SyntaxFlavour
	// The display term for schema or database.
	SchemaWord string
	// StatementLanguage is the server language for display and chat prompts.
	StatementLanguage string
	// FenceTag is how a fenced block of that language is opened in a reply of a model.
	FenceTag string
	// StatementExample is a sample statement for non-SQL languages.
	StatementExample string
	// StatementHint is the empty editor example.
	StatementHint string

	// QuoteIdentifier writes the name so the server reads it back exactly.
	QuoteIdentifier func(name string) string
	// BuildPlaceholder writes a bind placeholder, counted from one.
	BuildPlaceholder func(position int) string
	// CountExpression is the row count expression.
	CountExpression string
	// RowLockClause is the capture query locking clause, or empty for dialects without a row lock clause.
	RowLockClause string
	// QuoteTextLiteral writes a value as the server would read it, for a person to see.
	QuoteTextLiteral func(text string) string
	// CanCompareType is true if the server can compare this type with `=`.
	CanCompareType func(dataType string) bool
	// IdentityColumn is the column a new table numbers its rows with.
	IdentityColumn string
	// ColumnTypes is the server type for each imported value kind.
	ColumnTypes map[core.ColumnKind]string
	// BindLimit is the maximum placeholders per statement. Zero uses the default 16-bit protocol limit.
	BindLimit int
	// DropSchema, DropTrigger, and DropRoutine are dialect-specific DROP builders.
	DropSchema  func(dialect *Dialect, schema string) string
	DropTrigger func(dialect *Dialect, schema, name, table string) string
	DropRoutine func(dialect *Dialect, schema, name, identity string) string
	// NamesWithoutQuotes is an optional additional check for identifiers that need no quotes.
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

// defaultBindLimit is the default placeholder limit, matching the PostgreSQL and MySQL 16-bit protocol fields.
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

// typeKinds maps base types to value kinds. Unlisted types use text.
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

// ReadTypeKind returns the value kind for an imported column type.
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

// BoundValues collects parameters and numbers placeholders.
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
