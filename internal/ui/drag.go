package ui

type dragKind int

const (
	dragNothing dragKind = iota
	dragScrollbar
	dragEditorText
	dragSplitLine
	dragColumnEdge
	dragTreeEdge
)

type pointerDrag struct {
	kind dragKind
	// The bar being dragged, and where its thumb was taken hold of, in half cells. The
	// pointer may wander off the track and the drag holds, which is what a scroll bar does.
	bar  scrollHit
	grab int
	// True once a drag of a border has moved it at all, because a press that never moved
	// does something else: it hides the result, or it moves the keyboard to a pane.
	moved bool
	// True where the drag of the line began on the top border of the result. A press there
	// that never moved reaches the result, and one on the editor border hides it.
	splitFromResult bool
	// The column whose border is being dragged, the width it had when the drag began, and
	// the cell the pointer stood on then.
	column      int
	columnWidth int
	columnFrom  int
}

func (drag pointerDrag) running() bool { return drag.kind != dragNothing }

func (drag pointerDrag) holds(kind dragKind) bool { return drag.kind == kind }

func (drag *pointerDrag) takeScrollbar(bar scrollHit, grab int) {
	*drag = pointerDrag{kind: dragScrollbar, bar: bar, grab: grab}
}

func (drag *pointerDrag) takeEditorText() {
	*drag = pointerDrag{kind: dragEditorText}
}

func (drag *pointerDrag) takeSplitLine(fromResult bool) {
	*drag = pointerDrag{kind: dragSplitLine, splitFromResult: fromResult}
}

func (drag *pointerDrag) takeTreeEdge() {
	*drag = pointerDrag{kind: dragTreeEdge}
}

func (drag *pointerDrag) takeColumnEdge(column, width, from int) {
	*drag = pointerDrag{
		kind: dragColumnEdge, column: column, columnWidth: width, columnFrom: from,
	}
}

func (drag *pointerDrag) stop() { *drag = pointerDrag{} }
