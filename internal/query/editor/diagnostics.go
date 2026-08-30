package editor

import (
	"fmt"
	"sort"
	"strings"

	"github.com/turanmahmudov/masume/internal/query/statement"
	"github.com/turanmahmudov/masume/internal/query/syntax"
)

// Diagnostic is a fault in the statement, and its place in the buffer.
type Diagnostic struct {
	Message string
	Start   int
	End     int
}

// SchemaKnowledge is what a tab knows of the catalog.
type SchemaKnowledge struct {
	// False until the catalog is read. Nothing is reported against an empty catalog.
	Loaded       bool
	IsKnownTable func(reference statement.TableReference) bool
	// Keyed in lower case, under the table name and any alias. A relation not yet
	// read is missing.
	ColumnsByQualifier map[string][]string
}

// NothingKnown reports nothing, because the catalog has not been read.
func NothingKnown() SchemaKnowledge {
	return SchemaKnowledge{
		IsKnownTable:       func(statement.TableReference) bool { return true },
		ColumnsByQualifier: map[string][]string{},
	}
}

// runNames name each kind of unclosed token, for its message.
var runNames = map[syntax.TokenKind]string{
	syntax.TokenString:  "string",
	syntax.TokenQuoted:  "quoted name",
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
			name = "quoted run"
		}
		found = append(found, Diagnostic{
			Message: "this " + name + " is never closed", Start: token.Start, End: len(sql),
		})
	}
	return found
}

func isOperatorToken(sql string, token syntax.Token, text string) bool {
	return token.Kind == syntax.TokenOperator && sql[token.Start:token.End] == text
}

// findUnbalancedBrackets counts only a bracket the scanner read as an operator, so
// one inside a string is content.
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
				Message: "no ( is open for this )", Start: token.Start, End: token.End,
			})
			continue
		}
		opened = opened[:len(opened)-1]
	}

	for _, token := range opened {
		found = append(found, Diagnostic{
			Message: "this ( is never closed", Start: token.Start, End: token.End,
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
			Message: "no table called " + written, Start: reference.Start, End: reference.End,
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

// findUnknownColumns checks only a qualified name, because a bare word can be an
// alias, a function or a subquery column.
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

		// A schema before a table also arrives here, but the map never holds it, so
		// it is skipped.
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
			Message: fmt.Sprintf("%s has no column called %s", readName(sql, qualifier), column),
			Start:   named.Start,
			End:     named.End,
		})
		index += 2
	}
	return found
}

// FindLocalDiagnostics returns the faults one scan of the buffer finds, without the
// server.
func FindLocalDiagnostics(
	sql string, knowledge SchemaKnowledge, flavour syntax.SyntaxFlavour,
) []Diagnostic {
	if strings.TrimSpace(sql) == "" {
		return nil
	}

	tokens := syntax.Tokenize(sql, flavour)
	unterminated := findUnterminatedRuns(sql, tokens)
	// An unclosed token takes the rest of the buffer, so the brackets and names
	// after it would hide the one fault that matters.
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
