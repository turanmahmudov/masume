package ui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/turanmahmudov/masume/internal/app"
)

// A terminal narrower than the tree plus a readable pane draws the tree and the pane side by
// side, and the two together measure wider than the screen. Every row of the frame has to
// keep the width of the screen, or the borders of the panes break.
func TestANarrowTerminalDrawsNoRowWiderThanItself(t *testing.T) {
	for _, width := range []int{24, 30, 36, 44, 60} {
		model := buildOfflineModel(t, width, 20)
		if !model.Active().SidebarVisible {
			t.Fatal("the tree is not on screen, so the narrow frame is not measured")
		}
		for at, row := range readFrameRows(model.View().Content) {
			if measured := measureStyledWidth(row); measured > width {
				t.Errorf("at width %d row %d measures %d: %q",
					width, at, measured, strings.TrimRight(row, " "))
			}
		}
	}
}

// A question longer than the screen is cut to the rows that fit, and the rows it cut are
// counted. Without the count the reader answers a question whose body was silently short.
func TestALongQuestionIsCutToTheScreenAndCountsTheRowsItCut(t *testing.T) {
	model := buildOfflineModel(t, 80, 14)
	held, _ := model.Update(tea.WindowSizeMsg{Width: 80, Height: 14})
	model = held.(*Model)

	body := []string{"Save 40 connections to the config file?", ""}
	for at := range 40 {
		body = append(body, "shop-"+string(rune('a'+at%26))+"  postgres://host/shop")
	}
	model.confirm = &confirmState{
		Title: " save ", Body: strings.Join(body, "\n"), Yes: "save", No: "discard",
	}

	lines := model.buildConfirmLines(model.confirm, 64)
	if len(lines) > model.height-confirmCardChrome {
		t.Fatalf("the question holds %d rows on a screen of %d",
			len(lines), model.height)
	}
	joined := strings.Join(lines, "\n")
	if !strings.Contains(joined, "more rows") {
		t.Errorf("the question counts no cut rows: %q", joined)
	}
	if !strings.Contains(joined, "save") {
		t.Errorf("the question lost its key row: %q", joined)
	}
}

// A card once kept a floor of 40 columns after the clamp to the screen, so on a narrower
// terminal every card drew past the right edge and lost its border.
func TestEveryCardFitsATerminalNarrowerThanTheNarrowestCard(t *testing.T) {
	kinds := []app.OverlayKind{
		app.OverlayHelp, app.OverlayPalette, app.OverlayAiChat, app.OverlayActivity,
		app.OverlayRowDetail, app.OverlayConfirm, app.OverlayExport, app.OverlayImport,
		app.OverlayCellEdit, app.OverlayThemePicker, app.OverlayNotebooks,
		app.OverlayHistory, app.OverlaySaved, app.OverlayChart, app.OverlayValueFilter,
	}
	for _, width := range []int{20, 24, 30, 36, 39, 40, 80} {
		model := buildOfflineModel(t, width, 24)
		for _, kind := range kinds {
			got := model.resolveOverlayWidth(kind)
			if got > width {
				t.Errorf("on a screen of %d the %s card draws %d wide", width, kind, got)
			}
			if got < 1 {
				t.Errorf("on a screen of %d the %s card draws %d wide", width, kind, got)
			}
		}
	}
}
