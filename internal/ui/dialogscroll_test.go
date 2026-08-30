package ui

import (
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/turanmahmudov/masume/internal/app"
)

// Every card that draws a bar answers a drag of it. A card that keeps how far it has scrolled
// where a list keeps its cursor used to answer a drag by standing still, because the bar wrote
// an offset the card never read.
func TestEveryCardWithABarAnswersADragOfIt(t *testing.T) {
	for _, held := range []struct {
		name string
		open func(*Model, *app.Connection)
		read func(*app.Connection) int
	}{
		{
			name: "the help",
			open: func(model *Model, connection *app.Connection) {
				connection.Overlay = app.Overlay{
					Kind: app.OverlayHelp, Draft: app.NewEditorBuffer("", 0),
				}
			},
			read: func(connection *app.Connection) int {
				return connection.Overlay.List.Cursor
			},
		},
		{
			name: "the palette",
			open: func(model *Model, connection *app.Connection) {
				connection.Overlay = app.Overlay{
					Kind: app.OverlayPalette, Draft: app.NewEditorBuffer("", 0),
					Palette: model.buildPaletteActions(connection),
				}
			},
			read: func(connection *app.Connection) int {
				return connection.Overlay.List.Offset
			},
		},
	} {
		t.Run(held.name, func(t *testing.T) {
			model, connection, _ := buildEditingModel(t, "select id from orders", 0)
			held.open(model, connection)
			model.render()

			bar, found := findCardBar(model)
			if !found {
				t.Fatal("the card drew no bar")
			}
			// A press at the foot of the track brings the thumb there.
			model.readMouse(tea.MouseClickMsg{
				X: bar.column, Y: bar.top + bar.rows - 1, Button: tea.MouseLeft,
			})
			if held.read(connection) == 0 {
				t.Error("a press at the foot of the bar left the card where it was")
			}
			moved := held.read(connection)

			// And a drag back to the head brings it back.
			model.readMouseMotion(tea.MouseMotionMsg{
				X: bar.column, Y: bar.top, Button: tea.MouseLeft,
			})
			if after := held.read(connection); after >= moved {
				t.Errorf("a drag to the head of the bar left the card at %d, and it was %d",
					after, moved)
			}
		})
	}
}

// findCardBar answers the bar the card on show drew.
func findCardBar(model *Model) (scrollHit, bool) {
	for _, held := range model.layout.scrollbars {
		if held.rows > 1 {
			return held, true
		}
	}
	return scrollHit{}, false
}
