package statement_test

import (
	"testing"

	"github.com/turanmahmudov/masume/internal/core"
	"github.com/turanmahmudov/masume/internal/db/postgres"
	"github.com/turanmahmudov/masume/internal/db/sqlserver"
	"github.com/turanmahmudov/masume/internal/query/statement"
	"github.com/turanmahmudov/masume/internal/query/syntax"
)

func TestSplitStatementCutsAroundTheOrderByAndTheTrailingClauses(t *testing.T) {
	for _, held := range []struct {
		name       string
		sql        string
		body       string
		orderBy    string
		tail       string
		terminator string
	}{
		{"no clause of either kind", "select * from users",
			"select * from users", "", "", ""},
		{"an order by alone", "select * from t order by 1",
			"select * from t", "order by 1", "", ""},
		{"a limit alone", "select * from t limit 5",
			"select * from t", "", "limit 5", ""},
		{"an offset alone", "select * from t offset 10",
			"select * from t", "", "offset 10", ""},
		{"a fetch clause", "select * from t fetch first 10 rows only",
			"select * from t", "", "fetch first 10 rows only", ""},
		{"an order by before the paging clauses",
			"select * from a join b on a.id=b.id order by a.id limit 10 offset 5",
			"select * from a join b on a.id=b.id", "order by a.id", "limit 10 offset 5", ""},
		{"several sort keys", "select a, b from t where a = 1 and b = 2 order by a desc, b asc",
			"select a, b from t where a = 1 and b = 2", "order by a desc, b asc", "", ""},
		{"a terminator", "select * from t order by a;",
			"select * from t", "order by a", "", ";"},
		{"blanks after the terminator", "select * from t;   ",
			"select * from t", "", "", ";"},
		{"nothing", "", "", "", "", ""},
	} {
		t.Run(held.name, func(t *testing.T) {
			parts := statement.SplitStatement(held.sql, syntax.FlavourStandard)
			if parts.Body != held.body {
				t.Errorf("Body = %q, want %q", parts.Body, held.body)
			}
			if parts.OrderBy != held.orderBy {
				t.Errorf("OrderBy = %q, want %q", parts.OrderBy, held.orderBy)
			}
			if parts.HasOrderBy != (held.orderBy != "") {
				t.Errorf("HasOrderBy = %v, want %v", parts.HasOrderBy, held.orderBy != "")
			}
			if parts.Tail != held.tail {
				t.Errorf("Tail = %q, want %q", parts.Tail, held.tail)
			}
			if parts.Terminator != held.terminator {
				t.Errorf("Terminator = %q, want %q", parts.Terminator, held.terminator)
			}
		})
	}
}

func TestSplitStatementLeavesAKeywordInsideALiteralAlone(t *testing.T) {
	// Only a top-level keyword cuts the statement. One inside a literal, a comment or a
	// bracket is part of the body.
	for _, sql := range []string{
		"select 'order by a' from t",
		"select 1 /* order by a */",
		"select * from (select 1 order by a) x",
		"select 1;\n-- trailing",
	} {
		parts := statement.SplitStatement(sql, syntax.FlavourStandard)
		if parts.OrderBy != "" || parts.Tail != "" {
			t.Errorf("statement.SplitStatement(%q) cut at OrderBy=%q Tail=%q",
				sql, parts.OrderBy, parts.Tail)
		}
	}
}

func TestFindOrderByColumnsReadsPlainColumnsOnly(t *testing.T) {
	for _, held := range []struct {
		name string
		sql  string
		want []core.SortState
	}{
		{"one column", "select * from t order by a",
			[]core.SortState{{Column: "a", Direction: core.SortAscending}}},
		{"a direction written out", "select * from t order by a desc",
			[]core.SortState{{Column: "a", Direction: core.SortDescending}}},
		{"two keys with their own directions",
			"select a, b from t where a = 1 and b = 2 order by a desc, b asc",
			[]core.SortState{
				{Column: "a", Direction: core.SortDescending},
				{Column: "b", Direction: core.SortAscending},
			}},
		{"a quoted name", `select * from t order by "Odd Col"`,
			[]core.SortState{{Column: "Odd Col", Direction: core.SortAscending}}},
		{"no order by", "select * from t", nil},
		{"an ordinal is not a column", "select * from t order by 1", nil},
		{"a function is not a column", "select * from t order by lower(a)", nil},
		{"a qualified name is not read", "select * from t order by t.a", nil},
		{"a nulls clause is not read", "select * from t order by a nulls last", nil},
	} {
		t.Run(held.name, func(t *testing.T) {
			got := statement.FindOrderByColumns(held.sql, syntax.FlavourStandard)
			if len(got) != len(held.want) {
				t.Fatalf("statement.FindOrderByColumns(%q) = %v, want %v", held.sql, got, held.want)
			}
			for at, key := range got {
				if key != held.want[at] {
					t.Errorf("key %d = %v, want %v", at, key, held.want[at])
				}
			}
		})
	}
}

func TestApplyPagingWritesTheWindowOnAStatementThatHasNoneOfItsOwn(t *testing.T) {
	for _, held := range []struct {
		name   string
		sql    string
		limit  int
		offset int
		want   string
	}{
		{"a plain select", "select * from users", 10, 20,
			"select * from users\nlimit 10 offset 20"},
		{"the first page writes no offset", "select * from users", 10, 0,
			"select * from users\nlimit 10"},
		{"the order by stays above the window", "select * from t order by a", 10, 20,
			"select * from t\norder by a\nlimit 10 offset 20"},
		{"a terminator stays last", "select * from t;", 10, 0,
			"select * from t\nlimit 10;"},
	} {
		t.Run(held.name, func(t *testing.T) {
			got := statement.ApplyPaging(held.sql, held.limit, held.offset, postgres.Dialect)
			if got != held.want {
				t.Errorf("statement.ApplyPaging(%q, %d, %d)\n got %q\nwant %q",
					held.sql, held.limit, held.offset, got, held.want)
			}
		})
	}
}

func TestApplyPagingWrapsAStatementThatAlreadyPagesItself(t *testing.T) {
	// A second LIMIT beside the first would fight it, so the statement goes inside a
	// subquery and the window is laid over that.
	for _, held := range []struct {
		name string
		sql  string
		want string
	}{
		{"a limit of its own", "select * from t limit 5",
			"select * from (\nselect * from t\nlimit 5\n) as masume_page\nlimit 10 offset 20"},
		{"an offset of its own", "select * from t offset 10",
			"select * from (\nselect * from t\noffset 10\n) as masume_page\nlimit 10 offset 20"},
		{"a fetch clause of its own", "select * from t fetch first 10 rows only",
			"select * from (\nselect * from t\nfetch first 10 rows only\n) as masume_page\n" +
				"limit 10 offset 20"},
		{"an order by is kept inside the wrapper",
			"select * from a join b on a.id=b.id order by a.id limit 10 offset 5",
			"select * from (\nselect * from a join b on a.id=b.id\norder by a.id\n" +
				"limit 10 offset 5\n) as masume_page\nlimit 10 offset 20"},
	} {
		t.Run(held.name, func(t *testing.T) {
			got := statement.ApplyPaging(held.sql, 10, 20, postgres.Dialect)
			if got != held.want {
				t.Errorf("statement.ApplyPaging(%q)\n got %q\nwant %q", held.sql, got, held.want)
			}
		})
	}
}

func TestApplyPagingLeavesAStatementWithNoBodyAlone(t *testing.T) {
	for _, sql := range []string{"", "   ", ";"} {
		if got := statement.ApplyPaging(sql, 10, 20, postgres.Dialect); got != sql {
			t.Errorf("statement.ApplyPaging(%q) = %q, want it unchanged", sql, got)
		}
	}
}

// A SQL Server takes a page with OFFSET and FETCH, and it reads that window after a sort
// only. A read with no sort of its own gets one that keeps the rows as the server reads them.
func TestApplyPagingWritesTheWindowASqlServerReads(t *testing.T) {
	for _, held := range []struct {
		name   string
		sql    string
		limit  int
		offset int
		want   string
	}{
		{"a read with no sort", "select * from [dbo].[users]", 10, 20,
			"select * from [dbo].[users]\norder by (select null)\n" +
				"offset 20 rows fetch next 10 rows only"},
		// The first page takes no window at all: the client caps the rows it reads, and a
		// statement that reads the next value of a sequence takes no sort.
		{"the first page is left alone", "select next value for [dbo].[order_seq]", 10, 0,
			"select next value for [dbo].[order_seq]"},
		{"the sort of the read stays above the window", "select * from t order by a", 10, 20,
			"select * from t\norder by a\noffset 20 rows fetch next 10 rows only"},
		{"a window of its own goes inside a subquery",
			"select * from t order by a offset 5 rows fetch next 5 rows only", 10, 20,
			"select * from (\nselect * from t\norder by a\n" +
				"offset 5 rows fetch next 5 rows only\n) as masume_page\n" +
				"order by (select null)\noffset 20 rows fetch next 10 rows only"},
	} {
		t.Run(held.name, func(t *testing.T) {
			got := statement.ApplyPaging(held.sql, held.limit, held.offset, sqlserver.Dialect)
			if got != held.want {
				t.Errorf("statement.ApplyPaging(%q, %d, %d)\n got %q\nwant %q",
					held.sql, held.limit, held.offset, got, held.want)
			}
		})
	}
}

// A subquery of a SQL Server takes a sort together with a window, so the filter wrapper
// writes one that changes no row.
func TestApplyWhereKeepsTheSortLegalInsideTheWrapperOfASqlServer(t *testing.T) {
	got := statement.ApplyWhere("select a from t order by b", "a = 1", sqlserver.Dialect)
	want := "select * from (\nselect a from t\norder by b offset 0 rows\n" +
		") as masume_filter\nwhere a = 1"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

// A count of a read that pages itself keeps the window, and the window needs the sort.
func TestBuildCountSQLWritesTheSortASqlServerNeeds(t *testing.T) {
	got := statement.BuildCountSQL(
		"select * from t order by a offset 5 rows fetch next 5 rows only", sqlserver.Dialect)
	want := "select count_big(*) as total from (\nselect * from t\n" +
		"order by (select null)\noffset 5 rows fetch next 5 rows only\n) as masume_count"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestApplyOrderByWritesTheKeysInTheOrderTheyWereAdded(t *testing.T) {
	sort := []core.SortState{
		{Column: "a", Direction: core.SortDescending},
		{Column: "Odd Col", Direction: core.SortAscending},
	}
	got := statement.ApplyOrderBy("select * from t", sort, postgres.Dialect)
	want := "select * from t\norder by \"a\" desc, \"Odd Col\" asc"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestApplyOrderByReplacesTheOrderByTheStatementWrites(t *testing.T) {
	sort := []core.SortState{{Column: "b", Direction: core.SortAscending}}
	got := statement.ApplyOrderBy("select * from t order by a desc", sort, postgres.Dialect)
	if want := "select * from t\norder by \"b\" asc"; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestApplyOrderByWithNoKeysTakesTheOrderByAway(t *testing.T) {
	got := statement.ApplyOrderBy("select * from t order by a desc", nil, postgres.Dialect)
	if want := "select * from t"; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestBuildCountSQLDropsTheOrderByAndKeepsTheTrailingClauses(t *testing.T) {
	for _, held := range []struct {
		name string
		sql  string
		want string
	}{
		{"a plain select", "select * from users",
			"select count(*)::int8 as total from (\nselect * from users\n) as masume_count"},
		{"the order by is dropped", "select * from t order by 1",
			"select count(*)::int8 as total from (\nselect * from t\n) as masume_count"},
		{"a limit is kept, so the count matches the request", "select * from t limit 5",
			"select count(*)::int8 as total from (\nselect * from t\nlimit 5\n) as masume_count"},
		{"an order by is dropped and the paging kept",
			"select * from a join b on a.id=b.id order by a.id limit 10 offset 5",
			"select count(*)::int8 as total from (\nselect * from a join b on a.id=b.id\n" +
				"limit 10 offset 5\n) as masume_count"},
	} {
		t.Run(held.name, func(t *testing.T) {
			got := statement.BuildCountSQL(held.sql, postgres.Dialect)
			if got != held.want {
				t.Errorf("statement.BuildCountSQL(%q)\n got %q\nwant %q", held.sql, got, held.want)
			}
		})
	}
}

func TestBuildCountSQLLeavesAStatementWithNoBodyAlone(t *testing.T) {
	for _, sql := range []string{"", "   "} {
		if got := statement.BuildCountSQL(sql, postgres.Dialect); got != sql {
			t.Errorf("statement.BuildCountSQL(%q) = %q, want it unchanged", sql, got)
		}
	}
}

func TestApplyWhereSearchesTheWholeStatementAndPagesTheAnswer(t *testing.T) {
	// A predicate laid inside the LIMIT would search only the rows already fetched, so the
	// body goes in the wrapper alone and the LIMIT is written again outside it.
	got := statement.ApplyWhere("select * from t limit 5", "a = 1", postgres.Dialect)
	want := "select * from (\nselect * from t\n) as masume_filter\nwhere a = 1\nlimit 5"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestApplyWhereKeepsTheOrderByInsideTheWrapper(t *testing.T) {
	// The ORDER BY can name a column the projection does not return, so it stays with the
	// body it belongs to.
	got := statement.ApplyWhere("select a from t order by b", "a = 1", postgres.Dialect)
	want := "select * from (\nselect a from t\norder by b\n) as masume_filter\nwhere a = 1"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestApplyWhereWithNoPredicateLeavesTheStatementAlone(t *testing.T) {
	for _, predicate := range []string{"", "   "} {
		got := statement.ApplyWhere("select * from t", predicate, postgres.Dialect)
		if want := "select * from t"; got != want {
			t.Errorf("ApplyWhere with %q gave %q, want %q", predicate, got, want)
		}
	}
}

func TestDescribeRewriteNamesTheSortAndTheFilterLaidOver(t *testing.T) {
	sort := []core.SortState{
		{Column: "a", Direction: core.SortDescending},
		{Column: "b", Direction: core.SortAscending},
	}
	for _, held := range []struct {
		name   string
		sort   []core.SortState
		filter string
		want   string
	}{
		{"both", sort, "x = 1", "order by a desc, b asc · where x = 1"},
		{"a sort alone", sort, "", "order by a desc, b asc"},
		{"a filter alone", nil, "x = 1", "where x = 1"},
		{"a filter of blanks", nil, "   ", ""},
		{"neither", nil, "", ""},
	} {
		t.Run(held.name, func(t *testing.T) {
			if got := statement.DescribeRewrite(held.sort, held.filter); got != held.want {
				t.Errorf("got %q, want %q", got, held.want)
			}
		})
	}
}
