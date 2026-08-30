package mongo

import (
	"strings"
	"testing"

	"github.com/turanmahmudov/masume/internal/query/editor"
	"github.com/turanmahmudov/masume/internal/query/statement"
	"github.com/turanmahmudov/masume/internal/query/syntax"
)

// The risk of a call decides whether the user is asked before it runs. A call that
// removes documents read as a read would run with nobody told.
func TestResolveWriteRiskWeighsEveryKindOfCall(t *testing.T) {
	spoke := Support.Language
	for _, held := range []struct {
		written string
		want    statement.WriteRisk
	}{
		{`db.orders.find({})`, statement.RiskNone},
		{`db.orders.countDocuments({})`, statement.RiskNone},
		{`db.orders.insertOne({total: 5})`, statement.RiskWrite},
		{`db.orders.updateOne({_id: 1}, {$set: {total: 5}})`, statement.RiskWrite},
		{`db.orders.deleteOne({_id: 1})`, statement.RiskDelete},
		{`db.orders.drop()`, statement.RiskEveryRow},
		// A delete of many with no filter reaches every document of the collection.
		{`db.orders.deleteMany({})`, statement.RiskEveryRow},
		{`db.orders.deleteMany({status: "old"})`, statement.RiskDelete},
		{`db.orders.updateMany({}, {$set: {total: 0}})`, statement.RiskEveryRow},
		// A call this client does not know could do anything, so it counts as a write.
		{`db.orders.somethingNew({})`, statement.RiskWrite},
		// A batch takes the highest risk it holds.
		{"db.orders.find({})\ndb.orders.drop()", statement.RiskEveryRow},
	} {
		if answered := spoke.ResolveWriteRisk(held.written); answered != held.want {
			t.Errorf("%q reads as %q, wanted %q", held.written, answered, held.want)
		}
	}
}

// An aggregation reads until one of its stages writes. `$out` and `$merge` write into a
// collection, so a pipeline that holds one is not a read whatever the name of the call
// says, and an agent with read-only access must not run it.
func TestResolveWriteRiskReadsThePipelineOfAnAggregation(t *testing.T) {
	spoke := Support.Language
	for _, held := range []struct {
		written string
		want    statement.WriteRisk
	}{
		{`db.orders.aggregate([{$match: {status: "new"}}])`, statement.RiskNone},
		{`db.orders.aggregate([{$group: {_id: "$status"}}, {$sort: {_id: 1}}])`, statement.RiskNone},
		// `$out` replaces every document of the collection it names.
		{`db.orders.aggregate([{$match: {}}, {$out: "copy"}])`, statement.RiskEveryRow},
		{`db.orders.aggregate([{$out: {db: "other", coll: "copy"}}])`, statement.RiskEveryRow},
		{`db.orders.aggregate([{$merge: {into: "copy"}}])`, statement.RiskWrite},
		// A pipeline this client cannot read may hold a stage that writes.
		{`db.orders.aggregate("nonsense")`, statement.RiskWrite},
	} {
		if answered := spoke.ResolveWriteRisk(held.written); answered != held.want {
			t.Errorf("%q reads as %q, wanted %q", held.written, answered, held.want)
		}
	}
}

// runCommand hands the server any command there is, and the name of the call says nothing
// about which one. A dropDatabase read as a plain write would run with read-write access.
func TestResolveWriteRiskReadsTheCommandOfARunCommand(t *testing.T) {
	spoke := Support.Language
	for _, held := range []struct {
		written string
		want    statement.WriteRisk
	}{
		{`db.runCommand({find: "orders"})`, statement.RiskNone},
		{`db.runCommand({collStats: "orders"})`, statement.RiskNone},
		{`db.runCommand({insert: "orders", documents: [{a: 1}]})`, statement.RiskWrite},
		{`db.runCommand({delete: "orders", deletes: [{q: {}, limit: 0}]})`, statement.RiskDelete},
		{`db.runCommand({dropDatabase: 1})`, statement.RiskEveryRow},
		{`db.runCommand({drop: "orders"})`, statement.RiskEveryRow},
		{`db.adminCommand({shutdown: 1})`, statement.RiskEveryRow},
		// The pipeline of an aggregate command decides, as it does for the call.
		{`db.runCommand({aggregate: "orders", pipeline: [{$match: {}}], cursor: {}})`,
			statement.RiskNone},
		{`db.runCommand({aggregate: "orders", pipeline: [{$out: "copy"}], cursor: {}})`,
			statement.RiskEveryRow},
		// A command this client does not know may be one that writes.
		{`db.runCommand({unheardOf: 1})`, statement.RiskWrite},
		{`db.runCommand("nonsense")`, statement.RiskWrite},
	} {
		if answered := spoke.ResolveWriteRisk(held.written); answered != held.want {
			t.Errorf("%q reads as %q, wanted %q", held.written, answered, held.want)
		}
	}
}

// The old shell removes one document where `remove` is given a justOne, and every match
// where it is not. Read as a deleteMany, a remove of one would empty the collection.
func TestResolveWriteRiskReadsTheJustOneOfARemove(t *testing.T) {
	spoke := Support.Language
	for _, held := range []struct {
		written string
		want    statement.WriteRisk
	}{
		{`db.orders.remove({})`, statement.RiskEveryRow},
		{`db.orders.remove({}, false)`, statement.RiskEveryRow},
		{`db.orders.remove({}, true)`, statement.RiskDelete},
		{`db.orders.remove({}, {justOne: true})`, statement.RiskDelete},
		{`db.orders.remove({status: "old"})`, statement.RiskDelete},
	} {
		if answered := spoke.ResolveWriteRisk(held.written); answered != held.want {
			t.Errorf("%q reads as %q, wanted %q", held.written, answered, held.want)
		}
	}
}

// A remove that names a justOne removes one document, and the call the session sends has
// to be the one that does that.
func TestReadsJustOneOfARemove(t *testing.T) {
	for _, held := range []struct {
		written string
		want    bool
	}{
		{`db.orders.remove({}, true)`, true},
		{`db.orders.remove({}, {justOne: true})`, true},
		{`db.orders.remove({}, {justOne: false})`, false},
		{`db.orders.remove({}, false)`, false},
		{`db.orders.remove({})`, false},
		{`db.orders.deleteMany({})`, false},
	} {
		parsed, _, ok := ParseStatement(held.written)
		if !ok {
			t.Fatalf("%q did not parse", held.written)
		}
		if answered := readsJustOne(parsed.Calls[0]); answered != held.want {
			t.Errorf("%q reads justOne = %v, wanted %v", held.written, answered, held.want)
		}
	}
}

// A statement ends at the end of its line, and a document inside it carries it over as
// many lines as it needs.
func TestSplitStatementsEndsAStatementWhereItCloses(t *testing.T) {
	for _, held := range []struct {
		written string
		want    []string
	}{
		{"db.orders.find({})", []string{`db.orders.find({})`}},
		{
			"db.orders.find({})\ndb.orders.drop()",
			[]string{`db.orders.find({})`, `db.orders.drop()`},
		},
		{
			`db.orders.find({}); db.orders.drop()`,
			[]string{`db.orders.find({})`, `db.orders.drop()`},
		},
		// A document that is still open carries the statement over the line break.
		{
			"db.orders.find({\n  status: \"new\"\n})",
			[]string{"db.orders.find({\n  status: \"new\"\n})"},
		},
		// A line that opens with a dot carries the call chain on.
		{
			"db.orders.find({})\n  .limit(10)",
			[]string{"db.orders.find({})\n  .limit(10)"},
		},
		{"\n\n  \n", nil},
	} {
		answered := Support.Language.SplitStatements(held.written)
		if len(answered) != len(held.want) {
			t.Errorf("%q splits into %d statements, wanted %d: %q",
				held.written, len(answered), len(held.want), answered)
			continue
		}
		for at, one := range answered {
			if one != held.want[at] {
				t.Errorf("statement %d of %q reads %q, wanted %q",
					at, held.written, one, held.want[at])
			}
		}
	}
}

// A semicolon inside a value is part of the value, and would end the statement early if
// the splitter counted it.
func TestSplitStatementsKeepsASemicolonInsideAValue(t *testing.T) {
	written := `db.orders.find({note: "a; b"})`
	answered := Support.Language.SplitStatements(written)
	if len(answered) != 1 || answered[0] != written {
		t.Errorf("%q splits into %q", written, answered)
	}
}

// The tree of this client is a list of collections, so a call that adds or removes one
// leaves it stale.
func TestChangesCatalogNamesTheCallsThatMoveACollection(t *testing.T) {
	spoke := Support.Language
	for _, held := range []struct {
		written string
		want    bool
	}{
		{`db.orders.drop()`, true},
		{`db.createCollection("orders")`, true},
		{`db.orders.createIndex({total: 1})`, true},
		{`db.orders.find({})`, false},
		{`db.orders.insertOne({total: 5})`, false},
	} {
		if answered := spoke.ChangesCatalog(held.written); answered != held.want {
			t.Errorf("%q reads as %v, wanted %v", held.written, answered, held.want)
		}
	}
}

// Only a read is planned. Asking the server to plan a write would run it.
func TestCanExplainNamesTheReadsTheServerPlans(t *testing.T) {
	spoke := Support.Language
	for _, held := range []struct {
		written string
		want    bool
	}{
		{`db.orders.find({})`, true},
		{`db.orders.aggregate([{$match: {}}])`, true},
		{`db.orders.countDocuments({})`, true},
		{`db.orders.insertOne({total: 5})`, false},
		{`db.runCommand({ping: 1})`, false},
	} {
		if answered := spoke.CanExplain(held.written); answered != held.want {
			t.Errorf("%q reads as %v, wanted %v", held.written, answered, held.want)
		}
	}
}

// The faults the client finds itself are the ones the user sees while typing, before
// anything is sent.
func TestFindLocalDiagnosticsReportsWhatTheClientCanSee(t *testing.T) {
	for _, held := range []struct {
		written string
		wanted  string
	}{
		{`db.orders.find({})`, ""},
		{`orders.find({})`, "starts with db"},
		{`db.orders.somethingNew({})`, "not a call this client knows"},
		{`db.orders.find({a: 1)`, "never closes"},
	} {
		found := Support.Language.FindLocalDiagnostics(held.written, editor.NothingKnown())
		if held.wanted == "" {
			if len(found) > 0 {
				t.Errorf("%q answered %q", held.written, found[0].Message)
			}
			continue
		}
		if len(found) == 0 {
			t.Errorf("%q answered no fault, wanted one about %q", held.written, held.wanted)
			continue
		}
		if !strings.Contains(found[0].Message, held.wanted) {
			t.Errorf("%q answered %q, wanted one about %q",
				held.written, found[0].Message, held.wanted)
		}
	}
}

// The editor colours what it reads, so the word a statement opens with, a call, an
// operator and a value each have to be told apart.
func TestTokenizeTellsThePartsOfAStatementApart(t *testing.T) {
	written := `db.orders.find({$or: [{total: 5}]}) // a note`
	kinds := map[syntax.TokenKind]int{}
	for _, token := range Support.Language.Tokenize(written) {
		kinds[token.Kind]++
	}
	if kinds[syntax.TokenKeyword] < 2 {
		t.Errorf("db and find were not coloured as calls: %v", kinds)
	}
	if kinds[syntax.TokenType] == 0 {
		t.Errorf("the operator of the document was not coloured: %v", kinds)
	}
	if kinds[syntax.TokenNumber] == 0 {
		t.Errorf("the number was not coloured: %v", kinds)
	}
	if kinds[syntax.TokenComment] == 0 {
		t.Errorf("the note was not coloured as a comment: %v", kinds)
	}
}

// A quote that never closes is marked, so the editor can draw the run of text it opened.
func TestTokenizeMarksAQuoteThatNeverCloses(t *testing.T) {
	found := false
	for _, token := range Support.Language.Tokenize(`db.orders.find({a: "open`) {
		found = found || token.Unterminated
	}
	if !found {
		t.Error("the quote that never closes was not marked")
	}
}

// The completion writes the whole call chain back, because the word under the caret
// holds the dots that came before it.
func TestBuildCompletionsOffersTheWholeCallChain(t *testing.T) {
	sources := editor.CompletionSources{Tables: []string{"orders", "shop.orders"}}
	for _, held := range []struct {
		prefix string
		want   string
	}{
		{"d", "db"},
		{"db.or", "db.orders"},
		{"db.orders.fin", "db.orders.find"},
		{"db.getSi", "db.getSiblingDB"},
	} {
		found := Support.Language.BuildCompletions(held.prefix, sources, editor.CompletionContext{})
		if len(found) == 0 {
			t.Errorf("%q was offered nothing", held.prefix)
			continue
		}
		named := false
		for _, one := range found {
			named = named || one.Text == held.want
		}
		if !named {
			t.Errorf("%q was not offered %q, but %q", held.prefix, held.want, found[0].Text)
		}
	}
}

// A format collapses the blanks a user left, and leaves the text of a value alone.
func TestFormatStatementCollapsesTheBlanksOutsideAValue(t *testing.T) {
	answered := Support.Language.FormatStatement("db.orders.find( {  note:  \"a  b\" } )")
	if answered != `db.orders.find({note: "a  b"})` {
		t.Errorf("the statement reads %q", answered)
	}
}
