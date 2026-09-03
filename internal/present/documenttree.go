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

// A result displayed as documents. One row of the result is one folded row of the tree. An
// open row shows the columns. An open column that holds a document or an array shows its
// content, to any depth.
//
// A large result is drawn one window at a time, and a folded row is one row of the tree,
// whatever it contains. So only the open rows are read: their sizes give the position of a
// tree row, and nothing walks the rows between them.

// DocumentNode is one row of the tree.
type DocumentNode struct {
	// Path is the identifier of this node, and the entry of the set of open nodes.
	Path  string
	Depth int
	Key   string
	// Value is the text of the value column: the value itself, or a summary of a document
	// or an array, which the user opens instead of reading it on one line.
	Value string
	Type  string
	// Opens is true if the node has fields or elements to show.
	Opens bool
	Open  bool
	// RowIndex is the position of the row of this node among the rows the filter kept.
	// The paging of the tree uses it.
	RowIndex int
	// ResultRow is the position of that row in the whole result. The gutter draws it, so
	// the tree numbers a document in the same way as the grid.
	ResultRow int
	// Guides says, for each depth above this node, whether the branch at that depth
	// continues below this row. The pane draws the guide line from it.
	Guides []bool
	// Last is true if this node is the last child of its parent.
	Last bool
}

// DocumentTreeInput is the result of the tree and the set of open nodes.
type DocumentTreeInput struct {
	Columns []db.ResultColumn
	// Rows are every row of the result, as the server sent them.
	Rows [][]any
	// RowIndexes are the rows the screen filter kept, in display order. The rows
	// themselves are not copied: a result can hold hundreds of thousands of rows, and a
	// copy in every frame would cost the whole result.
	RowIndexes []int
	// Opened holds the path of every open node.
	Opened map[string]bool
}

// countRows returns the number of rows the screen filter kept.
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

// BuildRowPath returns the path of the node of one result row. The path uses the position in
// the whole result and not the display position, so a filter over the rows keeps the open
// documents open and keeps them on the same documents.
func BuildRowPath(resultRow int) string {
	return "r" + strconv.Itoa(resultRow)
}

// buildChildPath returns the path of a child node.
func buildChildPath(parent, key string) string {
	return parent + "\x1f" + key
}

// ReadDocumentPathKeys returns the keys of a path below the row. They identify the field of
// the node.
func ReadDocumentPathKeys(path string) []string {
	parts := strings.Split(path, "\x1f")
	if len(parts) < 2 {
		return nil
	}
	return parts[1:]
}

// DocumentTree is a result as a tree, ready to draw one window at a time. It holds the open
// rows and the number of rows each one adds, and nothing about the rows between them.
type DocumentTree struct {
	input DocumentTreeInput
	// opened are the open rows in display order, each one with the number of rows it adds
	// below itself.
	opened []openedRow
	// total is the number of rows of the whole tree.
	total int
}

// openedRow is one open row of the result and the number of rows below it.
type openedRow struct {
	at   int
	kept int
}

// BuildDocumentTree builds the tree from the set of open nodes. It reads the open rows only,
// so a result of a million rows costs the size of the open rows and no more.
func BuildDocumentTree(input DocumentTreeInput) DocumentTree {
	tree := DocumentTree{input: input, total: input.countRows()}

	// The loop walks the open rows and not the result. A user opens a few documents out
	// of the whole result, so a walk of the result would cost the whole result in every
	// frame.
	places := make([]int, 0, len(input.Opened))
	for path := range input.Opened {
		resultRow, isRow := readRowPath(path)
		if !isRow {
			continue
		}
		// The kept rows are in the order of the result, so the display position of a
		// row is found without a walk.
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

// readRowPath returns the row of a path, and false for a path that identifies a field below
// the row.
func readRowPath(path string) (int, bool) {
	if len(path) < 2 || path[0] != 'r' || strings.Contains(path, "\x1f") {
		return 0, false
	}
	at, err := strconv.Atoi(path[1:])
	return at, err == nil
}

// CountRows returns the number of rows of the tree.
func (tree DocumentTree) CountRows() int { return tree.total }

// ReadWindow returns up to count rows of the tree, starting at this position. The rows
// before the window are not built: the start of the window is calculated from the open rows
// alone.
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

// findAt returns the result row that contains a tree position, and the offset inside that
// row. It walks the open rows, which are few, and never the result rows.
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

// readRowFrom returns the tree rows of one result row, from this offset on.
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

	// The label is the first value of the row, which is the key of the document on the
	// server. It is the row position if the row has no value.
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

// readRowPlace returns the position of this row in the whole result.
func (tree DocumentTree) readRowPlace(rowIndex int) int {
	if rowIndex < len(tree.input.RowIndexes) {
		return tree.input.RowIndexes[rowIndex]
	}
	return rowIndex
}

// readColumnType returns the type of that column in the result.
func (tree DocumentTree) readColumnType(at int) string {
	if at < len(tree.input.Columns) {
		return tree.input.Columns[at].DataType
	}
	return ""
}

// expandRow returns every row below one document, to the depth of the open nodes.
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

// buildValueInput is one value of the tree and its position.
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

// maxDocumentDepth is the maximum depth of the tree. Without a limit a document that
// contains itself would never end, and no user follows a document below this depth.
const maxDocumentDepth = 32

// buildValueNodes returns the node of one value and every open node below it.
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

// buildInsideNodes returns the nodes of the content of one open document or array.
func (tree DocumentTree) buildInsideNodes(
	input buildValueInput, path string, inside core.JSONValue,
) []DocumentNode {
	// The branch of this node continues below its children if it is not the last child of
	// its own parent.
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

// readNodeContents returns the fields or the elements of a value that has them. A value the
// server sent as a document is parsed from its text. A parsed value is used unchanged.
func readNodeContents(value any, dataType string) (core.JSONValue, bool) {
	switch held := value.(type) {
	case core.JSONValue:
		return held, opensIntoContents(held)
	case core.DocumentValue:
		read, isJSON := core.ReadJSON(held.Text)
		return read, isJSON && opensIntoContents(read)
	case json.RawMessage:
		// A JSON column of a SQL server arrives as the bytes of the server, which are
		// already one JSON value.
		read, isJSON := core.ReadJSON(string(held))
		return read, isJSON && opensIntoContents(read)
	case []byte:
		// Bytes are a document only if the column type says so. Every other byte column
		// holds binary data.
		if !core.IsDocumentType(dataType) {
			return core.JSONValue{}, false
		}
		read, isJSON := core.ReadJSON(string(held))
		return read, isJSON && opensIntoContents(read)
	case string:
		// A server without a document type can still return a column of JSON text, and
		// the tree opens it if it parses as JSON.
		if !core.IsDocumentType(dataType) && !looksLikeDocument(held) {
			return core.JSONValue{}, false
		}
		read, isJSON := core.ReadJSON(held)
		return read, isJSON && opensIntoContents(read)
	}
	return core.JSONValue{}, false
}

// opensIntoContents is true if a value has fields or elements the user can open. An extended
// JSON wrapper is an object in JSON and one value for the user, so the tree draws it on one
// row and does not open it into the name of its own type.
func opensIntoContents(value core.JSONValue) bool {
	if !value.IsObject && !value.IsArray {
		return false
	}
	_, isWrapped := core.ReadDocumentScalar(value)
	return !isWrapped
}

// looksLikeDocument is true if the text starts like a document or an array, so text that is
// neither is never parsed as JSON.
func looksLikeDocument(text string) bool {
	trimmed := strings.TrimSpace(text)
	return strings.HasPrefix(trimmed, "{") || strings.HasPrefix(trimmed, "[")
}

// readNodeScalar returns one value of a document in display form, with its type.
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

// readNodeText returns one value of a row in the form the tree draws.
func readNodeText(value any, dataType string) string {
	if value == nil {
		return ""
	}
	return SafeText(core.CollapseWhitespace(core.FormatCell(value, dataType)))
}

// readNodeType returns the type of a value of the result. The result gives the type if the
// server does. If the result gives no type, or gives the several types a sample of a
// collection found, the type of the value itself is used: the tree draws one value, and that
// value has one type, whatever the rest of the collection holds.
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

// mixedColumnType is the type a sample of a collection gives a field it found with more than
// one type. It describes the column and not the value, so no tree row displays it.
const mixedColumnType = "mixed"

// HasDocumentColumn is true if a result holds a value the tree can open. It decides whether
// the tree is available.
func HasDocumentColumn(columns []db.ResultColumn, rows [][]any) bool {
	for at, column := range columns {
		if core.IsDocumentType(column.DataType) {
			return true
		}
		// A server that stores a document in a text column gives no document type, so
		// the values are parsed. Only the first rows are parsed, because a document
		// column is visible in the first rows.
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

// documentSniffRows is the number of rows parsed to find a document column that the server
// did not report as one.
const documentSniffRows = 20

// ReadDocumentNodeText returns the text of the value of one tree node: the whole document if
// the node opens into one, and the value itself if it does not. A row that shows a summary of
// a document copies the document and not the summary.
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

// walkDocumentKeys follows the keys of a path into a document and returns the value at that
// path. An element of an array is addressed by its index.
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
