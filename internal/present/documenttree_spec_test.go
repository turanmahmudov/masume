package present_test

import (
	"fmt"
	"strconv"
	"testing"

	"github.com/turanmahmudov/masume/internal/core"
	"github.com/turanmahmudov/masume/internal/db"
	"github.com/turanmahmudov/masume/internal/present"
)

// buildOrderColumns returns the columns of a collection of orders.
func buildOrderColumns() []db.ResultColumn {
	return []db.ResultColumn{
		{Name: "_id", DataType: core.DocumentTypeObjectID},
		{Name: "number", DataType: core.DocumentTypeString},
		{Name: "customer", DataType: core.DocumentTypeObject},
		{Name: "lines", DataType: core.DocumentTypeArray},
	}
}

// buildOrderRows returns rows in the form of a document collection, with the nested values
// written so that every type is kept.
func buildOrderRows(count int) [][]any {
	rows := make([][]any, 0, count)
	for at := range count {
		customer := fmt.Sprintf(
			`{"name":"customer %d","age":{"$numberInt":"%d"},`+
				`"joined":{"$date":{"$numberLong":"1600000000000"}}}`, at, 20+at%50)
		lines := `[{"sku":"sku-1","quantity":{"$numberInt":"2"}},` +
			`{"sku":"sku-2","quantity":{"$numberInt":"5"}}]`
		rows = append(rows, []any{
			fmt.Sprintf("%024x", at),
			fmt.Sprintf("ORD-%d", 100000+at),
			core.DocumentValue{Text: customer, Count: 3},
			core.DocumentValue{Text: lines, Count: 2, IsArray: true},
		})
	}
	return rows
}

func buildDocumentTreeInput(rows [][]any, opened map[string]bool) present.DocumentTreeInput {
	if opened == nil {
		opened = map[string]bool{}
	}
	indexes := make([]int, len(rows))
	for at := range rows {
		indexes[at] = at
	}
	return present.DocumentTreeInput{
		Columns: buildOrderColumns(), Rows: rows, RowIndexes: indexes, Opened: opened,
	}
}

// A folded document is one row of the tree, whatever it contains. Without an open node the
// user sees one row per document and scrolls them in the same way as the grid rows.
func TestAFoldedDocumentIsOneRow(t *testing.T) {
	tree := present.BuildDocumentTree(buildDocumentTreeInput(buildOrderRows(500), nil))

	if tree.CountRows() != 500 {
		t.Fatalf("the tree draws %d rows, wanted one per document", tree.CountRows())
	}
	nodes := tree.ReadWindow(0, 3)
	if len(nodes) != 3 {
		t.Fatalf("the window holds %d rows, wanted 3", len(nodes))
	}
	if nodes[0].Key != "000000000000000000000000" {
		t.Errorf("the first row reads %q, wanted the identity of the document", nodes[0].Key)
	}
	if nodes[0].Value != "{ 4 fields }" {
		t.Errorf("the first row says %q, wanted how many fields it holds", nodes[0].Value)
	}
	if !nodes[0].Opens || nodes[0].Open {
		t.Error("a folded document has to open, and has to be folded")
	}
}

// An open document shows its fields, one level deeper.
func TestOpeningADocumentShowsItsFields(t *testing.T) {
	opened := map[string]bool{present.BuildRowPath(0): true}
	tree := present.BuildDocumentTree(buildDocumentTreeInput(buildOrderRows(10), opened))

	if tree.CountRows() != 14 {
		t.Fatalf("the tree draws %d rows, wanted 10 documents and the 4 fields of one",
			tree.CountRows())
	}
	nodes := tree.ReadWindow(0, 6)
	wanted := []struct {
		key   string
		depth int
	}{
		{"000000000000000000000000", 0},
		{"_id", 1}, {"number", 1}, {"customer", 1}, {"lines", 1},
		{"000000000000000000000001", 0},
	}
	for at, held := range wanted {
		if nodes[at].Key != held.key || nodes[at].Depth != held.depth {
			t.Errorf("row %d is %q at depth %d, wanted %q at depth %d",
				at, nodes[at].Key, nodes[at].Depth, held.key, held.depth)
		}
	}
	if nodes[3].Value != "{ 3 fields }" {
		t.Errorf("the customer says %q, wanted the fields it holds", nodes[3].Value)
	}
	if nodes[4].Value != "[ 2 elements ]" {
		t.Errorf("the lines say %q, wanted the elements they hold", nodes[4].Value)
	}
}

// The type of a nested value is the type on the server and not the type JSON can express.
// This is the purpose of the tree.
func TestANestedValueKeepsTheTypeTheServerStores(t *testing.T) {
	opened := map[string]bool{
		present.BuildRowPath(0):                       true,
		present.BuildRowPath(0) + "\x1f" + "customer": true,
	}
	tree := present.BuildDocumentTree(buildDocumentTreeInput(buildOrderRows(3), opened))

	held := map[string]present.DocumentNode{}
	for _, node := range tree.ReadWindow(0, tree.CountRows()) {
		if node.Depth == 2 {
			held[node.Key] = node
		}
	}

	for _, want := range []struct {
		key   string
		value string
		named string
	}{
		{"name", "customer 0", core.DocumentTypeString},
		{"age", "20", core.DocumentTypeInt},
		{"joined", "2020-09-13 12:26:40.000", core.DocumentTypeDate},
	} {
		node, found := held[want.key]
		if !found {
			t.Errorf("the tree has no %q under the customer", want.key)
			continue
		}
		if node.Value != want.value {
			t.Errorf("%q reads %q, wanted %q", want.key, node.Value, want.value)
		}
		if node.Type != want.named {
			t.Errorf("%q is a %q, wanted a %q", want.key, node.Type, want.named)
		}
	}
}

// An array is opened by the index of each element, because an element has no name.
func TestAnArrayIsOpenedByThePlaceOfItsElements(t *testing.T) {
	opened := map[string]bool{
		present.BuildRowPath(0):                    true,
		present.BuildRowPath(0) + "\x1f" + "lines": true,
	}
	tree := present.BuildDocumentTree(buildDocumentTreeInput(buildOrderRows(3), opened))

	found := []string{}
	for _, node := range tree.ReadWindow(0, tree.CountRows()) {
		if node.Depth == 2 {
			found = append(found, node.Key+"="+node.Value)
		}
	}
	if len(found) != 2 {
		t.Fatalf("the array opened to %v, wanted its two elements", found)
	}
	if found[0] != "0={ 2 fields }" || found[1] != "1={ 2 fields }" {
		t.Errorf("the elements read %v, wanted them keyed by their place", found)
	}
}

// The window is the part the pane draws. It must contain the same rows through a read of the
// whole tree and through a direct read. This lets the tree skip the rows above the window.
func TestAWindowHoldsTheSameRowsAsAWalkFromTheTop(t *testing.T) {
	opened := map[string]bool{}
	// The user opens a few documents at different positions, and one nested value.
	for _, at := range []int{0, 3, 77, 512, 4998} {
		opened[present.BuildRowPath(at)] = true
	}
	opened[present.BuildRowPath(77)+"\x1f"+"customer"] = true
	opened[present.BuildRowPath(512)+"\x1f"+"lines"] = true

	tree := present.BuildDocumentTree(buildDocumentTreeInput(buildOrderRows(5000), opened))
	everything := tree.ReadWindow(0, tree.CountRows())
	if len(everything) != tree.CountRows() {
		t.Fatalf("reading the whole tree gave %d rows, wanted %d",
			len(everything), tree.CountRows())
	}

	for _, from := range []int{0, 1, 4, 5, 80, 90, 520, 530, 4999, 5010, tree.CountRows() - 3} {
		window := tree.ReadWindow(from, 12)
		wanted := everything[min(from, len(everything)):min(from+12, len(everything))]
		if len(window) != len(wanted) {
			t.Fatalf("the window at %d holds %d rows, wanted %d",
				from, len(window), len(wanted))
		}
		for at := range window {
			if window[at].Path != wanted[at].Path {
				t.Fatalf("row %d of the window at %d is %q, wanted %q",
					at, from, window[at].Path, wanted[at].Path)
			}
		}
	}
}

// A row after the end of the tree returns nothing and not the last row again.
func TestAWindowPastTheEndDrawsNothing(t *testing.T) {
	tree := present.BuildDocumentTree(buildDocumentTreeInput(buildOrderRows(4), nil))
	if held := tree.ReadWindow(4, 10); len(held) != 0 {
		t.Errorf("the window past the end holds %d rows, wanted none", len(held))
	}
	if held := tree.ReadWindow(3, 10); len(held) != 1 {
		t.Errorf("the last window holds %d rows, wanted the one row left", len(held))
	}
}

// The guide line on the left shows which branch continues below a row. The last field of a
// document ends its branch, and the fields above it keep it open.
func TestTheLastFieldClosesItsBranch(t *testing.T) {
	opened := map[string]bool{present.BuildRowPath(0): true}
	tree := present.BuildDocumentTree(buildDocumentTreeInput(buildOrderRows(2), opened))
	nodes := tree.ReadWindow(0, 5)

	for at := 1; at < 4; at++ {
		if nodes[at].Last {
			t.Errorf("%q closes its branch, wanted it left open", nodes[at].Key)
		}
	}
	if !nodes[4].Last {
		t.Errorf("%q leaves its branch open, wanted the last field to close it", nodes[4].Key)
	}
}

// The tree is available if a value can be opened. A result of plain columns has none, and a
// result with a column of JSON text has one, also if the server gave no document type.
func TestATreeIsOfferedWhereAValueOpens(t *testing.T) {
	plain := []db.ResultColumn{{Name: "id", DataType: "integer"}, {Name: "name", DataType: "text"}}
	if present.HasDocumentColumn(plain, [][]any{{int64(1), "ada"}}) {
		t.Error("a result of plain columns offered a tree")
	}
	if !present.HasDocumentColumn(buildOrderColumns(), buildOrderRows(1)) {
		t.Error("a result of documents offered no tree")
	}

	text := []db.ResultColumn{{Name: "id", DataType: "integer"}, {Name: "body", DataType: "text"}}
	if !present.HasDocumentColumn(text, [][]any{{int64(1), `{"a":1}`}}) {
		t.Error("a column of JSON text offered no tree")
	}
	if present.HasDocumentColumn(text, [][]any{{int64(1), "just words"}}) {
		t.Error("a column of words offered a tree")
	}
}

// A document that contains itself would open without an end. The tree stops at the depth
// limit instead of using all memory for one row.
func TestTheTreeStopsAtADepthItWillNotPass(t *testing.T) {
	deep := "1"
	for range 80 {
		deep = `{"down":` + deep + `}`
	}
	columns := []db.ResultColumn{{Name: "nest", DataType: core.DocumentTypeObject}}
	rows := [][]any{{core.DocumentValue{Text: deep, Count: 1}}}

	opened := map[string]bool{present.BuildRowPath(0): true}
	path := present.BuildRowPath(0) + "\x1f" + "nest"
	for range 80 {
		opened[path] = true
		path += "\x1f" + "down"
	}

	tree := present.BuildDocumentTree(present.DocumentTreeInput{
		Columns: columns, Rows: rows, RowIndexes: []int{0}, Opened: opened,
	})
	nodes := tree.ReadWindow(0, tree.CountRows())
	deepest := 0
	for _, node := range nodes {
		deepest = max(deepest, node.Depth)
	}
	if deepest > 33 {
		t.Errorf("the tree opened to depth %d, wanted it to stop", deepest)
	}
}

// The path of a node identifies its field, so the client can show the position of the
// cursor.
func TestThePathNamesTheFieldItStandsOn(t *testing.T) {
	path := present.BuildRowPath(4) + "\x1f" + "customer" + "\x1f" + "address"
	keys := present.ReadDocumentPathKeys(path)
	if len(keys) != 2 || keys[0] != "customer" || keys[1] != "address" {
		t.Errorf("the path reads as %v, wanted the two field names", keys)
	}
	if held := present.ReadDocumentPathKeys(present.BuildRowPath(4)); held != nil {
		t.Errorf("the path of a row reads as %v, wanted no field names", held)
	}
}

// A read of a window deep inside a large result must not cost the same as a read of every
// row before it. The tree is built from the open rows, so a folded result of a million rows
// is fast to scroll.
func BenchmarkReadADocumentWindow(b *testing.B) {
	for _, rows := range []int{1000, 100000, 1000000} {
		b.Run(strconv.Itoa(rows), func(b *testing.B) {
			opened := map[string]bool{}
			for _, at := range []int{2, 40, 900} {
				opened[present.BuildRowPath(at)] = true
			}
			input := buildDocumentTreeInput(buildOrderRows(rows), opened)

			b.ReportAllocs()
			b.ResetTimer()
			for b.Loop() {
				tree := present.BuildDocumentTree(input)
				tree.ReadWindow(tree.CountRows()-40, 40)
			}
		})
	}
}
