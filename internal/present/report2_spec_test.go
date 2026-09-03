package present_test

import (
	"strings"
	"testing"

	"github.com/turanmahmudov/masume/internal/present"
	"github.com/turanmahmudov/masume/internal/query/editor"
)

func TestFormatResultSizeNeverClaimsMoreThanTheClientKnows(t *testing.T) {
	// A partial read is marked, so a count is never read as the whole table before the
	// user requests the total.
	for _, held := range []struct {
		name      string
		shown     int
		truncated bool
		total     int64
		hasTotal  bool
		want      string
	}{
		{"a whole result", 3, false, 0, false, "3 rows"},
		{"one row", 1, false, 0, false, "1 row"},
		{"no rows", 0, false, 0, false, "0 rows"},
		{"a read that stopped early", 500, true, 0, false, "500+ rows"},
		{"a page of a counted result", 500, true, 12345, true, "500 of 12,345 rows"},
		{"a counted result the page holds whole", 3, false, 3, true, "3 rows"},
		{"a count below what was shown", 5, false, 3, true, "5 rows"},
	} {
		t.Run(held.name, func(t *testing.T) {
			got := present.FormatResultSize(held.shown, held.truncated, held.total, held.hasTotal)
			if got != held.want {
				t.Errorf("got %q, want %q", got, held.want)
			}
		})
	}
}

func TestCompactJSONPutsADocumentBackOnOneLine(t *testing.T) {
	for _, held := range []struct {
		name string
		text string
		want string
		is   bool
	}{
		{"an object", "{\n  \"a\": 1\n}", `{"a":1}`, true},
		{"a list", "[\n  1,\n  2\n]", "[1,2]", true},
		{"a number", "12", "12", true},
		{"text", `"ada"`, `"ada"`, true},
		{"a boolean", "true", "true", true},
		{"a null", "null", "null", true},
		{"what is not a document", "not json", "", false},
		{"nothing", "", "", false},
		{"blanks", "   ", "", false},
		{"a broken document", `{"a":`, "", false},
	} {
		t.Run(held.name, func(t *testing.T) {
			got, is := present.CompactJSON(held.text)
			if is != held.is {
				t.Fatalf("read as JSON = %v, want %v", is, held.is)
			}
			if is && got != held.want {
				t.Errorf("got %q, want %q", got, held.want)
			}
		})
	}
}

func TestFormatForViewerIndentsADocumentAndLeavesTheRestAlone(t *testing.T) {
	for _, held := range []struct {
		name     string
		value    any
		dataType string
		want     string
	}{
		{"a document column", `{"a":1}`, "json", "{\n  \"a\": 1\n}"},
		{"a text column holding a document", `{"a":1}`, "text", `{"a":1}`},
		{"plain text", "ada", "text", "ada"},
		{"a number", int64(7), "integer", "7"},
		{"nothing", nil, "text", "NULL"},
		{"a document column holding what is not a document", "not json", "json", "not json"},
	} {
		t.Run(held.name, func(t *testing.T) {
			got := present.FormatForViewer(held.value, held.dataType)
			if got != held.want {
				t.Errorf("got %q, want %q", got, held.want)
			}
		})
	}
}

func TestAlignCardHeightLeavesAnEvenNumberOfRowsAboveAndBelow(t *testing.T) {
	// A card is centred, so an odd gap cannot be split. The extra half row would move
	// the last row of the card off the screen, so the card takes that row.
	for _, held := range []struct {
		name      string
		height    int
		available int
		want      int
	}{
		{"a gap that shares already", 10, 30, 10},
		{"an odd gap on a card that fits", 11, 30, 12},
		{"an odd gap on a card taller than the pane", 31, 30, 30},
		{"a card exactly as tall", 30, 30, 30},
		{"a card of nothing", 0, 30, 0},
	} {
		t.Run(held.name, func(t *testing.T) {
			got := present.AlignCardHeight(held.height, held.available)
			if got != held.want {
				t.Errorf("AlignCardHeight(%d, %d) = %d, want %d",
					held.height, held.available, got, held.want)
			}
			if (held.available-got)%2 != 0 {
				t.Errorf("a height of %d leaves an odd gap in %d", got, held.available)
			}
		})
	}
}

func TestResolveLineSpanPointsAtTheLineAndColumnOfAFault(t *testing.T) {
	const text = "select a\nfrom t\nwhere b"
	for _, held := range []struct {
		name  string
		start int
		end   int
		line  int
	}{
		{"the first line", 0, 6, 0},
		{"the second line", 9, 13, 1},
		{"the third line", 16, 21, 2},
		{"a span over two lines is marked on the first", 7, 12, 0},
		{"the very end", 22, 23, 2},
	} {
		t.Run(held.name, func(t *testing.T) {
			span := present.ResolveLineSpan(text,
				editor.Diagnostic{Start: held.start, End: held.end})
			if span.Line != held.line {
				t.Errorf("Line = %d, want %d", span.Line, held.line)
			}
		})
	}
}

func TestResolveLineSpanAlwaysCoversAtLeastOneCell(t *testing.T) {
	// The span is a column range on its line. An error of zero width must still be
	// visible, so it covers the cell at its position.
	const text = "select a"
	for _, held := range []struct {
		name  string
		start int
		end   int
	}{
		{"a fault of no width", 3, 3},
		{"an end before the start", 5, 2},
		{"before the start of the text", -5, -1},
		{"past the end of the text", 500, 505},
	} {
		t.Run(held.name, func(t *testing.T) {
			span := present.ResolveLineSpan(text,
				editor.Diagnostic{Start: held.start, End: held.end})
			if span.End <= span.Start {
				t.Errorf("span %+v covers no cell", span)
			}
			if span.Line < 0 {
				t.Errorf("span %+v points at no line", span)
			}
		})
	}
}

func TestPlanDetailColumnsFitsTheColumnsToThePane(t *testing.T) {
	headers := []string{"name", "type", "definition"}
	rows := [][]string{
		{"id", "integer", "generated always as identity"},
		{"customer", "text", "not null"},
	}
	for _, available := range []int{20, 40, 80, 200} {
		widths := present.PlanDetailColumns(headers, rows, available, 1)
		if len(widths) != len(headers) {
			t.Fatalf("got %d widths, want %d", len(widths), len(headers))
		}
		spent := 0
		for _, width := range widths {
			if width < 1 {
				t.Errorf("width %v holds a column of nothing at %d", widths, available)
			}
			spent += width + 1
		}
		if spent-1 > available && available >= 40 {
			t.Errorf("at %d the columns spend %d: %v", available, spent-1, widths)
		}
	}
}

func TestPlanDetailColumnsTakesTheExtraWidthFromTheWidestColumn(t *testing.T) {
	// A name column keeps its names and a definition column shrinks.
	headers := []string{"name", "definition"}
	rows := [][]string{{"id", strings.Repeat("x", 200)}}
	widths := present.PlanDetailColumns(headers, rows, 40, 1)
	if widths[0] >= widths[1] {
		t.Errorf("the name column is %d and the definition %d, want the definition wider",
			widths[0], widths[1])
	}
}

func TestPlanDetailColumnsAnswersNothingForNoColumns(t *testing.T) {
	if widths := present.PlanDetailColumns(nil, nil, 40, 1); len(widths) != 0 {
		t.Errorf("got %v, want nothing", widths)
	}
}

func TestMatchesSubsequenceFindsTheTypedLettersInOrder(t *testing.T) {
	for _, held := range []struct {
		name      string
		candidate string
		needle    string
		want      bool
	}{
		{"an empty term matches everything", "tenant_88231", "", true},
		{"letters spread through the name", "tenant_88231", "t88", true},
		{"the whole name", "orders", "orders", true},
		{"any case", "Orders", "ORD", true},
		{"the letters out of order", "orders", "sred", false},
		{"a letter that is not there", "orders", "ordz", false},
		{"a term longer than the name", "id", "identity", false},
		{"an empty candidate", "", "a", false},
	} {
		t.Run(held.name, func(t *testing.T) {
			got := present.MatchesSubsequence(held.candidate, held.needle)
			if got != held.want {
				t.Errorf("MatchesSubsequence(%q, %q) = %v, want %v",
					held.candidate, held.needle, got, held.want)
			}
		})
	}
}

func TestDescribeScreenFilterExplainsEveryHiddenRow(t *testing.T) {
	columns := []string{"id", "status"}
	for _, held := range []struct {
		name   string
		build  func() present.ScreenFilter
		want   string
		anyOf  []string
		exact  bool
		search string
	}{
		{"nothing hidden", func() present.ScreenFilter {
			return present.NoScreenFilter()
		}, "", nil, true, ""},
		{"one value of one column", func() present.ScreenFilter {
			return present.ApplyValueFilter(present.NoScreenFilter(), 1,
				map[string]bool{"new": true}, 3)
		}, "showing status = new", nil, true, ""},
		{"a search alone", func() present.ScreenFilter {
			return present.ApplySearchTerm(present.NoScreenFilter(), "ada")
		}, "rows matching ada", nil, true, ""},
		{"a value and a search", func() present.ScreenFilter {
			held := present.ApplyValueFilter(present.NoScreenFilter(), 1,
				map[string]bool{"new": true}, 3)
			return present.ApplySearchTerm(held, "ada")
		}, "showing status = new · rows matching ada", nil, true, ""},
		{"a column the names do not reach", func() present.ScreenFilter {
			return present.ApplyValueFilter(present.NoScreenFilter(), 7,
				map[string]bool{"x": true}, 3)
		}, "showing column 8 = x", nil, true, ""},
	} {
		t.Run(held.name, func(t *testing.T) {
			got := present.DescribeScreenFilter(held.build(), columns)
			if got != held.want {
				t.Errorf("got %q, want %q", got, held.want)
			}
		})
	}
}

func TestDescribeScreenFilterCountsTheValuesWhereThereIsMoreThanOne(t *testing.T) {
	filter := present.ApplyValueFilter(present.NoScreenFilter(), 1,
		map[string]bool{"new": true, "paid": true}, 5)
	got := present.DescribeScreenFilter(filter, []string{"id", "status"})
	if got != "showing status of 2" {
		t.Errorf("got %q, want %q", got, "showing status of 2")
	}
}

func TestDescribeScreenFilterNamesTheColumnsInOrder(t *testing.T) {
	filter := present.ApplyValueFilter(present.NoScreenFilter(), 1,
		map[string]bool{"new": true}, 3)
	filter = present.ApplyValueFilter(filter, 0, map[string]bool{"7": true}, 3)
	got := present.DescribeScreenFilter(filter, []string{"id", "status"})
	if got != "showing id = 7 · status = new" {
		t.Errorf("got %q, want the columns in order", got)
	}
}
