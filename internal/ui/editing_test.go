package ui

import "testing"

func TestOpenLineWithIndent(t *testing.T) {
	cases := []struct {
		text   string
		caret  int
		wanted string
		at     int
	}{
		{"select 1", 8, "select 1\n", 9},
		{"  select 1", 10, "  select 1\n  ", 13},
		{"select (", 8, "select (\n  ", 11},
		{"select ()", 8, "select (\n  \n)", 11},
		{"\tselect", 7, "\tselect\n\t", 9},
	}
	for _, held := range cases {
		written, caret := OpenLineWithIndent(held.text, held.caret)
		if written != held.wanted || caret != held.at {
			t.Errorf("%q at %d gave %q at %d, wanted %q at %d",
				held.text, held.caret, written, caret, held.wanted, held.at)
		}
	}
}

func TestPlanIndentGuides(t *testing.T) {
	planned := PlanIndentGuides([]string{"select", "  a,", "", "  b", "from t"})
	wanted := [][]int{{}, {0}, {0}, {0}, {}}
	if len(planned) != len(wanted) {
		t.Fatalf("planned %d rows, wanted %d", len(planned), len(wanted))
	}
	for at := range wanted {
		if len(planned[at]) != len(wanted[at]) {
			t.Errorf("row %d gave %v, wanted %v", at, planned[at], wanted[at])
		}
	}
}

func TestFindBracketPair(t *testing.T) {
	pair, found := FindBracketPair("count(*)", 5, nil)
	if !found || pair.Open != 5 || pair.Close != 7 {
		t.Errorf("gave %+v %v, wanted 5 and 7", pair, found)
	}
	if _, found := FindBracketPair("count(*", 5, nil); found {
		t.Error("an unclosed bracket was paired")
	}
	if _, found := FindBracketPair("'('", 1, func(at int) bool { return at >= 0 && at < 3 }); found {
		t.Error("a bracket inside a string was paired")
	}
}
