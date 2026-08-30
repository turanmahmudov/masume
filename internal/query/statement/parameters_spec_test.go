package statement_test

import (
	"strings"
	"testing"

	"github.com/turanmahmudov/masume/internal/db/mysql"
	"github.com/turanmahmudov/masume/internal/db/postgres"
	"github.com/turanmahmudov/masume/internal/query/statement"
)

// The marks the user wrote are what the form asks for, so a mark missed here is a value the
// user is never asked to give.
func TestFindQueryParametersNamesEveryMarkOnce(t *testing.T) {
	for _, held := range []struct {
		name string
		sql  string
		want []string
	}{
		{"one mark", "select * from orders where id = :id", []string{"id"}},
		{"two marks", "select * from orders where id = :id and customer = :name",
			[]string{"id", "name"}},
		// A mark written twice is one value to ask for.
		{"a mark written twice", "select :id, :id", []string{"id"}},
		// The same name in another case is the same mark.
		{"a mark in another case", "select :id, :ID", []string{"id"}},

		{"no mark at all", "select * from orders", nil},
		{"nothing", "", nil},

		// A colon that is not a mark: a cast, a mark inside text, and one in a comment.
		{"a cast", "select id::text from orders", nil},
		{"a mark inside text", "select ':id'", nil},
		{"a mark in a line comment", "select 1 -- :id", nil},
		{"a mark in a block comment", "select /* :id */ 1", nil},
	} {
		t.Run(held.name, func(t *testing.T) {
			answered := statement.FindQueryParameters(held.sql)
			if len(answered) != len(held.want) {
				t.Fatalf("%q names %q, wanted %q", held.sql, answered, held.want)
			}
			for at := range answered {
				if !strings.EqualFold(answered[at], held.want[at]) {
					t.Errorf("%q names %q, wanted %q", held.sql, answered, held.want)
				}
			}
		})
	}
}

// A mark becomes a placeholder the driver binds, and the value the user gave never reaches
// the text of the statement.
func TestBindQueryParametersBindsAndNeverWritesTheValue(t *testing.T) {
	answered, err := statement.BindQueryParameters(
		"select * from orders where customer = :name",
		map[string]any{"name": "'; drop table orders --"}, postgres.Dialect, 1)
	if err != nil {
		t.Fatalf("the bind answered %v", err)
	}
	if strings.Contains(answered.SQL, "drop table") {
		t.Errorf("the value was written into the statement:\n%s", answered.SQL)
	}
	if !strings.Contains(answered.SQL, "$1") {
		t.Errorf("the mark did not become a placeholder:\n%s", answered.SQL)
	}
	if len(answered.Params) != 1 || answered.Params[0] != "'; drop table orders --" {
		t.Errorf("the bound values read %v", answered.Params)
	}
}

// A name written twice is bound twice, because a placeholder stands for one value each and
// both servers take the same value twice.
func TestBindQueryParametersBindsAMarkWrittenTwiceTwice(t *testing.T) {
	answered, err := statement.BindQueryParameters(
		"select :id, :id", map[string]any{"id": 7}, postgres.Dialect, 1)
	if err != nil {
		t.Fatalf("the bind answered %v", err)
	}
	if len(answered.Params) != 2 {
		t.Fatalf("the statement binds %d values, wanted 2", len(answered.Params))
	}
	if answered.Params[0] != 7 || answered.Params[1] != 7 {
		t.Errorf("the bound values read %v, wanted the one value twice", answered.Params)
	}
	if !strings.Contains(answered.SQL, "$1") || !strings.Contains(answered.SQL, "$2") {
		t.Errorf("the placeholders were not numbered in turn:\n%s", answered.SQL)
	}
}

// A filter laid over a statement of the user numbers its own values after the ones the
// statement already bound.
func TestBindQueryParametersStartsAtTheNumberItIsGiven(t *testing.T) {
	answered, err := statement.BindQueryParameters(
		"select * from orders where id = :id", map[string]any{"id": 1}, postgres.Dialect, 5)
	if err != nil {
		t.Fatalf("the bind answered %v", err)
	}
	if !strings.Contains(answered.SQL, "$5") {
		t.Errorf("the placeholder was not numbered from five:\n%s", answered.SQL)
	}
}

// A mark with no value cannot be bound, and the statement must not run with the mark still
// in it: the server would refuse it with a message about the colon.
func TestBindQueryParametersRefusesAMarkWithNoValue(t *testing.T) {
	_, err := statement.BindQueryParameters(
		"select * from orders where id = :id", map[string]any{}, postgres.Dialect, 1)
	if err == nil {
		t.Fatal("a mark with no value was bound")
	}
	if !strings.Contains(err.Error(), "id") {
		t.Errorf("the reason reads %q and does not name the mark", err)
	}
}

// MySQL numbers no placeholder, so each mark becomes the same mark and the order lines them up.
func TestBindQueryParametersWritesTheMarkOfTheServer(t *testing.T) {
	answered, err := statement.BindQueryParameters(
		"select :a, :b", map[string]any{"a": 1, "b": 2}, mysql.Dialect, 1)
	if err != nil {
		t.Fatalf("the bind answered %v", err)
	}
	if strings.Count(answered.SQL, "?") != 2 {
		t.Errorf("the statement holds %d marks:\n%s", strings.Count(answered.SQL, "?"), answered.SQL)
	}
	if len(answered.Params) != 2 {
		t.Errorf("the statement binds %d values, wanted 2", len(answered.Params))
	}
}

// The inline form is for the planner and the display, which need a statement with no
// placeholder in it. A value still has to be written so the server reads it as one value.
func TestInlineQueryParametersWritesTheValueSafely(t *testing.T) {
	written, err := statement.InlineQueryParameters(
		"select * from orders where customer = :name",
		map[string]any{"name": "o'brien"}, postgres.Dialect)
	if err != nil {
		t.Fatalf("the inline answered %v", err)
	}
	if strings.Contains(written, ":name") {
		t.Errorf("the mark is still in the statement:\n%s", written)
	}
	// The quote inside the value is doubled, so the value ends where it should.
	if !strings.Contains(written, "''") {
		t.Errorf("the quote inside the value was not doubled:\n%s", written)
	}
}

// The form the user fills in holds the marks of the statement, and keeps the values already
// given so a second run does not ask again.
func TestResolveParameterValuesKeepsWhatIsStillAskedFor(t *testing.T) {
	held := statement.ResolveParameterValues(
		[]string{"id", "name"},
		map[string]any{"id": 7, "gone": "dropped"})

	if held["id"] != 7 {
		t.Errorf("the value already given reads %v, wanted it kept", held["id"])
	}
	if _, there := held["gone"]; there {
		t.Error("a value for a mark no longer in the statement was kept")
	}
	if _, there := held["name"]; !there {
		t.Error("a mark with no value yet is not in the form")
	}
}

// The form is JSON, so a number stays a number and a null stays a null: a value read back as
// text would be bound as text and the server would compare the wrong types.
func TestReadParameterFormKeepsTheTypeOfEachValue(t *testing.T) {
	held, err := statement.ReadParameterForm(`{"id": 7, "name": "ada", "paid": null, "ok": true}`)
	if err != nil {
		t.Fatalf("the form answered %v", err)
	}
	if held["id"] != float64(7) {
		t.Errorf("the number reads %#v", held["id"])
	}
	if held["name"] != "ada" {
		t.Errorf("the text reads %#v", held["name"])
	}
	if held["paid"] != nil {
		t.Errorf("the null reads %#v", held["paid"])
	}
	if held["ok"] != true {
		t.Errorf("the flag reads %#v", held["ok"])
	}
}

// The names of the marks are matched without case, so the form keys them in lower case.
func TestReadParameterFormLowersTheNames(t *testing.T) {
	held, err := statement.ReadParameterForm(`{"ID": 7}`)
	if err != nil {
		t.Fatalf("the form answered %v", err)
	}
	if held["id"] != float64(7) {
		t.Errorf("the form reads %v, wanted the name in lower case", held)
	}
}

func TestReadParameterFormRefusesWhatIsNotAnObject(t *testing.T) {
	for _, text := range []string{"", "null", "[]", "7", `"ada"`, "{not json"} {
		if _, err := statement.ReadParameterForm(text); err == nil {
			t.Errorf("%q was read as a form of values", text)
		}
	}
}

// A new row keeps the case of its column names, because a column is named as the relation
// names it and a server may be told to care.
func TestReadRowFormKeepsTheCaseOfTheColumns(t *testing.T) {
	held, err := statement.ReadRowForm(`{"Customer": "ada"}`)
	if err != nil {
		t.Fatalf("the form answered %v", err)
	}
	if _, there := held["Customer"]; !there {
		t.Errorf("the row reads %v, wanted the column named as it was written", held)
	}
}

// The form the user is shown holds every mark, so nothing is filled in blind.
func TestBuildParameterFormHoldsEveryMark(t *testing.T) {
	written := statement.BuildParameterForm([]string{"id", "name"}, map[string]any{"id": 7})
	for _, name := range []string{"id", "name"} {
		if !strings.Contains(written, name) {
			t.Errorf("the form does not hold %q:\n%s", name, written)
		}
	}
	// The form reads back, so what it shows can be edited and used.
	held, err := statement.ReadParameterForm(written)
	if err != nil {
		t.Fatalf("the form it built does not read back: %v\n%s", err, written)
	}
	if held["id"] != float64(7) {
		t.Errorf("the value already given reads %#v", held["id"])
	}
}
