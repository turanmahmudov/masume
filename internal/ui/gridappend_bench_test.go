package ui

import (
	"fmt"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/turanmahmudov/masume/internal/db"
)

// One page landing and the frame that follows it, at the sizes a result reaches as a reader
// scrolls on. The rows before the page do not change, so this number has to stay flat: a
// number that grows with the result is a reader waiting longer for every page than for the
// one before it.
func BenchmarkDrawAPageThatLanded(b *testing.B) {
	for _, rows := range []int{2000, 20000, 100000} {
		b.Run(fmt.Sprintf("rows=%d", rows), func(b *testing.B) {
			model := buildDocumentModel(b, rows)
			tab := model.Active().Active()
			page := db.QueryResult{Rows: buildDocumentRows(rows, 200)}
			model.View()

			b.ReportAllocs()
			for b.Loop() {
				tab.Results.AppendRows(0, page)
				model.View()
			}
		})
	}
}

// One cursor step over a result of documents, and the frame after it: what a reader feels
// holding the key down.
func BenchmarkScrollDocuments(b *testing.B) {
	for _, rows := range []int{2000, 100000} {
		b.Run(fmt.Sprintf("rows=%d", rows), func(b *testing.B) {
			model := buildDocumentModel(b, rows)
			model.View()

			b.ReportAllocs()
			for b.Loop() {
				held, _ := model.Update(tea.KeyPressMsg{Code: tea.KeyDown})
				model = held.(*Model)
				model.View()
			}
		})
	}
}
