package editor

import (
	"fmt"
	"sort"
	"strings"

	"github.com/turanmahmudov/masume/internal/query/statement"
	"github.com/turanmahmudov/masume/internal/query/syntax"
)

// Diagnostic is a statement error and its buffer range.
type Diagnostic struct {
	Message string
	Start   int
	End     int
}

// SchemaKnowledge is the loaded catalog metadata for a tab.
type SchemaKnowledge struct {
	// False until the catalog is loaded. Unknown table checks require a loaded catalog.
	Loaded       bool
	IsKnownTable func(reference statement.TableReference) bool
	// Columns indexed by lowercase table names and aliases. Unloaded tables are absent.
	ColumnsByQualifier map[string][]string
}

// NothingKnown returns unloaded catalog metadata.
func NothingKnown() SchemaKnowledge {
	return SchemaKnowledge{
		IsKnownTable:       func(statement.TableReference) bool { return true },
		ColumnsByQualifier: map[string][]string{},
	}
}

// runNames is the diagnostic label for each unterminated token kind.
var runNames = map[syntax.TokenKind]string{
	syntax.TokenString:  "string",
	syntax.TokenQuoted:  "quoted identifier",
	syntax.TokenComment: "block comment",
}

func findUnterminatedRuns(sql string, tokens []syntax.Token) []Diagnostic {
	found := []Diagnostic{}
	for _, token := range tokens {
		if !token.Unterminated {
			continue
		}
		name, named := runNames[token.Kind]
		if !named {
			name = "quoted token"
		}
		found = append(found, Diagnostic{
			Message: "unterminated " + name, Start: token.Start, End: len(sql),
		})
	}
	return found
}

func isOperatorToken(sql string, token syntax.Token, text string) bool {
	return token.Kind == syntax.TokenOperator && sql[token.Start:token.End] == text
}

// findUnbalancedBrackets checks parentheses outside strings and comments.
func findUnbalancedBrackets(sql string, tokens []syntax.Token) []Diagnostic {
	found := []Diagnostic{}
	opened := []syntax.Token{}

	for _, token := range tokens {
		if isOperatorToken(sql, token, "(") {
			opened = append(opened, token)
			continue
		}
		if !isOperatorToken(sql, token, ")") {
			continue
		}
		if len(opened) == 0 {
			found = append(found, Diagnostic{
				Message: "unexpected closing parenthesis )", Start: token.Start, End: token.End,
			})
			continue
		}
		opened = opened[:len(opened)-1]
	}

	for _, token := range opened {
		found = append(found, Diagnostic{
			Message: "unclosed parenthesis (", Start: token.Start, End: token.End,
		})
	}
	return found
}

func findUnknownTables(
	references []statement.TableReference, cteNames map[string]bool, knowledge SchemaKnowledge,
) []Diagnostic {
	if !knowledge.Loaded {
		return nil
	}

	found := []Diagnostic{}
	for _, reference := range references {
		// A name the statement defines itself is not in the catalog.
		if !reference.HasSchema && cteNames[strings.ToLower(reference.Name)] {
			continue
		}
		if knowledge.IsKnownTable != nil && knowledge.IsKnownTable(reference) {
			continue
		}
		written := reference.Name
		if reference.HasSchema {
			written = reference.Schema + "." + reference.Name
		}
		found = append(found, Diagnostic{
			Message: "unknown table: " + written, Start: reference.Start, End: reference.End,
		})
	}
	return found
}

func readName(sql string, token syntax.Token) string {
	return syntax.UnquoteIdentifier(sql[token.Start:token.End])
}

func isNameKind(kind syntax.TokenKind) bool {
	return kind == syntax.TokenIdentifier || kind == syntax.TokenQuoted
}

// findUnknownColumns checks qualified names against loaded table columns.
func findUnknownColumns(sql string, tokens []syntax.Token, knowledge SchemaKnowledge) []Diagnostic {
	found := []Diagnostic{}

	for index := 0; index+2 < len(tokens); index++ {
		qualifier := tokens[index]
		if !isNameKind(qualifier.Kind) {
			continue
		}
		if !isOperatorToken(sql, tokens[index+1], ".") {
			continue
		}
		named := tokens[index+2]
		if !isNameKind(named.Kind) {
			continue
		}

		// Skip qualifiers without loaded columns.
		columns, known := knowledge.ColumnsByQualifier[strings.ToLower(readName(sql, qualifier))]
		if !known {
			continue
		}

		column := readName(sql, named)
		lowered := strings.ToLower(column)
		held := false
		for _, candidate := range columns {
			if strings.ToLower(candidate) == lowered {
				held = true
				break
			}
		}
		if held {
			continue
		}

		found = append(found, Diagnostic{
			Message: fmt.Sprintf("unknown column: %s.%s", readName(sql, qualifier), column),
			Start:   named.Start,
			End:     named.End,
		})
		index += 2
	}
	return found
}

// FindLocalDiagnostics checks the buffer without a server request.
func FindLocalDiagnostics(
	sql string, knowledge SchemaKnowledge, flavour syntax.SyntaxFlavour,
) []Diagnostic {
	if strings.TrimSpace(sql) == "" {
		return nil
	}

	tokens := syntax.Tokenize(sql, flavour)
	unterminated := findUnterminatedRuns(sql, tokens)
	// Stop after unterminated tokens.
	if len(unterminated) > 0 {
		return unterminated
	}

	found := findUnbalancedBrackets(sql, tokens)
	found = append(found, findUnknownTables(
		statement.FindTableReferences(sql, flavour), statement.FindCteNames(sql, flavour), knowledge)...)
	found = append(found, findUnknownColumns(sql, tokens, knowledge)...)
	sort.SliceStable(found, func(left, right int) bool {
		return found[left].Start < found[right].Start
	})
	return found
}
