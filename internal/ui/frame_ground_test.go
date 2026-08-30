package ui

import (
	"strings"
	"testing"

	"github.com/turanmahmudov/masume/internal/app"
	"github.com/turanmahmudov/masume/internal/db"
	"github.com/turanmahmudov/masume/internal/hist"
)

// findDroppedGround answers where a row lets its ground go: a reset with text after it that
// opens no colours of its own. A terminal draws that text on its own ground and not on the
// ground of the pane, so a row with one reads as a hole in the pane.
//
// Every part of a row is written with its own colours and closed with a reset, so the byte
// after a reset is either another escape or the end of the row.
func findDroppedGround(row string) int {
	at := 0
	for at < len(row) {
		if row[at] != 0x1b {
			at++
			continue
		}
		length := measureEscape(row[at:])
		if row[at:at+length] != resetSequence {
			at += length
			continue
		}
		if at+length < len(row) && row[at+length] != 0x1b {
			return at + length
		}
		at += length
	}
	return -1
}

// buildPaintedModel answers a model with a catalog, a wide result and a conversation, so every
// pane of the frame has something in it.
func buildPaintedModel(t *testing.T) (*Model, *app.Connection, *app.Tab) {
	t.Helper()
	model := buildOfflineModel(t, 120, 34)
	connection := model.Active()
	tab := connection.Active()

	connection.Catalog.Tables = []db.TableRef{
		{Schema: "public", Name: "orders", Kind: db.RelationTable},
		{Schema: "public", Name: "customers", Kind: db.RelationTable},
		{Schema: "archive", Name: "orders_2024", Kind: db.RelationView},
	}
	connection.Catalog.Loading = false
	tab.Editor = app.NewEditorBuffer(
		"select o.id, c.name\n  from public.orders as o\n  join public.customers as c"+
			" on c.id = o.customer_id", 0)
	tab.Results.Start([]string{"select * from orders"}, 200)
	tab.Results.Succeed(0, db.ComposedRead{Text: "select * from orders"}, db.QueryResult{
		Columns: []db.ResultColumn{
			{Name: "id", DataType: "integer"},
			{Name: "customer", DataType: "text"},
			{Name: "password", DataType: "text"},
		},
		Rows: [][]any{
			{int64(1), "ada", "secret"},
			{int64(2), nil, "secret"},
			{int64(3), "a much longer customer name than the rest", "secret"},
		},
	})
	connection.Chat.Messages = []app.ChatMessage{
		{Role: hist.ChatRoleUser, Content: "which table holds the orders?"},
		{
			Role:    hist.ChatRoleAssistant,
			Content: "public.orders does.\n\n```sql\nselect * from public.orders;\n```",
		},
	}
	return model, connection, tab
}

// No pane may let its ground go. The rows of the grid, of the tree and of a block of code are
// each written part by part, and a part that closes without the next one opening leaves a cell
// on the ground of the terminal.
func TestNoRowOfTheFrameLetsItsGroundGo(t *testing.T) {
	model, connection, tab := buildPaintedModel(t)

	for _, held := range []struct {
		name string
		open func()
	}{
		{"the grid", func() { tab.Focus = app.PaneResult }},
		{"the grid with the cursor on a cell", func() {
			tab.Focus, tab.GridRow, tab.GridColumn = app.PaneResult, 1, 1
		}},
		{"the grid with the masking taken off", func() {
			tab.Focus, tab.Unmasked = app.PaneResult, true
		}},
		{"the tree", func() { tab.Focus = app.PaneSidebar }},
		{"the tree opened", func() {
			tab.Focus = app.PaneSidebar
			connection.Tree.Expanded["schema:public"] = true
		}},
		{"the editor", func() { tab.Focus = app.PaneEditor }},
		{"the fields", func() { tab.View = app.ViewFields }},
		{"the chat", func() {
			connection.Overlay = app.Overlay{
				Kind: app.OverlayAiChat, Draft: app.NewEditorBuffer("", 0),
			}
		}},
		{"the help card", func() {
			connection.Overlay = app.Overlay{Kind: app.OverlayHelp}
		}},
	} {
		connection.Overlay = app.Overlay{}
		held.open()
		for at, row := range strings.Split(model.View().Content, "\n") {
			if mark := findDroppedGround(row); mark >= 0 {
				t.Errorf("%s: row %d lets its ground go at byte %d: %q",
					held.name, at, mark, row)
				break
			}
		}
	}
}

// The reader of a row has to find a ground that was let go, or the test above passes on a
// frame full of holes.
func TestFindDroppedGroundReadsARowThatLetItsGroundGo(t *testing.T) {
	opening := resolveOpening(nil, buildOfflineModel(t, 80, 24).styles.Theme.Panel)
	cases := []struct {
		name    string
		row     string
		dropped bool
	}{
		{name: "plain text with no colours at all", row: "held", dropped: false},
		{
			name: "one part that closes at the end of the row",
			row:  opening + "held" + resetSequence, dropped: false,
		},
		{
			name:    "two parts, each opening its own colours",
			row:     opening + "held" + resetSequence + opening + "again" + resetSequence,
			dropped: false,
		},
		{
			name: "a part that closes and text after it that opens nothing",
			row:  opening + "held" + resetSequence + "again", dropped: true,
		},
		{
			name: "a blank after a part that closed",
			row:  opening + "held" + resetSequence + " ", dropped: true,
		},
	}

	for _, held := range cases {
		t.Run(held.name, func(t *testing.T) {
			mark := findDroppedGround(held.row)
			if held.dropped && mark < 0 {
				t.Errorf("the row lets its ground go and was read as whole: %q", held.row)
			}
			if !held.dropped && mark >= 0 {
				t.Errorf("the row was read as letting its ground go at byte %d: %q",
					mark, held.row)
			}
		})
	}
}
