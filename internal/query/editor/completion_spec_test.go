package editor_test

import (
	"strings"
	"testing"

	"github.com/turanmahmudov/masume/internal/db/postgres"
	"github.com/turanmahmudov/masume/internal/query/editor"
)

// The list stands over the statement while the user types. It offers what the caret can
// take there, so an offer the statement cannot hold is an offer that writes a broken
// statement.

var completionSources = editor.CompletionSources{
	Schemas:   []string{"public", "other"},
	Tables:    []string{"users", "user_roles", "Orders"},
	Functions: []string{"now", "public.now"},
	Columns: []editor.CompletionColumn{
		{Name: "id", Detail: "int4"},
		{Name: "name", Detail: "text"},
	},
	ColumnsByQualifier: map[string][]editor.CompletionColumn{
		"u":     {{Name: "id", Detail: "int4"}, {Name: "email", Detail: "text"}},
		"users": {{Name: "id", Detail: "int4"}},
	},
}

// listCompletions answers the offers as `text:kind`, so a case reads as one line.
func listCompletions(
	prefix string, position editor.NamePosition, allowQualified bool,
) []string {
	written := []string{}
	for _, offer := range editor.BuildCompletions(prefix, completionSources,
		editor.CompletionContext{AllowQualified: allowQualified, NamePosition: position}) {
		written = append(written, offer.Text+":"+string(offer.Kind))
	}
	return written
}

func requireCompletions(t *testing.T, got []string, want ...string) {
	t.Helper()
	if strings.Join(got, " ") != strings.Join(want, " ") {
		t.Errorf("\n got %v\nwant %v", got, want)
	}
}

func TestBuildCompletionsOffersNothingUnaskedWhereTheStatementExpectsNoName(t *testing.T) {
	// With no prefix and no place for a name, a list would stand over the statement for
	// nothing.
	requireCompletions(t, listCompletions("", editor.PositionNone, true))
}

func TestBuildCompletionsOffersTheColumnsWhereAColumnBelongs(t *testing.T) {
	requireCompletions(t, listCompletions("", editor.PositionColumn, true),
		"id:column", "email:column", "name:column", "now:function", "public.now:function")
}

func TestBuildCompletionsOffersTheRelationsWhereARelationBelongs(t *testing.T) {
	requireCompletions(t, listCompletions("", editor.PositionRelation, true),
		"users:table", "user_roles:table", "Orders:table", "public:schema", "other:schema")
}

func TestBuildCompletionsNarrowsToTheQualifierTheUserTyped(t *testing.T) {
	// `u.` names one relation, so the columns of every other one would be wrong here.
	requireCompletions(t, listCompletions("u.", editor.PositionColumn, true),
		"u.id:column", "u.email:column")
	requireCompletions(t, listCompletions("u.i", editor.PositionColumn, true),
		"u.id:column")
}

func TestBuildCompletionsWritesTheColumnAloneWhereAQualifiedNameIsRefused(t *testing.T) {
	// `update t set a.name = …` is refused by the server, so the offer carries the
	// column without its qualifier even though the user typed one.
	requireCompletions(t, listCompletions("u.", editor.PositionColumn, false),
		"id:column", "email:column")
}

func TestBuildCompletionsOffersAQualifiedFunctionUnderItsSchema(t *testing.T) {
	requireCompletions(t, listCompletions("public.", editor.PositionRelation, true),
		"public.now:function")
}

func TestBuildCompletionsReadsThePrefixWhateverTheCase(t *testing.T) {
	requireCompletions(t, listCompletions("nam", editor.PositionColumn, true),
		"name:column")
	requireCompletions(t, listCompletions("ORD", editor.PositionRelation, true),
		"Orders:table", "order by:keyword")
	requireCompletions(t, listCompletions("us", editor.PositionRelation, true),
		"users:table", "user_roles:table")
}

func TestBuildCompletionsOffersAKeywordOnlyOnceThePrefixIsTyped(t *testing.T) {
	// A keyword fits anywhere, so it would drown the names it stands beside. It is
	// offered only once the user has typed something for it to match.
	for _, offer := range listCompletions("", editor.PositionColumn, true) {
		if strings.HasSuffix(offer, ":keyword") {
			t.Errorf("a keyword was offered with no prefix: %s", offer)
		}
	}
	requireCompletions(t, listCompletions("se", editor.PositionNone, true),
		"set:keyword", "select:keyword", "case:keyword", "else:keyword", "offset:keyword",
		"insert into:keyword", "users:table", "user_roles:table")
}

func TestBuildCompletionsOffersNothingForAPrefixNothingMatches(t *testing.T) {
	for _, position := range []editor.NamePosition{
		editor.PositionColumn, editor.PositionRelation, editor.PositionNone,
	} {
		requireCompletions(t, listCompletions("zzz", position, true))
	}
}

func TestApplyCompletionWritesTheOfferOverTheWordBeingTyped(t *testing.T) {
	sql, offset := editor.ApplyCompletion("select nam", 10,
		editor.Completion{Text: "name", Kind: editor.CompleteColumn}, postgres.Dialect)
	if sql != "select name" || offset != 11 {
		t.Errorf("got %q at %d, want %q at 11", sql, offset, "select name")
	}
}

func TestApplyCompletionWritesTheBracketsOfAFunctionAndTheCaretInsideThem(t *testing.T) {
	sql, offset := editor.ApplyCompletion("select no", 9,
		editor.Completion{Text: "now", Kind: editor.CompleteFunction}, postgres.Dialect)
	if sql != "select now()" || offset != 11 {
		t.Errorf("got %q at %d, want %q at 11", sql, offset, "select now()")
	}
}

func TestApplyCompletionQuotesANameTheServerWouldNotReadBack(t *testing.T) {
	sql, offset := editor.ApplyCompletion("select Od", 9,
		editor.Completion{Text: "Odd Col", Kind: editor.CompleteColumn}, postgres.Dialect)
	if sql != `select "Odd Col"` || offset != 16 {
		t.Errorf("got %q at %d, want %q at 16", sql, offset, `select "Odd Col"`)
	}
}
