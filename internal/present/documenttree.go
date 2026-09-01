package present

import (
	"encoding/json"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/turanmahmudov/masume/internal/core"
	"github.com/turanmahmudov/masume/internal/db"
)

// A result read as documents. One row of the result is one row of the tree, folded; opening
// it shows the columns; opening a column that holds a document or an array shows what is
// inside it, as deep as the reader goes.
//
// A result of many rows is drawn a window at a time, and a folded row is one row of the tree
// whatever it holds. So the rows the reader opened are the only ones ever read: their number
// says where a row of the tree sits, and nothing walks the rows between them.

// DocumentNode is one row the tree draws.
type DocumentNode struct {
	// Path names this node, and is what the set of opened nodes holds.
	Path  string
	Depth int
	Key   string
	// Value is what the value column draws: the value itself, or the shape of a document
	// or an array, which is opened rather than read on one line.
	Value string
	Type  string
	// Opens is true where the node holds fields or elements to show.
	Opens bool
	Open  bool
	// RowIndex is the place of the row this node belongs to among the rows kept, which is
	// what the paging of the tree counts against.
	RowIndex int
	// ResultRow is the place of that row in the whole result, which the gutter draws so
	// that the tree numbers a document the way the grid numbers it.
	ResultRow int
	// Guides says, for each depth above this node, whether the branch there carries on
	// below this row. The pane draws the line of the tree from it.
	Guides []bool
	// Last is true where this node is the last one of the node above it.
	Last bool
}

// DocumentTreeInput is the result the tree is read from, and how far the reader opened it.
type DocumentTreeInput struct {
	Columns []db.ResultColumn
	// Rows are every row of the result, as the server sent them.
	Rows [][]any
	// RowIndexes names the rows the screen filter kept, in the order they are drawn. The
	// rows themselves are not copied out: a result scrolled far enough holds hundreds of
	// thousands of them, and a frame that copied them would cost the whole result.
	RowIndexes []int
	// Opened holds the path of every node the reader opened.
	Opened map[string]bool
}

// countRows returns how many rows the screen filter kept.
func (input DocumentTreeInput) countRows() int { return len(input.RowIndexes) }

// readRow returns one kept row of the result.
func (input DocumentTreeInput) readRow(at int) []any {
	if at < 0 || at >= len(input.RowIndexes) {
		return nil
	}
	place := input.RowIndexes[at]
	if place < 0 || place >= len(input.Rows) {
		return nil
	}
	return input.Rows[place]
}

// BuildRowPath names the node of one row of the result. The row is named by its place in
// the whole result and not by where it is drawn, so a filter laid over the rows leaves the
// documents the reader opened open, and leaves them on the documents they were opened on.
func BuildRowPath(resultRow int) string {
	return "r" + strconv.Itoa(resultRow)
}

// buildChildPath names a node under another one.
func buildChildPath(parent, key string) string {
	return parent + "\x1f" + key
}

// ReadDocumentPathKeys returns the keys of a path below the row, which names the field a
// reader is on.
func ReadDocumentPathKeys(path string) []string {
	parts := strings.Split(path, "\x1f")
	if len(parts) < 2 {
		return nil
	}
	return parts[1:]
}

// DocumentTree is a result laid out as a tree, ready to be drawn a window at a time. It holds
// the opened rows and how much each one grew by, and nothing about the rows between them.
type DocumentTree struct {
	input DocumentTreeInput
	// opened are the rows the reader opened, in the order they stand, each with the number
	// of rows it adds under itself.
	opened []openedRow
	// total is how many rows the tree draws in all.
	total int
}

// openedRow is one opened row of the result and how many rows stand under it.
type openedRow struct {
	at   int
	kept int
}

// BuildDocumentTree reads how far the result was opened. It reads only the rows the reader
// opened, so a result of a million rows costs what the reader opened and no more.
func BuildDocumentTree(input DocumentTreeInput) DocumentTree {
	tree := DocumentTree{input: input, total: input.countRows()}

	// The opened rows are read, and not the result. A reader opens a handful of documents
	// out of however many the result holds, so walking the result to find them would cost
	// the whole result on every frame.
	places := make([]int, 0, len(input.Opened))
	for path := range input.Opened {
		resultRow, isRow := readRowPath(path)
		if !isRow {
			continue
		}
		// The rows kept stand in the order the result answered them, so the place a row
		// is drawn at is found without walking them.
		at, held := slices.BinarySearch(input.RowIndexes, resultRow)
		if held {
			places = append(places, at)
		}
	}
	slices.Sort(places)

	for _, at := range places {
		kept := len(tree.expandRow(at))
		if kept == 0 {
			continue
		}
		tree.opened = append(tree.opened, openedRow{at: at, kept: kept})
		tree.total += kept
	}
	return tree
}

// readRowPath returns the row a path names, and false for a path that names a field under
// one rather than the row itself.
func readRowPath(path string) (int, bool) {
	if len(path) < 2 || path[0] != 'r' || strings.Contains(path, "\x1f") {
		return 0, false
	}
	at, err := strconv.Atoi(path[1:])
	return at, err == nil
}

// CountRows returns how many rows the tree draws.
func (tree DocumentTree) CountRows() int { return tree.total }

// ReadWindow returns the rows of the tree from this one on, and no more than this many. The
// rows before the window are never built: where the window begins is worked out from the
// opened rows alone.
func (tree DocumentTree) ReadWindow(from, count int) []DocumentNode {
	if from < 0 {
		from = 0
	}
	if count <= 0 || from >= tree.total {
		return nil
	}

	rowIndex, within := tree.findAt(from)
	nodes := make([]DocumentNode, 0, count)
	for rowIndex < tree.input.countRows() && len(nodes) < count {
		nodes = append(nodes, tree.readRowFrom(rowIndex, within, count-len(nodes))...)
		rowIndex++
		within = 0
	}
	return nodes
}

// findAt returns the row of the result a place in the tree falls in, and how far into that
// row it falls. It walks the opened rows, which are few, and never the rows themselves.
func (tree DocumentTree) findAt(offset int) (rowIndex, within int) {
	grown := 0
	for _, opened := range tree.opened {
		standsAt := opened.at + grown
		if offset <= standsAt {
			break
		}
		if offset <= standsAt+opened.kept {
			return opened.at, offset - standsAt
		}
		grown += opened.kept
	}
	return offset - grown, 0
}

// readRowFrom returns the rows one row of the result draws, from this one of them on.
func (tree DocumentTree) readRowFrom(rowIndex, within, count int) []DocumentNode {
	held := []DocumentNode{tree.buildRowNode(rowIndex)}
	if held[0].Open {
		held = append(held, tree.expandRow(rowIndex)...)
	}
	if within >= len(held) {
		return nil
	}
	held = held[within:]
	if len(held) > count {
		held = held[:count]
	}
	return held
}

// buildRowNode returns the folded row of one document.
func (tree DocumentTree) buildRowNode(rowIndex int) DocumentNode {
	path := BuildRowPath(tree.readRowPlace(rowIndex))
	row := tree.input.readRow(rowIndex)

	// The label is the first value of the row, which names the document the way the server
	// keys it, and the place of the row where the read answers no value at all.
	label := "row " + strconv.Itoa(tree.readRowPlace(rowIndex)+1)
	if len(row) > 0 && row[0] != nil {
		if written := readNodeText(row[0], tree.readColumnType(0)); written != "" {
			label = written
		}
	}
	return DocumentNode{
		Path: path, Depth: 0, Key: label, Type: core.DocumentTypeObject,
		Value:     core.DocumentValue{Count: len(tree.input.Columns)}.DescribeShape(),
		Opens:     len(tree.input.Columns) > 0,
		Open:      tree.input.Opened[path],
		RowIndex:  rowIndex,
		ResultRow: tree.readRowPlace(rowIndex),
	}
}

// readRowPlace returns where this row stands in the whole result.
func (tree DocumentTree) readRowPlace(rowIndex int) int {
	if rowIndex < len(tree.input.RowIndexes) {
		return tree.input.RowIndexes[rowIndex]
	}
	return rowIndex
}

// readColumnType returns the type the result gives that column.
func (tree DocumentTree) readColumnType(at int) string {
	if at < len(tree.input.Columns) {
		return tree.input.Columns[at].DataType
	}
	return ""
}

// expandRow returns every row under one document, as far as the reader opened it.
func (tree DocumentTree) expandRow(rowIndex int) []DocumentNode {
	row := tree.input.readRow(rowIndex)
	parent := BuildRowPath(tree.readRowPlace(rowIndex))
	nodes := []DocumentNode{}

	for at, column := range tree.input.Columns {
		var value any
		if at < len(row) {
			value = row[at]
		}
		nodes = append(nodes, tree.buildValueNodes(buildValueInput{
			parent:   parent,
			key:      column.Name,
			value:    value,
			dataType: column.DataType,
			depth:    1,
			last:     at == len(tree.input.Columns)-1,
			guides:   []bool{},
			rowIndex: rowIndex,
		})...)
	}
	return nodes
}

// buildValueInput is one value the tree lays out, and where it sits.
type buildValueInput struct {
	parent   string
	key      string
	value    any
	dataType string
	depth    int
	last     bool
	guides   []bool
	rowIndex int
}

// maxDocumentDepth is how deep the tree opens. A document that holds itself would otherwise
// never end, and no reader follows a document past this many levels.
const maxDocumentDepth = 32

// buildValueNodes returns the node of one value and every node under it that is open.
func (tree DocumentTree) buildValueNodes(input buildValueInput) []DocumentNode {
	path := buildChildPath(input.parent, input.key)
	node := DocumentNode{
		Path: path, Depth: input.depth, Key: input.key, RowIndex: input.rowIndex,
		ResultRow: tree.readRowPlace(input.rowIndex),
		Guides:    input.guides, Last: input.last, Open: tree.input.Opened[path],
	}

	inside, opens := readNodeContents(input.value, input.dataType)
	node.Opens = opens && input.depth < maxDocumentDepth
	switch {
	case !opens:
		held := readNodeScalar(input.value, input.dataType)
		node.Value, node.Type = held.Text, held.Type
	case inside.IsArray:
		node.Type = core.DocumentTypeArray
		node.Value = core.DocumentValue{Count: len(inside.Items), IsArray: true}.DescribeShape()
	default:
		node.Type = core.DocumentTypeObject
		node.Value = core.DocumentValue{Count: len(inside.Members)}.DescribeShape()
	}

	if !node.Opens || !node.Open {
		return []DocumentNode{node}
	}
	return append([]DocumentNode{node}, tree.buildInsideNodes(input, path, inside)...)
}

// buildInsideNodes returns the nodes of what one open document or array holds.
func (tree DocumentTree) buildInsideNodes(
	input buildValueInput, path string, inside core.JSONValue,
) []DocumentNode {
	// The branch of this node carries on below its children wherever it is not the last
	// child of its own parent.
	guides := append(append([]bool{}, input.guides...), !input.last)
	nodes := []DocumentNode{}

	if inside.IsArray {
		for at, item := range inside.Items {
			nodes = append(nodes, tree.buildValueNodes(buildValueInput{
				parent: path, key: strconv.Itoa(at), value: item, dataType: "",
				depth: input.depth + 1, last: at == len(inside.Items)-1,
				guides: guides, rowIndex: input.rowIndex,
			})...)
		}
		return nodes
	}
	for at, member := range inside.Members {
		nodes = append(nodes, tree.buildValueNodes(buildValueInput{
			parent: path, key: member.Name, value: member.Value, dataType: "",
			depth: input.depth + 1, last: at == len(inside.Members)-1,
			guides: guides, rowIndex: input.rowIndex,
		})...)
	}
	return nodes
}

// readNodeContents returns what a value holds where it holds fields or elements. A value the
// server sent as a document is read from its text; a value already read stands as it is.
func readNodeContents(value any, dataType string) (core.JSONValue, bool) {
	switch held := value.(type) {
	case core.JSONValue:
		return held, opensIntoContents(held)
	case core.DocumentValue:
		read, isJSON := core.ReadJSON(held.Text)
		return read, isJSON && opensIntoContents(read)
	case json.RawMessage:
		// A JSON column of a SQL server arrives as the bytes the server wrote, which are
		// already one JSON value.
		read, isJSON := core.ReadJSON(string(held))
		return read, isJSON && opensIntoContents(read)
	case []byte:
		// Bytes are a document only where the column says so. Every other column of bytes
		// holds something that is no text at all.
		if !core.IsDocumentType(dataType) {
			return core.JSONValue{}, false
		}
		read, isJSON := core.ReadJSON(string(held))
		return read, isJSON && opensIntoContents(read)
	case string:
		// A server that names no document type still answers a column of JSON text, and
		// a reader opens it wherever it reads as one.
		if !core.IsDocumentType(dataType) && !looksLikeDocument(held) {
			return core.JSONValue{}, false
		}
		read, isJSON := core.ReadJSON(held)
		return read, isJSON && opensIntoContents(read)
	}
	return core.JSONValue{}, false
}

// opensIntoContents is true where a value holds fields or elements a reader opens. A value
// written as an extended JSON wrapper is an object to JSON and one reading to a reader, so
// it is drawn on its own row rather than opened into the name of its own type.
func opensIntoContents(value core.JSONValue) bool {
	if !value.IsObject && !value.IsArray {
		return false
	}
	_, isWrapped := core.ReadDocumentScalar(value)
	return !isWrapped
}

// looksLikeDocument is true where text begins the way a document or an array does, so text
// that is plainly neither is never read as JSON.
func looksLikeDocument(text string) bool {
	trimmed := strings.TrimSpace(text)
	return strings.HasPrefix(trimmed, "{") || strings.HasPrefix(trimmed, "[")
}

// readNodeScalar returns one value of a document as a reader sees it, with its type.
func readNodeScalar(value any, dataType string) core.DocumentScalar {
	if held, isJSON := value.(core.JSONValue); isJSON {
		return core.ReadDocumentValue(held)
	}
	if value == nil {
		return core.DocumentScalar{Text: NullDisplay, Type: core.DocumentTypeNull}
	}
	return core.DocumentScalar{
		Text: readNodeText(value, dataType), Type: readNodeType(value, dataType),
	}
}

// readNodeText writes one value of a row as the tree draws it.
func readNodeText(value any, dataType string) string {
	if value == nil {
		return ""
	}
	return SafeText(core.CollapseWhitespace(core.FormatCell(value, dataType)))
}

// readNodeType names the type of a value the result already read. The result names it where
// the server does. Where the result names nothing, or names the several types a sample of a
// collection found across its documents, the value in hand is read instead: the tree draws
// one value, and that value has one type whatever the rest of the collection holds.
func readNodeType(value any, dataType string) string {
	if dataType != "" && dataType != mixedColumnType {
		return dataType
	}
	switch value.(type) {
	case nil:
		return core.DocumentTypeNull
	case bool:
		return core.DocumentTypeBool
	case time.Time:
		return core.DocumentTypeDate
	case float32, float64:
		return core.DocumentTypeDouble
	case int, int8, int16, int32, int64, uint, uint8, uint16, uint32, uint64:
		return core.DocumentTypeLong
	case []byte:
		return core.DocumentTypeBinary
	}
	return core.DocumentTypeString
}

// mixedColumnType is what a sample of a collection calls a field it found under more than
// one type. It names the column and not the value, so a row of the tree never draws it.
const mixedColumnType = "mixed"

// HasDocumentColumn is true where a result holds a value a tree opens, which is what decides
// whether the tree is offered at all.
func HasDocumentColumn(columns []db.ResultColumn, rows [][]any) bool {
	for at, column := range columns {
		if core.IsDocumentType(column.DataType) {
			return true
		}
		// A server that stores a document in a column of text names no document type, so
		// the values themselves are read. Only the first rows are, because a column of
		// documents shows itself at once.
		for _, row := range rows[:min(len(rows), documentSniffRows)] {
			if at >= len(row) {
				continue
			}
			if _, opens := readNodeContents(row[at], column.DataType); opens {
				return true
			}
		}
	}
	return false
}

// documentSniffRows is how many rows are read to find a column of documents a server did not
// name as one.
const documentSniffRows = 20

// ReadDocumentNodeText returns the text of the value one node of the tree stands for: the
// whole document where the node opens into one, and the value itself where it does not. A
// row that draws the shape of a document copies the document and not the shape.
func ReadDocumentNodeText(
	columns []db.ResultColumn, row []any, node DocumentNode,
) (string, bool) {
	keys := ReadDocumentPathKeys(node.Path)
	if len(keys) == 0 {
		return "", false
	}

	at := -1
	for index, column := range columns {
		if column.Name == keys[0] {
			at = index
			break
		}
	}
	if at == -1 || at >= len(row) {
		return "", false
	}

	inside, opens := readNodeContents(row[at], columns[at].DataType)
	if !opens {
		return readNodeScalar(row[at], columns[at].DataType).Text, true
	}
	held, found := walkDocumentKeys(inside, keys[1:])
	if !found {
		return "", false
	}
	if opensIntoContents(held) {
		return held.WriteIndented("  "), true
	}
	return core.ReadDocumentValue(held).Text, true
}

// walkDocumentKeys follows the keys of a path into a document, and returns the value they
// name. An element of an array is named by its place.
func walkDocumentKeys(value core.JSONValue, keys []string) (core.JSONValue, bool) {
	held := value
	for _, key := range keys {
		switch {
		case held.IsArray:
			at, err := strconv.Atoi(key)
			if err != nil || at < 0 || at >= len(held.Items) {
				return core.JSONValue{}, false
			}
			held = held.Items[at]
		case held.IsObject:
			found := false
			for _, member := range held.Members {
				if member.Name == key {
					held, found = member.Value, true
					break
				}
			}
			if !found {
				return core.JSONValue{}, false
			}
		default:
			return core.JSONValue{}, false
		}
	}
	return held, true
}
