package cli

import (
	"slices"
	"testing"

	"github.com/turanmahmudov/masume/internal/notebook"
)

// A --only id no cell carries would otherwise run nothing and exit zero, so a typo in a
// scheduled run looks like a run that passed.
func TestFindUnknownCellIDsNamesEveryIDTheNotebookHasNot(t *testing.T) {
	book := notebook.Notebook{Cells: []notebook.Cell{
		{ID: "load", Kind: notebook.CellSQL},
		{ID: "report", Kind: notebook.CellSQL},
	}}

	if missing := findUnknownCellIDs(nil, book); len(missing) != 0 {
		t.Errorf("a run of every cell reports %v", missing)
	}
	if missing := findUnknownCellIDs([]string{"load", "report"}, book); len(missing) != 0 {
		t.Errorf("a run of two known cells reports %v", missing)
	}

	missing := findUnknownCellIDs([]string{"load", "typo", "gone"}, book)
	if !slices.Equal(missing, []string{"typo", "gone"}) {
		t.Errorf("the run reports %v, wanted the two unknown ids", missing)
	}
}
