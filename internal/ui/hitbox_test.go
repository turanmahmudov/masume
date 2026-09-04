package ui

import (
	"strconv"
	"strings"
	"testing"

	"github.com/turanmahmudov/masume/internal/app"
	"github.com/turanmahmudov/masume/internal/cfg"
	"github.com/turanmahmudov/masume/internal/core"
	"github.com/turanmahmudov/masume/internal/query/result"
)

// A hit box that stands one cell off the text it was drawn for sends every press to the row
// beside it, and nothing else in the client reports that. These tests render a frame, cut the
// cells each recorded box covers, and fail where the text under the box is not the text the
// box stands for.

// checkRowsHold reads back a block of rows drawn one item per row. It fails where the cells
// of a row do not hold the item the block says is there.
func checkRowsHold(t *testing.T, name string, frame []string, block rowsHit, items []string) {
	t.Helper()
	if block.count < 1 {
		t.Fatalf("%s recorded no row at all", name)
	}
	for at := 0; at < block.count; at++ {
		item := block.offset + at
		if item >= len(items) {
			break
		}
		row := block.top + at
		if row < 0 || row >= len(frame) {
			t.Fatalf("%s puts item %d on row %d, and the frame has %d rows",
				name, item, row, len(frame))
		}
		text := cutRowText(frame[row], block.from, block.to)
		if !strings.Contains(text, items[item]) {
			t.Errorf("%s says row %d holds %q, and the row reads %q",
				name, row, items[item], text)
		}
	}
}

// buildMenuModel answers a model with an object menu open on a table.
func buildMenuModel(t *testing.T) (*Model, *app.Connection, []string) {
	t.Helper()
	model, connection, _ := buildEditingModel(t, "select id from orders", 0)
	labels := []string{"Alpha", "Bravo", "Charlie", "Delta", "Echo"}
	actions := make([]app.MenuAction, 0, len(labels))
	for at, label := range labels {
		actions = append(actions, app.MenuAction{
			ID: label, Label: label, Chord: string(rune('a' + at)),
		})
	}
	connection.Overlay = app.Overlay{
		Kind: app.OverlayObjectMenu, Title: " public.orders ", Actions: actions,
	}
	return model, connection, labels
}

func TestTheRowsOfAMenuStandWhereTheyAreDrawn(t *testing.T) {
	model, _, labels := buildMenuModel(t)
	frame := strings.Split(model.render(), "\n")
	checkRowsHold(t, "the object menu", frame, model.layout.overlayRows, labels)
}

// The same card draws the palette, the history and every other list, so one of them standing
// right is not enough.
func TestTheRowsOfThePaletteStandWhereTheyAreDrawn(t *testing.T) {
	model, connection, _ := buildEditingModel(t, "select id from orders", 0)
	connection.Overlay = app.Overlay{
		Kind: app.OverlayPalette, Palette: model.buildPaletteActions(connection),
		Draft: app.NewEditorBuffer("", 0),
	}
	frame := strings.Split(model.render(), "\n")

	block := model.layout.overlayRows
	if block.count < 1 {
		t.Fatal("the palette recorded no row at all")
	}
	actions := model.filterPalette(connection.Overlay)
	if len(actions) == 0 {
		t.Fatal("the palette listed nothing")
	}
	labels := make([]string, 0, len(actions))
	for _, action := range actions {
		labels = append(labels, action.Label)
	}
	checkRowsHold(t, "the palette", frame, block, labels)
}

func TestTheRowsOfTheTreeStandWhereTheyAreDrawn(t *testing.T) {
	model := buildLoadedModel(t, 2, 6, 10, 3)
	connection := model.Active()
	connection.Tree.Expanded[core.BuildSchemaID("schema_00")] = true
	frame := strings.Split(model.render(), "\n")

	rows := model.treeRows(connection)
	labels := make([]string, 0, len(rows))
	for _, row := range rows {
		labels = append(labels, row.Label)
	}
	checkRowsHold(t, "the tree", frame, model.layout.treeRows, labels)
}

func TestTheRowsOfTheConnectionListStandWhereTheyAreDrawn(t *testing.T) {
	model := buildLoadedModel(t, 1, 2, 4, 2)
	frame := strings.Split(model.render(), "\n")

	labels := make([]string, 0, model.connections.count())
	for _, connection := range model.connections.all() {
		labels = append(labels, connection.Profile().Name)
	}
	checkRowsHold(t, "the connection list", frame, model.layout.connections, labels)
}

func TestTheRowsOfTheGridStandWhereTheyAreDrawn(t *testing.T) {
	model := buildLoadedModel(t, 1, 2, 40, 3)
	connection := model.Active()
	tab := connection.Active()
	frame := strings.Split(model.render(), "\n")

	shape := model.buildGridShape(connection, tab)
	labels := make([]string, 0, len(shape.Text))
	for at := range shape.Text {
		labels = append(labels, "value 0 of row "+strconv.Itoa(at))
	}
	checkRowsHold(t, "the grid", frame, model.layout.gridRows, labels)
}

// A press on a name orders the read by it, so a name one column out orders by its neighbour.
func TestTheNamesOfTheGridStandWhereTheyAreDrawn(t *testing.T) {
	model := buildLoadedModel(t, 1, 2, 12, 4)
	connection := model.Active()
	tab := connection.Active()
	frame := strings.Split(model.render(), "\n")

	shape := model.buildGridShape(connection, tab)
	row := frame[model.layout.gridHeaderRow]
	for _, column := range model.layout.gridColumns {
		if column.index >= len(shape.Columns) {
			t.Fatalf("a name is recorded for column %d of %d", column.index, len(shape.Columns))
		}
		// A name is written from the first cell of its column, so the cells the box covers
		// have to open with it. One cell out and a press orders by the column beside it.
		text := cutRowText(row, column.from, column.to)
		if !strings.HasPrefix(text, shape.Columns[column.index].Name) {
			t.Errorf("the name of column %d covers %q, and the column is %q",
				column.index, text, shape.Columns[column.index].Name)
		}
	}
}

func TestTheChipsOfTheResultStandWhereTheyAreDrawn(t *testing.T) {
	model := buildLoadedModel(t, 1, 2, 8, 3)
	connection := model.Active()
	tab := connection.Active()
	frame := strings.Split(model.render(), "\n")

	views := tab.Views(connection.Session)
	if len(model.layout.viewChips) != len(views) {
		t.Fatalf("%d chips were recorded for %d views", len(model.layout.viewChips), len(views))
	}
	for _, chip := range model.layout.viewChips {
		text := cutRowText(frame[chip.row], chip.from, chip.to)
		if !strings.Contains(text, string(views[chip.index])) {
			t.Errorf("the chip of %q covers %q", views[chip.index], text)
		}
	}
}

func TestTheTabsStandWhereTheyAreDrawn(t *testing.T) {
	model, connection, _ := buildEditingModel(t, "select id from orders", 0)
	connection.OpenQueryTab("select two from three")
	connection.OpenQueryTab("select four from five")
	frame := strings.Split(model.render(), "\n")

	if len(model.layout.tabs) != len(connection.Tabs) {
		t.Fatalf("%d tabs were recorded for %d tabs",
			len(model.layout.tabs), len(connection.Tabs))
	}
	row := frame[model.layout.tabRow]
	for _, held := range model.layout.tabs {
		text := cutRowText(row, held.from, held.to)
		if !strings.Contains(text, strconv.Itoa(held.index+1)) {
			t.Errorf("tab %d covers %q", held.index, text)
		}
		if held.closeTo < held.closeFrom {
			continue
		}
		if mark := cutRowText(row, held.closeFrom, held.closeTo); !strings.Contains(
			mark, strings.TrimSpace(model.buildCloseMark())) {
			t.Errorf("the close mark of tab %d covers %q", held.index, mark)
		}
	}
}

func TestTheRowsOfTheProfilePickerStandWhereTheyAreDrawn(t *testing.T) {
	model := buildOfflineModel(t, 120, 34)
	model.screen = ScreenPickingProfile
	model.connections = openConnections{}
	model.profiles = []cfg.Profile{
		{Name: "alpha", Engine: "postgres", Environment: cfg.EnvironmentDev},
		{Name: "bravo", Engine: "mysql", Environment: cfg.EnvironmentProd},
		{Name: "charlie", Engine: "sqlite", Environment: cfg.EnvironmentTest},
	}
	frame := strings.Split(model.render(), "\n")

	labels := make([]string, 0, len(model.profiles))
	for _, profile := range model.profiles {
		labels = append(labels, profile.Name)
	}
	checkRowsHold(t, "the profile picker", frame, model.layout.pickerRows, labels)
}

func TestTheRowsOfTheConnectionFormStandWhereTheyAreDrawn(t *testing.T) {
	model := buildOfflineModel(t, 120, 40)
	model.screen = ScreenEditingConnection
	model.form = NewFormState(cfg.Profile{Name: "alpha", Engine: "postgres"}, true, nil)
	frame := strings.Split(model.render(), "\n")

	fields := model.form.Shown()
	labels := make([]string, 0, len(fields))
	for _, field := range fields {
		labels = append(labels, field.Label)
	}
	checkRowsHold(t, "the connection form", frame, model.layout.formRows, labels)
}

// A card that asks a question draws its answers as chips, and a press on one answers it.
func TestTheAnswersOfAQuestionStandWhereTheyAreDrawn(t *testing.T) {
	model, connection, _ := buildEditingModel(t, "delete from orders", 0)
	connection.Overlay = app.Overlay{
		Kind: app.OverlayConfirm, Title: " run it ", Body: "this reads no rows back",
	}
	frame := strings.Split(model.render(), "\n")

	if len(model.layout.overlayChips) != 2 {
		t.Fatalf("a question drew %d answers", len(model.layout.overlayChips))
	}
	wanted := []string{"run", "cancel"}
	for at, chip := range model.layout.overlayChips {
		text := cutRowText(frame[chip.row], chip.from, chip.to)
		if !strings.Contains(text, wanted[at]) {
			t.Errorf("the %q answer covers %q", wanted[at], text)
		}
	}
}

// A card with a list of answers marks the row a press landed on.
func TestTheAnswersOfAChoiceStandWhereTheyAreDrawn(t *testing.T) {
	model, connection, _ := buildEditingModel(t, "select id from orders", 0)
	connection.Overlay = app.Overlay{
		Kind: app.OverlayChoice, Title: " which one ", Body: "the name is used twice",
		Choices: []app.Choice{
			{ID: "one", Label: "public.orders", Key: "1"},
			{ID: "two", Label: "sales.orders", Key: "2"},
		},
	}
	frame := strings.Split(model.render(), "\n")

	labels := []string{"public.orders", "sales.orders"}
	checkRowsHold(t, "the choice card", frame, model.layout.formRows, labels)
}

// The list that stands over the statement has to answer the row a reader sees. It is placed on
// the frame of the workspace, and the title bar is put over that frame afterwards, so a row of
// the screen is one more than a row of the box.
func TestTheRowsOfTheCompletionListStandWhereTheyAreDrawn(t *testing.T) {
	model, connection, tab := buildScannedModel(t)
	tab.Focus = app.PaneEditor
	tab.Editor = app.NewEditorBuffer("select * from ", len("select * from "))
	model.refreshCompletion(connection, tab)
	if !tab.Completion.IsListing() {
		t.Skip("the statement offered nothing")
	}
	frame := strings.Split(model.render(), "\n")

	labels := make([]string, 0, len(tab.Completion.Candidates))
	for _, candidate := range tab.Completion.Candidates {
		labels = append(labels, candidate.Text)
	}
	checkRowsHold(t, "the completion list", frame, model.layout.completionRows, labels)
}

// The card that writes a result to a file marks the field a press landed on.
func TestTheFieldsOfTheExportCardStandWhereTheyAreDrawn(t *testing.T) {
	model := buildLoadedModel(t, 1, 2, 6, 3)
	connection := model.Active()
	connection.Overlay = app.Overlay{
		Kind:   app.OverlayExport,
		Export: app.ExportRequest{RowCount: 6, Path: "rows.csv", Format: result.ExportCSV},
	}
	frame := strings.Split(model.render(), "\n")

	fields := BuildExportFields(connection.Overlay)
	// A name too long for its column keeps the words that fit, so the first word of each
	// one is what the row is read for.
	labels := make([]string, 0, len(fields))
	for _, field := range fields {
		labels = append(labels, strings.Fields(field.Label)[0])
	}
	checkRowsHold(t, "the export card", frame, model.layout.formRows, labels)
}
