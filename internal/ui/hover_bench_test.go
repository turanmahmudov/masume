package ui

import (
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/turanmahmudov/masume/internal/app"
)

// One move of the pointer is one message and the frame that follows it. The terminal reports
// a move for every cell the pointer crosses, so this is the number that matters.
func benchmarkPointerMove(b *testing.B, step int) {
	model := buildLoadedModel(b, 20, 250, 2000, 30)
	model.Active().Active().Focus = app.PaneResult
	model.View()
	block := model.layout.gridRows

	b.ReportAllocs()
	at := 0
	for b.Loop() {
		at = (at + step) % block.count
		held, _ := model.Update(tea.MouseMotionMsg{
			X: block.from + 4, Y: block.top + at, Button: tea.MouseNone,
		})
		model = held.(*Model)
		model.View()
	}
}

// A move onto another row draws the frame again.
func BenchmarkPointerOntoAnotherRow(b *testing.B) { benchmarkPointerMove(b, 1) }

// A move that stays on the same row changes nothing, and must cost no frame.
func BenchmarkPointerOnTheSameRow(b *testing.B) { benchmarkPointerMove(b, 0) }

// One step of a drag over the cells of the frame is one message and the frame that follows it.
// A drag reports a move for every cell the pointer crosses, as a rest does.
func BenchmarkDragOverTheGrid(b *testing.B) {
	model := buildLoadedModel(b, 20, 250, 2000, 30)
	model.Active().Active().Focus = app.PaneResult
	model.View()
	block := model.layout.gridRows
	model.readMouse(tea.MouseClickMsg{
		X: block.from + 4, Y: block.top, Button: tea.MouseLeft,
	})

	b.ReportAllocs()
	at := 0
	for b.Loop() {
		at = (at + 1) % 20
		held, _ := model.Update(tea.MouseMotionMsg{
			X: block.from + 20, Y: block.top + at, Button: tea.MouseLeft,
		})
		model = held.(*Model)
		model.View()
	}
}
