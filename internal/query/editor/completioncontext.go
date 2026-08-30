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

// endsSetList names the words that end the assignment list of an UPDATE.
var endsSetList = map[string]bool{"from": true, "where": true, "returning": true}

// setListPlace says where a statement stands in an UPDATE, one token at a time.
type setListPlace string

const (
	placeOutside  setListPlace = "outside"
	placeInUpdate setListPlace = "in-update"
	placeOnTarget setListPlace = "on-target"
	placeInValue  setListPlace = "in-value"
)

// readNextPlace returns the place after this token. Only a token outside every
// bracket is read.
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

// IsUpdateSetTarget is true where the server refuses a qualified name, so
// `set a.name = …` is rejected.
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

// columnOpeners name the words a column can follow.
var columnOpeners = map[string]bool{
	"select": true, "where": true, "and": true, "or": true, "on": true, "having": true,
	"by": true, "distinct": true, "case": true, "when": true, "then": true, "else": true,
	"returning": true, "using": true, "not": true, "set": true,
}

// relationOpeners name the words a relation can follow.
var relationOpeners = map[string]bool{
	"from": true, "join": true, "into": true, "update": true, "table": true,
}

// columnMarks name the operators a column can follow. A star is left out, because
// after `select *` the statement needs a FROM clause.
var columnMarks = map[string]bool{
	",": true, "(": true, "=": true, "<": true, ">": true, "<=": true, ">=": true,
	"<>": true, "!=": true, "+": true, "-": true, "/": true, "||": true,
}

// ResolveNamePosition returns what the statement expects at the caret. The raw
// tokens are read, because the caret can be in a comment or an unfinished string.
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

	// A token that reaches the caret is being typed now: a word, or the content of a
	// string or a comment.
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
