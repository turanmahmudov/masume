package syntax

import "testing"

// readKinds answers the kind of each token, and the text it covers, so a case reads as the
// scanner saw the buffer.
func readKinds(sql string, flavour SyntaxFlavour) []string {
	written := []string{}
	for _, token := range Tokenize(sql, flavour) {
		written = append(written, string(token.Kind)+":"+sql[token.Start:token.End])
	}
	return written
}

func equalWritten(held, wanted []string) bool {
	if len(held) != len(wanted) {
		return false
	}
	for at := range held {
		if held[at] != wanted[at] {
			return false
		}
	}
	return true
}

// Everything above the scanner trusts these kinds: the colours of the editor, where a
// statement ends, and whether a statement writes. A word read as the wrong kind reaches all
// three.
func TestTokenizeReadsTheKindsOfAStatement(t *testing.T) {
	for _, held := range []struct {
		name    string
		sql     string
		flavour SyntaxFlavour
		want    []string
	}{
		{"a keyword and a name", "select id", FlavourStandard,
			[]string{"keyword:select", "identifier:id"}},
		{"a type is its own kind", "cast(id as bigint)", FlavourStandard,
			[]string{"keyword:cast", "operator:(", "identifier:id", "keyword:as",
				"type:bigint", "operator:)"}},
		{"a string", "select 'ada'", FlavourStandard,
			[]string{"keyword:select", "string:'ada'"}},
		{"a quoted name", `select "order"`, FlavourStandard,
			[]string{"keyword:select", `quoted:"order"`}},
		{"a number", "select 42", FlavourStandard,
			[]string{"keyword:select", "number:42"}},
		{"a line comment", "select 1 -- why", FlavourStandard,
			[]string{"keyword:select", "number:1", "comment:-- why"}},
		{"a block comment", "select /* why */ 1", FlavourStandard,
			[]string{"keyword:select", "comment:/* why */", "number:1"}},
		// A bind placeholder of the driver is its own kind. A `:name` mark is not: it is
		// found by the statement layer, which asks the user for its value before a run.
		{"a bind placeholder", "select $1", FlavourStandard,
			[]string{"keyword:select", "parameter:$1"}},
		{"a named mark is not a token of its own", "select :name", FlavourStandard,
			[]string{"keyword:select", "operator::", "identifier:name"}},

		// A quoted name holds what SQL would otherwise read as a keyword, and the scanner
		// must not read it as one.
		{"a keyword inside a quoted name", `select "select"`, FlavourStandard,
			[]string{"keyword:select", `quoted:"select"`}},
		// A keyword inside a string is text.
		{"a keyword inside a string", "select 'delete'", FlavourStandard,
			[]string{"keyword:select", "string:'delete'"}},
	} {
		t.Run(held.name, func(t *testing.T) {
			answered := readKinds(held.sql, held.flavour)
			if !equalWritten(answered, held.want) {
				t.Errorf("%q reads as\n  %v\nwanted\n  %v", held.sql, answered, held.want)
			}
		})
	}
}

// MySQL opens a line comment with `#`, which standard SQL does not.
func TestTokenizeReadsAHashCommentOnlyForMysql(t *testing.T) {
	const sql = "select 1 # why"

	held := readKinds(sql, FlavourMysql)
	want := []string{"keyword:select", "number:1", "comment:# why"}
	if !equalWritten(held, want) {
		t.Errorf("mysql reads %v, wanted %v", held, want)
	}

	// Standard SQL has no such comment, so the same text is not one.
	for _, token := range Tokenize(sql, FlavourStandard) {
		if token.Kind == TokenComment {
			t.Errorf("standard SQL read %q as a comment", sql[token.Start:token.End])
		}
	}
}

// MySQL reads a backslash inside a string as an escape, and standard SQL does not. A string
// that ends in the wrong place would let what follows be read as SQL.
func TestTokenizeReadsABackslashInAStringByFlavour(t *testing.T) {
	const sql = `select 'a\' , 1`

	mysql := Tokenize(sql, FlavourMysql)
	if len(mysql) == 0 {
		t.Fatal("mysql read no tokens")
	}
	// The escaped quote does not close the string, so it runs to the end of the buffer.
	last := mysql[len(mysql)-1]
	if last.Kind != TokenString || !last.Unterminated {
		t.Errorf("mysql read the string as %q terminated=%v, wanted an unterminated string",
			last.Kind, !last.Unterminated)
	}

	// Standard SQL closes the string at the second quote, so what follows is SQL again.
	standard := readKinds(sql, FlavourStandard)
	want := []string{"keyword:select", `string:'a\'`, "operator:,", "number:1"}
	if !equalWritten(standard, want) {
		t.Errorf("standard SQL reads %v, wanted %v", standard, want)
	}
}

// A part that never closes is reported rather than dropped, so the editor can mark it and the
// splitter does not treat the rest of the buffer as SQL.
func TestTokenizeReportsAPartThatNeverCloses(t *testing.T) {
	for _, held := range []struct {
		name string
		sql  string
		kind TokenKind
	}{
		{"a string", "select 'ada", TokenString},
		{"a quoted name", `select "order`, TokenQuoted},
		{"a block comment", "select /* why", TokenComment},
	} {
		t.Run(held.name, func(t *testing.T) {
			tokens := Tokenize(held.sql, FlavourStandard)
			if len(tokens) == 0 {
				t.Fatal("the scanner read no tokens")
			}
			last := tokens[len(tokens)-1]
			if last.Kind != held.kind {
				t.Fatalf("the last token is %q, wanted %q", last.Kind, held.kind)
			}
			if !last.Unterminated {
				t.Error("the part never closes and was not reported as unterminated")
			}
			if last.End != len(held.sql) {
				t.Errorf("the part ends at %d, wanted the end of the buffer at %d",
					last.End, len(held.sql))
			}
		})
	}
}

// A line comment needs no closing mark, so it is never unterminated.
func TestTokenizeNeverReportsALineCommentAsUnterminated(t *testing.T) {
	for _, token := range Tokenize("select 1 -- why", FlavourStandard) {
		if token.Kind == TokenComment && token.Unterminated {
			t.Error("a line comment was reported as unterminated")
		}
	}
}

// The offsets cover the buffer once, in order, with nothing counted twice. The editor draws
// from them, so a gap or an overlap would drop or double a character on screen.
func TestTokenizeAnswersOffsetsInOrder(t *testing.T) {
	for _, sql := range []string{
		"select id, name from orders where id = $1 -- why",
		`insert into "order" values ('a', 1, null)`,
		"  select /* a */ 1 ;  ",
		"",
		"   ",
	} {
		at := 0
		for _, token := range Tokenize(sql, FlavourStandard) {
			if token.Start < at {
				t.Errorf("%q: a token starts at %d, behind %d", sql, token.Start, at)
			}
			if token.End < token.Start || token.End > len(sql) {
				t.Errorf("%q: a token covers %d to %d", sql, token.Start, token.End)
			}
			at = token.End
		}
	}
}

func TestIsKeywordReadsTheWordsAStatementOpensWith(t *testing.T) {
	for _, word := range []string{"select", "insert", "update", "delete", "from", "where"} {
		if !IsKeyword(word) {
			t.Errorf("%q does not read as a keyword", word)
		}
	}
	for _, word := range []string{"", "orders", "customer_id", "masume"} {
		if IsKeyword(word) {
			t.Errorf("%q reads as a keyword", word)
		}
	}
}

// ReadCommandWord names what a statement does, and skips the comments before it.
func TestReadCommandWordSkipsWhatComesBeforeTheStatement(t *testing.T) {
	for _, held := range []struct {
		sql  string
		want string
	}{
		{"select 1", "select"},
		{"  select 1", "select"},
		{"-- a note\nselect 1", "select"},
		{"/* a note */ delete from orders", "delete"},
		{"", ""},
		{"-- nothing but a note", ""},
	} {
		if answered := ReadCommandWord(held.sql, FlavourStandard); answered != held.want {
			t.Errorf("%q opens with %q, wanted %q", held.sql, answered, held.want)
		}
	}
}

func TestTokenizeReadsADollarQuotedBody(t *testing.T) {
	// PostgreSQL writes a routine body between `$tag$` marks, and everything between
	// them is text however it is written.
	for _, held := range []struct {
		name string
		sql  string
		want string
	}{
		{"no tag", "select $$body$$", "$$body$$"},
		{"a tag", "select $tag$body$tag$", "$tag$body$tag$"},
		{"a tag with an underscore", "select $a_b$body$a_b$", "$a_b$body$a_b$"},
		{"a quote inside the body", "select $$it's$$", "$$it's$$"},
		{"a semicolon inside the body", "select $$a; b$$", "$$a; b$$"},
		{"a shorter tag inside the body", "select $tag$a $ b$tag$", "$tag$a $ b$tag$"},
	} {
		t.Run(held.name, func(t *testing.T) {
			got := ""
			for _, token := range Tokenize(held.sql, FlavourStandard) {
				if token.Kind == TokenString {
					got = held.sql[token.Start:token.End]
					break
				}
			}
			if got != held.want {
				t.Errorf("got %q, want %q", got, held.want)
			}
		})
	}
}

func TestTokenizeMarksADollarQuoteThatNeverCloses(t *testing.T) {
	const sql = "select $tag$ never closed"
	for _, token := range Tokenize(sql, FlavourStandard) {
		if token.Kind == TokenString {
			if !token.Unterminated {
				t.Error("the unclosed body was not marked")
			}
			if token.End != len(sql) {
				t.Errorf("the body ends at %d, want the end of the buffer", token.End)
			}
			return
		}
	}
	t.Error("no text was read")
}

func TestTokenizeReadsADollarThatOpensNoBodyAsAnOperator(t *testing.T) {
	// `a$b` is a plain name, and a lone `$` is not the start of a body.
	for _, sql := range []string{"select a$b", "select 1 $ 2"} {
		for _, token := range Tokenize(sql, FlavourStandard) {
			if token.Kind == TokenString {
				t.Errorf("%q was read as text in %q", sql[token.Start:token.End], sql)
			}
		}
	}
}

func TestTokenizeReadsANumberWithItsFractionAndExponent(t *testing.T) {
	for _, held := range []struct {
		name string
		sql  string
		want string
	}{
		{"a whole number", "select 12", "12"},
		{"a fraction", "select 2.5", "2.5"},
		{"an exponent", "select 2e3", "2e3"},
		{"a capital exponent", "select 2E3", "2E3"},
		{"a signed exponent", "select 2.5e-3", "2.5e-3"},
		{"a positive exponent", "select 2.5e+3", "2.5e+3"},
		{"a dot with no digit after it is not a fraction", "select 2.a", "2"},
		{"an exponent with no digit is not an exponent", "select 2ex", "2"},
		{"a signed exponent with no digit", "select 2e-x", "2"},
	} {
		t.Run(held.name, func(t *testing.T) {
			got := ""
			for _, token := range Tokenize(held.sql, FlavourStandard) {
				if token.Kind == TokenNumber {
					got = held.sql[token.Start:token.End]
					break
				}
			}
			if got != held.want {
				t.Errorf("got %q, want %q", got, held.want)
			}
		})
	}
}

func TestTokenizeReadsAnEscapedStringOnlyWhereAQuoteFollowsTheE(t *testing.T) {
	// `E'…'` is the one string where the server reads a backslash, so the closing quote
	// has to survive a `\'` inside it.
	for _, held := range []struct {
		name string
		sql  string
		want string
	}{
		{"a capital E", `select E'a\'b'`, `E'a\'b'`},
		{"a small e", `select e'a\'b'`, `e'a\'b'`},
		{"a newline escape", `select E'\n'`, `E'\n'`},
		{"an empty escaped string", `select E''`, `E''`},
	} {
		t.Run(held.name, func(t *testing.T) {
			got := ""
			for _, token := range Tokenize(held.sql, FlavourStandard) {
				if token.Kind == TokenString {
					got = held.sql[token.Start:token.End]
					break
				}
			}
			if got != held.want {
				t.Errorf("got %q, want %q", got, held.want)
			}
		})
	}
}

func TestTokenizeReadsAnEAsANameWhereNoQuoteFollowsIt(t *testing.T) {
	for _, sql := range []string{"select E", "select e from t", "select employees"} {
		for _, token := range Tokenize(sql, FlavourStandard) {
			if token.Kind == TokenString {
				t.Errorf("%q was read as text in %q", sql[token.Start:token.End], sql)
			}
		}
	}
}

func TestUnquoteIdentifierTakesTheMarksOffAndUndoublesTheOnesInside(t *testing.T) {
	for _, held := range []struct {
		name  string
		given string
		want  string
	}{
		{"a double-quoted name", `"Odd Name"`, "Odd Name"},
		{"a backtick name", "`Odd Name`", "Odd Name"},
		{"a doubled mark inside", `"with""quote"`, `with"quote`},
		{"a doubled tick inside", "`with``tick`", "with`tick"},
		{"a bare name", "plain", "plain"},
		{"nothing", "", ""},
		{"an opening mark with no closing one", `"Odd Name`, "Odd Name"},
		{"the mark alone", `"`, ""},
		{"a name in the other server's mark", "`a`", "a"},
	} {
		t.Run(held.name, func(t *testing.T) {
			if got := UnquoteIdentifier(held.given); got != held.want {
				t.Errorf("UnquoteIdentifier(%q) = %q, want %q", held.given, got, held.want)
			}
		})
	}
}
