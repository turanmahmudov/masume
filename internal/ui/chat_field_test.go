package ui

import (
	"strings"
	"testing"

	"github.com/turanmahmudov/masume/internal/app"
)

func TestResolveChatFieldRowsGrowsWithTheQuestion(t *testing.T) {
	cases := []struct {
		name  string
		lines int
		rows  int
	}{
		{"nothing written keeps the least", 0, 3},
		{"one line keeps the least", 1, 3},
		{"two lines keep the least", 2, 3},
		{"three lines fill the least", 3, 3},
		{"four lines take a row more", 4, 4},
		{"five lines take another", 5, 5},
		{"six lines fill the most", 6, 6},
		{"seven lines stop at the most", 7, 6},
		{"twenty lines stop at the most", 20, 6},
	}

	for _, held := range cases {
		t.Run(held.name, func(t *testing.T) {
			written := strings.Repeat("a\n", max(0, held.lines-1)) + "a"
			if held.lines == 0 {
				written = ""
			}
			buffer := app.NewEditorBuffer(written, len(written))
			if rows := resolveChatFieldRows(buffer); rows != held.rows {
				t.Errorf("%d lines asked for %d rows, want %d",
					held.lines, rows, held.rows)
			}
		})
	}
}

func TestResolveChatFieldRowsAnswersWithoutABuffer(t *testing.T) {
	if rows := resolveChatFieldRows(nil); rows != chatFieldRowsLeast {
		t.Errorf("no buffer asked for %d rows, want %d", rows, chatFieldRowsLeast)
	}
}
