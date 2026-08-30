package ui

import (
	"testing"

	"github.com/turanmahmudov/masume/internal/app"
)

func TestBuildCellEditorOpensAFieldSixRowsTall(t *testing.T) {
	overlay := buildCellEditor(app.Overlay{Kind: app.OverlayCellEdit}, "one line")

	if overlay.Draft == nil || overlay.Draft.Text != "one line" {
		t.Fatalf("the field holds %+v", overlay.Draft)
	}
	if overlay.Draft.Caret != 0 {
		t.Errorf("the caret stands at %d, wanted the first cell", overlay.Draft.Caret)
	}
	if overlay.ContentRows != minCellEditorRows {
		t.Errorf("the card gives %d rows, wanted %d", overlay.ContentRows, minCellEditorRows)
	}
}

func TestBuildCellEditorGrowsForAValueOfManyLines(t *testing.T) {
	overlay := buildCellEditor(app.Overlay{Kind: app.OverlayCellEdit},
		"1\n2\n3\n4\n5\n6\n7\n8")

	if overlay.ContentRows != 8 {
		t.Errorf("the card gives %d rows, wanted 8", overlay.ContentRows)
	}
}

func TestBuildCellEditorMarksTheValueTheCellHolds(t *testing.T) {
	overlay := buildCellEditor(app.Overlay{
		Kind: app.OverlayCellEdit, Cell: app.CellTarget{Choices: []string{"pending", "packed", "sent"}},
	}, "packed")

	if overlay.Draft != nil {
		t.Error("a cell that is picked holds a field")
	}
	if overlay.List.Cursor != 1 {
		t.Errorf("the cursor stands on %d, wanted 1", overlay.List.Cursor)
	}
	if overlay.ContentRows != 3 {
		t.Errorf("the card gives %d rows, wanted 3", overlay.ContentRows)
	}
}

func TestBuildCellEditorMarksTheFirstValueForACellWithNone(t *testing.T) {
	overlay := buildCellEditor(app.Overlay{
		Kind: app.OverlayCellEdit, Cell: app.CellTarget{Choices: []string{"true", "false"}},
	}, "")

	if overlay.List.Cursor != 0 {
		t.Errorf("the cursor stands on %d, wanted the first row", overlay.List.Cursor)
	}
}
