package syntax

import "strings"

// CodeToken is a token with meaning, with its text in lower case for the comparison.
type CodeToken struct {
	Kind  TokenKind
	Text  string
	Start int
	End   int
}

// IsWordKind is true for a kind a bare word can have. A keyword is always one of these.
func IsWordKind(kind TokenKind) bool {
	return kind == TokenKeyword || kind == TokenType || kind == TokenIdentifier
}

// ReadCodeTokens removes comments and lowercases token text, preserving token kinds.
func ReadCodeTokens(sql string, flavour SyntaxFlavour) []CodeToken {
	scanned := Tokenize(sql, flavour)
	tokens := make([]CodeToken, 0, len(scanned))
	for _, token := range scanned {
		if token.Kind == TokenComment {
			continue
		}
		tokens = append(tokens, CodeToken{
			Kind:  token.Kind,
			Text:  strings.ToLower(sql[token.Start:token.End]),
			Start: token.Start,
			End:   token.End,
		})
	}
	return tokens
}

// TokenAt returns the token at that index, and whether there is one.
func TokenAt(tokens []CodeToken, index int) (CodeToken, bool) {
	if index < 0 || index >= len(tokens) {
		return CodeToken{}, false
	}
	return tokens[index], true
}

// IsOperator is true where the token at that index is that operator.
func IsOperator(tokens []CodeToken, index int, text string) bool {
	token, present := TokenAt(tokens, index)
	return present && token.Kind == TokenOperator && token.Text == text
}

// IsOperatorAnywhere is true where that operator stands anywhere in the statement.
func IsOperatorAnywhere(tokens []CodeToken, text string) bool {
	for _, token := range tokens {
		if token.Kind == TokenOperator && token.Text == text {
			return true
		}
	}
	return false
}

// isWordKeyword detects keywords containing letters.
func isWordKeyword(keyword string) bool {
	return strings.ContainsAny(keyword, "abcdefghijklmnopqrstuvwxyz")
}

// matchesKeywordAt matches an operator or consecutive keyword tokens.
func matchesKeywordAt(tokens []CodeToken, index int, keyword string) bool {
	if !isWordKeyword(keyword) {
		return IsOperator(tokens, index, keyword)
	}
	for offset, word := range strings.Split(keyword, " ") {
		token, present := TokenAt(tokens, index+offset)
		if !present || !IsWordKind(token.Kind) || token.Text != word {
			return false
		}
	}
	return true
}

// CountKeywordTokens returns how many tokens the keyword covers.
func CountKeywordTokens(keyword string) int {
	if !isWordKeyword(keyword) {
		return 1
	}
	return len(strings.Split(keyword, " "))
}

// KeywordHit is a keyword found in a statement, and its place.
type KeywordHit struct {
	Keyword string
	Start   int
	// The place of the match in the token stream, to read what follows.
	Index int
}

func collectKeywordHits(tokens []CodeToken, keywords []string, anyDepth bool) []KeywordHit {
	hits := []KeywordHit{}
	depth := 0
	index := 0

	for index < len(tokens) {
		token := tokens[index]

		if IsOperator(tokens, index, "(") {
			depth++
			index++
			continue
		}
		if IsOperator(tokens, index, ")") {
			if depth > 0 {
				depth--
			}
			index++
			continue
		}

		if depth == 0 || anyDepth {
			matched := ""
			for _, keyword := range keywords {
				if matchesKeywordAt(tokens, index, keyword) {
					matched = keyword
					break
				}
			}
			if matched != "" {
				hits = append(hits, KeywordHit{Keyword: matched, Start: token.Start, Index: index})
				index += CountKeywordTokens(matched)
				continue
			}
		}
		index++
	}
	return hits
}

// FindKeywordsIn returns where the keywords appear outside every bracket.
func FindKeywordsIn(tokens []CodeToken, keywords []string) []KeywordHit {
	return collectKeywordHits(tokens, keywords, false)
}

// FindKeywordsAnywhere finds keywords at any parenthesis depth.
func FindKeywordsAnywhere(tokens []CodeToken, keywords []string) []KeywordHit {
	return collectKeywordHits(tokens, keywords, true)
}

// HoldsCode is true when the input has a non-comment token.
func HoldsCode(sql string, flavour SyntaxFlavour) bool {
	for _, token := range Tokenize(sql, flavour) {
		if token.Kind != TokenComment {
			return true
		}
	}
	return false
}

// FindTopLevelKeywords finds keywords outside parentheses.
func FindTopLevelKeywords(sql string, keywords []string, flavour SyntaxFlavour) []KeywordHit {
	return FindKeywordsIn(ReadCodeTokens(sql, flavour), keywords)
}

// ReadOpeningWord returns the first token text when the token is a word.
func ReadOpeningWord(tokens []CodeToken) string {
	first, present := TokenAt(tokens, 0)
	if !present || !IsWordKind(first.Kind) {
		return ""
	}
	return first.Text
}

// UnquoteIdentifier removes the quotes of a name, whichever mark the server uses.
func UnquoteIdentifier(name string) string {
	if name == "" {
		return name
	}
	quote := name[0]
	if quote != '"' && quote != '`' {
		return name
	}
	inner := name[1:]
	if strings.HasSuffix(inner, string(quote)) && len(inner) > 0 {
		inner = inner[:len(inner)-1]
	}
	return strings.ReplaceAll(inner, string(quote)+string(quote), string(quote))
}

// ReadCommandWord returns the opening statement keyword.
func ReadCommandWord(sql string, flavour SyntaxFlavour) string {
	return ReadOpeningWord(ReadCodeTokens(sql, flavour))
}

// SelectsIntoTarget detects a top-level INTO in a SELECT statement.
func SelectsIntoTarget(tokens []CodeToken) bool {
	if ReadOpeningWord(tokens) != "select" {
		return false
	}
	return len(FindKeywordsIn(tokens, []string{"into"})) > 0
}

// ReadIdentifier returns the name of a token, without the quotes.
func ReadIdentifier(sql string, tokens []CodeToken, index int) (string, bool) {
	token, present := TokenAt(tokens, index)
	if !present {
		return "", false
	}
	raw := sql[token.Start:token.End]
	if token.Kind == TokenQuoted {
		return UnquoteIdentifier(raw), true
	}
	if IsWordKind(token.Kind) {
		return raw, true
	}
	return "", false
}

// IsNameToken is true for identifier and quoted identifier tokens.
func IsNameToken(tokens []CodeToken, index int) bool {
	token, present := TokenAt(tokens, index)
	return present && (token.Kind == TokenIdentifier || token.Kind == TokenQuoted)
}

// SkipBracketGroup skips a parenthesis group or leaves the index unchanged when no group starts there.
func SkipBracketGroup(tokens []CodeToken, index int) int {
	if !IsOperator(tokens, index, "(") {
		return index
	}
	depth := 0
	cursor := index
	for cursor < len(tokens) {
		if IsOperator(tokens, cursor, "(") {
			depth++
		} else if IsOperator(tokens, cursor, ")") {
			depth--
			if depth == 0 {
				return cursor + 1
			}
		}
		cursor++
	}
	return cursor
}
