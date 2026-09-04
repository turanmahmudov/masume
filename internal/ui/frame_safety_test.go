package ui

import (
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/turanmahmudov/masume/internal/app"
	"github.com/turanmahmudov/masume/internal/core"
	"github.com/turanmahmudov/masume/internal/db"
	"github.com/turanmahmudov/masume/internal/present"
)

// nastyValues are the values a server can hand over that a terminal cannot draw as they
// stand: bytes that are no text, an escape that would colour the rest of the row, and the
// control characters that move a cursor.
var nastyValues = []any{
	[]byte{0x9a, 0xab, 0x4a, 0x28, 0xa0, 0x11, 0x11, 0xf1},
	"red\x1b[31mtext",
	"bell\atab\tend",
	"carriage\rreturn",
	"null\x00byte",
	"delete\x7fhere",
	"\x9bcsi",
	string([]byte{0xff, 0xfe, 0x41}),
	"line\nbreak",
	strings.Repeat("wide KANJI ", 12),
	"\x1b]0;title\a",
	"",
}

// stripFrameStyles removes the colour escapes the painter writes, and nothing else.
func stripFrameStyles(row string) string {
	built := &strings.Builder{}
	for at := 0; at < len(row); {
		if row[at] == 0x1b && at+1 < len(row) && row[at+1] == '[' {
			end := at + 2
			for end < len(row) && row[end] != 'm' {
				end++
			}
			if end < len(row) {
				at = end + 1
				continue
			}
		}
		built.WriteByte(row[at])
		at++
	}
	return built.String()
}

// readFrameRows answers each row of a drawn frame with its escapes taken off.
func readFrameRows(frame string) []string {
	rows := strings.Split(frame, "\n")
	plain := make([]string, 0, len(rows))
	for _, row := range rows {
		plain = append(plain, stripFrameStyles(row))
	}
	return plain
}

// isUndrawable is true for a character a terminal acts on rather than draws.
func isUndrawable(character rune) bool {
	if character == utf8.RuneError {
		return false
	}
	return character < 0x20 || character == 0x7f || (character >= 0x80 && character <= 0x9f)
}

// TestFrameHoldsNothingATerminalCannotDraw draws a result of values a terminal cannot draw as
// they stand, and reads every row of the frame back. A row that carries a control character
// moves the cursor of the terminal, which breaks the borders of every pane after it, and a row
// that measures wider or narrower than the screen breaks them too.
func TestFrameHoldsNothingATerminalCannotDraw(t *testing.T) {
	const width, height = 120, 34

	columns := make([]db.ResultColumn, 0, len(nastyValues))
	row := make([]any, 0, len(nastyValues))
	for at, value := range nastyValues {
		columns = append(columns, db.ResultColumn{
			Name: "col" + string(rune('a'+at)), DataType: "text",
		})
		row = append(row, value)
	}

	model := buildOfflineModel(t, width, height)
	connection := model.Active()
	tab := connection.Active()
	// A name the server wrote reaches the frame as well as a value, so the names carry
	// the same bytes.
	columns[0].Name = "id\x1b[31m"
	columns[1].Name = "bell\aname"
	tab.Editor.SetText("select \x07 from nasty\x1b[31m")
	tab.Results.Start([]string{"select * from nasty"}, 100)
	tab.Results.Succeed(0, db.ComposedRead{Text: "select * from nasty"}, db.QueryResult{
		Columns: columns, Rows: [][]any{row, row},
	})

	for _, held := range []struct {
		name string
		open func()
	}{
		{"the grid", func() { tab.View = app.ViewData }},
		{"the fields", func() { tab.View = app.ViewFields }},
		{"the document tree", func() {
			// The keys and the values of a document reach the frame straight out of the
			// text the server sent, so both carry bytes a terminal would act on.
			tab.View = app.ViewTree
			nested := "{\"na\x1b[31mme\":\"bell\"," +
				"\"deep\":{\"cr\rlf\":\"tab\there\"}," +
				"\"list\":[\"esc\x1b[0m\",null]}"
			documentColumn := db.ResultColumn{Name: "doc\x1b[31m", DataType: "object"}
			tab.Results.Succeed(0, db.ComposedRead{Text: "select * from nasty"},
				db.QueryResult{
					Columns: append(append([]db.ResultColumn{}, columns...), documentColumn),
					Rows: [][]any{append(append([]any{}, row...),
						core.DocumentValue{Text: nested, Count: 3})},
				})
			held := present.BuildRowPath(0)
			field := held + documentSeparator + "doc\x1b[31m"
			tab.Opened = map[string]bool{
				held: true, field: true,
				field + documentSeparator + "deep": true,
				field + documentSeparator + "list": true,
			}
		}},
		{"the row card", func() {
			connection.Overlay = app.Overlay{
				Kind:   app.OverlayRowDetail,
				Window: app.RowWindow{Columns: columns, Rows: [][]any{row}, Index: 0},
			}
		}},
		{"the cell card", func() {
			connection.Overlay = app.Overlay{
				Kind: app.OverlayCell,
				Cell: app.CellTarget{Column: columns[0], Value: row[0]},
			}
		}},
		{"the editor", func() { tab.Focus = app.PaneEditor }},
		{"a report on the bar", func() {
			connection.ShowError("the server said: bad\x1b[31mthing\a")
		}},
		{"the message card", func() {
			connection.Overlay = app.Overlay{
				Kind: app.OverlayMessage, Title: " report ",
				Body: "line one\x1b[31m\nline\atwo",
			}
		}},
		{"the server dashboard", func() {
			// Every text of this card comes from the server: the statement of a
			// session, the name of a relation and the mode of a lock all reach the
			// frame as the server wrote them.
			nasty := "alter\x1b[31m table\a orders\r"
			connection.Overlay = app.Overlay{
				Kind: app.OverlayActivity,
				Sessions: []db.Activity{{
					PID: 4417, User: "wri\x1bter", ApplicationName: "ps\aql",
					State: "act\rive", Duration: 4 * time.Minute, Query: nasty,
				}},
				Server: app.ServerReading{
					HasLoad: true, HasLocks: true,
					Load: db.ServerLoad{
						Connections: 84, MaxConnections: 100,
						StartedAt: time.Now().Add(-41 * 24 * time.Hour),
					},
					Locks: []db.LockWait{{
						BlockedPID: 4520, BlockedQuery: nasty, Waiting: 4 * time.Minute,
						Mode: "Access\x1b[31mExclusiveLock", Relation: "ord\aers",
						BlockingPID: 4417, BlockingQuery: nasty,
						BlockingFor: 5 * time.Minute,
					}},
					ReadAt: time.Now(),
				},
			}
		}},
		{"the server dashboard with its panel folded", func() {
			connection.Overlay = app.Overlay{
				Kind: app.OverlayActivity,
				Server: app.ServerReading{
					HasLocks: true,
					Locks: []db.LockWait{{
						BlockedPID: 4520, BlockedQuery: "select \a1", Waiting: time.Minute,
						BlockingPID: 4417, BlockingQuery: "alter\x1b[31m table",
					}},
					ReadAt: time.Now(),
				},
				View: app.DashboardView{Folded: map[string]bool{app.PanelBlocking: true}},
			}
		}},
		{"the cell editor", func() {
			connection.Overlay = buildCellEditor(app.Overlay{
				Kind: app.OverlayCellEdit,
				Cell: app.CellTarget{Column: columns[1], RowIndex: 0, ColumnIndex: 1},
			}, present.FormatForViewer(row[1], "text"))
		}},
	} {
		connection.Overlay = app.Overlay{}
		held.open()
		for at, drawn := range readFrameRows(model.View().Content) {
			if measured := present.MeasureText(drawn); measured != width {
				t.Errorf("%s: row %d measures %d, wanted %d: %q",
					held.name, at, measured, width, drawn)
			}
			if mark := strings.IndexFunc(drawn, isUndrawable); mark >= 0 {
				t.Errorf("%s: row %d carries a character a terminal acts on: %q",
					held.name, at, drawn)
			}
			if !utf8.ValidString(drawn) {
				t.Errorf("%s: row %d is not text: %q", held.name, at, drawn)
			}
		}
	}
}

// documentSeparator is what a path of the document tree puts between one key and the next.
const documentSeparator = "\x1f"
