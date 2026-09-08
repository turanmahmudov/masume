// Package syntax tokenizes statements and finds keywords without a server connection.
package syntax

import "strings"

// SyntaxFlavour is the lexer variant. MySQL mode supports # comments and backslash escapes
// in ordinary strings. SQL Server mode supports bracket names, N-prefixed strings and
// @variables.
type SyntaxFlavour string

// The three ways a buffer is scanned.
const (
	FlavourStandard  SyntaxFlavour = "standard"
	FlavourMysql     SyntaxFlavour = "mysql"
	FlavourSqlserver SyntaxFlavour = "sqlserver"
)

// TokenKind is the kind of one part of the buffer.
type TokenKind string

// The kinds the scanner reads.
const (
	TokenKeyword    TokenKind = "keyword"
	TokenType       TokenKind = "type"
	TokenString     TokenKind = "string"
	TokenComment    TokenKind = "comment"
	TokenNumber     TokenKind = "number"
	TokenIdentifier TokenKind = "identifier"
	TokenQuoted     TokenKind = "quoted"
	TokenOperator   TokenKind = "operator"
	TokenParameter  TokenKind = "parameter"
)

// Token is one part of the buffer, and its kind. The offsets count bytes.
type Token struct {
	Kind  TokenKind
	Start int
	End   int
	// True for a part that reached the end of the buffer without its closing mark.
	Unterminated bool
}

var keywords = map[string]bool{}
var types = map[string]bool{}

func init() {
	for word := range strings.FieldsSeq(`add all alter analyze and any as asc begin between by
		cascade case cast check column commit constraint create cross default delete desc distinct
		drop else end except exists explain false fetch first for foreign from full group having
		if ilike in index inner insert intersect into is join key left like limit lateral
		materialized natural not null nulls offset on only or order outer over partition primary
		references rename restrict returning right rollback row rows select set table then to true
		truncate union unique update using vacuum values view when where window with`) {
		keywords[word] = true
	}
	for word := range strings.FieldsSeq(`bigint bigserial boolean bytea char character date decimal
		double float int int2 int4 int8 integer interval json jsonb money numeric real serial
		smallint text time timestamp timestamptz uuid varchar xml`) {
		types[word] = true
	}
}

// IsKeyword is true for words in the lexer keyword or type lists.
func IsKeyword(word string) bool {
	lowered := strings.ToLower(word)
	return keywords[lowered] || types[lowered]
}

// operatorCharacters are the single marks the scanner reads as an operator.
const operatorCharacters = `+-*/<>=~!@#%^&|?:,;.()[]`

// isIdentifierQuote accepts double quotes and backticks in both lexer variants.
func isIdentifierQuote(character byte) bool {
	return character == '"' || character == '`'
}

func isWordStart(character byte) bool {
	return character == '_' ||
		(character >= 'a' && character <= 'z') || (character >= 'A' && character <= 'Z')
}

func isWordPart(character byte) bool {
	return isWordStart(character) || character == '$' || isDigit(character)
}

func isDigit(character byte) bool {
	return character >= '0' && character <= '9'
}

func at(sql string, index int) byte {
	if index < 0 || index >= len(sql) {
		return 0
	}
	return sql[index]
}

func readWordEnd(sql string, start int) int {
	index := start
	for index < len(sql) && isWordPart(sql[index]) {
		index++
	}
	return index
}

// run is a token end offset and termination state.
type run struct {
	end        int
	terminated bool
}

// readQuotedEnd scans doubled quotes and optional backslash escapes until the closing quote.
func readQuotedEnd(sql string, start int, quote byte, escapes bool) run {
	index := start + 1
	for index < len(sql) {
		if escapes && sql[index] == '\\' {
			index += 2
			continue
		}
		if sql[index] == quote {
			if at(sql, index+1) == quote {
				index += 2
				continue
			}
			return run{end: index + 1, terminated: true}
		}
		index++
	}
	return run{end: len(sql)}
}

// readBracketEnd scans a `[name]` bracket name. A doubled `]]` is one character of the name.
func readBracketEnd(sql string, start int) run {
	index := start + 1
	for index < len(sql) {
		if sql[index] == ']' {
			if at(sql, index+1) == ']' {
				index += 2
				continue
			}
			return run{end: index + 1, terminated: true}
		}
		index++
	}
	return run{end: len(sql)}
}

// readBlockCommentEnd counts the inner comments, because PostgreSQL nests them.
func readBlockCommentEnd(sql string, start int) run {
	depth := 0
	index := start
	for index < len(sql) {
		if sql[index] == '/' && at(sql, index+1) == '*' {
			depth++
			index += 2
			continue
		}
		if sql[index] == '*' && at(sql, index+1) == '/' {
			depth--
			index += 2
			if depth == 0 {
				return run{end: index, terminated: true}
			}
			continue
		}
		index++
	}
	return run{end: len(sql)}
}

// readNumberEnd reads a number with its fraction and exponent, as in `2.5e-3`.
func readNumberEnd(sql string, start int) int {
	index := start
	for isDigit(at(sql, index)) {
		index++
	}
	if at(sql, index) == '.' && isDigit(at(sql, index+1)) {
		index++
		for isDigit(at(sql, index)) {
			index++
		}
	}
	exponent := at(sql, index)
	if exponent == 'e' || exponent == 'E' {
		signed := at(sql, index+1) == '+' || at(sql, index+1) == '-'
		first := index + 1
		if signed {
			first = index + 2
		}
		if isDigit(at(sql, first)) {
			index = first
			for isDigit(at(sql, index)) {
				index++
			}
		}
	}
	return index
}

// longOperators are tried longest first, so `->>` is never read as `->` and `>`.
var longOperators = []string{
	"!~~*", "!~~", "~~*", "~~", "!~*", "!~", "~*",
	"->>", "#>>", "->", "#>", "#-", "||", "::",
	"<=", ">=", "<>", "!=", "<<", ">>", "@>", "<@", "&&", "|/",
}

func readLongOperator(sql string, start int) string {
	for _, operator := range longOperators {
		if strings.HasPrefix(sql[start:], operator) {
			return operator
		}
	}
	return ""
}

// readDollarQuoteEnd reads a `$tag$ … $tag$` body.
func readDollarQuoteEnd(sql string, start int) (run, bool) {
	index := start + 1
	for index < len(sql) && (isWordStart(sql[index]) || sql[index] == '_') {
		index++
	}
	if at(sql, index) != '$' {
		return run{}, false
	}
	tag := sql[start : index+1]
	closing := strings.Index(sql[index+1:], tag)
	if closing == -1 {
		return run{end: len(sql)}, true
	}
	return run{end: index + 1 + closing + len(tag), terminated: true}, true
}

func buildRunToken(kind TokenKind, start int, found run) Token {
	return Token{Kind: kind, Start: start, End: found.end, Unterminated: !found.terminated}
}

type scanner func(sql string, index int) (Token, bool)

func scanLineComment(sql string, index int) (Token, bool) {
	if sql[index] != '-' || at(sql, index+1) != '-' {
		return Token{}, false
	}
	return Token{Kind: TokenComment, Start: index, End: findLineEnd(sql, index)}, true
}

func scanHashComment(sql string, index int) (Token, bool) {
	if sql[index] != '#' {
		return Token{}, false
	}
	return Token{Kind: TokenComment, Start: index, End: findLineEnd(sql, index)}, true
}

func findLineEnd(sql string, index int) int {
	broke := strings.IndexByte(sql[index:], '\n')
	if broke == -1 {
		return len(sql)
	}
	return index + broke
}

func scanBlockComment(sql string, index int) (Token, bool) {
	if sql[index] != '/' || at(sql, index+1) != '*' {
		return Token{}, false
	}
	return buildRunToken(TokenComment, index, readBlockCommentEnd(sql, index)), true
}

// scanMysqlBlockComment scans MySQL /*! and MariaDB /*M! executable comment bodies as code. Other block comments remain comments.
func scanMysqlBlockComment(sql string, index int) (Token, bool) {
	if sql[index] != '/' || at(sql, index+1) != '*' {
		return Token{}, false
	}
	if at(sql, index+2) == '!' {
		return Token{Kind: TokenOperator, Start: index, End: index + 3}, true
	}
	if held := at(sql, index+2); (held == 'M' || held == 'm') && at(sql, index+3) == '!' {
		return Token{Kind: TokenOperator, Start: index, End: index + 4}, true
	}
	return buildRunToken(TokenComment, index, readBlockCommentEnd(sql, index)), true
}

func scanString(sql string, index int) (Token, bool) {
	if sql[index] != '\'' {
		return Token{}, false
	}
	return buildRunToken(TokenString, index, readQuotedEnd(sql, index, '\'', false)), true
}

// scanEscapingString reads a MySQL string, where `\'` is a quote inside it.
func scanEscapingString(sql string, index int) (Token, bool) {
	if sql[index] != '\'' {
		return Token{}, false
	}
	return buildRunToken(TokenString, index, readQuotedEnd(sql, index, '\'', true)), true
}

// scanEscapedString scans PostgreSQL E-prefixed strings with backslash escapes.
func scanEscapedString(sql string, index int) (Token, bool) {
	character := sql[index]
	if character != 'E' && character != 'e' {
		return Token{}, false
	}
	if at(sql, index+1) != '\'' {
		return Token{}, false
	}
	return buildRunToken(TokenString, index, readQuotedEnd(sql, index+1, '\'', true)), true
}

func scanQuotedName(sql string, index int) (Token, bool) {
	character := sql[index]
	if !isIdentifierQuote(character) {
		return Token{}, false
	}
	return buildRunToken(TokenQuoted, index, readQuotedEnd(sql, index, character, false)), true
}

func scanDollarString(sql string, index int) (Token, bool) {
	if sql[index] != '$' {
		return Token{}, false
	}
	found, isBody := readDollarQuoteEnd(sql, index)
	if !isBody {
		return Token{}, false
	}
	return buildRunToken(TokenString, index, found), true
}

// scanBracketName scans a SQL Server `[name]`.
func scanBracketName(sql string, index int) (Token, bool) {
	if sql[index] != '[' {
		return Token{}, false
	}
	return buildRunToken(TokenQuoted, index, readBracketEnd(sql, index)), true
}

// scanUnicodeString scans a SQL Server `N'text'`.
func scanUnicodeString(sql string, index int) (Token, bool) {
	character := sql[index]
	if character != 'N' && character != 'n' {
		return Token{}, false
	}
	if at(sql, index+1) != '\'' {
		return Token{}, false
	}
	return buildRunToken(TokenString, index, readQuotedEnd(sql, index+1, '\'', false)), true
}

// scanVariable scans a SQL Server `@name` parameter and a `@@name` server variable.
func scanVariable(sql string, index int) (Token, bool) {
	if sql[index] != '@' {
		return Token{}, false
	}
	start := index + 1
	if at(sql, start) == '@' {
		start++
	}
	if !isWordStart(at(sql, start)) {
		return Token{}, false
	}
	return Token{Kind: TokenParameter, Start: index, End: readWordEnd(sql, start)}, true
}

func scanParameter(sql string, index int) (Token, bool) {
	if sql[index] != '$' || !isDigit(at(sql, index+1)) {
		return Token{}, false
	}
	return Token{Kind: TokenParameter, Start: index, End: readWordEnd(sql, index+1)}, true
}

func scanNumber(sql string, index int) (Token, bool) {
	if !isDigit(sql[index]) {
		return Token{}, false
	}
	return Token{Kind: TokenNumber, Start: index, End: readNumberEnd(sql, index)}, true
}

func scanWord(sql string, index int) (Token, bool) {
	if !isWordStart(sql[index]) {
		return Token{}, false
	}
	end := readWordEnd(sql, index)
	word := strings.ToLower(sql[index:end])
	kind := TokenIdentifier
	switch {
	case keywords[word]:
		kind = TokenKeyword
	case types[word]:
		kind = TokenType
	}
	return Token{Kind: kind, Start: index, End: end}, true
}

func scanOperator(sql string, index int) (Token, bool) {
	if long := readLongOperator(sql, index); long != "" {
		return Token{Kind: TokenOperator, Start: index, End: index + len(long)}, true
	}
	if strings.IndexByte(operatorCharacters, sql[index]) == -1 {
		return Token{}, false
	}
	return Token{Kind: TokenOperator, Start: index, End: index + 1}, true
}

// standardScanners is the ordered scanner list. The first match consumes the token.
var standardScanners = []scanner{
	scanLineComment, scanBlockComment, scanString, scanEscapedString, scanQuotedName,
	scanDollarString, scanParameter, scanNumber, scanWord, scanOperator,
}

var mysqlScanners = []scanner{
	scanLineComment, scanHashComment, scanMysqlBlockComment, scanEscapingString, scanQuotedName,
	scanParameter, scanNumber, scanWord, scanOperator,
}

var sqlserverScanners = []scanner{
	scanLineComment, scanBlockComment, scanString, scanUnicodeString, scanQuotedName,
	scanBracketName, scanVariable, scanNumber, scanWord, scanOperator,
}

func resolveScanners(flavour SyntaxFlavour) []scanner {
	switch flavour {
	case FlavourMysql:
		return mysqlScanners
	case FlavourSqlserver:
		return sqlserverScanners
	}
	return standardScanners
}

// Tokenize scans token kinds and byte ranges without parsing statement structure.
func Tokenize(sql string, flavour SyntaxFlavour) []Token {
	scanners := resolveScanners(flavour)
	tokens := make([]Token, 0, len(sql)/4+1)
	index := 0

	for index < len(sql) {
		found := false
		for _, scan := range scanners {
			token, read := scan(sql, index)
			if !read {
				continue
			}
			tokens = append(tokens, token)
			index = token.End
			found = true
			break
		}
		if !found {
			index++
		}
	}
	return tokens
}
