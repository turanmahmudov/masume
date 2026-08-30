package editor

import (
	"regexp"
	"slices"
	"sort"
	"strings"

	"github.com/turanmahmudov/masume/internal/query"
)

// CompletionKind says what a suggestion names.
type CompletionKind string

// The kinds a suggestion can have.
const (
	CompleteKeyword  CompletionKind = "keyword"
	CompleteSchema   CompletionKind = "schema"
	CompleteTable    CompletionKind = "table"
	CompleteColumn   CompletionKind = "column"
	CompleteFunction CompletionKind = "function"
)

// Completion is one suggestion in the completion list.
type Completion struct {
	Text string
	Kind CompletionKind
	// What the catalog says about the name, shown beside it.
	Detail string
}

// CompletionColumn is a column offered by name, with its type from the catalog.
type CompletionColumn struct {
	Name   string
	Detail string
}

// CompletionSources is everything the catalog offers for completion.
type CompletionSources struct {
	Schemas []string
	Tables  []string
	// Functions and procedures, under both the bare name and the qualified one.
	Functions []string
	// The columns of the result on screen, offered for a bare word.
	Columns []CompletionColumn
	// Keyed in lower case, under the table name and any alias of the statement.
	ColumnsByQualifier map[string][]CompletionColumn
}

// CompletionContext says where the caret is, for the places where SQL limits what
// may follow.
type CompletionContext struct {
	// False where a qualified name is refused, such as the column of an UPDATE.
	AllowQualified bool
	// The kind of name the statement expects where there is no word to complete.
	NamePosition NamePosition
}

var completionKeywords = []string{
	"select", "from", "where", "group by", "having", "order by", "limit", "offset",
	"insert into", "values", "update", "set", "delete from", "join", "left join",
	"inner join", "on", "and", "or", "not", "null", "is null", "is not null",
	"distinct", "as", "asc", "desc", "count", "sum", "avg", "min", "max",
	"coalesce", "case", "when", "then", "else", "end",
}

// maxCompletions is how many suggestions the popup holds.
const maxCompletions = 12

// splitQualifier returns the name before the last dot, and the text typed after it.
func splitQualifier(prefix string) (string, string, bool) {
	dot := strings.LastIndex(prefix, ".")
	if dot <= 0 {
		return "", "", false
	}
	return prefix[:dot], prefix[dot+1:], true
}

var prefixWord = regexp.MustCompile(`[A-Za-z_][A-Za-z0-9_.$]*$`)

// ReadPrefix returns the word being typed, which a completion replaces. It is empty
// where none is.
func ReadPrefix(sql string, offset int) string {
	if offset > len(sql) {
		offset = len(sql)
	}
	return prefixWord.FindString(sql[:offset])
}

// How well a candidate matches what was typed, the best first. A rank of rankNoMatch or
// worse is not offered, which drops the exact match as well: a word already typed in full
// needs no suggestion.
const (
	rankPrefix   = 0
	rankContains = 1
	rankNoMatch  = 2
	rankExact    = 3
)

// rankCandidate returns how well the candidate matches what was typed. Lower comes
// first.
func rankCandidate(lowered, needle string) int {
	if lowered == needle {
		return rankExact
	}
	if strings.HasPrefix(lowered, needle) {
		return rankPrefix
	}
	if strings.Contains(lowered, needle) {
		return rankContains
	}
	return rankNoMatch
}

// kindOrder says what each place in a statement expects first. Without this the
// shortest name wins, so `a` where a column belongs offered `as`, `and` and `asc`
// before any column of the relation.
var kindOrder = map[NamePosition][]CompletionKind{
	PositionColumn: {
		CompleteColumn, CompleteFunction, CompleteTable, CompleteSchema, CompleteKeyword,
	},
	PositionRelation: {
		CompleteTable, CompleteSchema, CompleteFunction, CompleteColumn, CompleteKeyword,
	},
	// Where the statement expects no kind, a bare word is most often the next clause.
	PositionNone: {CompleteKeyword},
}

func rankKind(kind CompletionKind, position NamePosition) int {
	order := kindOrder[position]
	for at, candidate := range order {
		if candidate == kind {
			return at
		}
	}
	return len(order)
}

type rankedCompletion struct {
	completion Completion
	rank       int
}

// collector keeps the ranked candidates, one suggestion per kind and text.
type collector struct {
	needle string
	kept   []rankedCompletion
	seen   map[string]bool
}

func newCollector(needle string) *collector {
	return &collector{needle: needle, seen: map[string]bool{}}
}

func (kept *collector) add(text string, kind CompletionKind, detail string) {
	kept.addAgainst(text, kind, kept.needle, detail)
}

// addAgainst keeps a candidate weighed against other text than the prefix. The text
// after a dot is weighed on its own, and an empty one matches every column.
func (kept *collector) addAgainst(text string, kind CompletionKind, against, detail string) {
	lowered := strings.ToLower(text)
	rank := rankCandidate(lowered, against)
	if rank >= rankNoMatch {
		return
	}

	// Two candidates of one kind with the same text are one suggestion.
	key := string(kind) + ":" + lowered
	if kept.seen[key] {
		return
	}
	kept.seen[key] = true
	kept.kept = append(kept.kept, rankedCompletion{
		completion: Completion{Text: text, Kind: kind, Detail: detail}, rank: rank,
	})
}

func (kept *collector) take(limit int) []Completion {
	if limit > len(kept.kept) {
		limit = len(kept.kept)
	}
	taken := make([]Completion, 0, limit)
	for _, entry := range kept.kept[:limit] {
		taken = append(taken, entry.completion)
	}
	return taken
}

// sortedQualifiers returns the qualifiers in a stable order, so two runs with the
// same catalog offer the same list.
func sortedQualifiers(byQualifier map[string][]CompletionColumn) []string {
	names := make([]string, 0, len(byQualifier))
	for name := range byQualifier {
		names = append(names, name)
	}
	slices.Sort(names)
	return names
}

// buildPositionCompletions returns what the place expects, in the order of the
// catalog, because there is no prefix to rank by.
func buildPositionCompletions(sources CompletionSources, position NamePosition) []Completion {
	kept := newCollector("")

	if position == PositionColumn {
		for _, qualifier := range sortedQualifiers(sources.ColumnsByQualifier) {
			for _, column := range sources.ColumnsByQualifier[qualifier] {
				kept.add(column.Name, CompleteColumn, column.Detail)
			}
		}
		for _, column := range sources.Columns {
			kept.add(column.Name, CompleteColumn, column.Detail)
		}
		// A routine call can go where a column can, but a column is more likely.
		for _, name := range sources.Functions {
			kept.add(name, CompleteFunction, "")
		}
	}

	if position == PositionRelation {
		for _, table := range sources.Tables {
			kept.add(table, CompleteTable, "")
		}
		for _, schema := range sources.Schemas {
			kept.add(schema, CompleteSchema, "")
		}
	}
	return kept.take(maxCompletions)
}

func collectColumnCandidates(
	prefix string, sources CompletionSources, context CompletionContext, kept *collector,
) {
	qualifier, partial, qualified := splitQualifier(prefix)

	if !qualified {
		// A bare word can be a column of any relation the statement reads. The
		// columns come from the catalog, so they are offered before the statement runs.
		for _, name := range sortedQualifiers(sources.ColumnsByQualifier) {
			for _, column := range sources.ColumnsByQualifier[name] {
				kept.add(column.Name, CompleteColumn, column.Detail)
			}
		}
		return
	}

	columns := sources.ColumnsByQualifier[strings.ToLower(qualifier)]

	// A name before a dot names the relation of the column, so the whole name is
	// offered and replaces what was typed.
	if context.AllowQualified {
		for _, column := range columns {
			kept.add(qualifier+"."+column.Name, CompleteColumn, column.Detail)
		}
		return
	}

	// A qualifier is refused here, so the bare column is offered for the text after
	// the dot, and it replaces the qualifier too.
	lowered := strings.ToLower(partial)
	for _, column := range columns {
		kept.addAgainst(column.Name, CompleteColumn, lowered, column.Detail)
	}
}

// BuildCompletions returns what could be typed next. The match comes first, then
// the kind the statement expects. On a tie the shorter name wins, so "customer"
// comes before "customer_id" for "cust".
func BuildCompletions(
	prefix string, sources CompletionSources, context CompletionContext,
) []Completion {
	position := context.NamePosition
	if position == "" {
		position = PositionNone
	}
	if len(prefix) < 1 {
		return buildPositionCompletions(sources, position)
	}

	kept := newCollector(strings.ToLower(prefix))
	collectColumnCandidates(prefix, sources, context, kept)

	for _, column := range sources.Columns {
		kept.add(column.Name, CompleteColumn, column.Detail)
	}
	for _, text := range sources.Tables {
		kept.add(text, CompleteTable, "")
	}
	for _, text := range sources.Functions {
		kept.add(text, CompleteFunction, "")
	}
	for _, text := range sources.Schemas {
		kept.add(text, CompleteSchema, "")
	}
	for _, text := range completionKeywords {
		kept.add(text, CompleteKeyword, "")
	}

	ranked := kept.kept
	sort.SliceStable(ranked, func(left, right int) bool {
		if ranked[left].rank != ranked[right].rank {
			return ranked[left].rank < ranked[right].rank
		}
		leftKind := rankKind(ranked[left].completion.Kind, position)
		rightKind := rankKind(ranked[right].completion.Kind, position)
		if leftKind != rightKind {
			return leftKind < rightKind
		}
		return len(ranked[left].completion.Text) < len(ranked[right].completion.Text)
	})
	kept.kept = ranked
	return kept.take(maxCompletions)
}

// buildInsertText writes the chosen candidate. A name the server would change case
// on is quoted, part by part.
func buildInsertText(completion Completion, dialect *query.Dialect) string {
	if completion.Kind == CompleteKeyword {
		return completion.Text
	}
	parts := strings.Split(completion.Text, ".")
	quoted := make([]string, 0, len(parts))
	for _, part := range parts {
		quoted = append(quoted, dialect.QuoteIdentifierIfNeeded(part))
	}
	name := strings.Join(quoted, ".")
	// A routine is called with brackets, and the caret goes between them.
	if completion.Kind == CompleteFunction {
		return name + "()"
	}
	return name
}

// ApplyCompletion replaces the word under the caret with the chosen candidate.
func ApplyCompletion(
	sql string, offset int, completion Completion, dialect *query.Dialect,
) (string, int) {
	written := buildInsertText(completion, dialect)
	prefix := ReadPrefix(sql, offset)
	start := offset - len(prefix)
	// The caret goes inside the brackets of a routine, and after anything else.
	rest := 0
	if completion.Kind == CompleteFunction {
		rest = 1
	}
	return sql[:start] + written + sql[offset:], start + len(written) - rest
}
