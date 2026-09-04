package query_test

import (
	"strings"
	"testing"

	"github.com/turanmahmudov/masume/internal/db/mysql"
	"github.com/turanmahmudov/masume/internal/db/postgres"
	"github.com/turanmahmudov/masume/internal/db/sqlite"
	"github.com/turanmahmudov/masume/internal/query"
)

// A placeholder is numbered where the server numbers them, and counted where it counts them.
// A statement that binds two sets of values numbers the second after the first, so a filter
// laid over a statement of the user does not take the numbers the statement already used.
func TestBoundValuesNumberFromWhereTheyAreTold(t *testing.T) {
	bound := query.NewBoundValues(postgres.Dialect, 1)
	first := bound.Bind("ada")
	second := bound.Bind(42)

	if first != "$1" || second != "$2" {
		t.Errorf("the placeholders read %q and %q, wanted $1 and $2", first, second)
	}
	if len(bound.Params) != 2 || bound.Params[0] != "ada" {
		t.Errorf("the values read %v", bound.Params)
	}
}

func TestBoundValuesCarryOnFromALaterNumber(t *testing.T) {
	// The statement of the user already bound two values, so the filter starts at three.
	bound := query.NewBoundValues(postgres.Dialect, 3)
	if held := bound.Bind("ada"); held != "$3" {
		t.Errorf("the first placeholder reads %q, wanted $3", held)
	}
	if held := bound.Bind("grace"); held != "$4" {
		t.Errorf("the second placeholder reads %q, wanted $4", held)
	}
}

// MySQL numbers no placeholder, so every one of them is the same mark and the order of the
// values is what lines them up.
func TestBoundValuesWriteTheMarkMysqlUses(t *testing.T) {
	bound := query.NewBoundValues(mysql.Dialect, 1)
	first := bound.Bind("ada")
	second := bound.Bind("grace")

	if first != "?" || second != "?" {
		t.Errorf("the placeholders read %q and %q, wanted a mark each", first, second)
	}
	if len(bound.Params) != 2 {
		t.Errorf("the values read %v", bound.Params)
	}
}

// A name is quoted the way its server quotes one, or a relation called after a keyword cannot
// be read at all.
func TestQuoteIdentifierUsesTheMarkOfTheServer(t *testing.T) {
	for _, held := range []struct {
		name    string
		dialect *query.Dialect
		mark    string
	}{
		{"postgres quotes with a double quote", postgres.Dialect, `"`},
		{"mysql quotes with a backtick", mysql.Dialect, "`"},
	} {
		t.Run(held.name, func(t *testing.T) {
			written := held.dialect.QuoteIdentifier("order")
			if !strings.HasPrefix(written, held.mark) || !strings.HasSuffix(written, held.mark) {
				t.Errorf("a name quotes as %q, wanted %s around it", written, held.mark)
			}
		})
	}
}

// A quote inside a name has to be doubled, or the name would end early and the rest of it
// would be read as SQL.
func TestQuoteIdentifierDoublesTheMarkInsideAName(t *testing.T) {
	written := postgres.Dialect.QuoteIdentifier(`a"b`)
	if written != `"a""b"` {
		t.Errorf(`a name holding a quote wrote as %s, wanted "a""b"`, written)
	}

	held := mysql.Dialect.QuoteIdentifier("a`b")
	if held != "`a``b`" {
		t.Errorf("a name holding a backtick wrote as %s", held)
	}
}

// A qualified name is the schema and the name, each quoted, so neither can end the other.
func TestBuildQualifiedNameQuotesBothParts(t *testing.T) {
	written := postgres.Dialect.BuildQualifiedName(
		query.QualifiedName{Schema: "public", Name: "orders"})
	if written != `"public"."orders"` {
		t.Errorf("the name wrote as %s", written)
	}

}

// Every relation the client holds carries a schema, because a catalog read fills it in: the
// name of the namespace for PostgreSQL, the database for MySQL, `main` for a file. This
// records that the writer takes one, and quotes an empty schema rather than leaving it out.
func TestBuildQualifiedNameAlwaysWritesASchema(t *testing.T) {
	written := postgres.Dialect.BuildQualifiedName(query.QualifiedName{Name: "orders"})
	if !strings.Contains(written, ".") {
		t.Errorf("a name with no schema wrote as %s, wanted the schema part kept", written)
	}
}

// The base of a type is what the client matches on, because a server names a type with its
// size and its array marks and those do not change what it holds.
func TestReadBaseTypeTakesOffTheSizeAndTheArrayMarks(t *testing.T) {
	for _, held := range []struct {
		dataType string
		want     string
	}{
		{"integer", "integer"},
		{"numeric(10,2)", "numeric"},
		{"character varying(64)", "character varying"},
		{"text[]", "text"},
		// The catalog names an array of any depth with one pair of marks, so one pair is
		// what comes off.
		{"integer[]", "integer"},
		{"", ""},
	} {
		if answered := query.ReadBaseType(held.dataType); answered != held.want {
			t.Errorf("%q reads as %q, wanted %q", held.dataType, answered, held.want)
		}
	}
}

// A reply of a model carries a proposed statement in a fenced block. The block is tagged
// with the language of the connected server, so every tag this client speaks has to be
// read as a statement, and nothing else.
func TestSplitMessageSegmentsReadsTheFenceOfEveryLanguage(t *testing.T) {
	for _, held := range []struct {
		tag   string
		found bool
	}{
		{"sql", true}, {"js", true}, {"javascript", true},
		{"mongodb", true}, {"mongosh", true}, {"", true},
		// A block of anything else is text the user reads, not a statement to run.
		{"json", false}, {"text", false}, {"bash", false},
	} {
		reply := "before\n```" + held.tag + "\ndb.orders.find({})\n```\nafter"
		block, found := query.FindSQLBlock(reply)
		if found != held.found {
			t.Errorf("a ```%s block reads as a statement=%v, wanted %v",
				held.tag, found, held.found)
			continue
		}
		if found && block != "db.orders.find({})" {
			t.Errorf("a ```%s block reads %q", held.tag, block)
		}
	}
}

func TestQuoteIdentifierIfNeededQuotesOnlyANameTheServerWouldReadDifferently(t *testing.T) {
	// A bare name the server lower-cases, or reads as a keyword, or cannot read at all,
	// has to be quoted. Anything else is left plain, so a written statement stays legible.
	for _, held := range []struct {
		name     string
		given    string
		postgres string
		mysql    string
	}{
		{"a plain word", "plain", "plain", "plain"},
		{"a leading underscore", "_under", "_under", "_under"},
		{"a dollar inside", "a$b", "a$b", "a$b"},
		{"a space", "Weird Name", `"Weird Name"`, "`Weird Name`"},
		{"upper case", "UPPER", `"UPPER"`, "`UPPER`"},
		{"a keyword", "select", `"select"`, "`select`"},
		{"a dot", "a.b", `"a.b"`, "`a.b`"},
		{"a colon", "user:1", `"user:1"`, "`user:1`"},
		{"a leading digit", "1start", `"1start"`, "`1start`"},
		{"nothing", "", `""`, "``"},
		{"the mark of the other server", `with"quote`, `"with""quote"`, "`with\"quote`"},
		{"the mark of this server", "with`tick", "\"with`tick\"", "`with``tick`"},
		{"a backslash", `back\slash`, `"back\slash"`, "`back\\slash`"},
	} {
		t.Run(held.name, func(t *testing.T) {
			if got := postgres.Dialect.QuoteIdentifierIfNeeded(held.given); got != held.postgres {
				t.Errorf("postgres: got %q, want %q", got, held.postgres)
			}
			if got := mysql.Dialect.QuoteIdentifierIfNeeded(held.given); got != held.mysql {
				t.Errorf("mysql: got %q, want %q", got, held.mysql)
			}
		})
	}
}

func TestBuildPlaceholderWritesTheMarkOfTheServer(t *testing.T) {
	for _, position := range []int{1, 2, 7} {
		if got := mysql.Dialect.BuildPlaceholder(position); got != "?" {
			t.Errorf("mysql placeholder %d = %q, want ?", position, got)
		}
	}
	for _, held := range []struct {
		position int
		want     string
	}{{1, "$1"}, {2, "$2"}, {7, "$7"}} {
		if got := postgres.Dialect.BuildPlaceholder(held.position); got != held.want {
			t.Errorf("postgres placeholder %d = %q, want %q", held.position, got, held.want)
		}
	}
}

func TestBuildDropSchemaWritesWhatEachEngineCallsASchema(t *testing.T) {
	for _, held := range []struct {
		name    string
		dialect *query.Dialect
		want    string
	}{
		{"postgres", postgres.Dialect, `drop schema "sch" restrict;`},
		{"mysql", mysql.Dialect, "drop database `sch`;"},
		{"sqlite", sqlite.Dialect, `detach database "sch";`},
	} {
		t.Run(held.name, func(t *testing.T) {
			if got := held.dialect.BuildDropSchema("sch"); got != held.want {
				t.Errorf("got %q, want %q", got, held.want)
			}
		})
	}
}

func TestBuildDropTriggerNamesTheTableOnlyWhereTheServerNeedsIt(t *testing.T) {
	for _, held := range []struct {
		name    string
		dialect *query.Dialect
		want    string
	}{
		{"postgres drops it from its table", postgres.Dialect,
			`drop trigger "tr" on "sch"."tbl";`},
		{"mysql drops it by name", mysql.Dialect, "drop trigger `sch`.`tr`;"},
		{"sqlite drops it by name", sqlite.Dialect, `drop trigger "sch"."tr";`},
	} {
		t.Run(held.name, func(t *testing.T) {
			if got := held.dialect.BuildDropTrigger("sch", "tr", "tbl"); got != held.want {
				t.Errorf("got %q, want %q", got, held.want)
			}
		})
	}
}

func TestBuildDropRoutineWritesTheWordTheServerReads(t *testing.T) {
	for _, held := range []struct {
		name     string
		dialect  *query.Dialect
		identity string
		want     string
	}{
		{"postgres drops either kind as a routine", postgres.Dialect, "function:sch.fn",
			`drop routine "sch"."fn";`},
		{"postgres drops a procedure as a routine too", postgres.Dialect, "procedure:sch.fn",
			`drop routine "sch"."fn";`},
		{"mysql names a function", mysql.Dialect, "function:sch.fn",
			"drop function `sch`.`fn`;"},
		{"mysql names a procedure", mysql.Dialect, "procedure:sch.fn",
			"drop procedure `sch`.`fn`;"},
		{"sqlite keeps none", sqlite.Dialect, "function:sch.fn",
			"-- sqlite keeps no stored routine"},
	} {
		t.Run(held.name, func(t *testing.T) {
			if got := held.dialect.BuildDropRoutine("sch", "fn", held.identity); got != held.want {
				t.Errorf("got %q, want %q", got, held.want)
			}
		})
	}
}

func TestEveryDialectNamesItsEngineItsSyntaxAndItsWords(t *testing.T) {
	for _, held := range []struct {
		name       string
		dialect    *query.Dialect
		engine     string
		syntax     string
		schemaWord string
		count      string
		identity   string
	}{
		{"postgres", postgres.Dialect, "postgres", "standard", "schema",
			"count(*)::int8", "id bigserial primary key"},
		{"mysql", mysql.Dialect, "mysql", "mysql", "database",
			"count(*)", "id bigint auto_increment primary key"},
		{"sqlite", sqlite.Dialect, "sqlite", "standard", "database",
			"count(*)", "id integer primary key autoincrement"},
	} {
		t.Run(held.name, func(t *testing.T) {
			if string(held.dialect.Engine) != held.engine {
				t.Errorf("Engine = %q, want %q", held.dialect.Engine, held.engine)
			}
			if string(held.dialect.Syntax) != held.syntax {
				t.Errorf("Syntax = %q, want %q", held.dialect.Syntax, held.syntax)
			}
			if held.dialect.SchemaWord != held.schemaWord {
				t.Errorf("SchemaWord = %q, want %q", held.dialect.SchemaWord, held.schemaWord)
			}
			if held.dialect.CountExpression != held.count {
				t.Errorf("CountExpression = %q, want %q",
					held.dialect.CountExpression, held.count)
			}
			if held.dialect.IdentityColumn != held.identity {
				t.Errorf("IdentityColumn = %q, want %q",
					held.dialect.IdentityColumn, held.identity)
			}
		})
	}
}
