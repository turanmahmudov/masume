package ui

import (
	"strings"
	"testing"
)

// The paint of a selection runs on every frame while a drag is held, so it reads only the
// rows the drag reaches. Reading a whole frame into cells costs about seven times as much.

// buildBusyFrame writes a frame with as many colour changes per row as a real one has.
func buildBusyFrame(rows, columns int) string {
	row := strings.Builder{}
	for at := 0; at < columns; at += 6 {
		row.WriteString("\x1b[38;2;191;189;182m\x1b[48;2;13;16;23mabcdef")
	}
	lines := make([]string, rows)
	for at := range lines {
		lines[at] = row.String()
	}
	return strings.Join(lines, "\n")
}

func BenchmarkPaintSelection(bench *testing.B) {
	frame := buildBusyFrame(34, 120)
	model := &Model{width: 120, height: 34, styles: NewStyles(NewThemeRegistry())}
	model.selection = screenSelection{
		fromX: 40, fromY: 4, toX: 90, toY: 8, held: true,
		block:   blockRect{fromX: 37, toX: 118, fromY: 3, toY: 31},
		bounded: true,
	}
	for bench.Loop() {
		_ = model.paintSelection(frame)
	}
}

func BenchmarkReadWholeFrameIntoCells(bench *testing.B) {
	frame := buildBusyFrame(34, 120)
	for bench.Loop() {
		for line := range strings.SplitSeq(frame, "\n") {
			_ = writeCells(mapCells(line))
		}
	}
}
