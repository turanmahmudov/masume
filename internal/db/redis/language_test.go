package redis

import (
	"fmt"
	"strings"
	"testing"

	"github.com/turanmahmudov/masume/internal/query/editor"
	"github.com/turanmahmudov/masume/internal/query/statement"
	"github.com/turanmahmudov/masume/internal/query/syntax"
)

// The risk of a command decides whether an agent may run it and whether the user is asked
// first. A command that removes keys read as a read would run with nobody told.
func TestResolveWriteRiskWeighsEveryKindOfCommand(t *testing.T) {
	spoke := Support.Language

	for _, held := range []struct {
		command string
		want    statement.WriteRisk
	}{
		// Reads.
		{"GET order:1", statement.RiskNone},
		{"KEYS *", statement.RiskNone},
		{"HGETALL order:1", statement.RiskNone},
		{"DBSIZE", statement.RiskNone},

		// Writes.
		{"SET order:1 ada", statement.RiskWrite},
		{"HSET order:1 customer ada", statement.RiskWrite},
		{"EXPIRE order:1 60", statement.RiskWrite},
		{"RENAME order:1 order:2", statement.RiskWrite},

		// Commands that remove data.
		{"DEL order:1", statement.RiskDelete},
		{"UNLINK order:1", statement.RiskDelete},
		{"HDEL order:1 customer", statement.RiskDelete},
		{"GETDEL order:1", statement.RiskDelete},

		// Commands that sweep the whole database and name no key. These are the worst a
		// key store takes, so they carry the highest risk.
		{"FLUSHDB", statement.RiskEveryRow},
		{"FLUSHALL", statement.RiskEveryRow},
		{"SHUTDOWN", statement.RiskEveryRow},
	} {
		t.Run(held.command, func(t *testing.T) {
			if answered := spoke.ResolveWriteRisk(held.command); answered != held.want {
				t.Errorf("%q reads as %q, wanted %q", held.command, answered, held.want)
			}
		})
	}
}

// A script runs a body the server executes, and the words of the call say nothing about
// what the body does. EVAL read as a plain write would let read-write access call FLUSHALL
// through a script.
func TestResolveWriteRiskWeighsAScriptAtTheHighestRisk(t *testing.T) {
	spoke := Support.Language

	for _, held := range []struct {
		command string
		want    statement.WriteRisk
	}{
		{`EVAL "return redis.call('FLUSHALL')" 0`, statement.RiskEveryRow},
		{"EVALSHA abc123 0", statement.RiskEveryRow},
		{"FCALL wipe 0", statement.RiskEveryRow},
		{`SCRIPT LOAD "return 1"`, statement.RiskEveryRow},
		{"FUNCTION FLUSH", statement.RiskEveryRow},

		// The server holds these three to reads, so they read.
		{`EVAL_RO "return 1" 0`, statement.RiskNone},
		{"EVALSHA_RO abc123 0", statement.RiskNone},
		{"FCALL_RO count 0", statement.RiskNone},
	} {
		t.Run(held.command, func(t *testing.T) {
			if answered := spoke.ResolveWriteRisk(held.command); answered != held.want {
				t.Errorf("%q reads as %q, wanted %q", held.command, answered, held.want)
			}
		})
	}
}

// A command that holds subcommands reads and writes under one name, so the subcommand
// decides the risk. CONFIG SET read as a read would let an agent with read-only access
// rewrite the settings of the server.
func TestResolveWriteRiskWeighsTheSubcommand(t *testing.T) {
	spoke := Support.Language

	for _, held := range []struct {
		command string
		want    statement.WriteRisk
	}{
		{"CONFIG GET maxmemory", statement.RiskNone},
		{"CONFIG SET maxmemory 100mb", statement.RiskWrite},
		{"CONFIG REWRITE", statement.RiskWrite},
		{"CONFIG RESETSTAT", statement.RiskWrite},
		{"CLIENT LIST", statement.RiskNone},
		{"CLIENT KILL ID 4", statement.RiskWrite},
		{"CLIENT PAUSE 1000", statement.RiskWrite},
		{"SLOWLOG GET 10", statement.RiskNone},
		{"SLOWLOG RESET", statement.RiskWrite},
		{"MEMORY USAGE order:1", statement.RiskNone},
		{"MEMORY PURGE", statement.RiskWrite},
		{"OBJECT ENCODING order:1", statement.RiskNone},
		{"COMMAND COUNT", statement.RiskNone},

		// The case a user typed changes nothing, and a subcommand nothing knows counts
		// as a write.
		{"config set maxmemory 100mb", statement.RiskWrite},
		{"CONFIG NOTASUBCOMMAND", statement.RiskWrite},
	} {
		t.Run(held.command, func(t *testing.T) {
			if answered := spoke.ResolveWriteRisk(held.command); answered != held.want {
				t.Errorf("%q reads as %q, wanted %q", held.command, answered, held.want)
			}
		})
	}
}

// The name of a command is read whatever case it was typed in, because a terminal user types
// either and the risk must not depend on it.
func TestResolveWriteRiskReadsACommandInAnyCase(t *testing.T) {
	spoke := Support.Language
	for _, command := range []string{"flushdb", "FlushDb", "FLUSHDB"} {
		if held := spoke.ResolveWriteRisk(command); held != statement.RiskEveryRow {
			t.Errorf("%q reads as %q, wanted the highest risk", command, held)
		}
	}
}

// A command nothing knows must not read as a read: the server may still act on it, and the
// user has to be asked.
func TestResolveWriteRiskDoesNotTrustACommandItDoesNotKnow(t *testing.T) {
	spoke := Support.Language
	for _, command := range []string{"NOTACOMMAND order:1", "EVAL script 0"} {
		if held := spoke.ResolveWriteRisk(command); held == statement.RiskNone {
			t.Errorf("%q reads as a read although nothing knows it", command)
		}
	}
}

// Every command of a buffer runs, so the buffer is as risky as the worst one in it.
func TestResolveWriteRiskOfABufferTakesTheWorstCommand(t *testing.T) {
	spoke := Support.Language
	held := spoke.ResolveWriteRisk("GET order:1\nFLUSHDB\nGET order:2")
	if held != statement.RiskEveryRow {
		t.Errorf("a buffer holding FLUSHDB reads as %q", held)
	}
}

// A key may hold any character, a semicolon too, so only a line break ends a command.
func TestSplitStatementsEndsACommandOnlyAtALineBreak(t *testing.T) {
	spoke := Support.Language

	for _, held := range []struct {
		name  string
		text  string
		count int
	}{
		{"one command", "GET order:1", 1},
		{"two commands", "GET order:1\nGET order:2", 2},
		{"a blank line between", "GET order:1\n\nGET order:2", 2},
		{"nothing", "", 0},
		{"only space", "   \n  ", 0},

		// A semicolon is part of the key, not the end of a command.
		{"a key holding a semicolon", "GET order;1", 1},
		{"two keys holding semicolons", "GET a;b\nGET c;d", 2},
	} {
		t.Run(held.name, func(t *testing.T) {
			if answered := spoke.SplitStatements(held.text); len(answered) != held.count {
				t.Errorf("%q split into %q, wanted %d commands",
					held.text, answered, held.count)
			}
		})
	}
}

// The words of a command are what the client colours and completes. Redis takes both quote
// marks, and a quoted word holds the spaces inside it.
func TestReadRedisWordsSplitsOnSpaceAndKeepsAQuotedWord(t *testing.T) {
	for _, held := range []struct {
		name  string
		line  string
		words []string
	}{
		{"a command and a key", "GET order:1", []string{"GET", "order:1"}},
		{"several spaces", "GET    order:1", []string{"GET", "order:1"}},
		// A quoted word answers the value inside the quotes, because that is what the
		// server is sent. The place still covers the quotes, so the editor colours them.
		{"a value in double quotes", `SET k "a b"`, []string{"SET", "k", "a b"}},
		{"a value in single quotes", `SET k 'a b'`, []string{"SET", "k", "a b"}},
		{"nothing", "", nil},
		{"only space", "   ", nil},
	} {
		t.Run(held.name, func(t *testing.T) {
			answered := ReadRedisWords(held.line, 0)
			if len(answered) != len(held.words) {
				t.Fatalf("%q read as %d words, wanted %d", held.line, len(answered), len(held.words))
			}
			for at, word := range answered {
				if word.Text != held.words[at] {
					t.Errorf("word %d reads %q, wanted %q", at, word.Text, held.words[at])
				}
				// The places have to point inside the line, because the editor colours from them.
				if word.Start < 0 || word.End > len(held.line) || word.End < word.Start {
					t.Errorf("word %d covers %d to %d of %d",
						at, word.Start, word.End, len(held.line))
				}
			}
		})
	}
}

// A quoted word answers the value without its quotes, and its place covers them, so the
// value sent to the server and the text coloured on screen are both right.
func TestReadRedisWordsCoversTheQuotesAndAnswersTheValue(t *testing.T) {
	const line = `SET k "a b"`
	words := ReadRedisWords(line, 0)
	if len(words) != 3 {
		t.Fatalf("the line read as %d words", len(words))
	}
	value := words[2]
	if value.Text != "a b" {
		t.Errorf("the value reads %q, wanted it without the quotes", value.Text)
	}
	if line[value.Start] != '"' {
		t.Errorf("the place starts at %q, wanted the opening quote", line[value.Start])
	}
	if value.End != len(line) {
		t.Errorf("the place ends at %d, wanted the closing quote at %d", value.End, len(line))
	}
}

// The offset is added to every place, so the words of one line of a buffer point at the
// buffer and not at the line.
func TestReadRedisWordsAddsTheOffsetOfTheLine(t *testing.T) {
	words := ReadRedisWords("GET order:1", 100)
	if len(words) == 0 {
		t.Fatal("the line read as no words")
	}
	if words[0].Start != 100 {
		t.Errorf("the first word starts at %d, wanted the offset of the line", words[0].Start)
	}
}

// A key store plans nothing and makes no catalog stale, and the panes that need either are
// left out rather than reporting a refusal.
func TestTheKeyStoreReportsWhatItCannotDo(t *testing.T) {
	spoke := Support.Language
	for _, command := range []string{"GET order:1", "SET order:1 ada", "FLUSHDB"} {
		if spoke.CanExplain(command) {
			t.Errorf("%q reads as a command the server plans", command)
		}
		if spoke.ChangesCatalog(command) {
			t.Errorf("%q reads as a command that makes the catalog stale", command)
		}
	}
}

func TestLanguageTokenizesTheFirstWordAsTheCommand(t *testing.T) {
	tokens := Language.Tokenize("SET user:1 \"a name\" 42\nGET user:1")
	kinds := []syntax.TokenKind{}
	for _, token := range tokens {
		kinds = append(kinds, token.Kind)
	}
	want := []syntax.TokenKind{
		syntax.TokenKeyword, syntax.TokenIdentifier, syntax.TokenString, syntax.TokenNumber,
		syntax.TokenKeyword, syntax.TokenIdentifier,
	}
	if len(kinds) != len(want) {
		t.Fatalf("got %v, want %v", kinds, want)
	}
	for at, kind := range kinds {
		if kind != want[at] {
			t.Errorf("token %d is %q, want %q", at, kind, want[at])
		}
	}
}

func TestLanguageMarksAQuoteThatNeverCloses(t *testing.T) {
	tokens := Language.Tokenize(`SET k "never closed`)
	last := tokens[len(tokens)-1]
	if !last.Unterminated {
		t.Error("the unclosed word was not marked")
	}
}

func TestLanguageReadsTheCommandTheCaretIsIn(t *testing.T) {
	const text = "SET a 1\nGET a\nDEL a"
	for _, held := range []struct {
		name   string
		offset int
		want   string
	}{
		{"the first line", 0, "SET a 1"},
		{"the end of the first line", 7, "SET a 1"},
		{"the second line", 9, "GET a"},
		{"the last line", 16, "DEL a"},
		{"past the end", 500, "DEL a"},
	} {
		t.Run(held.name, func(t *testing.T) {
			if got := Language.ReadStatementAtOffset(text, held.offset); got != held.want {
				t.Errorf("got %q, want %q", got, held.want)
			}
		})
	}
}

func TestLanguageReadsAnEmptyBufferAsNothing(t *testing.T) {
	if got := Language.ReadStatementAtOffset("   ", 0); got != "" {
		t.Errorf("got %q, want nothing", got)
	}
}

func TestFormatStatementWritesTheCommandInCapitalsAndOneSpaceBetweenWords(t *testing.T) {
	for _, held := range []struct {
		name string
		text string
		want string
	}{
		{"a lower case command", "set   a    1", "SET a 1"},
		{"an argument keeps its case", "set Key Value", "SET Key Value"},
		{"a quoted word keeps its quotes", `set k "a name"`, `SET k "a name"`},
		{"a quote inside a quoted word", `set k "say \"hi\""`, `SET k "say \"hi\""`},
		{"several commands", "set a 1\nget a", "SET a 1\nGET a"},
	} {
		t.Run(held.name, func(t *testing.T) {
			if got := Language.FormatStatement(held.text); got != held.want {
				t.Errorf("got %q, want %q", got, held.want)
			}
		})
	}
}

func TestFindLocalDiagnosticsMarksAnUnclosedQuoteAndAnUnknownCommand(t *testing.T) {
	for _, held := range []struct {
		name string
		text string
		want []string
	}{
		{"a good command", "SET a 1", nil},
		{"a command in lower case", "set a 1", nil},
		{"an unclosed quote", `SET a "open`, []string{"this quote never closes"}},
		{"a command the client does not know", "FROBNICATE a",
			[]string{"FROBNICATE is not a command this client knows"}},
		{"a quoted first word is no command", `"SET" a 1`, nil},
		{"an empty line", "", nil},
		{"a fault on the second line only", "SET a 1\nFROBNICATE b",
			[]string{"FROBNICATE is not a command this client knows"}},
	} {
		t.Run(held.name, func(t *testing.T) {
			messages := []string{}
			for _, found := range Language.FindLocalDiagnostics(held.text, editor.NothingKnown()) {
				messages = append(messages, found.Message)
			}
			if len(messages) != len(held.want) {
				t.Fatalf("got %v, want %v", messages, held.want)
			}
			for at, message := range messages {
				if message != held.want[at] {
					t.Errorf("message %d = %q, want %q", at, message, held.want[at])
				}
			}
		})
	}
}

func TestFindLocalDiagnosticsPointsAtTheWordOnTheLineItIsOn(t *testing.T) {
	const text = "SET a 1\nFROBNICATE b"
	found := Language.FindLocalDiagnostics(text, editor.NothingKnown())
	if len(found) != 1 {
		t.Fatalf("got %d faults, want 1", len(found))
	}
	if text[found[0].Start:found[0].End] != "FROBNICATE" {
		t.Errorf("the fault covers %q", text[found[0].Start:found[0].End])
	}
}

func TestBuildCompletionsOffersACommandForTheFirstWordAndAKeyAfterIt(t *testing.T) {
	sources := editor.CompletionSources{Tables: []string{"user:1", "order:9"}}
	context := editor.CompletionContext{AllowQualified: true, NamePosition: editor.PositionNone}

	for _, held := range []struct {
		name   string
		prefix string
		want   []string
	}{
		{"nothing typed offers nothing", "", nil},
		{"a command", "GE", []string{"GET:keyword", "GETSET:keyword", "GETDEL:keyword"}},
		{"a command in lower case", "ge",
			[]string{"GET:keyword", "GETSET:keyword", "GETDEL:keyword"}},
		{"a key", "user", []string{"user:1:table"}},
		{"nothing matches", "zzz", nil},
	} {
		t.Run(held.name, func(t *testing.T) {
			written := []string{}
			for _, offer := range Language.BuildCompletions(held.prefix, sources, context) {
				written = append(written, offer.Text+":"+string(offer.Kind))
			}
			if strings.Join(written, " ") != strings.Join(held.want, " ") {
				t.Errorf("got %v, want %v", written, held.want)
			}
		})
	}
}

func TestBuildCompletionsNeverOffersMoreThanTheListCanShow(t *testing.T) {
	keys := make([]string, 0, 80)
	for at := range 80 {
		keys = append(keys, fmt.Sprintf("key:%02d", at))
	}
	offers := Language.BuildCompletions("key",
		editor.CompletionSources{Tables: keys},
		editor.CompletionContext{NamePosition: editor.PositionNone})
	if len(offers) != 50 {
		t.Errorf("got %d offers, want 50", len(offers))
	}
}

func TestTheKeyStorePlansNothingAndKeepsNoCatalog(t *testing.T) {
	// The tree is built by a scan of the key space, so a write changes only what the
	// next scan finds, and a command says what it does rather than being planned.
	for _, text := range []string{"SET a 1", "FLUSHDB", "GET a"} {
		if Language.ChangesCatalog(text) {
			t.Errorf("%q was said to change a catalog", text)
		}
		if Language.CanExplain(text) {
			t.Errorf("%q was said to be plannable", text)
		}
	}
	if Language.LineComment() != "" {
		t.Errorf("LineComment = %q, want nothing", Language.LineComment())
	}
}
