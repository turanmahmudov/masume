package engines_test

import (
	"strings"
	"testing"

	"github.com/turanmahmudov/masume/internal/core"
	"github.com/turanmahmudov/masume/internal/db"
	"github.com/turanmahmudov/masume/internal/db/engines"
	"github.com/turanmahmudov/masume/internal/db/mongo"
	"github.com/turanmahmudov/masume/internal/db/postgres"
	"github.com/turanmahmudov/masume/internal/db/redis"
	"github.com/turanmahmudov/masume/internal/query"
)

var ordersTable = db.TableRef{Schema: "public", Name: "orders", Kind: db.RelationTable}

func composerOf(dialect *query.Dialect) db.Composer {
	return engines.ResolveSupport(dialect.Engine).Compose
}

func TestComposeRelationReadWritesTheSortIntoTheStatement(t *testing.T) {
	read := composerOf(postgres.Dialect).ComposeRelationRead(
		ordersTable, core.ReadRewrite{
			Sort: []core.SortState{
				{Column: "customer", Direction: core.SortAscending},
				{Column: "total", Direction: core.SortDescending},
			},
		})

	written := strings.ToLower(read.Text)
	if !strings.Contains(written, "order by") {
		t.Fatalf("the read holds no order:\n%s", read.Text)
	}
	if strings.Index(written, "customer") > strings.Index(written, "total") {
		t.Errorf("the columns of the sort are the wrong way round:\n%s", read.Text)
	}
	if !strings.Contains(written, "desc") {
		t.Errorf("the direction of the second column was lost:\n%s", read.Text)
	}
	if !read.Pageable {
		t.Error("a read of one relation reads as one that cannot be paged")
	}
}

func TestComposeRelationReadBindsTheValueOfAFilter(t *testing.T) {
	read := composerOf(postgres.Dialect).ComposeRelationRead(
		ordersTable, core.ReadRewrite{
			Filter: []core.FilterStep{{
				Kind: core.FilterCompare, Column: "customer",
				Test: core.FilterEquals, Value: "'; drop table orders --",
			}},
		})

	if strings.Contains(read.Text, "drop table") {
		t.Errorf("the value was written into the statement:\n%s", read.Text)
	}
	if len(read.Params) != 1 {
		t.Fatalf("the read binds %d values, wanted the one of the filter", len(read.Params))
	}
	if read.Params[0] != "'; drop table orders --" {
		t.Errorf("the bound value reads %v", read.Params[0])
	}
	if !strings.Contains(strings.ToLower(read.Text), "where") {
		t.Errorf("the read holds no where clause:\n%s", read.Text)
	}
}

func TestComposeRelationReadShowsTheValueAndRunsTheBoundForm(t *testing.T) {
	read := composerOf(postgres.Dialect).ComposeRelationRead(
		ordersTable, core.ReadRewrite{
			Filter: []core.FilterStep{{
				Kind: core.FilterCompare, Column: "customer",
				Test: core.FilterEquals, Value: "ada",
			}},
		})

	if !strings.Contains(read.Display, "ada") {
		t.Errorf("the display does not show the value:\n%s", read.Display)
	}
	if strings.Contains(read.Text, "ada") {
		t.Errorf("the form that runs holds the value:\n%s", read.Text)
	}
}

func TestComposeRelationReadBindsNothingForATestWithNoValue(t *testing.T) {
	for _, test := range []core.FilterTest{core.FilterIsNull, core.FilterIsNotNull} {
		read := composerOf(postgres.Dialect).ComposeRelationRead(
			ordersTable, core.ReadRewrite{
				Filter: []core.FilterStep{{
					Kind: core.FilterCompare, Column: "total", Test: test,
				}},
			})
		if len(read.Params) != 0 {
			t.Errorf("%q bound %d values, wanted none", test, len(read.Params))
		}
		if !strings.Contains(strings.ToLower(read.Text), "null") {
			t.Errorf("%q wrote no null test:\n%s", test, read.Text)
		}
	}
}

// A statement the user wrote is wrapped, not merged, because its own WHERE, GROUP BY or LIMIT
// would change meaning if a filter were added to it.
func TestComposeStatementReadWrapsTheStatementOfTheUser(t *testing.T) {
	written := db.BoundText{Text: "select customer, count(*) from orders group by customer"}
	read := composerOf(postgres.Dialect).ComposeStatementRead(
		written, core.ReadRewrite{
			Filter: []core.FilterStep{{
				Kind: core.FilterCompare, Column: "customer",
				Test: core.FilterEquals, Value: "ada",
			}},
		})

	if !strings.Contains(read.Text, "group by") {
		t.Errorf("the statement of the user was not kept whole:\n%s", read.Text)
	}
	// The filter is laid outside the statement, so the group by still groups every row.
	if strings.Index(strings.ToLower(read.Text), "group by") >
		strings.LastIndex(strings.ToLower(read.Text), "where") {
		t.Errorf("the filter was written inside the statement:\n%s", read.Text)
	}
}

// The statement binds its own values first, so a filter numbers its marks after them.
func TestComposeStatementReadNumbersTheFilterAfterTheStatement(t *testing.T) {
	written := db.BoundText{
		Text:   "select * from orders where id = $1",
		Params: []any{7},
	}
	read := composerOf(postgres.Dialect).ComposeStatementRead(
		written, core.ReadRewrite{
			Filter: []core.FilterStep{{
				Kind: core.FilterCompare, Column: "customer",
				Test: core.FilterEquals, Value: "ada",
			}},
		})

	if len(read.Params) != 2 {
		t.Fatalf("the read binds %d values, wanted the one of the statement and the filter",
			len(read.Params))
	}
	if read.Params[0] != 7 || read.Params[1] != "ada" {
		t.Errorf("the bound values read %v, wanted them in that order", read.Params)
	}
	if !strings.Contains(read.Text, "$2") {
		t.Errorf("the filter did not take the number after the statement:\n%s", read.Text)
	}
}

func TestComposeRelationReadWritesACommandForAKeyStore(t *testing.T) {
	read := composerOf(redis.Dialect).ComposeRelationRead(
		db.TableRef{Schema: "0", Name: "order", Kind: db.RelationTable}, core.ReadRewrite{})

	if strings.Contains(strings.ToLower(read.Text), "select") {
		t.Errorf("a key store was asked in SQL:\n%s", read.Text)
	}
	if read.Text == "" {
		t.Error("the read of a key store is empty")
	}
}

func TestComposeRelationReadWritesAFindForMongo(t *testing.T) {
	read := composerOf(mongo.Dialect).ComposeRelationRead(
		db.TableRef{Schema: "shop", Name: "orders", Kind: db.RelationTable},
		core.ReadRewrite{})

	if strings.Contains(strings.ToLower(read.Text), "select") {
		t.Errorf("a collection was asked in SQL:\n%s", read.Text)
	}
	if !strings.Contains(read.Text, "orders.find") {
		t.Errorf("the read is %q, wanted a find of the collection", read.Text)
	}
}

func TestComposeStatementReadLeavesARedisCommandAsItIs(t *testing.T) {
	written := db.BoundText{Text: "GET order:1"}
	read := composerOf(redis.Dialect).ComposeStatementRead(written, core.ReadRewrite{
		Sort: []core.SortState{{Column: "key", Direction: core.SortAscending}},
	})

	if read.Text != written.Text {
		t.Errorf("the command was rewritten as %q", read.Text)
	}
	if strings.Contains(strings.ToLower(read.Text), "select") {
		t.Errorf("a command was wrapped as SQL:\n%s", read.Text)
	}
}

func TestComposeStatementReadLaysAFilterOverAMongoFind(t *testing.T) {
	read := composerOf(mongo.Dialect).ComposeStatementRead(
		db.BoundText{Text: `db.orders.find({status: "new"})`},
		core.ReadRewrite{
			Sort: []core.SortState{{Column: "total", Direction: core.SortDescending}},
		})

	if strings.Contains(strings.ToLower(read.Text), "select") {
		t.Errorf("a find was wrapped as SQL:\n%s", read.Text)
	}
	if !strings.Contains(read.Text, `.sort(`) {
		t.Errorf("the sort of the tab was lost:\n%s", read.Text)
	}
}

func TestBuildChangesFollowsTheEngine(t *testing.T) {
	for _, held := range []struct {
		name    string
		dialect *query.Dialect
		target  db.ChangeTarget
		editAt  int
		want    string
		forbid  string
	}{
		{
			name:    "sql",
			dialect: postgres.Dialect,
			target: db.ChangeTarget{
				Table:      ordersTable,
				Columns:    []db.ResultColumn{{Name: "id"}, {Name: "customer"}},
				Rows:       [][]any{{int64(1), "ada"}},
				KeyColumns: []string{"id"},
			},
			editAt: 1,
			want:   "update",
			forbid: "",
		},
		{
			name:    "mongodb",
			dialect: mongo.Dialect,
			target: db.ChangeTarget{
				Table: db.TableRef{Schema: "shop", Name: "orders", Kind: db.RelationTable},
				Columns: []db.ResultColumn{
					{Name: mongo.IdentityField, DataType: mongo.TypeObjectID},
					{Name: "note", DataType: mongo.TypeString},
				},
				Rows:       [][]any{{"507f1f77bcf86cd799439011", "late"}},
				KeyColumns: []string{mongo.IdentityField},
			},
			editAt: 1,
			want:   "update",
			forbid: "update ",
		},
		{
			name:    "redis",
			dialect: redis.Dialect,
			target: db.ChangeTarget{
				Table: db.TableRef{Schema: "0", Name: "order", Kind: db.RelationTable},
				Columns: []db.ResultColumn{
					{Name: redis.KeyColumnKey}, {Name: redis.KeyColumnType},
					{Name: redis.KeyColumnTTL}, {Name: redis.KeyColumnValue},
				},
				Rows:       [][]any{{"order:1", "string", int64(-1), "ada"}},
				KeyColumns: []string{redis.KeyColumnKey},
			},
			editAt: 3,
			want:   "order:1",
			forbid: "update",
		},
	} {
		t.Run(held.name, func(t *testing.T) {
			pending := core.NewPendingChanges()
			pending.Edits[core.BuildEditKey(0, held.editAt)] = core.CellEdit{
				RowIndex: 0, ColumnIndex: held.editAt,
				Value: core.CellValue{Kind: core.CellText, Text: "grace"},
			}
			changes, err := composerOf(held.dialect).BuildChanges(held.target, pending)
			if err != nil {
				t.Fatalf("the changes answered %v", err)
			}
			if len(changes) != 1 {
				t.Fatalf("the staged work became %d changes, wanted 1", len(changes))
			}
			written := strings.ToLower(changes[0].Display)
			if !strings.Contains(written, held.want) {
				t.Errorf("the change reads %q, wanted it to hold %q",
					changes[0].Display, held.want)
			}
			if held.forbid != "" && strings.Contains(written, held.forbid) {
				t.Errorf("the change reads %q, which is SQL", changes[0].Display)
			}
			if changes[0].Description == "" {
				t.Error("the change carries no description for the review card")
			}
		})
	}
}

func TestBuildChangesRefusesARowWithNoKey(t *testing.T) {
	target := db.ChangeTarget{
		Table:   ordersTable,
		Columns: []db.ResultColumn{{Name: "notes", DataType: "json"}},
		Rows:    [][]any{{"{}"}},
		// No key column, and the one column there is cannot be compared.
		KeyColumns: nil,
	}
	pending := core.NewPendingChanges()
	pending.DeletedRows[0] = true

	if _, err := composerOf(postgres.Dialect).BuildChanges(target, pending); err == nil {
		t.Error("a row that nothing identifies was written anyway")
	}
}

func TestFindStatementSourceReadsTheRelationOfEveryLanguage(t *testing.T) {
	for _, held := range []struct {
		name    string
		dialect *query.Dialect
		written string
		wanted  string
	}{
		{"a plain select", postgres.Dialect, "select * from orders", "orders"},
		{"a select of several relations", postgres.Dialect,
			"select * from orders join lines on lines.order_id = orders.id", ""},
		{"a mongodb find", mongo.Dialect, `db.orders.find({status: "new"})`, "orders"},
		{"a mongodb aggregation", mongo.Dialect,
			`db.orders.aggregate([{$match: {}}])`, ""},
		// A command names no relation, and the SQL reader answers none for it either.
		{"a redis command", redis.Dialect, "GET order:1", ""},
	} {
		t.Run(held.name, func(t *testing.T) {
			source, found := composerOf(held.dialect).FindStatementSource(held.written)
			if found != (held.wanted != "") {
				t.Fatalf("%q reads as editable=%v", held.written, found)
			}
			if found && source.Name != held.wanted {
				t.Errorf("%q names %q, wanted %q", held.written, source.Name, held.wanted)
			}
		})
	}
}

// SQL binds the value of a `:name` mark, so nothing the user typed is ever read as SQL. A
// server that takes a command binds nothing, so the value has to be written into the
// statement itself or the server reads the placeholder as part of the document.
func TestBuildChangesGuardsAWriteOnATableWithNoKey(t *testing.T) {
	target := db.ChangeTarget{
		Table: ordersTable,
		Columns: []db.ResultColumn{
			{Name: "id", DataType: "integer"},
			{Name: "customer", DataType: "text"},
		},
		Rows:       [][]any{{int64(1), "ada"}},
		KeyColumns: nil,
	}
	pending := core.NewPendingChanges()
	pending.Edits[core.BuildEditKey(0, 1)] = core.CellEdit{
		RowIndex: 0, ColumnIndex: 1,
		Value: core.CellValue{Kind: core.CellText, Text: "grace"},
	}

	changes, err := composerOf(postgres.Dialect).BuildChanges(target, pending)
	if err != nil {
		t.Fatalf("the changes answered %v", err)
	}
	if len(changes) != 1 {
		t.Fatalf("the staged work became %d changes, wanted 1", len(changes))
	}
	if changes[0].Guard == nil {
		t.Fatal("the update carries no count, and the table has no key")
	}
	if changes[0].Expect != 1 {
		t.Errorf("the count expects %d rows, wanted 1", changes[0].Expect)
	}
}

func TestBindStatementParametersWritesTheValueTheWayTheServerTakesIt(t *testing.T) {
	values := map[string]any{"wanted": "new"}

	held, err := composerOf(postgres.Dialect).BindParameters(
		"select * from orders where status = :wanted", values)
	if err != nil {
		t.Fatalf("SQL refused a mark it has a value for: %v", err)
	}
	if len(held.Params) != 1 || held.Params[0] != "new" {
		t.Errorf("SQL did not bind the value: %+v", held)
	}
	if strings.Contains(held.Text, "new") {
		t.Errorf("SQL wrote the value into the statement: %s", held.Text)
	}

	held, err = composerOf(mongo.Dialect).BindParameters(
		`db.orders.find({status: :wanted})`, values)
	if err != nil {
		t.Fatalf("mongodb refused a mark it has a value for: %v", err)
	}
	if len(held.Params) != 0 {
		t.Errorf("mongodb bound a value the server never reads: %+v", held)
	}
	if held.Text != `db.orders.find({status: "new"})` {
		t.Errorf("the statement reads %q", held.Text)
	}
	// The statement that comes out has to be one the engine can still read.
	if _, err := mongo.ReadStatement(held.Text); err != nil {
		t.Errorf("the bound statement no longer reads: %v", err)
	}
}

// A value the user typed is quoted by the dialect, so a quote mark in it cannot end the
// value and change the statement.
func TestBindStatementParametersQuotesWhatTheUserTyped(t *testing.T) {
	held, bindErr := composerOf(mongo.Dialect).BindParameters(
		`db.orders.find({note: :text})`, map[string]any{"text": `a"}) drop`})
	if bindErr != nil {
		t.Fatalf("mongodb refused a mark it has a value for: %v", bindErr)
	}

	parsed, err := mongo.ReadStatement(held.Text)
	if err != nil {
		t.Fatalf("the statement no longer reads: %v", err)
	}
	filter, readErr := mongo.ReadDocument(parsed.Calls[0].ReadArgument(0))
	if readErr != nil {
		t.Fatalf("the filter answered %v", readErr)
	}
	if len(filter) != 1 || filter[0].Value != `a"}) drop` {
		t.Errorf("the value reads %v", filter)
	}
}

// A mark with no value is reported, and no statement comes back. Answering with the raw
// text would run it with the mark still in it.
func TestBindStatementParametersRefuseAMarkWithNoValue(t *testing.T) {
	for _, held := range []struct {
		name string
		bind func() (db.BoundText, error)
	}{
		{"SQL", func() (db.BoundText, error) {
			return composerOf(postgres.Dialect).BindParameters(
				"select * from orders where status = :wanted", map[string]any{})
		}},
		{"mongodb", func() (db.BoundText, error) {
			return composerOf(mongo.Dialect).BindParameters(
				`db.orders.find({status: :wanted})`, map[string]any{})
		}},
	} {
		bound, err := held.bind()
		if err == nil {
			t.Errorf("%s bound a mark it had no value for: %+v", held.name, bound)
			continue
		}
		if bound.Text != "" || len(bound.Params) != 0 {
			t.Errorf("%s answered a statement beside the error: %+v", held.name, bound)
		}
	}
}
