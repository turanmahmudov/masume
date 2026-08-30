package ui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/turanmahmudov/masume/internal/app"
	"github.com/turanmahmudov/masume/internal/cfg"
	"github.com/turanmahmudov/masume/internal/core"
	"github.com/turanmahmudov/masume/internal/db"
	"github.com/turanmahmudov/masume/internal/present"
)

// buildObjectMenuModel answers a model with the object menu open on a table.
func buildObjectMenuModel(t *testing.T) (*Model, *app.Connection) {
	t.Helper()
	model := buildLoadedModel(t, 1, 3, 8, 3)
	connection := model.Active()
	connection.Session.(*offlineSession).capabilities = core.Capabilities{
		SortsRead: true, WritesDDL: true, TruncatesTable: true,
	}
	node := present.TreeNode{
		Kind:  present.NodeTable,
		Table: db.TableRef{Schema: "public", Name: "orders", Kind: db.RelationTable},
	}
	connection.Overlay = app.Overlay{
		Kind: app.OverlayObjectMenu, Title: " table orders ",
		Draft:   app.NewEditorBuffer("", 0),
		Actions: app.BuildObjectActions(node, connection.Session.Capabilities()),
	}
	return model, connection
}

// Every row of the object menu acts on a different kind of thing, so each one carries the
// glyph of what it acts on.
func TestTheRowsOfTheObjectMenuCarryTheirGlyph(t *testing.T) {
	model, connection := buildObjectMenuModel(t)
	frame := strings.Split(model.render(), "\n")

	block := model.layout.overlayRows
	for at, action := range connection.Overlay.Actions {
		if at >= block.count {
			break
		}
		if action.Icon == "" {
			t.Errorf("the row %q names no glyph", action.Label)
			continue
		}
		text := cutRowText(frame[block.top+at], block.from, block.to)
		glyph := model.icons.Icon(action.Icon)
		if !strings.Contains(text, glyph+" "+action.Label) {
			t.Errorf("the row of %q reads %q, and its glyph is %q",
				action.Label, strings.TrimSpace(text), glyph)
		}
	}
}

// A menu whose rows all act on the same thing keeps no column for a glyph.
func TestTheCopyMenuKeepsNoGlyphColumn(t *testing.T) {
	model := buildLoadedModel(t, 1, 3, 8, 3)
	connection := model.Active()
	connection.Overlay = app.Overlay{
		Kind: app.OverlayCopyMenu, Title: " copy ",
		Draft: app.NewEditorBuffer("", 0), Actions: buildCopyMenuActions(),
	}
	frame := strings.Split(model.render(), "\n")

	block := model.layout.overlayRows
	text := cutRowText(frame[block.top], block.from, block.to)
	if !strings.HasPrefix(strings.TrimLeft(text, " "), "Cell") {
		t.Errorf("the first row of the copy menu reads %q", strings.TrimSpace(text))
	}
}

// Each chip of the view strip carries the glyph of its view, so a reader picks one by its
// mark as well as by its name.
func TestTheViewChipsCarryTheirGlyph(t *testing.T) {
	model := buildLoadedModel(t, 1, 3, 8, 3)
	tab := model.Active().Active()
	tab.Focus = app.PaneResult
	frame := strings.Split(model.render(), "\n")

	views := tab.Views(model.Active().Session)
	for _, chip := range model.layout.viewChips {
		view := views[chip.index]
		glyph := model.icons.Icon(viewIcons[view])
		if glyph == "" {
			t.Errorf("the view %q names no glyph", view)
			continue
		}
		text := cutRowText(frame[chip.row], chip.from, chip.to)
		if !strings.Contains(text, glyph+" "+string(view)) {
			t.Errorf("the chip of %q reads %q, and its glyph is %q", view, text, glyph)
		}
	}
}

// A report the reader has to act on carries its mark, so the bar is read by its shape before
// it is read as words.
func TestTheStatusReportCarriesItsMark(t *testing.T) {
	model, connection, _ := buildEditingModel(t, "select id from orders", 0)
	connection.Autocommit = false
	frame := strings.Split(model.render(), "\n")

	bar := stripStyles(frame[model.layout.hintRow])
	if !strings.Contains(bar, model.icons.Icon(cfg.IconNote)+" autocommit is off") {
		t.Errorf("the bar reads %q", strings.TrimSpace(bar))
	}
}

// A read the server can be told to stop names the key beside the wheel, and a press on the
// word stops it.
func TestARunningStatementNamesTheKeyThatStopsIt(t *testing.T) {
	model := buildLoadedModel(t, 1, 3, 8, 3)
	connection := model.Active()
	connection.Session.(*offlineSession).capabilities = core.Capabilities{
		SortsRead: true, CancelsRunningQuery: true,
	}
	tab := connection.Active()
	tab.Focus = app.PaneResult
	tab.Results.Start([]string{"select pg_sleep(30)"}, 100)
	frame := strings.Split(model.render(), "\n")

	held, found := findWaitButton(model, ActionCancelQuery, model.layout.hintRow)
	if !found {
		t.Fatal("the running pane recorded no key that stops the read")
	}
	if text := cutRowText(frame[held.row], held.from, held.to); !strings.Contains(
		text, "stop") {
		t.Errorf("the stop key covers %q", text)
	}
}

// A server that cannot stop a read names no key for it.
func TestAServerThatCannotStopAReadNamesNoKey(t *testing.T) {
	model := buildLoadedModel(t, 1, 3, 8, 3)
	tab := model.Active().Active()
	tab.Focus = app.PaneResult
	tab.Results.Start([]string{"select pg_sleep(30)"}, 100)
	model.render()

	if _, found := findWaitButton(model, ActionCancelQuery, model.layout.hintRow); found {
		t.Error("the running pane named a key the server does not answer")
	}
}

// findWaitButton answers the key of an action drawn anywhere but on the status bar.
func findWaitButton(model *Model, action ActionID, barRow int) (buttonHit, bool) {
	for _, held := range model.layout.buttons {
		if held.action == action && held.row != barRow {
			return held, true
		}
	}
	return buttonHit{}, false
}

// A press on the stop key stops the read, as the key does.
func TestAPressOnTheStopKeyStopsTheRead(t *testing.T) {
	model := buildLoadedModel(t, 1, 3, 8, 3)
	connection := model.Active()
	connection.Session.(*offlineSession).capabilities = core.Capabilities{
		SortsRead: true, CancelsRunningQuery: true,
	}
	tab := connection.Active()
	tab.Focus = app.PaneResult
	tab.Results.Start([]string{"select pg_sleep(30)"}, 100)
	model.render()

	held, _ := findWaitButton(model, ActionCancelQuery, model.layout.hintRow)
	if _, command := model.readMouse(tea.MouseClickMsg{
		X: held.from, Y: held.row, Button: tea.MouseLeft,
	}); command == nil {
		t.Error("a press on the stop key asked the server for nothing")
	}
}
