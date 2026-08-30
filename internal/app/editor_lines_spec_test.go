package app_test

import (
	"testing"

	"github.com/turanmahmudov/masume/internal/app"
)

func TestSelectedLineRangeTakesTheWholeLineUnderTheCaret(t *testing.T) {
	buffer := app.NewEditorBuffer("select id\nfrom orders\nwhere id = 1", 12)

	start, end := buffer.SelectedLineRange()
	if written := buffer.Text[start:end]; written != "from orders" {
		t.Errorf("answered %q, wanted the whole line under the caret", written)
	}
}

func TestSelectedLineRangeStopsAtTheLineAboveASelectionEndingOnALineStart(t *testing.T) {
	buffer := app.NewEditorBuffer("one\ntwo\nthree", 0)
	// From the top down to the first cell of "two", which is the line below "one".
	buffer.SelectRange(0, 4)

	start, end := buffer.SelectedLineRange()
	if written := buffer.Text[start:end]; written != "one" {
		t.Errorf("answered %q, wanted only the line the selection covers", written)
	}
}

func TestCommentLinesWritesTheMarkAtTheShallowestIndent(t *testing.T) {
	// Every line is indented, so the mark belongs at column 4 and the block keeps its shape.
	buffer := app.NewEditorBuffer("    select id\n        from orders", 0)
	buffer.SelectRange(0, len(buffer.Text))

	if !buffer.CommentLines("--") {
		t.Fatal("reported that it changed nothing")
	}
	wanted := "    -- select id\n    --     from orders"
	if buffer.Text != wanted {
		t.Errorf("answered %q, wanted %q", buffer.Text, wanted)
	}
}

func TestCommentLinesWritesTheMarkAtTheLeftWhereALineHasNoIndent(t *testing.T) {
	buffer := app.NewEditorBuffer("select id\n    from orders", 0)
	buffer.SelectRange(0, len(buffer.Text))

	if !buffer.CommentLines("--") {
		t.Fatal("reported that it changed nothing")
	}
	wanted := "-- select id\n--     from orders"
	if buffer.Text != wanted {
		t.Errorf("answered %q, wanted %q", buffer.Text, wanted)
	}
}

func TestCommentLinesTakesTheMarkAwayWhereEveryLineCarriesIt(t *testing.T) {
	buffer := app.NewEditorBuffer("-- select id\n-- from orders", 0)
	buffer.SelectRange(0, len(buffer.Text))

	if !buffer.CommentLines("--") {
		t.Fatal("reported that it changed nothing")
	}
	wanted := "select id\nfrom orders"
	if buffer.Text != wanted {
		t.Errorf("answered %q, wanted the mark taken off both lines", buffer.Text)
	}
}

func TestCommentLinesCommentsABlockThatIsHalfCommented(t *testing.T) {
	buffer := app.NewEditorBuffer("-- select id\nfrom orders", 0)
	buffer.SelectRange(0, len(buffer.Text))

	if !buffer.CommentLines("--") {
		t.Fatal("reported that it changed nothing")
	}
	wanted := "-- -- select id\n-- from orders"
	if buffer.Text != wanted {
		t.Errorf("answered %q, wanted every line commented", buffer.Text)
	}
}

func TestCommentLinesKeepsABlankLineAsItIs(t *testing.T) {
	buffer := app.NewEditorBuffer("select id\n\nfrom orders", 0)
	buffer.SelectRange(0, len(buffer.Text))

	if !buffer.CommentLines("--") {
		t.Fatal("reported that it changed nothing")
	}
	wanted := "-- select id\n\n-- from orders"
	if buffer.Text != wanted {
		t.Errorf("answered %q, wanted the blank line left alone", buffer.Text)
	}
}

func TestCommentLinesChangesNothingWithoutAMark(t *testing.T) {
	buffer := app.NewEditorBuffer("select id", 0)

	if buffer.CommentLines("") {
		t.Error("reported a change for an engine that has no comment mark")
	}
	if buffer.Text != "select id" {
		t.Errorf("the buffer now holds %q", buffer.Text)
	}
}

func TestCommentLinesChangesNothingWhereEveryLineIsBlank(t *testing.T) {
	buffer := app.NewEditorBuffer("\n\n", 0)
	buffer.SelectRange(0, len(buffer.Text))

	if buffer.CommentLines("--") {
		t.Error("reported a change for a block that holds nothing")
	}
}

func TestIndentLinesMovesEveryLineOfTheSelection(t *testing.T) {
	buffer := app.NewEditorBuffer("select id\nfrom orders", 0)
	buffer.SelectRange(0, len(buffer.Text))

	if !buffer.IndentLines(2) {
		t.Fatal("reported that it changed nothing")
	}
	wanted := "  select id\n  from orders"
	if buffer.Text != wanted {
		t.Errorf("answered %q, wanted %q", buffer.Text, wanted)
	}
}

func TestIndentLinesKeepsTheSelectionOverTheSameLines(t *testing.T) {
	buffer := app.NewEditorBuffer("select id\nfrom orders", 0)
	buffer.SelectRange(0, len(buffer.Text))
	buffer.IndentLines(2)

	// A second press has to move the same block again.
	if !buffer.IndentLines(2) {
		t.Fatal("the second press changed nothing")
	}
	wanted := "    select id\n    from orders"
	if buffer.Text != wanted {
		t.Errorf("answered %q, wanted the same block moved twice", buffer.Text)
	}
}

func TestIndentLinesChangesNothingForAWidthBelowOne(t *testing.T) {
	buffer := app.NewEditorBuffer("select id", 0)

	if buffer.IndentLines(0) {
		t.Error("reported a change for a width of nothing")
	}
	if buffer.Text != "select id" {
		t.Errorf("the buffer now holds %q", buffer.Text)
	}
}

func TestOutdentLinesTakesAsMuchAsEachLineCanGive(t *testing.T) {
	buffer := app.NewEditorBuffer("    select id\n  from orders", 0)
	buffer.SelectRange(0, len(buffer.Text))

	if !buffer.OutdentLines(4) {
		t.Fatal("reported that it changed nothing")
	}
	wanted := "select id\nfrom orders"
	if buffer.Text != wanted {
		t.Errorf("answered %q, wanted each line moved as far as it could", buffer.Text)
	}
}

func TestOutdentLinesReportsNoChangeWhereNoLineIsIndented(t *testing.T) {
	buffer := app.NewEditorBuffer("select id\nfrom orders", 0)
	buffer.SelectRange(0, len(buffer.Text))

	if buffer.OutdentLines(4) {
		t.Error("reported a change for a block already at the left")
	}
	if buffer.Text != "select id\nfrom orders" {
		t.Errorf("the buffer now holds %q", buffer.Text)
	}
}

func TestOutdentLinesTakesATabAsOneStep(t *testing.T) {
	buffer := app.NewEditorBuffer("\tselect id", 0)
	buffer.SelectRange(0, len(buffer.Text))

	if !buffer.OutdentLines(4) {
		t.Fatal("reported that it changed nothing")
	}
	if buffer.Text != "select id" {
		t.Errorf("answered %q, wanted the tab taken off", buffer.Text)
	}
}

func TestCommentLinesUndoesInOneStep(t *testing.T) {
	buffer := app.NewEditorBuffer("select id\nfrom orders", 0)
	buffer.SelectRange(0, len(buffer.Text))
	buffer.CommentLines("--")

	buffer.Undo()
	if buffer.Text != "select id\nfrom orders" {
		t.Errorf("one undo answered %q, wanted the whole comment taken back", buffer.Text)
	}
}

func TestReplaceMatchesWritesEveryOneAndCountsThem(t *testing.T) {
	buffer := app.NewEditorBuffer("select id from orders where id = id", 0)

	if written := buffer.ReplaceMatches("id", "key"); written != 3 {
		t.Errorf("answered %d, wanted every match written", written)
	}
	wanted := "select key from orders where key = key"
	if buffer.Text != wanted {
		t.Errorf("answered %q, wanted %q", buffer.Text, wanted)
	}
}

func TestReplaceMatchesAnswersNothingForATermThatIsNotThere(t *testing.T) {
	buffer := app.NewEditorBuffer("select id", 0)

	if written := buffer.ReplaceMatches("orders", "rows"); written != 0 {
		t.Errorf("answered %d, wanted none", written)
	}
	if buffer.Text != "select id" {
		t.Errorf("the buffer now holds %q", buffer.Text)
	}
}

func TestReplaceMatchesUndoesInOneStep(t *testing.T) {
	buffer := app.NewEditorBuffer("id and id", 0)
	buffer.ReplaceMatches("id", "key")

	buffer.Undo()
	if buffer.Text != "id and id" {
		t.Errorf("one undo answered %q, wanted the whole replace taken back", buffer.Text)
	}
}
