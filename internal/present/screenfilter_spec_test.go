package present_test

import (
	"testing"

	"github.com/turanmahmudov/masume/internal/core"
	"github.com/turanmahmudov/masume/internal/present"
)

// The banner is a summary: a column with several kept values shows a count, so two different
// filters have the same banner. A caller that caches the result of a filter must see the
// difference, which is the purpose of the fingerprint.
func TestScreenFilterFingerprintTellsApartWhatTheBannerDoesNot(t *testing.T) {
	first := present.ScreenFilter{Values: map[int]map[string]bool{
		0: {"ada": true, "grace": true, "alan": true},
	}}
	second := present.ScreenFilter{Values: map[int]map[string]bool{
		0: {"ada": true, "grace": true, "held": true},
	}}

	names := []string{"customer"}
	if present.DescribeScreenFilter(first, names) != present.DescribeScreenFilter(second, names) {
		t.Skip("the banner already tells these two apart, so there is nothing to guard")
	}
	if first.Fingerprint() == second.Fingerprint() {
		t.Error("two filters keeping different values answer one fingerprint")
	}
}

// The same filter gives the same fingerprint, whatever the order of its maps, or a frame
// would redraw without a change.
func TestScreenFilterFingerprintIsTheSameForTheSameFilter(t *testing.T) {
	build := func() present.ScreenFilter {
		return present.ScreenFilter{
			Values: map[int]map[string]bool{
				0: {"ada": true, "grace": true},
				3: {"held": true},
			},
			Search: "or",
		}
	}
	first := build().Fingerprint()
	for range 20 {
		if build().Fingerprint() != first {
			t.Fatal("the same filter answered two fingerprints")
		}
	}
}

// Every part of a filter changes the fingerprint, because each one hides a different set of
// rows.
func TestScreenFilterFingerprintFollowsEveryPart(t *testing.T) {
	base := present.ScreenFilter{
		Values: map[int]map[string]bool{0: {"ada": true}},
		Search: "or",
	}

	for _, held := range []struct {
		name   string
		change func(filter *present.ScreenFilter)
	}{
		{"a value kept", func(filter *present.ScreenFilter) {
			filter.Values = map[int]map[string]bool{0: {"grace": true}}
		}},
		{"another value as well", func(filter *present.ScreenFilter) {
			filter.Values = map[int]map[string]bool{0: {"ada": true, "grace": true}}
		}},
		{"another column", func(filter *present.ScreenFilter) {
			filter.Values = map[int]map[string]bool{1: {"ada": true}}
		}},
		{"a column as well", func(filter *present.ScreenFilter) {
			filter.Values = map[int]map[string]bool{
				0: {"ada": true}, 1: {"grace": true},
			}
		}},
		{"the text searched for", func(filter *present.ScreenFilter) {
			filter.Search = "and"
		}},
		{"nothing searched for", func(filter *present.ScreenFilter) {
			filter.Search = ""
		}},
		{"no filter at all", func(filter *present.ScreenFilter) {
			*filter = present.NoScreenFilter()
		}},
	} {
		t.Run(held.name, func(t *testing.T) {
			changed := present.ScreenFilter{
				Values: map[int]map[string]bool{0: {"ada": true}}, Search: "or",
			}
			held.change(&changed)
			if changed.Fingerprint() == base.Fingerprint() {
				t.Error("the filter changed and answers the same fingerprint")
			}
		})
	}
}

func TestCountColumnValuesRanksTheMostCommonFirst(t *testing.T) {
	rows := [][]string{
		{"1", "new"},
		{"2", "paid"},
		{"3", "new"},
		{"4", "sent"},
		{"5", "new"},
		{"6", "paid"},
	}
	counted := present.CountColumnValues(rows, 1)
	want := []present.ValueCount{
		{Value: "new", Count: 3},
		{Value: "paid", Count: 2},
		{Value: "sent", Count: 1},
	}
	if len(counted) != len(want) {
		t.Fatalf("got %v, want %v", counted, want)
	}
	for at, held := range counted {
		if held != want[at] {
			t.Errorf("row %d = %v, want %v", at, held, want[at])
		}
	}
}

func TestCountColumnValuesKeepsTheOrderTheyWereFoundForATie(t *testing.T) {
	rows := [][]string{{"b"}, {"a"}, {"c"}}
	counted := present.CountColumnValues(rows, 0)
	for at, want := range []string{"b", "a", "c"} {
		if counted[at].Value != want {
			t.Errorf("row %d = %q, want %q", at, counted[at].Value, want)
		}
	}
}

func TestCountColumnValuesReadsAColumnOutsideTheRowAsEmpty(t *testing.T) {
	rows := [][]string{{"1"}, {"2"}}
	for _, columnIndex := range []int{-1, 5} {
		counted := present.CountColumnValues(rows, columnIndex)
		if len(counted) != 1 || counted[0].Value != "" || counted[0].Count != 2 {
			t.Errorf("column %d gave %v", columnIndex, counted)
		}
	}
}

func TestApplyValueFilterKeepsOnlyTheValuesChosen(t *testing.T) {
	filter := present.ApplyValueFilter(present.NoScreenFilter(), 1,
		map[string]bool{"new": true}, 3)
	if !present.IsRowShown([]string{"1", "new"}, nil, filter) {
		t.Error("a row of a kept value was hidden")
	}
	if present.IsRowShown([]string{"2", "paid"}, nil, filter) {
		t.Error("a row of a value not kept was shown")
	}
}

func TestApplyValueFilterClearsTheColumnWhereNothingIsHidden(t *testing.T) {
	// A selection of every value, or of no value, hides nothing, so the entry is removed
	// and not kept as a filter without effect.
	for _, held := range []struct {
		name      string
		kept      map[string]bool
		available int
	}{
		{"no value", map[string]bool{}, 3},
		{"every value", map[string]bool{"a": true, "b": true, "c": true}, 3},
		{"more than are there", map[string]bool{"a": true, "b": true}, 1},
	} {
		t.Run(held.name, func(t *testing.T) {
			filter := present.ApplyValueFilter(present.NoScreenFilter(), 1,
				held.kept, held.available)
			if !filter.IsEmpty() {
				t.Errorf("the filter still hides something: %v", filter.Values)
			}
		})
	}
}

func TestApplyValueFilterLeavesTheOtherColumnsAndTheSearchAlone(t *testing.T) {
	first := present.ApplyValueFilter(present.NoScreenFilter(), 0,
		map[string]bool{"1": true}, 3)
	first = present.ApplySearchTerm(first, "ada")
	second := present.ApplyValueFilter(first, 1, map[string]bool{"new": true}, 3)

	if second.Search != "ada" {
		t.Errorf("Search = %q, want it kept", second.Search)
	}
	if len(second.Values) != 2 {
		t.Errorf("the filter holds %d columns, want 2", len(second.Values))
	}
	if len(first.Values) != 1 {
		t.Error("the filter it was built from was changed")
	}
}

func TestApplySearchTermKeepsTheTermWithoutItsBlanks(t *testing.T) {
	filter := present.ApplySearchTerm(present.NoScreenFilter(), "  ada  ")
	if filter.Search != "ada" {
		t.Errorf("Search = %q, want %q", filter.Search, "ada")
	}
	if !present.IsRowShown([]string{"1", "Ada"}, nil, filter) {
		t.Error("a row holding the term was hidden")
	}
	if present.IsRowShown([]string{"2", "Grace"}, nil, filter) {
		t.Error("a row without the term was shown")
	}
}

func TestApplySearchTermWithABlankTermClearsTheSearch(t *testing.T) {
	filter := present.ApplySearchTerm(present.NoScreenFilter(), "ada")
	for _, term := range []string{"", "   "} {
		cleared := present.ApplySearchTerm(filter, term)
		if cleared.Search != "" {
			t.Errorf("term %q left Search = %q", term, cleared.Search)
		}
	}
}

// A cell with a document shows a summary, so the text the user searches for is not on the
// screen. The search reads the document itself, or a search for a city inside an address
// would find nothing.
func TestASearchReadsTheDocumentACellHolds(t *testing.T) {
	filter := present.ApplySearchTerm(present.NoScreenFilter(), "berlin")
	shape := []string{"1", "{ 2 fields }"}
	values := []any{
		int64(1),
		core.DocumentValue{Text: `{"city":"berlin","zip":"10115"}`, Count: 2},
	}

	if !present.IsRowShown(shape, values, filter) {
		t.Error("a row whose document holds the term was hidden")
	}
	if present.IsRowShown(shape, []any{
		int64(1), core.DocumentValue{Text: `{"city":"hamburg","zip":"20095"}`, Count: 2},
	}, filter) {
		t.Error("a row whose document does not hold the term was shown")
	}
	// The summary in the cell is not the text the user searches for.
	if present.IsRowShown(shape, values,
		present.ApplySearchTerm(present.NoScreenFilter(), "zzz")) {
		t.Error("a row was shown for a term nothing holds")
	}
}
