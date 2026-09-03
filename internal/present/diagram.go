package present

import (
	"strings"

	"github.com/turanmahmudov/masume/internal/db"
	"github.com/turanmahmudov/masume/internal/query"
)

// An ER diagram drawn with box characters: the table in the middle, the tables that refer to
// it on the left, and the tables it refers to on the right.

// DiagramColumn is one column of a box of the diagram.
type DiagramColumn struct {
	Name    string
	Primary bool
	Foreign bool
}

// DiagramTable is one table of a diagram.
type DiagramTable struct {
	Schema      string
	Name        string
	Columns     []DiagramColumn
	ForeignKeys []query.ForeignKey
}

// The size of one box, and the number of columns it shows.
const (
	diagramBoxWidth    = 26
	diagramGap         = 8
	diagramHeaderLines = 3
	diagramMaxColumns  = 10
)

// QualifyDiagramTable joins the schema and the name of a table into one name.
func QualifyDiagramTable(schema, name string) string {
	return schema + "." + name
}

// diagramCanvas is the cell grid the boxes and the arrows are drawn into.
type diagramCanvas struct {
	rows [][]rune
}

// set writes the text from that cell to the right and replaces the previous content.
func (canvas *diagramCanvas) set(x, y int, text string) {
	for len(canvas.rows) <= y {
		canvas.rows = append(canvas.rows, []rune{})
	}
	row := canvas.rows[y]
	for index, character := range []rune(text) {
		for len(row) <= x+index {
			row = append(row, ' ')
		}
		row[x+index] = character
	}
	canvas.rows[y] = row
}

// setSoft writes into a blank cell only, so it never overwrites a box.
func (canvas *diagramCanvas) setSoft(x, y int, character rune) {
	for len(canvas.rows) <= y {
		canvas.rows = append(canvas.rows, []rune{})
	}
	row := canvas.rows[y]
	for len(row) <= x {
		row = append(row, ' ')
	}
	if row[x] == ' ' {
		row[x] = character
	}
	canvas.rows[y] = row
}

func (canvas *diagramCanvas) toLines() []string {
	width := 0
	for _, row := range canvas.rows {
		if len(row) > width {
			width = len(row)
		}
	}
	lines := make([]string, 0, len(canvas.rows))
	for _, row := range canvas.rows {
		lines = append(lines, PadText(string(row), width))
	}
	return lines
}

// padDiagramCell cuts a cell with an ellipsis, or pads it with spaces.
func padDiagramCell(text string, width int) string {
	if len([]rune(text)) > width {
		return string([]rune(text)[:width-1]) + "…"
	}
	return PadText(text, width)
}

// buildDiagramBox returns the box of one table: the name, and then the columns with the role
// of each one.
func buildDiagramBox(table DiagramTable) []string {
	inner := diagramBoxWidth - 2
	shown := table.Columns
	if len(shown) > diagramMaxColumns {
		shown = shown[:diagramMaxColumns]
	}
	lines := []string{
		"╭" + strings.Repeat("─", inner) + "╮",
		"│" + padDiagramCell(" "+
			QualifyDiagramTable(table.Schema, table.Name), inner) + "│",
		"├" + strings.Repeat("─", inner) + "┤",
	}
	for _, column := range shown {
		role := "  "
		switch {
		case column.Primary:
			role = "PK"
		case column.Foreign:
			role = "FK"
		}
		lines = append(lines, "│"+padDiagramCell(" "+column.Name, inner-3)+role+" │")
	}
	if len(table.Columns) > len(shown) {
		lines = append(lines, "│"+padDiagramCell(
			" … "+FormatCount(int64(len(table.Columns)-len(shown)))+" more", inner)+"│")
	}
	return append(lines, "╰"+strings.Repeat("─", inner)+"╯")
}

// findDiagramColumnRow returns the row of a box that holds that column.
func findDiagramColumnRow(table DiagramTable, columnName string) (int, bool) {
	shown := table.Columns
	if len(shown) > diagramMaxColumns {
		shown = shown[:diagramMaxColumns]
	}
	for index, column := range shown {
		if strings.EqualFold(column.Name, columnName) {
			return diagramHeaderLines + index, true
		}
	}
	return 0, false
}

// diagramPlacement is the position and the height of one box.
type diagramPlacement struct {
	table  DiagramTable
	x      int
	y      int
	height int
}

func placeDiagramBox(canvas *diagramCanvas, table DiagramTable, x, y int) diagramPlacement {
	lines := buildDiagramBox(table)
	for index, line := range lines {
		canvas.set(x, y+index, line)
	}
	return diagramPlacement{table: table, x: x, y: y, height: len(lines)}
}

// connectDiagram draws an arrow from one row to another through a vertical channel.
func connectDiagram(canvas *diagramCanvas, fromX, fromY, toX, toY int) {
	step := max((toX-fromX)/2, 2)
	channel := fromX + step

	for x := fromX; x < channel; x++ {
		canvas.setSoft(x, fromY, '─')
	}

	top, bottom := fromY, toY
	if top > bottom {
		top, bottom = toY, fromY
	}
	for y := top; y <= bottom; y++ {
		canvas.setSoft(channel, y, '│')
	}

	switch {
	case fromY == toY:
		canvas.set(channel, fromY, "─")
	case fromY < toY:
		canvas.set(channel, fromY, "╮")
		canvas.set(channel, toY, "╰")
	default:
		canvas.set(channel, fromY, "╯")
		canvas.set(channel, toY, "╭")
	}

	for x := channel + 1; x < toX-1; x++ {
		canvas.setSoft(x, toY, '─')
	}
	canvas.set(toX-1, toY, "▶")
}

// RenderErDiagram draws the table and its neighbours and connects every foreign key column
// to the column it refers to.
func RenderErDiagram(root DiagramTable, related []DiagramTable) []string {
	canvas := &diagramCanvas{}
	rootName := QualifyDiagramTable(root.Schema, root.Name)

	findRelated := func(schema, name string) (DiagramTable, bool) {
		for _, table := range related {
			if QualifyDiagramTable(table.Schema, table.Name) ==
				QualifyDiagramTable(schema, name) {
				return table, true
			}
		}
		return DiagramTable{}, false
	}

	outgoing := []query.ForeignKey{}
	for _, key := range root.ForeignKeys {
		if _, found := findRelated(key.TargetSchema, key.TargetTable); found {
			outgoing = append(outgoing, key)
		}
	}

	type incomingKey struct {
		table DiagramTable
		key   query.ForeignKey
	}
	incoming := []incomingKey{}
	for _, table := range related {
		for _, key := range table.ForeignKeys {
			if QualifyDiagramTable(key.TargetSchema, key.TargetTable) == rootName {
				incoming = append(incoming, incomingKey{table: table, key: key})
			}
		}
	}

	leftX := 0
	middleX := 0
	if len(incoming) > 0 {
		middleX = diagramBoxWidth + diagramGap
	}
	rightX := middleX + diagramBoxWidth + diagramGap

	leftY := 0
	leftPlacements := make([]diagramPlacement, 0, len(incoming))
	for _, held := range incoming {
		placed := placeDiagramBox(canvas, held.table, leftX, leftY)
		leftY += placed.height + 1
		leftPlacements = append(leftPlacements, placed)
	}

	rootPlacement := placeDiagramBox(canvas, root, middleX, 0)

	rightY := 0
	for _, key := range outgoing {
		target, found := findRelated(key.TargetSchema, key.TargetTable)
		if !found {
			continue
		}
		placed := placeDiagramBox(canvas, target, rightX, rightY)
		rightY += placed.height + 1

		fromRow, hasFrom := findDiagramColumnRow(root, firstOf(key.Columns))
		toRow, hasTo := findDiagramColumnRow(placed.table, firstOf(key.TargetColumns))
		if !hasFrom || !hasTo {
			continue
		}
		connectDiagram(canvas,
			middleX+diagramBoxWidth, rootPlacement.y+fromRow, placed.x, placed.y+toRow)
	}

	for index, held := range incoming {
		if index >= len(leftPlacements) {
			continue
		}
		placed := leftPlacements[index]
		fromRow, hasFrom := findDiagramColumnRow(placed.table, firstOf(held.key.Columns))
		toRow, hasTo := findDiagramColumnRow(root, firstOf(held.key.TargetColumns))
		if !hasFrom || !hasTo {
			continue
		}
		connectDiagram(canvas,
			leftX+diagramBoxWidth, placed.y+fromRow, middleX, rootPlacement.y+toRow)
	}

	return canvas.toLines()
}

func firstOf(names []string) string {
	if len(names) == 0 {
		return ""
	}
	return names[0]
}

// CollectDiagramNeighbours returns the names of the tables at both ends of a foreign key of
// this table.
func CollectDiagramNeighbours(
	root DiagramTable, relationships []db.Relationship,
) map[string]bool {
	rootName := QualifyDiagramTable(root.Schema, root.Name)
	names := map[string]bool{}
	for _, key := range root.ForeignKeys {
		names[QualifyDiagramTable(key.TargetSchema, key.TargetTable)] = true
	}
	for _, relationship := range relationships {
		if QualifyDiagramTable(
			relationship.TargetSchema, relationship.TargetTable) == rootName {
			names[QualifyDiagramTable(relationship.Schema, relationship.Table)] = true
		}
	}
	return names
}
