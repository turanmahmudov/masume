package app_test

import (
	"testing"

	"github.com/turanmahmudov/masume/internal/app"
)

// The caret is an offset into the text, and every move has to leave it inside. A caret past
// the end would panic the moment the text is sliced to draw it.
func TestMoveCaretStaysInsideTheText(t *testing.T) {
	const text = "select 1"
	for _, start := range []int{0, 4, len(text)} {
		for _, step := range []int{-100, -1, 0, 1, 100} {
			buffer := app.NewEditorBuffer(text, start)
			buffer.MoveCaret(step, false)
			if buffer.Caret < 0 || buffer.Caret > len(text) {
				t.Errorf("a caret at %d moved by %d landed at %d", start, step, buffer.Caret)
			}
		}
	}
}

// The caret moves one character at a time, and the step says only which way. A caret walks a
// rune at a time and not a byte, so a wide character is stepped over whole rather than left
// half crossed.
func TestMoveCaretStepsOneRuneWhicheverStepItIsGiven(t *testing.T) {
	buffer := app.NewEditorBuffer("ada", 0)
	buffer.MoveCaret(100, false)
	if buffer.Caret != 1 {
		t.Errorf("a step of a hundred landed at %d, wanted one character along", buffer.Caret)
	}

	// A wide character is more than one byte, and the caret steps over all of it.
	wide := app.NewEditorBuffer("漢字", 0)
	wide.MoveCaret(1, false)
	if wide.Caret != 3 {
		t.Errorf("the caret landed at %d, wanted it past the whole character", wide.Caret)
	}
	wide.MoveCaret(-1, false)
	if wide.Caret != 0 {
		t.Errorf("the caret came back to %d, wanted the start", wide.Caret)
	}
}

// A buffer built with a caret outside the text has to settle it, because a restored tab
// carries a caret from a file that may no longer match.
func TestNewEditorBufferSettlesACaretOutsideTheText(t *testing.T) {
	for _, caret := range []int{-5, 100} {
		buffer := app.NewEditorBuffer("select 1", caret)
		if buffer.Caret < 0 || buffer.Caret > len(buffer.Text) {
			t.Errorf("a caret of %d was kept as %d", caret, buffer.Caret)
		}
	}
}

// Typing puts the text at the caret and moves the caret past it, so the next character lands
// after the last.
func TestInsertPutsTheTextAtTheCaret(t *testing.T) {
	buffer := app.NewEditorBuffer("select from orders", 7)
	buffer.Insert("id ")

	if buffer.Text != "select id from orders" {
		t.Errorf("the text reads %q", buffer.Text)
	}
	if buffer.Caret != 10 {
		t.Errorf("the caret stands at %d, wanted it after what was typed", buffer.Caret)
	}
}

// Typing over a selection takes its place, which is what a reader expects of any editor.
func TestInsertTakesThePlaceOfTheSelection(t *testing.T) {
	buffer := app.NewEditorBuffer("select id from orders", 0)
	buffer.SelectAll()
	buffer.Insert("select 1")

	if buffer.Text != "select 1" {
		t.Errorf("the text reads %q", buffer.Text)
	}
	if buffer.HasSelection() {
		t.Error("the selection was kept after it was typed over")
	}
}

func TestDeleteBackwardAndForwardTakeOneCharacter(t *testing.T) {
	buffer := app.NewEditorBuffer("select", 3)
	buffer.DeleteBackward()
	if buffer.Text != "seect" {
		t.Errorf("a delete back gave %q", buffer.Text)
	}
	if buffer.Caret != 2 {
		t.Errorf("the caret stands at %d after a delete back", buffer.Caret)
	}

	buffer.DeleteForward()
	if buffer.Text != "sect" {
		t.Errorf("a delete forward gave %q", buffer.Text)
	}
	if buffer.Caret != 2 {
		t.Errorf("the caret moved on a delete forward: %d", buffer.Caret)
	}
}

// A delete at either end has nothing to take, and must not reach outside the text.
func TestDeleteAtTheEndsTakesNothing(t *testing.T) {
	first := app.NewEditorBuffer("select", 0)
	first.DeleteBackward()
	if first.Text != "select" {
		t.Errorf("a delete back at the start gave %q", first.Text)
	}

	last := app.NewEditorBuffer("select", 6)
	last.DeleteForward()
	if last.Text != "select" {
		t.Errorf("a delete forward at the end gave %q", last.Text)
	}
}

// A delete with a selection takes the selection, not one character.
func TestDeleteTakesTheSelectionWhereThereIsOne(t *testing.T) {
	buffer := app.NewEditorBuffer("select id from orders", 0)
	buffer.SelectAll()
	buffer.DeleteBackward()

	if buffer.Text != "" {
		t.Errorf("a delete over the whole text gave %q", buffer.Text)
	}
	if buffer.Caret != 0 {
		t.Errorf("the caret stands at %d", buffer.Caret)
	}
}

// The selection is read the same way whichever end it was dragged from, so a drag backwards
// answers the same text as one forwards.
func TestSelectionReadsTheSameFromEitherEnd(t *testing.T) {
	const text = "select id from orders"

	forward := app.NewEditorBuffer(text, 7)
	forward.MoveCaret(1, true)
	forward.MoveCaret(1, true)

	backward := app.NewEditorBuffer(text, 9)
	backward.MoveCaret(-1, true)
	backward.MoveCaret(-1, true)

	if forward.Selection() != backward.Selection() {
		t.Errorf("a drag forward read %q and one back read %q",
			forward.Selection(), backward.Selection())
	}
	if forward.Selection() != "id" {
		t.Errorf("the selection reads %q, wanted the two characters", forward.Selection())
	}
}

func TestSelectionRangeAnswersTheLowerEndFirst(t *testing.T) {
	buffer := app.NewEditorBuffer("select id", 9)
	buffer.MoveCaret(-2, true)

	from, to := buffer.SelectionRange()
	if from > to {
		t.Errorf("the range reads %d to %d, wanted the lower end first", from, to)
	}
}

// A move without selecting lets the selection go, because the caret moved away from it.
func TestMovingWithoutSelectingLetsTheSelectionGo(t *testing.T) {
	buffer := app.NewEditorBuffer("select id", 0)
	buffer.SelectAll()
	if !buffer.HasSelection() {
		t.Fatal("select all left no selection")
	}

	buffer.MoveCaret(1, false)
	if buffer.HasSelection() {
		t.Error("a move without selecting kept the selection")
	}
}

// The lines are what the editor draws, and the caret has to be reported on the line it
// stands in, or the cursor is drawn in the wrong place.
func TestCaretPositionAnswersTheLineAndTheColumn(t *testing.T) {
	const text = "select id\nfrom orders\nwhere id = 1"

	for _, held := range []struct {
		caret int
		line  int
		want  int
	}{
		{0, 0, 0},
		{6, 0, 6},
		{9, 0, 9},
		// The first character of the second line.
		{10, 1, 0},
		{15, 1, 5},
		{len(text), 2, len("where id = 1")},
	} {
		buffer := app.NewEditorBuffer(text, held.caret)
		line, column := buffer.CaretPosition()
		if line != held.line || column != held.want {
			t.Errorf("a caret at %d reads as line %d column %d, wanted %d and %d",
				held.caret, line, column, held.line, held.want)
		}
	}
}

func TestLinesSplitTheTextAndKeepAnEmptyOne(t *testing.T) {
	buffer := app.NewEditorBuffer("select 1\n\nselect 2", 0)
	lines := buffer.Lines()
	if len(lines) != 3 {
		t.Fatalf("the text split into %d lines, wanted 3", len(lines))
	}
	if lines[1] != "" {
		t.Errorf("the blank line reads %q", lines[1])
	}
}

// An empty buffer still has one line, because the editor draws a row for the caret to sit in.
func TestLinesAnswersOneLineForAnEmptyBuffer(t *testing.T) {
	if held := app.NewEditorBuffer("", 0).Lines(); len(held) != 1 {
		t.Errorf("an empty buffer has %d lines, wanted 1", len(held))
	}
}

func TestLineStartAndLineEndFindTheEndsOfTheLine(t *testing.T) {
	const text = "select id\nfrom orders"
	buffer := app.NewEditorBuffer(text, 0)

	// A caret inside the second line finds the ends of that line.
	if held := buffer.LineStart(15); held != 10 {
		t.Errorf("the line starts at %d, wanted 10", held)
	}
	if held := buffer.LineEnd(15); held != len(text) {
		t.Errorf("the line ends at %d, wanted %d", held, len(text))
	}
	// A caret on the first line finds its own ends, not the ones of the buffer.
	if held := buffer.LineEnd(3); held != 9 {
		t.Errorf("the first line ends at %d, wanted 9", held)
	}
}

// Moving between lines keeps the caret inside the text, whatever the lines hold.
func TestMoveLineStaysInsideTheText(t *testing.T) {
	const text = "select id\nfrom orders\nwhere 1"
	for _, caret := range []int{0, 5, 12, len(text)} {
		for _, step := range []int{-5, -1, 1, 5} {
			buffer := app.NewEditorBuffer(text, caret)
			buffer.MoveLine(step, false)
			if buffer.Caret < 0 || buffer.Caret > len(text) {
				t.Errorf("a caret at %d moved %d lines and landed at %d",
					caret, step, buffer.Caret)
			}
		}
	}
}

// A new line keeps the indent of the one before it, so a statement written over several lines
// stays lined up without the reader typing the spaces again.
func TestIndentAtCaretAnswersTheSpacesOfTheLine(t *testing.T) {
	for _, held := range []struct {
		name  string
		text  string
		caret int
		want  string
	}{
		{"no indent", "select 1", 8, ""},
		{"two spaces", "  select 1", 10, "  "},
		{"a tab", "\tselect 1", 9, "\t"},
		{"the indent of the caret line", "select 1\n    and 2", 18, "    "},
	} {
		t.Run(held.name, func(t *testing.T) {
			buffer := app.NewEditorBuffer(held.text, held.caret)
			if answered := buffer.IndentAtCaret(); answered != held.want {
				t.Errorf("the indent reads %q, wanted %q", answered, held.want)
			}
		})
	}
}

// Replacing the text settles the caret, because the new text may be shorter than where the
// caret stood.
func TestSetTextSettlesTheCaret(t *testing.T) {
	buffer := app.NewEditorBuffer("select id from orders", 20)
	buffer.SetText("select 1")

	if buffer.Caret > len(buffer.Text) {
		t.Errorf("the caret stands at %d in a text of %d", buffer.Caret, len(buffer.Text))
	}
	if buffer.HasSelection() {
		t.Error("a selection was kept over text that was replaced")
	}
}
