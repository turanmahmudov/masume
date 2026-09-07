package app

import (
	"slices"
	"strings"

	"github.com/turanmahmudov/masume/internal/core"
	"github.com/turanmahmudov/masume/internal/notebook"
)

// The cells of a notebook are moved, added and deleted from the cell list. Every change of
// the list is one step of an undo stack of its own, apart from the undo of a buffer.

// notebookUndoDepth is how many structural changes can be undone.
const notebookUndoDepth = 50

// FocusCell moves the list to that cell.
func (book *Notebook) FocusCell(index int) {
	book.Focused = core.ClampIndex(index, len(book.Cells))
	book.Editing = false
	// The list follows the focused cell again, whatever the wheel rolled to.
	book.Rolled = false
}

// StepCell moves the list up or down, and stops at the ends.
func (book *Notebook) StepCell(step int) {
	book.FocusCell(book.Focused + step)
}

// AddCell inserts an empty statement cell above or below the focused one, and moves to it.
func (book *Notebook) AddCell(below bool) *NotebookCell {
	book.rememberCells()
	cell := book.buildCell(notebook.CellSQL)
	at := book.Focused
	if below {
		at++
	}
	at = min(max(at, 0), len(book.Cells))
	book.Cells = slices.Insert(book.Cells, at, cell)
	book.FocusCell(at)
	book.Dirty = true
	return cell
}

// DeleteCell deletes the focused cell. The last cell is replaced by an empty one.
func (book *Notebook) DeleteCell() bool {
	if len(book.Cells) == 0 {
		return false
	}
	book.rememberCells()
	at := core.ClampIndex(book.Focused, len(book.Cells))
	book.Cells = slices.Delete(book.Cells, at, at+1)
	if len(book.Cells) == 0 {
		book.Cells = []*NotebookCell{book.buildCell(notebook.CellSQL)}
	}
	book.FocusCell(at)
	book.Dirty = true
	return true
}

// MoveCell moves the focused cell up or down, and the focus goes with it.
func (book *Notebook) MoveCell(step int) bool {
	at := core.ClampIndex(book.Focused, len(book.Cells))
	to := at + step
	if to < 0 || to >= len(book.Cells) {
		return false
	}
	book.rememberCells()
	book.Cells[at], book.Cells[to] = book.Cells[to], book.Cells[at]
	book.FocusCell(to)
	book.Dirty = true
	return true
}

// SetCellKind changes what the focused cell holds.
func (book *Notebook) SetCellKind(kind notebook.CellKind) {
	cell := book.GetFocusedCell()
	if cell.Kind == kind {
		return
	}
	book.rememberCells()
	cell.Kind = kind
	cell.Fence = ""
	cell.FirstResult, cell.ResultCount = -1, 0
	book.Dirty = true
}

// CopyCell returns a copy of the focused cell for the clipboard of the cells.
func (book *Notebook) CopyCell() *NotebookCell {
	cell := book.GetFocusedCell()
	return &NotebookCell{
		ID: cell.ID, Kind: cell.Kind, Fence: cell.Fence,
		Attrs: slices.Clone(cell.Attrs),
		// The copy carries the text, and none of the results of the original.
		Editor: NewEditorBuffer(cell.Editor.Text, 0), FirstResult: -1,
	}
}

// PasteCell inserts a copy of that cell below the focused one.
func (book *Notebook) PasteCell(held *NotebookCell) bool {
	if held == nil {
		return false
	}
	book.rememberCells()
	cell := &NotebookCell{
		ID: book.resolveFreeID(held.ID), Kind: held.Kind, Fence: held.Fence,
		Attrs:  slices.Clone(held.Attrs),
		Editor: NewEditorBuffer(held.Editor.Text, 0), FirstResult: -1,
	}
	at := min(book.Focused+1, len(book.Cells))
	book.Cells = slices.Insert(book.Cells, at, cell)
	book.FocusCell(at)
	book.Dirty = true
	return true
}

// FoldCell folds the focused cell away, or opens it again.
func (book *Notebook) FoldCell() {
	cell := book.GetFocusedCell()
	cell.Folded = !cell.Folded
}

// FoldEveryCell folds every cell away, or opens every one.
func (book *Notebook) FoldEveryCell() {
	folded := false
	for _, cell := range book.Cells {
		if !cell.Folded {
			folded = true
			break
		}
	}
	for _, cell := range book.Cells {
		cell.Folded = folded
	}
}

// MarkCell marks the focused cell for a partial run, or takes the mark off.
func (book *Notebook) MarkCell() {
	cell := book.GetFocusedCell()
	cell.Marked = !cell.Marked
}

// rememberCells keeps the order of the cells for one undo step.
func (book *Notebook) rememberCells() {
	book.undone = append(book.undone, slices.Clone(book.Cells))
	if len(book.undone) > notebookUndoDepth {
		book.undone = book.undone[1:]
	}
	book.redone = nil
}

// UndoCellChange undoes the last change of the list.
func (book *Notebook) UndoCellChange() bool {
	if len(book.undone) == 0 {
		return false
	}
	book.redone = append(book.redone, slices.Clone(book.Cells))
	book.Cells = book.undone[len(book.undone)-1]
	book.undone = book.undone[:len(book.undone)-1]
	book.FocusCell(book.Focused)
	book.Dirty = true
	return true
}

// RedoCellChange restores an undone change of the list.
func (book *Notebook) RedoCellChange() bool {
	if len(book.redone) == 0 {
		return false
	}
	book.undone = append(book.undone, slices.Clone(book.Cells))
	book.Cells = book.redone[len(book.redone)-1]
	book.redone = book.redone[:len(book.redone)-1]
	book.FocusCell(book.Focused)
	book.Dirty = true
	return true
}

// RunScope is which cells one run sends.
type RunScope string

// The four scopes of a run.
const (
	// RunCell sends the focused cell.
	RunCell RunScope = "cell"
	// RunFromCell sends the focused cell and every cell below it.
	RunFromCell RunScope = "below"
	// RunEveryCell sends every cell.
	RunEveryCell RunScope = "all"
	// RunMarkedCells sends the cells the reader marked.
	RunMarkedCells RunScope = "marked"
)

// RunPlan is what one run sends, and the cell each statement came from.
type RunPlan struct {
	Statements []string
	// CellOf holds the cell index of each statement.
	CellOf []int
	// The cells the run covers, in the order they stand.
	Cells []int
}

// IsEmpty is true for a plan with no statement to send.
func (plan RunPlan) IsEmpty() bool {
	return len(plan.Statements) == 0
}

// BuildRunPlan returns the statements of one run. Splitting a cell into statements is the
// work of the engine, so the caller hands the splitter in.
func (book *Notebook) BuildRunPlan(
	scope RunScope, split func(string) []string,
) RunPlan {
	plan := RunPlan{}
	for _, at := range book.listCellsOfScope(scope) {
		cell := book.Cells[at]
		plan.Cells = append(plan.Cells, at)
		if !cell.RunsStatements() {
			continue
		}
		for _, written := range split(cell.Editor.Text) {
			if strings.TrimSpace(written) == "" {
				continue
			}
			plan.Statements = append(plan.Statements, written)
			plan.CellOf = append(plan.CellOf, at)
		}
	}
	return plan
}

// ExpandRunPlan writes the statement of a named cell in place of every `{{cell:id}}` of the
// plan, and returns the reason a reference cannot be expanded.
func (book *Notebook) ExpandRunPlan(plan RunPlan) (RunPlan, error) {
	sources := book.buildReferenceSources()
	statements := make([]string, 0, len(plan.Statements))
	for _, written := range plan.Statements {
		expanded, err := notebook.ExpandReferences(written, sources)
		if err != nil {
			return plan, err
		}
		statements = append(statements, expanded)
	}
	plan.Statements = statements
	return plan, nil
}

// buildReferenceSources returns the statement of every cell a reference may name.
func (book *Notebook) buildReferenceSources() map[string]string {
	sources := map[string]string{}
	for _, cell := range book.Cells {
		if cell.RunsStatements() {
			sources[cell.ID] = cell.Editor.Text
		}
	}
	return sources
}

// HoldsReferences is true where any cell of this notebook names another cell.
func (book *Notebook) HoldsReferences() bool {
	for _, cell := range book.Cells {
		if cell.RunsStatements() && notebook.HoldsReference(cell.Editor.Text) {
			return true
		}
	}
	return false
}

// listCellsOfScope returns the cells one scope covers.
func (book *Notebook) listCellsOfScope(scope RunScope) []int {
	switch scope {
	case RunCell:
		return []int{core.ClampIndex(book.Focused, len(book.Cells))}
	case RunFromCell:
		cells := []int{}
		for at := core.ClampIndex(book.Focused, len(book.Cells)); at < len(book.Cells); at++ {
			cells = append(cells, at)
		}
		return cells
	case RunMarkedCells:
		return book.ListMarkedCells()
	}
	cells := make([]int, 0, len(book.Cells))
	for at := range book.Cells {
		cells = append(cells, at)
	}
	return cells
}

// ApplyRunPlan writes the results of a run onto the cells it covers. The store of the tab
// holds the results of one run, so a cell outside this run keeps no place in it: one that
// answered before is marked stale, and the reader is told its rows are gone.
func (book *Notebook) ApplyRunPlan(plan RunPlan) {
	covered := map[int]bool{}
	for _, at := range plan.Cells {
		covered[at] = true
	}
	for at, cell := range book.Cells {
		if covered[at] {
			cell.FirstResult, cell.ResultCount, cell.Stale = -1, 0, false
			continue
		}
		cell.Stale = cell.Stale || cell.ResultCount > 0
		cell.FirstResult, cell.ResultCount = -1, 0
	}
	for index, at := range plan.CellOf {
		cell := book.Cells[at]
		if cell.FirstResult < 0 {
			cell.FirstResult = index
		}
		cell.ResultCount++
	}
}

// ListFoldedCells returns the id of every cell that is folded away.
func (book *Notebook) ListFoldedCells() []string {
	folded := []string{}
	for _, cell := range book.Cells {
		if cell.Folded {
			folded = append(folded, cell.ID)
		}
	}
	return folded
}
