package ui

import (
	"strings"
	"testing"

	"github.com/turanmahmudov/masume/internal/db"
	"github.com/turanmahmudov/masume/internal/db/mongo"
	"github.com/turanmahmudov/masume/internal/db/postgres"
	"github.com/turanmahmudov/masume/internal/present"
	"github.com/turanmahmudov/masume/internal/query"
)

// A border row is always as wide as the box, whatever it carries, or every row under it
// stands one cell out.
func TestABorderRowKeepsTheWidthOfTheBox(t *testing.T) {
	styles := buildOfflineModel(t, 120, 34).styles
	theme := styles.Theme

	for _, held := range []struct {
		name        string
		title, note string
		inner       int
	}{
		{"nothing at all", "", "", 20},
		{"a title alone", " query ", "", 20},
		{"a note alone", "", " 1:1 ", 20},
		{"both of them", " query ", " 12:40 ", 40},
		{"a note with no room left", " a very long title indeed ", " 12:40 ", 30},
		{"a title wider than the box", strings.Repeat("x", 60), " 1:1 ", 20},
	} {
		row := styles.renderBorderRow(
			cornerBottomLeft, cornerBottomRight, held.title, held.note,
			held.inner, theme.Border, theme.Panel)
		if measured := measureStyledWidth(row); measured != held.inner+2 {
			t.Errorf("%s measures %d, wanted %d", held.name, measured, held.inner+2)
		}
	}
}

// The note is held against the right corner, and it goes rather than crowd the title.
func TestABorderNoteStandsAtTheRightCorner(t *testing.T) {
	styles := buildOfflineModel(t, 120, 34).styles
	theme := styles.Theme

	wide := styles.renderBorderRow(
		cornerBottomLeft, cornerBottomRight, " query ", " 3:8 ", 40, theme.Border, theme.Panel)
	if !strings.Contains(wide, "3:8") {
		t.Errorf("the note is missing from %q", wide)
	}
	if !strings.HasSuffix(stripStyles(wide), "3:8 "+borderHorizontal+cornerBottomRight) {
		t.Errorf("the note does not stand at the corner: %q", stripStyles(wide))
	}

	narrow := styles.renderBorderRow(
		cornerBottomLeft, cornerBottomRight, " query ", " 3:8 ", 8, theme.Border, theme.Panel)
	if strings.Contains(narrow, "3:8") {
		t.Errorf("the note crowded the title in %q", stripStyles(narrow))
	}
}

// stripStyles answers the text of a row without the escapes that colour it.
func stripStyles(row string) string {
	written := strings.Builder{}
	for at := 0; at < len(row); {
		if row[at] == 0x1b {
			for at < len(row) && row[at] != 'm' {
				at++
			}
			at++
			continue
		}
		written.WriteByte(row[at])
		at++
	}
	return written.String()
}

// The editor says where the caret stands, so a fault the server reports at a line and a
// column can be walked to.
func TestTheEditorSaysWhereTheCaretStands(t *testing.T) {
	model, _, tab := buildEditingModel(t, "select id\nfrom orders\nwhere id = 1", 0)

	// The caret stands after the sixth character of the second line, which is its seventh
	// cell, and that is how the fault row counts one too.
	tab.Editor.PlaceCaret(len("select id\nfrom o"), false)
	if place := describeEditorPlace(tab); place != " 2:7 " {
		t.Errorf("the caret reads %q, wanted the second line and the seventh cell", place)
	}
	drawn := strings.Join(model.renderEditor(model.Active(), tab, 60, 10), "\n")
	if !strings.Contains(drawn, "2:7") {
		t.Error("the pane does not draw where the caret stands")
	}
}

func TestTheEditorSaysHowMuchStandsSelected(t *testing.T) {
	_, _, tab := buildEditingModel(t, "select id\nfrom orders\nwhere id = 1", 0)

	tab.Editor.SelectAll()
	if place := describeEditorPlace(tab); place != " 3 lines selected " {
		t.Errorf("the whole statement reads %q", place)
	}
	// A selection inside one line is counted in characters, not in lines.
	tab.Editor.PlaceCaret(0, false)
	tab.Editor.PlaceCaret(6, true)
	if place := describeEditorPlace(tab); place != " 6 selected " {
		t.Errorf("a selection of one word reads %q", place)
	}
}

// The number of the line the caret is on is drawn in the accent, as the number of the row
// the cursor is on is drawn in the grid.
func TestTheEditorMarksTheNumberOfTheCaretLine(t *testing.T) {
	model, _, tab := buildEditingModel(t, "select id\nfrom orders", 0)
	theme := model.styles.Theme
	tab.Editor.PlaceCaret(len("select id\n"), false)

	rows := model.renderEditor(model.Active(), tab, 60, 10)
	// The border takes the first row, so the two lines of the statement follow it.
	if !strings.Contains(rows[2], resolveOpening(theme.Accent, theme.Zebra)) {
		t.Errorf("the caret line carries no accent: %q", rows[2])
	}
	if strings.Contains(rows[1], resolveOpening(theme.Accent, theme.Zebra)) {
		t.Errorf("a line the caret is not on carries the accent: %q", rows[1])
	}
}

// A failure reads the same way in both panes: the mark the editor writes in its gutter opens
// the message the result draws.
func TestAFailedStatementOpensWithTheFaultMark(t *testing.T) {
	model := buildOfflineModel(t, 120, 34)
	theme := model.styles.Theme

	rows := model.wrapMessage("no such table: nope", 40, 3, theme.Error)
	if !strings.Contains(rows[0], model.describeProblemSign()) {
		t.Errorf("the message opens as %q", rows[0])
	}
	if !strings.Contains(rows[0], "no such table: nope") {
		t.Errorf("the message itself is missing from %q", rows[0])
	}
}

// A message of several rows lines up under its own first row, so the mark does not leave a
// step in the block.
func TestAWrappedFailureLinesUpUnderItsFirstRow(t *testing.T) {
	model := buildOfflineModel(t, 120, 34)
	rows := model.wrapMessage(strings.Repeat("a long message ", 6), 30, 5,
		model.styles.Theme.Error)

	mark := model.describeProblemSign()
	first := measureIndent(stripStyles(rows[0]), mark)
	second := measureIndent(stripStyles(rows[1]), mark)
	if first != second {
		t.Errorf("the first row starts at %d and the second at %d", first, second)
	}
	if measured := present.MeasureText(strings.TrimRight(stripStyles(rows[0]), " ")); measured > 30 {
		t.Errorf("a row of the message measures %d, wider than the pane", measured)
	}
}

// measureIndent answers the cells a row holds before the message itself: the blanks it opens
// with, and the mark of a fault where it carries one.
func measureIndent(row string, mark string) int {
	return present.MeasureText(row) -
		present.MeasureText(strings.TrimLeft(row, " "+mark))
}

// The columns outside the window are counted at the edges of the header but not named, so
// the footer says which of them the cursor is on.
func TestTheGridFooterNamesTheColumnOfTheCursor(t *testing.T) {
	model, connection, tab := buildGridModel(t)
	shape := model.buildGridShape(connection, tab)

	tab.GridColumn = 1
	_, where := model.describeGridFooter(tab, shape)
	if !strings.Contains(where, "col 2/"+present.FormatCount(int64(len(shape.Columns)))) {
		t.Errorf("the footer reads %q", where)
	}
	if !strings.Contains(where, "row 1") {
		t.Errorf("the footer lost the row it names: %q", where)
	}
}

// A result of one column has nothing to place the cursor among, so the count is left out.
func TestTheGridFooterLeavesOutTheCountOfOneColumn(t *testing.T) {
	model, connection, tab := buildGridModel(t)
	tab.Results.Start([]string{"select id from orders"}, 200)
	tab.Results.Succeed(0, db.ComposedRead{Text: "select id from orders"},
		db.QueryResult{
			Columns: []db.ResultColumn{{Name: "id", DataType: "integer"}},
			Rows:    [][]any{{int64(1)}},
		})
	shape := model.buildGridShape(connection, tab)

	if _, where := model.describeGridFooter(tab, shape); strings.Contains(where, "col ") {
		t.Errorf("a result of one column reads %q", where)
	}
}

// Not every server this client opens has SQL, so the pane names what is written in it and
// never the language that writes it.
func TestTheEditorPaneNamesTheQueryAndNotTheLanguage(t *testing.T) {
	model, _, tab := buildEditingModel(t, "select id from orders", 0)

	title := model.describeEditorTitle(tab, 0)
	if !strings.Contains(title, "query") {
		t.Errorf("the pane calls itself %q", title)
	}
	if strings.Contains(strings.ToLower(title), "sql") {
		t.Errorf("the pane calls itself %q, which is the language and not the pane", title)
	}
	if faulty := model.describeEditorTitle(tab, 2); !strings.Contains(faulty, "query") ||
		strings.Contains(strings.ToLower(faulty), "sql") {
		t.Errorf("a pane reporting a fault calls itself %q", faulty)
	}
}

// An empty pane says the shape of one statement of the server it is bound to, so a reader on
// a server that has no SQL is not offered a select.
func TestAnEmptyEditorSaysWhatThisServerTakes(t *testing.T) {
	for _, held := range []struct {
		name    string
		dialect *query.Dialect
		wanted  string
	}{
		{"postgres", postgres.Dialect, "select … from …"},
		{"mongodb", mongo.Dialect, "db.collection.find({…})"},
	} {
		t.Run(held.name, func(t *testing.T) {
			model, connection, tab := buildEditingModel(t, "", 0)
			connection.Session.(*offlineSession).dialect = held.dialect

			if hint := describeEditorHint(connection); hint != held.wanted {
				t.Errorf("the pane says %q, wanted %q", hint, held.wanted)
			}
			drawn := strings.Join(model.renderEditor(connection, tab, 60, 8), "\n")
			if !strings.Contains(stripStyles(drawn), held.wanted) {
				t.Errorf("the pane draws no hint of its own")
			}
		})
	}
}
