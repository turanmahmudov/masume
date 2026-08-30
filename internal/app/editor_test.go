package app

import "testing"

// Home reaches the top of the text and End the bottom, whatever line the caret stands on,
// which is what every list of this client does.
func TestHomeAndEndReachTheEndsOfTheText(t *testing.T) {
	buffer := NewEditorBuffer("select one\nselect two\nselect three", 0)
	buffer.MoveLine(1, false)
	buffer.MoveCaret(3, false)

	buffer.MoveToStart(false)
	if buffer.Caret != 0 {
		t.Errorf("Home from the middle line answers %d, wanted the first cell", buffer.Caret)
	}
	buffer.MoveToEnd(false)
	if buffer.Caret != len(buffer.Text) {
		t.Errorf("End answers %d, wanted %d", buffer.Caret, len(buffer.Text))
	}
}

func TestHomeAndEndSelectFromWhereTheCaretStood(t *testing.T) {
	buffer := NewEditorBuffer("one\ntwo", 4)

	buffer.MoveToStart(true)
	if buffer.Selection() != "one\n" {
		t.Errorf("Home with Shift selects %q", buffer.Selection())
	}
	buffer = NewEditorBuffer("one\ntwo", 4)
	buffer.MoveToEnd(true)
	if buffer.Selection() != "two" {
		t.Errorf("End with Shift selects %q", buffer.Selection())
	}
}

// A line opened at the caret leaves no selection behind it. The caret and the anchor are
// written together, or the character typed next takes the place of the rest of the buffer.
func TestSetTextWithCaretLeavesNoSelection(t *testing.T) {
	buffer := NewEditorBuffer("select a\nfrom t", 8)
	buffer.SetTextWithCaret("select a\n\nfrom t", 9)

	if buffer.HasSelection() {
		t.Fatalf("the buffer came back holding %q", buffer.Selection())
	}
	buffer.Insert("b")
	if buffer.Text != "select a\nb\nfrom t" {
		t.Errorf("typing after it answers %q", buffer.Text)
	}
}

func TestMoveToLineStartAndEndStayOnTheLine(t *testing.T) {
	buffer := NewEditorBuffer("select one\n  from two\nwhere three", 15)

	buffer.MoveToLineEnd(false)
	if buffer.Caret != len("select one\n  from two") {
		t.Errorf("End answers %d, wanted the end of the second line", buffer.Caret)
	}
	buffer.MoveToLineStart(false)
	if buffer.Caret != len("select one\n  ") {
		t.Errorf("Home answers %d, wanted the first word of the line", buffer.Caret)
	}
	// A second press reaches the first cell, before the indent.
	buffer.MoveToLineStart(false)
	if buffer.Caret != len("select one\n") {
		t.Errorf("a second Home answers %d, wanted the first cell", buffer.Caret)
	}
}

func TestMoveWordStepsOverOneWord(t *testing.T) {
	buffer := NewEditorBuffer("select id, name from orders", 0)

	buffer.MoveWord(1, false)
	if buffer.Caret != len("select") {
		t.Errorf("one step forward answers %d, wanted the end of the first word", buffer.Caret)
	}
	buffer.MoveWord(1, false)
	if buffer.Caret != len("select id") {
		t.Errorf("two steps forward answer %d", buffer.Caret)
	}
	buffer.MoveWord(-1, false)
	if buffer.Caret != len("select ") {
		t.Errorf("one step back answers %d, wanted the start of that word", buffer.Caret)
	}
}

func TestMoveWordSelectsWhileSelecting(t *testing.T) {
	buffer := NewEditorBuffer("select id from orders", 0)
	buffer.MoveWord(1, true)
	if buffer.Selection() != "select" {
		t.Errorf("a step with Shift selects %q", buffer.Selection())
	}
}

func TestDeleteWordTakesOneWordAtATime(t *testing.T) {
	buffer := NewEditorBuffer("select id from orders", len("select id from"))
	buffer.DeleteWordBackward()
	if buffer.Text != "select id  orders" {
		t.Errorf("a delete back answers %q", buffer.Text)
	}

	buffer = NewEditorBuffer("select id from orders", len("select id "))
	buffer.DeleteWordForward()
	if buffer.Text != "select id  orders" {
		t.Errorf("a delete forward answers %q", buffer.Text)
	}
}

func TestDeleteWordTakesTheSelectionWhereThereIsOne(t *testing.T) {
	buffer := NewEditorBuffer("select id from orders", 0)
	buffer.SelectAll()
	buffer.DeleteWordBackward()
	if buffer.Text != "" {
		t.Errorf("the selection was left as %q", buffer.Text)
	}
}

// A run of typing is taken back one word at a time, so an undo of a long statement is
// neither one letter nor the whole of it.
func TestUndoTakesBackOneWord(t *testing.T) {
	buffer := NewEditorBuffer("", 0)
	for _, character := range "select id" {
		buffer.Insert(string(character))
	}

	if !buffer.Undo() {
		t.Fatal("there was nothing to undo")
	}
	if buffer.Text != "select" {
		t.Errorf("one undo answers %q, wanted the word before the last", buffer.Text)
	}
	if !buffer.Redo() {
		t.Fatal("there was nothing to redo")
	}
	if buffer.Text != "select id" {
		t.Errorf("a redo answers %q", buffer.Text)
	}
}

func TestUndoBringsBackWhatAFormatWroteOver(t *testing.T) {
	buffer := NewEditorBuffer("select id from orders", 21)
	buffer.SetText("select id\nfrom orders")

	if !buffer.Undo() {
		t.Fatal("there was nothing to undo")
	}
	if buffer.Text != "select id from orders" {
		t.Errorf("the undo answers %q", buffer.Text)
	}
}

func TestUndoReportsNothingOnAFreshBuffer(t *testing.T) {
	buffer := NewEditorBuffer("select id", 0)
	if buffer.Undo() {
		t.Error("a fresh buffer had a step to take back")
	}
	if buffer.Redo() {
		t.Error("a fresh buffer had a step to write again")
	}
}

// A new edit throws away what a redo would have written, or the two would disagree.
func TestAnEditAfterAnUndoEndsTheRedo(t *testing.T) {
	buffer := NewEditorBuffer("", 0)
	buffer.Insert("a")
	buffer.MoveCaret(-1, false)
	buffer.Insert("b")
	buffer.Undo()
	buffer.Insert("c")

	if buffer.Redo() {
		t.Error("a redo was still offered after an edit")
	}
}

func TestSelectWordAtTakesTheWordUnderTheOffset(t *testing.T) {
	buffer := NewEditorBuffer("select id from orders", 0)
	buffer.SelectWordAt(len("select i"))
	if buffer.Selection() != "id" {
		t.Errorf("the word under the offset reads %q", buffer.Selection())
	}
	// A press past the last word of the line still takes that word.
	buffer.SelectWordAt(len("select id from orders"))
	if buffer.Selection() != "orders" {
		t.Errorf("a press at the end reads %q", buffer.Selection())
	}
}

func TestSelectLineAtTakesTheLineWithItsBreak(t *testing.T) {
	buffer := NewEditorBuffer("select id\nfrom orders", 2)
	buffer.SelectLineAt(2)
	if buffer.Selection() != "select id\n" {
		t.Errorf("the line reads %q", buffer.Selection())
	}
	buffer.SelectLineAt(len("select id\nfrom"))
	if buffer.Selection() != "from orders" {
		t.Errorf("the last line reads %q", buffer.Selection())
	}
}

func TestFindOffsetAtAnswersTheCellThePointerStandsOn(t *testing.T) {
	const text = "select id\nfrom orders"
	buffer := NewEditorBuffer(text, 0)

	for _, held := range []struct {
		line, column, want int
	}{
		{0, 0, 0},
		{0, 6, 6},
		// A column past the end of the line answers its end.
		{0, 40, len("select id")},
		{1, 4, len("select id\nfrom")},
		// A line past the last one answers the end of the text.
		{9, 0, len(text)},
	} {
		if answered := buffer.FindOffsetAt(held.line, held.column); answered != held.want {
			t.Errorf("line %d column %d answers %d, wanted %d",
				held.line, held.column, answered, held.want)
		}
	}
}

func TestPlaceCaretGrowsTheSelectionWhileSelecting(t *testing.T) {
	buffer := NewEditorBuffer("select id from orders", 0)
	buffer.PlaceCaret(6, false)
	buffer.PlaceCaret(9, true)
	if buffer.Selection() != " id" {
		t.Errorf("a drag selects %q", buffer.Selection())
	}
}

func TestMovePageStepsAsManyLinesAsThePaneShows(t *testing.T) {
	buffer := NewEditorBuffer("one\ntwo\nthree\nfour\nfive", 0)
	buffer.MovePage(1, 3, false)
	if line, _ := buffer.CaretPosition(); line != 3 {
		t.Errorf("a page down answers line %d, wanted the fourth", line)
	}
	buffer.MovePage(-1, 3, false)
	if line, _ := buffer.CaretPosition(); line != 0 {
		t.Errorf("a page up answers line %d, wanted the first", line)
	}
}

// An arrow that is not selecting stands the caret at the end of the selection it lets go,
// so the text it covered is not stepped over twice.
func TestAnArrowCollapsesTheSelectionItLetsGo(t *testing.T) {
	buffer := NewEditorBuffer("select id", 0)
	buffer.SelectAll()
	buffer.MoveCaret(-1, false)
	if buffer.Caret != 0 {
		t.Errorf("a step back answers %d, wanted the start of the selection", buffer.Caret)
	}

	buffer.SelectAll()
	buffer.MoveCaret(1, false)
	if buffer.Caret != len("select id") {
		t.Errorf("a step on answers %d, wanted the end of the selection", buffer.Caret)
	}
}
