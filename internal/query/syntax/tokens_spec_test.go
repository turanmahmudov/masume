package syntax_test

import (
	"testing"

	"github.com/turanmahmudov/masume/internal/query/syntax"
)

func TestReadCommandWordReadsTheOpeningWord(t *testing.T) {
	if held := syntax.ReadCommandWord("select * from orders", syntax.FlavourStandard); held != "select" {
		t.Errorf("the command reads %q", held)
	}
	if held := syntax.ReadCommandWord("/* note */ update orders set x = 1", syntax.FlavourStandard); held != "update" {
		t.Errorf("a comment hid the command: %q", held)
	}
}

func TestSelectsIntoTargetFindsASelectInto(t *testing.T) {
	into := syntax.ReadCodeTokens("select * into archive from orders", syntax.FlavourStandard)
	if !syntax.SelectsIntoTarget(into) {
		t.Error("select into was not found")
	}
	plain := syntax.ReadCodeTokens("select * from orders", syntax.FlavourStandard)
	if syntax.SelectsIntoTarget(plain) {
		t.Error("a plain select was read as select into")
	}
}

func TestFindKeywordsInSkipsAKeywordInsideBrackets(t *testing.T) {
	tokens := syntax.ReadCodeTokens(
		"select * from orders where id in (select id from lines) union select 1",
		syntax.FlavourStandard)
	top := syntax.FindKeywordsIn(tokens, []string{"union", "select"})
	if len(top) != 3 {
		t.Fatalf("top-level hits: %+v", top)
	}
	anywhere := syntax.FindKeywordsAnywhere(tokens, []string{"select"})
	if len(anywhere) != 3 {
		t.Fatalf("hits at any depth: %+v", anywhere)
	}
}

func TestSkipBracketGroupWalksABalancedGroup(t *testing.T) {
	tokens := syntax.ReadCodeTokens("select (a + (b)) from t", syntax.FlavourStandard)
	open := -1
	for at, token := range tokens {
		if token.Kind == syntax.TokenOperator && token.Text == "(" {
			open = at
			break
		}
	}
	if open < 0 {
		t.Fatal("no opening bracket")
	}
	after := syntax.SkipBracketGroup(tokens, open)
	if after <= open {
		t.Errorf("the group ended at %d, started at %d", after, open)
	}
	if syntax.SkipBracketGroup(tokens, 0) != 0 {
		t.Error("a token that is not a bracket was skipped")
	}
}

func TestReadIdentifierStripsQuotes(t *testing.T) {
	sql := `select "Order" from t`
	tokens := syntax.ReadCodeTokens(sql, syntax.FlavourStandard)
	name, found := syntax.ReadIdentifier(sql, tokens, 1)
	if !found || name != "Order" {
		t.Errorf("the name reads %q, found=%v", name, found)
	}
}
