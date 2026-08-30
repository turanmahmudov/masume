package core

import "testing"

func TestApplySortColumnActsOnTheSortItIsGiven(t *testing.T) {
	statementSort := []SortState{
		{Column: "created_on", Direction: SortDescending},
		{Column: "id", Direction: SortDescending},
	}

	cases := []struct {
		name   string
		sort   []SortState
		column string
		add    bool
		wanted []SortState
	}{
		{
			name: "adding a column already in the sort turns it over in its place",
			sort: statementSort, column: "id", add: true,
			wanted: []SortState{
				{Column: "created_on", Direction: SortDescending},
				{Column: "id", Direction: SortAscending},
			},
		},
		{
			name: "adding the first column keeps it first",
			sort: statementSort, column: "created_on", add: true,
			wanted: []SortState{
				{Column: "created_on", Direction: SortAscending},
				{Column: "id", Direction: SortDescending},
			},
		},
		{
			name: "adding a column that is not in the sort puts it last",
			sort: statementSort, column: "company", add: true,
			wanted: []SortState{
				{Column: "created_on", Direction: SortDescending},
				{Column: "id", Direction: SortDescending},
				{Column: "company", Direction: SortAscending},
			},
		},
		{
			name: "asking without adding leaves that column alone",
			sort: statementSort, column: "id", add: false,
			wanted: []SortState{{Column: "id", Direction: SortAscending}},
		},
		{
			name:   "a column ordered up turns down",
			sort:   []SortState{{Column: "id", Direction: SortAscending}},
			column: "id", add: true,
			wanted: []SortState{{Column: "id", Direction: SortDescending}},
		},
		{
			name: "a first press on nothing orders up",
			sort: nil, column: "id", add: true,
			wanted: []SortState{{Column: "id", Direction: SortAscending}},
		},
		{
			name: "a column named twice is turned over wherever it stands",
			sort: []SortState{
				{Column: "id", Direction: SortAscending},
				{Column: "id", Direction: SortAscending},
			},
			column: "id", add: true,
			wanted: []SortState{
				{Column: "id", Direction: SortDescending},
				{Column: "id", Direction: SortDescending},
			},
		},
	}

	for _, held := range cases {
		t.Run(held.name, func(t *testing.T) {
			given := append([]SortState{}, held.sort...)
			built := ApplySortColumn(held.sort, held.column, held.add)
			if len(built) != len(held.wanted) {
				t.Fatalf("built %v, want %v", built, held.wanted)
			}
			for at := range built {
				if built[at] != held.wanted[at] {
					t.Fatalf("built %v, want %v", built, held.wanted)
				}
			}
			// The sort it was given is not written into, because the caller still holds it.
			for at := range given {
				if held.sort[at] != given[at] {
					t.Errorf("the sort given was changed at %d", at)
				}
			}
		})
	}
}
