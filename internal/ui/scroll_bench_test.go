package ui

import (
	"fmt"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/turanmahmudov/masume/internal/app"
	"github.com/turanmahmudov/masume/internal/core"
	"github.com/turanmahmudov/masume/internal/db"
	"github.com/turanmahmudov/masume/internal/hist"
)

// buildLoadedModel answers a model on a catalog of many relations, with a wide result in the
// grid: the shape a reader of a real server looks at.
func buildLoadedModel(reporter testing.TB, schemas, perSchema, rows, columns int) *Model {
	reporter.Helper()
	model := buildOfflineModelFor(reporter, 160, 48)
	connection := model.Active()

	tables := make([]db.TableRef, 0, schemas*perSchema)
	for s := range schemas {
		for at := range perSchema {
			tables = append(tables, db.TableRef{
				Schema: fmt.Sprintf("schema_%02d", s),
				Name:   fmt.Sprintf("table_%04d", at), Kind: db.RelationTable,
			})
		}
	}
	connection.Catalog.Tables = tables
	connection.Catalog.Loading = false

	held := make([]db.ResultColumn, 0, columns)
	for at := range columns {
		held = append(held, db.ResultColumn{
			Name: fmt.Sprintf("column_%02d", at), DataType: "text",
		})
	}
	values := make([][]any, 0, rows)
	for at := range rows {
		row := make([]any, 0, columns)
		for column := range columns {
			row = append(row, fmt.Sprintf("value %d of row %d", column, at))
		}
		values = append(values, row)
	}
	tab := connection.Active()
	// The editor holds a statement, because the header reads the sort out of it and that
	// reads every token of the buffer.
	tab.Editor = app.NewEditorBuffer("select c.id, c.name, o.placed_at, o.total\n"+
		"  from schema_00.table_0000 as o\n"+
		"  join schema_00.table_0001 as c on c.id = o.customer_id\n"+
		" where o.placed_at >= now() - interval '30 days'\n"+
		" order by o.placed_at desc, c.name asc", 0)
	tab.Results.Start([]string{"select * from wide"}, 200)
	tab.Results.Succeed(0, db.ComposedRead{Text: "select * from wide"},
		db.QueryResult{Columns: held, Rows: values})
	return model
}

// One scroll step is one key press and the frame that follows it. This is what a reader feels
// when they hold a cursor key down, so it is the number that matters.
func benchmarkScrollStep(b *testing.B, focus app.Pane) {
	model := buildLoadedModel(b, 20, 250, 2000, 30)
	model.Active().Active().Focus = focus
	// The first frame builds what the later ones keep.
	model.View()

	b.ReportAllocs()
	for b.Loop() {
		held, _ := model.Update(tea.KeyPressMsg{Code: tea.KeyDown})
		model = held.(*Model)
		model.View()
	}
}

func BenchmarkScrollTheTree(b *testing.B)   { benchmarkScrollStep(b, app.PaneSidebar) }
func BenchmarkScrollTheResult(b *testing.B) { benchmarkScrollStep(b, app.PaneResult) }

// BenchmarkScrollTheOpenTree is the same tree with every schema opened, so the rows the pane
// holds are thousands and not tens. A reader who opens a schema of a real server is here.
func BenchmarkScrollTheOpenTree(b *testing.B) {
	model := buildLoadedModel(b, 20, 250, 2000, 30)
	connection := model.Active()
	for schema := range 20 {
		connection.Tree.Expanded[core.BuildSchemaID(fmt.Sprintf("schema_%02d", schema))] = true
	}
	connection.Active().Focus = app.PaneSidebar
	model.View()

	b.ReportAllocs()
	for b.Loop() {
		held, _ := model.Update(tea.KeyPressMsg{Code: tea.KeyDown})
		model = held.(*Model)
		model.View()
	}
}

// buildChattingModel answers a model with a long conversation open in the chat panel.
func buildChattingModel(reporter testing.TB, turns int) *Model {
	reporter.Helper()
	model := buildLoadedModel(reporter, 20, 250, 2000, 30)
	connection := model.Active()

	said := "the orders table holds one row per order, and the customer it belongs to is " +
		"named by customer_id. A read of the last month goes through the placed_at index."
	messages := make([]app.ChatMessage, 0, turns)
	for at := range turns {
		messages = append(messages, app.ChatMessage{
			Role: hist.ChatRoleUser, Content: fmt.Sprintf("question %d", at),
		})
		messages = append(messages, app.ChatMessage{
			Role:    hist.ChatRoleAssistant,
			Content: said + "\n\n```sql\nselect * from orders limit 10;\n```",
		})
	}
	connection.Chat.Messages = messages
	connection.Overlay = app.Overlay{Kind: app.OverlayAiChat, Draft: app.NewEditorBuffer("", 0)}
	return model
}

// One roll of the wheel over a long conversation, and the frame that follows it.
func BenchmarkScrollTheChat(b *testing.B) {
	model := buildChattingModel(b, 60)
	model.View()

	b.ReportAllocs()
	for b.Loop() {
		held, _ := model.Update(tea.MouseWheelMsg{Button: tea.MouseWheelDown})
		model = held.(*Model)
		model.View()
	}
}
