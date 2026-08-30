package ui

import (
	"strings"
	"testing"
)

func buildSelectionModel(frame string, fromX, fromY, toX, toY int) *Model {
	model := &Model{styles: NewStyles(NewThemeRegistry()), frame: screenFrame{text: frame}}
	model.selection = screenSelection{
		fromX: fromX, fromY: fromY, toX: toX, toY: toY, held: true,
	}
	return model
}

func TestSpanOnRow(t *testing.T) {
	selection := screenSelection{fromX: 3, fromY: 1, toX: 5, toY: 3, held: true}
	cases := []struct {
		row, from, to int
		covers        bool
	}{
		{0, 0, 0, false},
		{1, 3, 9, true},
		{2, 0, 9, true},
		{3, 0, 5, true},
		{4, 0, 0, false},
	}
	for _, held := range cases {
		from, to, covers := selection.spanOnRow(held.row, 10)
		if covers != held.covers || (covers && (from != held.from || to != held.to)) {
			t.Errorf("row %d gave %d..%d %v, wanted %d..%d %v",
				held.row, from, to, covers, held.from, held.to, held.covers)
		}
	}
}

func TestSpanOnRowReadsADragBackwards(t *testing.T) {
	forwards := screenSelection{fromX: 2, fromY: 0, toX: 6, toY: 0, held: true}
	backwards := screenSelection{fromX: 6, fromY: 0, toX: 2, toY: 0, held: true}
	first, last, _ := forwards.spanOnRow(0, 10)
	otherFirst, otherLast, _ := backwards.spanOnRow(0, 10)
	if first != otherFirst || last != otherLast {
		t.Errorf("a drag backwards gave %d..%d, wanted %d..%d",
			otherFirst, otherLast, first, last)
	}
}

func TestReadSelectedText(t *testing.T) {
	frame := strings.Join([]string{
		"abcdefghij",
		"klmnopqrst",
		"uvwxyz    ",
	}, "\n")

	if written := buildSelectionModel(frame, 2, 0, 4, 0).readSelectedText(frame); written != "cde" {
		t.Errorf("one row gave %q, wanted %q", written, "cde")
	}
	written := buildSelectionModel(frame, 8, 0, 1, 1).readSelectedText(frame)
	if written != "ij\nkl" {
		t.Errorf("two rows gave %q, wanted %q", written, "ij\nkl")
	}
	// The blanks at the end of a row are padding, not content.
	if written := buildSelectionModel(frame, 0, 2, 9, 2).readSelectedText(frame); written != "uvwxyz" {
		t.Errorf("a padded row gave %q, wanted %q", written, "uvwxyz")
	}
	// A drag over blanks only takes nothing.
	blank := "          "
	if written := buildSelectionModel(blank, 1, 0, 5, 0).readSelectedText(blank); written != "" {
		t.Errorf("blanks gave %q, wanted nothing", written)
	}
}

func TestPaintSelectionKeepsTheText(t *testing.T) {
	frame := "abcdefghij\nklmnopqrst"
	model := buildSelectionModel(frame, 2, 0, 4, 1)
	painted := model.paintSelection(frame)
	if stripEscapes(painted) != frame {
		t.Errorf("the text changed to %q", stripEscapes(painted))
	}
	if !strings.Contains(painted, "\x1b[") {
		t.Error("the selection was not painted")
	}
}

func TestDescribeCopied(t *testing.T) {
	if said := describeCopied("a"); said != "copied 1 character" {
		t.Errorf("one character reads %q", said)
	}
	if said := describeCopied("ab"); said != "copied 2 characters" {
		t.Errorf("two characters read %q", said)
	}
	if said := describeCopied("ab\ncd"); said != "copied 5 characters over 2 lines" {
		t.Errorf("two lines read %q", said)
	}
}

func TestReadSelectedTextReadsTheSameCellsThePaintCovers(t *testing.T) {
	// A wide glyph takes two columns and one rune. The paint counts columns, so the copy
	// has to count them too, or the two cover different text.
	const frame = "ab日本cd"
	for _, held := range []struct {
		name  string
		fromX int
		toX   int
		want  string
	}{
		{"the narrow head", 0, 1, "ab"},
		{"one wide glyph", 2, 3, "日"},
		{"both wide glyphs", 2, 5, "日本"},
		// Column 4 is the first of the two `本` takes, so the whole glyph is copied.
		{"across the boundary", 1, 4, "b日本"},
		{"the whole row", 0, 7, "ab日本cd"},
	} {
		t.Run(held.name, func(t *testing.T) {
			model := buildSelectionModel(frame, held.fromX, 0, held.toX, 0)
			if got := model.readSelectedText(frame); got != held.want {
				t.Errorf("copied %q, want %q", got, held.want)
			}
		})
	}
}

func TestReadSelectedTextAndPaintSelectionCoverTheSameRun(t *testing.T) {
	// The painted cells and the copied text must describe one span, whatever the row holds.
	for _, frame := range []string{"ab日本cd", "plain text", "日本語", "a日b本c"} {
		width := len(mapCells(frame))
		for from := range width {
			for to := from; to < width; to++ {
				model := buildSelectionModel(frame, from, 0, to, 0)
				copied := model.readSelectedText(frame)
				painted := stripEscapes(model.paintSelection(frame))
				if !strings.Contains(painted, strings.TrimRight(copied, " ")) {
					t.Fatalf("frame %q cells %d..%d: copied %q is not in the painted row %q",
						frame, from, to, copied, painted)
				}
			}
		}
	}
}
