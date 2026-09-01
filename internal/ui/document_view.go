package ui

import (
	"strconv"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/turanmahmudov/masume/internal/app"
	"github.com/turanmahmudov/masume/internal/cfg"
	"github.com/turanmahmudov/masume/internal/present"
)

// The two views that read a row as a document: the tree, which opens one value at a time,
// and the whole document written out in the form that carries every type.

// The widths of the tree: the gutter that numbers the documents, the key with the guides
// before it, and the name of the type at the end. The value takes what is left.
const (
	documentGutterWidth = 5
	documentKeyWidth    = 34
	documentTypeWidth   = 11
	documentColumnGap   = 2
)

// buildDocumentTree returns the tree of the result on screen, as far as the reader opened it.
func (model *Model) buildDocumentTree(
	connection *app.Connection, tab *app.Tab,
) present.DocumentTree {
	shape := model.buildGridShape(connection, tab)
	return present.BuildDocumentTree(present.DocumentTreeInput{
		Columns: shape.Columns, Rows: shape.Rows, RowIndexes: shape.RowIndexes,
		Opened: tab.Opened,
	})
}

// renderDocumentTree draws the rows as documents, one window of the tree at a time.
func (model *Model) renderDocumentTree(
	connection *app.Connection, tab *app.Tab, width, height int,
) []string {
	theme := model.styles.Theme
	tree := model.buildDocumentTree(connection, tab)
	if tree.CountRows() == 0 {
		return model.renderEmptyState(width, height, "this read answered no rows", nil)
	}

	tab.TreeRow = clamp(tab.TreeRow, tree.CountRows())
	// The wheel moves the rows and not the cursor, so the cursor stands off screen until
	// a key moves it again. Holding it in view here would pull the rows straight back.
	if !tab.TreeRolled {
		tab.TreeRowOffset = holdCursorInView(tab.TreeRowOffset, tab.TreeRow, height)
	}
	tab.TreeRowOffset = clampOffset(tab.TreeRowOffset, height, tree.CountRows())

	model.layout.documentRows = height
	nodes := tree.ReadWindow(tab.TreeRowOffset, height)
	// Where the rows were drawn, so a press lands on the row it looks like.
	model.layout.documentRowsHit = rowsHit{
		top: model.layout.detailTop, count: len(nodes), offset: tab.TreeRowOffset,
		from: model.editorLeft + 1, to: model.editorLeft + width,
	}
	focused := tab.Focus == app.PaneResult && !connection.Overlay.IsOpen()

	lines := make([]string, 0, height)
	for at, node := range nodes {
		lines = append(lines, model.renderDocumentNode(
			node, tab.TreeRowOffset+at == tab.TreeRow && focused, width))
	}
	for len(lines) < height {
		lines = append(lines, "")
	}
	return model.drawScrollTrack(lines, scrollView{
		offset: tab.TreeRowOffset, rows: height, total: tree.CountRows(),
		// A drag that reaches the foot of the tree asks for the next page, the way the
		// same drag over the grid does.
		moveTo: func(offset int) tea.Cmd {
			tab.TreeRowOffset = offset
			return model.approachDrawnDocumentEnd(connection, tab)
		},
	}, model.layout.detailTop, model.editorLeft+1, width, theme.Panel)
}

// approachDocumentEnd reads the next page where the document the cursor stands on is inside
// the lookahead of the last one read, so a reader who walks to the foot of the tree is not
// stopped at the end of the page. The grid does the same for its rows: a tree that did not
// would end at the first page whatever the collection holds.
func (model *Model) approachDocumentEnd(
	connection *app.Connection, tab *app.Tab, place int,
) tea.Cmd {
	if !model.canPrefetchGrid(tab) {
		return nil
	}
	documents := len(model.buildGridShape(connection, tab).RowIndexes)
	if documents == 0 || place < documents-resolveGridLookahead(model.layout.documentRows) {
		return nil
	}
	return model.readMoreRows(connection, tab)
}

// approachDrawnDocumentEnd reads the next page where the foot of the tree on screen is inside
// the lookahead. Nothing here moved the cursor: the wheel rolled, or a page arrived and made
// the tree longer, so the row that approaches the end is the last one drawn.
func (model *Model) approachDrawnDocumentEnd(
	connection *app.Connection, tab *app.Tab,
) tea.Cmd {
	tree := model.buildDocumentTree(connection, tab)
	last := tree.ReadWindow(
		min(tab.TreeRowOffset+model.layout.documentRows, max(tree.CountRows()-1, 0)), 1)
	if len(last) == 0 {
		return nil
	}
	return model.approachDocumentEnd(connection, tab, last[0].RowIndex)
}

// renderDocumentNode draws one row of the tree: the guides, the fold mark, the key, the
// value and the name of the type.
func (model *Model) renderDocumentNode(
	node present.DocumentNode, onCursor bool, width int,
) string {
	theme := model.styles.Theme
	ground := theme.Panel
	if onCursor {
		ground = theme.BorderFocus
	}

	// A document is numbered in the gutter the way the grid numbers its rows. A field
	// under it is not, because the number belongs to the document and not to the field.
	gutter := ""
	if node.Depth == 0 {
		gutter = strconv.Itoa(node.ResultRow + 1)
	}

	keyInk, valueInk, typeInk := theme.Accent, theme.Text, theme.Muted
	if onCursor {
		keyInk, valueInk, typeInk = theme.OnAccent, theme.OnAccent, theme.OnAccent
	}
	if node.Value == present.NullDisplay {
		valueInk = theme.Muted
		if onCursor {
			valueInk = theme.OnAccent
		}
	}

	label := buildDocumentGuides(node) + model.describeFoldMark(node) + node.Key
	room := max(width-documentGutterWidth-documentKeyWidth-documentTypeWidth-
		documentColumnGap*2, 8)

	written := strings.Builder{}
	writeTextOn(&written, theme.Muted, ground,
		buildGutterText(gutter, documentGutterWidth))
	writeTextOn(&written, keyInk, ground, present.FitText(label, documentKeyWidth))
	writeBlanksOn(&written, ground, documentColumnGap)
	writeTextOn(&written, valueInk, ground, present.FitText(node.Value, room))
	writeBlanksOn(&written, ground, documentColumnGap)
	writeTextOn(&written, typeInk, ground, present.FitText(node.Type, documentTypeWidth))

	used := documentGutterWidth + documentKeyWidth + room + documentTypeWidth +
		documentColumnGap*2
	if used > width {
		return padStyledOn(truncateStyled(written.String(), width), width, ground)
	}
	writeBlanksOn(&written, ground, width-used)
	return written.String()
}

// describeFoldMark returns the mark that says a node opens, and whether it is open.
func (model *Model) describeFoldMark(node present.DocumentNode) string {
	if !node.Opens {
		return "  "
	}
	if node.Open {
		return model.icons.Icon(cfg.IconFoldOpen) + " "
	}
	return model.icons.Icon(cfg.IconFoldClosed) + " "
}

// The line down the left of the document tree, drawn the way the tree of the server draws
// its own: a branch that carries on, a branch that ends at this row, and the air left where
// a branch ended above it.
const (
	documentGuideContinues = "\u2502 "
	documentGuideLast      = "\u2570 "
	documentGuideClear     = "  "
)

// buildDocumentGuides writes the line down the left of one row, one cell per depth above it.
func buildDocumentGuides(node present.DocumentNode) string {
	if node.Depth == 0 {
		return ""
	}
	var guide strings.Builder
	for _, carries := range node.Guides {
		if carries {
			guide.WriteString(documentGuideContinues)
			continue
		}
		guide.WriteString(documentGuideClear)
	}
	if node.Last {
		guide.WriteString(documentGuideLast)
		return guide.String()
	}
	guide.WriteString(documentGuideContinues)
	return guide.String()
}

// holdCursorInView moves the window so the cursor stands inside it.
func holdCursorInView(offset, cursor, rows int) int {
	if rows <= 0 {
		return offset
	}
	if cursor < offset {
		return cursor
	}
	if cursor >= offset+rows {
		return cursor - rows + 1
	}
	return offset
}

// findDocumentNode returns the node the cursor of the tree stands on.
func (model *Model) findDocumentNode(
	connection *app.Connection, tab *app.Tab,
) (present.DocumentNode, bool) {
	tree := model.buildDocumentTree(connection, tab)
	if tree.CountRows() == 0 {
		return present.DocumentNode{}, false
	}
	held := tree.ReadWindow(clamp(tab.TreeRow, tree.CountRows()), 1)
	if len(held) == 0 {
		return present.DocumentNode{}, false
	}
	return held[0], true
}

// runDocumentTreeAction returns the keys of the tree view: the same keys that open the tree
// of the server, because it is the same gesture on the same kind of row.
func (model *Model) runDocumentTreeAction(
	connection *app.Connection, tab *app.Tab, match Match,
) (tea.Model, tea.Cmd) {
	tree := model.buildDocumentTree(connection, tab)
	count := tree.CountRows()

	switch match.Action {
	case ActionCursorUp:
		tab.TreeRow--
	case ActionCursorDown:
		tab.TreeRow++
	case ActionCursorPageUp:
		tab.TreeRow -= listPage
	case ActionCursorPageDown:
		tab.TreeRow += listPage
	case ActionCursorFirstRow:
		tab.TreeRow = 0
	case ActionCursorLastRow:
		tab.TreeRow = count - 1
	case ActionOpenNode:
		return model, model.toggleDocumentNode(connection, tab, openToggle)
	case ActionUnfoldRow:
		return model, model.toggleDocumentNode(connection, tab, openOnly)
	case ActionFoldRow:
		return model, model.toggleDocumentNode(connection, tab, closeOnly)
	case ActionCopyValue:
		return model, model.copyDocumentValue(connection, tab)
	case ActionCopyPath:
		return model, model.copyDocumentPath(connection, tab)
	case ActionCountRows:
		return model.countRows(connection, tab)
	case ActionSearchColumns:
		connection.Overlay = app.Overlay{
			Kind: app.OverlayPrompt, Prompt: app.PromptSearch, Title: "search",
			Hint:  "searches the rows on screen; empty clears it",
			Draft: app.NewEditorBuffer(tab.Screen.Search, len(tab.Screen.Search)),
		}
		return model, nil
	case ActionClearRewrites:
		return model.clearRewrites(connection, tab)
	case ActionPopFilter:
		return model.popFilter(connection, tab)
	default:
		return model, nil
	}
	tab.TreeRow = clamp(tab.TreeRow, count)
	tab.TreeRolled = false

	node, found := model.findDocumentNode(connection, tab)
	if !found {
		return model, nil
	}
	return model, model.approachDocumentEnd(connection, tab, node.RowIndex)
}

// How a key changes whether a node is open.
type openChange int

const (
	openToggle openChange = iota
	openOnly
	closeOnly
)

// toggleDocumentNode opens or closes the node the cursor stands on. A key that closes a node
// already closed steps out to the node that holds it, which is what a tree does everywhere.
func (model *Model) toggleDocumentNode(
	connection *app.Connection, tab *app.Tab, change openChange,
) tea.Cmd {
	node, found := model.findDocumentNode(connection, tab)
	if !found {
		return nil
	}
	if tab.Opened == nil {
		tab.Opened = map[string]bool{}
	}

	switch {
	case change == closeOnly && !node.Open:
		model.stepOutOfDocumentNode(connection, tab, node)
		return nil
	case change == openOnly && node.Open:
		tab.TreeRow++
		tab.TreeRow = clamp(tab.TreeRow, model.buildDocumentTree(connection, tab).CountRows())
		return nil
	case !node.Opens:
		return nil
	}

	if node.Open {
		delete(tab.Opened, node.Path)
		return nil
	}
	tab.Opened[node.Path] = true
	return nil
}

// stepOutOfDocumentNode moves the cursor to the node that holds this one.
func (model *Model) stepOutOfDocumentNode(
	connection *app.Connection, tab *app.Tab, node present.DocumentNode,
) {
	if node.Depth == 0 {
		return
	}
	tree := model.buildDocumentTree(connection, tab)
	for at := tab.TreeRow - 1; at >= 0; at-- {
		held := tree.ReadWindow(at, 1)
		if len(held) == 1 && held[0].Depth < node.Depth {
			tab.TreeRow = at
			return
		}
	}
}

// describeDocumentTreeFooter says how many rows the tree draws and where the cursor stands
// in the document it is inside.
func (model *Model) describeDocumentTreeFooter(
	connection *app.Connection, tab *app.Tab,
) (string, string) {
	active := tab.Results.Active()
	if active == nil || active.State.Kind != app.QuerySucceeded {
		return "", ""
	}
	// The left of the footer counts the rows of the result, not the rows of the tree: a
	// document that was opened adds rows to the tree and none to the collection.
	shape := model.buildGridShape(connection, tab)
	size := model.describeResultSize(tab, active, len(shape.RowIndexes))

	node, found := model.findDocumentNode(connection, tab)
	if !found {
		return size, ""
	}
	// The footer names the result row, so it agrees with the gutter.
	where := "row " + present.FormatCount(int64(node.ResultRow+1))
	if keys := present.ReadDocumentPathKeys(node.Path); len(keys) > 0 {
		where += " · " + strings.Join(keys, ".")
	}
	if node.Type != "" {
		where += " · " + node.Type
	}
	return size, where
}

// describeDocumentFooter returns the row under a document view, and nothing for a view that
// carries none.
func (model *Model) describeDocumentFooter(
	connection *app.Connection, tab *app.Tab, drawn app.ResultView,
) (string, string) {
	switch drawn {
	case app.ViewTree:
		return model.describeDocumentTreeFooter(connection, tab)
	}
	return "", ""
}

// copyDocumentValue puts the value under the cursor on the clipboard. A document is copied
// whole, as the text it holds, and not as the shape the row draws in its place.
func (model *Model) copyDocumentValue(
	connection *app.Connection, tab *app.Tab,
) tea.Cmd {
	node, found := model.findDocumentNode(connection, tab)
	if !found {
		return nil
	}
	written, held := model.readDocumentNodeText(connection, tab, node)
	if !held {
		return nil
	}
	connection.Show("the value is on the clipboard")
	return model.keepOnClipboard(written)
}

// readDocumentNodeText returns the text of the value one node stands for.
func (model *Model) readDocumentNodeText(
	connection *app.Connection, tab *app.Tab, node present.DocumentNode,
) (string, bool) {
	shape := model.buildGridShape(connection, tab)
	if node.RowIndex < 0 || node.RowIndex >= len(shape.RowIndexes) {
		return "", false
	}
	row := shape.Rows[shape.RowIndexes[node.RowIndex]]
	return present.ReadDocumentNodeText(shape.Columns, row, node)
}

// copyDocumentPath puts the name of the field under the cursor on the clipboard, so a reader
// who found a value deep in a document can write a filter for it.
func (model *Model) copyDocumentPath(
	connection *app.Connection, tab *app.Tab,
) tea.Cmd {
	node, found := model.findDocumentNode(connection, tab)
	if !found {
		return nil
	}
	keys := present.ReadDocumentPathKeys(node.Path)
	if len(keys) == 0 {
		return nil
	}
	written := strings.Join(keys, ".")
	connection.Show("the field name is on the clipboard")
	return model.keepOnClipboard(written)
}

// pressDocumentTree moves the cursor of the tree to the row a press landed on. A second press
// on the same row opens it, the way a second press on a row of the grid opens the row.
func (model *Model) pressDocumentTree(
	connection *app.Connection, tab *app.Tab, mouse tea.Mouse,
) (tea.Model, tea.Cmd) {
	row, found := model.layout.documentRowsHit.holds(mouse.X, mouse.Y)
	if !found || row >= model.buildDocumentTree(connection, tab).CountRows() {
		return model, nil
	}
	tab.TreeRow, tab.TreeRolled = row, false

	if model.clicks.count("document-"+strconv.Itoa(row), time.Now()) < 2 {
		// The row pressed is a row of the tree, and the page is asked for by the document
		// that row belongs to.
		node, held := model.findDocumentNode(connection, tab)
		if !held {
			return model, nil
		}
		return model, model.approachDocumentEnd(connection, tab, node.RowIndex)
	}
	return model.runDocumentTreeAction(connection, tab,
		Match{Action: ActionOpenNode, Scope: cfg.ScopeDocument})
}
