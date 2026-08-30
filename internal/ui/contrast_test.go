package ui

import (
	"regexp"
	"strconv"
	"strings"
	"testing"

	"github.com/turanmahmudov/masume/internal/app"
	"github.com/turanmahmudov/masume/internal/cfg"
	"github.com/turanmahmudov/masume/internal/core"
)

// The mark of the pointer fills a row, ground and ink together, as the row under the cursor is
// filled. Both have to be answered together: an ink chosen against the ground of the pane
// cannot be read on another ground, and laying a ground without the ink is what made the
// quietest text of a marked row disappear. These tests read the ink and the ground of every
// marked cell back off the frame and measure the contrast between them.

// readableContrast is the least contrast a cell of a marked row is allowed. A filled row is
// held to the same floor as any other text on a filled ground.
const readableContrast = TextContrastFloor

var (
	inkEscape    = regexp.MustCompile(`38;2;(\d+);(\d+);(\d+)`)
	groundEscape = regexp.MustCompile(`48;2;(\d+);(\d+);(\d+)`)
)

// findLastColor answers the colour the escapes of a cell last set, and whether they set one.
func findLastColor(pattern *regexp.Regexp, sgr string) (float64, float64, float64, bool) {
	found := pattern.FindAllStringSubmatch(sgr, -1)
	if len(found) == 0 {
		return 0, 0, 0, false
	}
	held := found[len(found)-1]
	red, _ := strconv.Atoi(held[1])
	green, _ := strconv.Atoi(held[2])
	blue, _ := strconv.Atoi(held[3])
	return float64(red), float64(green), float64(blue), true
}

// calculateLuminance answers how bright a colour is, as the web measures it.
func calculateLuminance(red, green, blue float64) float64 {
	channel := func(value float64) float64 {
		value /= 255
		if value <= 0.03928 {
			return value / 12.92
		}
		held := (value + 0.055) / 1.055
		return held * held
	}
	return 0.2126*channel(red) + 0.7152*channel(green) + 0.0722*channel(blue)
}

// calculateCellContrast answers the contrast between the ink and the ground of one cell, and
// whether the cell names both.
func calculateCellContrast(sgr string) (float64, bool) {
	inkRed, inkGreen, inkBlue, hasInk := findLastColor(inkEscape, sgr)
	groundRed, groundGreen, groundBlue, hasGround := findLastColor(groundEscape, sgr)
	if !hasInk || !hasGround {
		return 0, false
	}
	lighter := calculateLuminance(inkRed, inkGreen, inkBlue)
	darker := calculateLuminance(groundRed, groundGreen, groundBlue)
	if lighter < darker {
		lighter, darker = darker, lighter
	}
	return (lighter + 0.05) / (darker + 0.05), true
}

// findWorstContrast answers the least contrast of the cells a span covers, and the text of the
// cell that holds it.
func findWorstContrast(line string, from, to int) (float64, string, bool) {
	worst, written, measured := 0.0, "", false
	for at, cell := range mapCells(line) {
		if at < from || at > to || strings.TrimSpace(cell.text) == "" {
			continue
		}
		ratio, named := calculateCellContrast(cell.sgr)
		if !named {
			continue
		}
		if !measured || ratio < worst {
			worst, written, measured = ratio, cell.text, true
		}
	}
	return worst, written, measured
}

// checkHoverKeepsItReadable moves the pointer onto a cell and fails where the mark left any
// cell of the row it marked unreadable.
func checkHoverKeepsItReadable(t *testing.T, model *Model, name string, x, y int) *Model {
	t.Helper()
	next := movePointer(model, x, y)
	target := next.frame.hover
	if !target.isSomething() {
		// A row already drawn on a ground of its own takes no mark, which is the answer
		// this test is written for.
		return next
	}
	marked := strings.Split(next.frame.shown, "\n")[target.row]
	worst, written, measured := findWorstContrast(marked, target.from, target.to)
	if !measured {
		return next
	}
	if worst < readableContrast {
		t.Errorf("the mark on %s leaves %q at a contrast of %.2f, and %.2f is the least",
			name, written, worst, readableContrast)
	}
	return next
}

// buildHoverModel answers a model with a tree, a grid, two tabs and two statements, so every
// part a pointer can stand on is drawn.
func buildHoverModel(t *testing.T) (*Model, *app.Connection) {
	t.Helper()
	model := buildLoadedModel(t, 2, 5, 20, 4)
	connection := model.Active()
	connection.Tree.Expanded[core.BuildSchemaID("schema_00")] = true
	connection.OpenQueryTab("select two from three")
	connection.ActivateTab(0)
	model.View()
	return model, connection
}

// Every row the pointer can mark stays readable under the mark, whether the pane holds the
// keyboard or not.
func TestTheMarkOfThePointerKeepsEveryRowReadable(t *testing.T) {
	for _, focus := range []app.Pane{app.PaneSidebar, app.PaneEditor, app.PaneResult} {
		model, connection := buildHoverModel(t)
		tab := connection.Active()
		tab.Focus = focus
		model.View()

		block := model.layout.treeRows
		for at := 0; at < block.count; at++ {
			model = checkHoverKeepsItReadable(t, model,
				"a row of the tree", block.from+6, block.top+at)
		}
		held := model.layout.connections
		for at := 0; at < held.count; at++ {
			model = checkHoverKeepsItReadable(t, model,
				"a row of the connection list", held.from+6, held.top+at)
		}
		for _, item := range model.layout.tabs {
			model = checkHoverKeepsItReadable(t, model,
				"a tab", item.from+2, model.layout.tabRow)
		}
		for _, chip := range model.layout.viewChips {
			model = checkHoverKeepsItReadable(t, model,
				"a chip of the view strip", chip.from+2, chip.row)
		}
		for _, column := range model.layout.gridColumns {
			model = checkHoverKeepsItReadable(t, model,
				"the name of a column", column.from+2, model.layout.gridHeaderRow)
		}
		grid := model.layout.gridRows
		for at := 0; at < grid.count && at < 6; at++ {
			model = checkHoverKeepsItReadable(t, model,
				"a row of the grid", grid.from+6, grid.top+at)
		}
	}
}

// The rows of a card stay readable under the mark as well, and the row under the cursor takes
// none.
func TestTheMarkOfThePointerKeepsACardReadable(t *testing.T) {
	model, connection := buildHoverModel(t)
	connection.Overlay = app.Overlay{
		Kind: app.OverlayObjectMenu, Title: " table orders ",
		Draft: app.NewEditorBuffer("", 0),
		Actions: []app.MenuAction{
			{ID: "a", Label: "Select 100 rows"}, {ID: "b", Label: "Count rows"},
			{ID: "c", Label: "Show DDL"}, {ID: "d", Label: "ER diagram"},
		},
	}
	model.View()

	block := model.layout.overlayRows
	for at := range 4 {
		model = checkHoverKeepsItReadable(t, model,
			"a row of the menu", block.from+6, block.top+at)
	}
}

// The rows of the connection picker stay readable under the mark.
func TestTheMarkOfThePointerKeepsThePickerReadable(t *testing.T) {
	model := buildOfflineModel(t, 120, 34)
	model.screen = ScreenPickingProfile
	model.connections = openConnections{}
	model.profiles = []cfg.Profile{
		{Name: "alpha", Engine: "postgres", Environment: cfg.EnvironmentDev},
		{Name: "bravo", Engine: "mysql", Environment: cfg.EnvironmentProd},
		{Name: "charlie", Engine: "sqlite", Environment: cfg.EnvironmentTest},
	}
	model.View()

	block := model.layout.pickerRows
	for at := 0; at < block.count; at++ {
		model = checkHoverKeepsItReadable(t, model,
			"a row of the picker", block.from+4, block.top+at)
	}
}

// A row already drawn on a ground of its own takes no mark at all, because its ink was chosen
// against that ground.
func TestARowDrawnOnItsOwnGroundTakesNoMark(t *testing.T) {
	model, connection := buildHoverModel(t)
	tab := connection.Active()
	tab.Focus = app.PaneSidebar
	model.View()

	block := model.layout.treeRows
	cursor := block.top + connection.Tree.Cursor - block.offset
	model = movePointer(model, block.from+6, cursor)
	if model.frame.hover.isSomething() {
		t.Errorf("the row under the cursor of the tree took the mark %+v", model.frame.hover)
	}

	// The same row takes the mark once the pane no longer holds the keyboard, because the
	// cursor of a pane that is not focused is drawn quietly and reads under the mark.
	tab.Focus = app.PaneResult
	model = movePointer(model, block.from+7, cursor)
	if !model.frame.hover.isSomething() {
		t.Error("the row took no mark once the pane lost the keyboard")
	}
}
