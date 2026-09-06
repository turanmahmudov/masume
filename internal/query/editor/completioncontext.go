package editor

import (
	"strings"

	"github.com/turanmahmudov/masume/internal/query/syntax"
)

// NamePosition is the kind of name that can follow a caret with no word under it.
type NamePosition string

// The three places a caret can stand in.
const (
	PositionColumn   NamePosition = "column"
	PositionRelation NamePosition = "relation"
	PositionNone     NamePosition = "none"
)

// endsSetList is the set of keywords that end an UPDATE assignment list.
var endsSetList = map[string]bool{"from": true, "where": true, "returning": true}

// setListPlace is the parser position within an UPDATE assignment list.
type setListPlace string

const (
	placeOutside  setListPlace = "outside"
	placeInUpdate setListPlace = "in-update"
	placeOnTarget setListPlace = "on-target"
	placeInValue  setListPlace = "in-value"
)

// readNextPlace advances the assignment state for a top-level token.
func readNextPlace(
	place setListPlace, tokens []syntax.CodeToken, index int, token syntax.CodeToken,
) setListPlace {
	if syntax.IsOperator(tokens, index, ";") {
		return placeOutside
	}

	if place == placeOnTarget || place == placeInValue {
		// The assigned name ends at the equals sign. What follows is an expression.
		if syntax.IsOperator(tokens, index, "=") {
			return placeInValue
		}
		if syntax.IsOperator(tokens, index, ",") {
			return placeOnTarget
		}
		if endsSetList[token.Text] {
			return placeInUpdate
		}
		return place
	}

	if token.Text == "update" {
		return placeInUpdate
	}
	if place == placeInUpdate && token.Text == "set" {
		return placeOnTarget
	}
	return place
}

// IsUpdateSetTarget detects an UPDATE assignment target position.
func IsUpdateSetTarget(sql string, offset int) bool {
	depth := 0
	place := placeOutside
	tokens := syntax.ReadCodeTokens(sql, syntax.FlavourStandard)

	for index, token := range tokens {
		if token.Start >= offset {
			break
		}
		if syntax.IsOperator(tokens, index, "(") {
			depth++
			continue
		}
		if syntax.IsOperator(tokens, index, ")") {
			if depth > 0 {
				depth--
			}
			continue
		}
		if depth > 0 {
			continue
		}
		place = readNextPlace(place, tokens, index, token)
	}
	return place == placeOnTarget
}

// columnOpeners is the set of keywords followed by column suggestions.
var columnOpeners = map[string]bool{
	"select": true, "where": true, "and": true, "or": true, "on": true, "having": true,
	"by": true, "distinct": true, "case": true, "when": true, "then": true, "else": true,
	"returning": true, "using": true, "not": true, "set": true,
}

// relationOpeners is the set of keywords followed by table suggestions.
var relationOpeners = map[string]bool{
	"from": true, "join": true, "into": true, "update": true, "table": true,
}

// columnMarks is the set of operators followed by column suggestions. The wildcard * is excluded.
var columnMarks = map[string]bool{
	",": true, "(": true, "=": true, "<": true, ">": true, "<=": true, ">=": true,
	"<>": true, "!=": true, "+": true, "-": true, "/": true, "||": true,
}

// ResolveNamePosition determines the suggestion category from tokens before the caret.
func ResolveNamePosition(sql string, offset int) NamePosition {
	if offset > len(sql) {
		offset = len(sql)
	}
	head := sql[:offset]
	tokens := syntax.Tokenize(head, syntax.FlavourStandard)
	if len(tokens) == 0 {
		return PositionNone
	}
	last := tokens[len(tokens)-1]

	// Exclude positions inside a word, string, or comment.
	if last.End >= offset && last.Kind != syntax.TokenOperator {
		return PositionNone
	}

	text := strings.ToLower(head[last.Start:last.End])
	if syntax.IsWordKind(last.Kind) {
		if relationOpeners[text] {
			return PositionRelation
		}
		if columnOpeners[text] {
			return PositionColumn
		}
		return PositionNone
	}
	if last.Kind == syntax.TokenOperator && columnMarks[text] {
		return PositionColumn
	}
	return PositionNone
}
