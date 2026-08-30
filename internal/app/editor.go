package app

import (
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/turanmahmudov/masume/internal/core"
	"github.com/turanmahmudov/masume/internal/query/language"
)

// EditorBuffer is the statement being written, and the caret in it. The offsets count bytes,
// because every reader of the buffer counts them the same way.
type EditorBuffer struct {
	Text  string
	Caret int
	// The other end of a selection. A selection is empty while it equals the caret.
	Anchor int
	// The column the caret keeps while it moves up and down over short lines.
	wantedColumn int
	hasWanted    bool
	// The steps an undo goes back to, and the ones a redo comes forward to.
	undone []editorStep
	redone []editorStep
	// The kind of the last edit, so a run of typing is taken back in one press.
	lastEdit editKind
	// The group the last character typed was in, so a word is taken back on its own.
	lastGroup int
}

// editorStep is the buffer as it stood before one edit.
type editorStep struct {
	Text   string
	Caret  int
	Anchor int
}

// editKind groups the edits that join into one step of the undo.
type editKind int

const (
	// editNone is the state after a move, which ends the run of edits before it.
	editNone editKind = iota
	// editTyping is one character written at the caret.
	editTyping
	// editDeleting is one character taken at the caret.
	editDeleting
	// editWhole is an edit that stands on its own, such as a paste or a format.
	editWhole
)

// undoDepth is how many steps back the editor can go.
const undoDepth = 500

// NewEditorBuffer starts a buffer on the text and the caret a restored tab held.
func NewEditorBuffer(text string, caret int) *EditorBuffer {
	held := core.ClampWithin(caret, len(text))
	return &EditorBuffer{Text: text, Caret: held, Anchor: held}
}

// Selection returns the text between the caret and the anchor.
func (buffer *EditorBuffer) Selection() string {
	start, end := buffer.SelectionRange()
	return buffer.Text[start:end]
}

// SelectionRange returns the two offsets of the selection, the lower one first.
func (buffer *EditorBuffer) SelectionRange() (int, int) {
	start, end := buffer.Caret, buffer.Anchor
	if start > end {
		start, end = end, start
	}
	return core.ClampWithin(start, len(buffer.Text)), core.ClampWithin(end, len(buffer.Text))
}

// HasSelection is true while the caret and the anchor stand apart.
func (buffer *EditorBuffer) HasSelection() bool {
	start, end := buffer.SelectionRange()
	return end > start
}

// ClearSelection lets the selection go, which every ordinary edit does.
func (buffer *EditorBuffer) ClearSelection() {
	buffer.Anchor = buffer.Caret
}

// SelectAll takes the whole buffer, which is what the editor binds Ctrl+A to.
func (buffer *EditorBuffer) SelectAll() {
	buffer.Anchor = 0
	buffer.Caret = len(buffer.Text)
	buffer.hasWanted = false
}

// SetText replaces the buffer and puts the caret at the end.
func (buffer *EditorBuffer) SetText(text string) {
	buffer.SetTextWithCaret(text, len(text))
}

// SetTextWithCaret replaces the buffer and puts the caret where it is named. The caret and
// the anchor are set together, so the buffer never comes back holding a selection nobody
// asked for.
func (buffer *EditorBuffer) SetTextWithCaret(text string, caret int) {
	buffer.rememberBefore(editWhole)
	buffer.Text = text
	buffer.Caret = core.ClampWithin(caret, len(text))
	buffer.ClearSelection()
	buffer.hasWanted = false
}

// Insert writes the text at the caret, over the selection where there is one.
func (buffer *EditorBuffer) Insert(written string) {
	kind := editTyping
	switch {
	case buffer.HasSelection(), len([]rune(written)) != 1,
		strings.ContainsAny(written, "\n\t"):
		kind = editWhole
	default:
		// A blank after a word ends the run, so an undo takes back one word and not the
		// whole of what was typed.
		group := classifyRune(readRuneAt(written, 0))
		if buffer.lastEdit == editTyping && group == blankRune &&
			buffer.lastGroup != blankRune {
			buffer.lastEdit = editNone
		}
		buffer.lastGroup = group
	}
	buffer.rememberBefore(kind)
	start, end := buffer.SelectionRange()
	buffer.Text = buffer.Text[:start] + written + buffer.Text[end:]
	buffer.Caret = start + len(written)
	buffer.ClearSelection()
	buffer.hasWanted = false
}

// DeleteBackward removes the selection, or the character before the caret.
func (buffer *EditorBuffer) DeleteBackward() {
	if buffer.HasSelection() {
		buffer.Insert("")
		return
	}
	if buffer.Caret == 0 {
		return
	}
	buffer.rememberBefore(editDeleting)
	_, width := utf8.DecodeLastRuneInString(buffer.Text[:buffer.Caret])
	buffer.Text = buffer.Text[:buffer.Caret-width] + buffer.Text[buffer.Caret:]
	buffer.Caret -= width
	buffer.ClearSelection()
	buffer.hasWanted = false
}

// DeleteForward removes the selection, or the character after the caret.
func (buffer *EditorBuffer) DeleteForward() {
	if buffer.HasSelection() {
		buffer.Insert("")
		return
	}
	if buffer.Caret >= len(buffer.Text) {
		return
	}
	buffer.rememberBefore(editDeleting)
	_, width := utf8.DecodeRuneInString(buffer.Text[buffer.Caret:])
	buffer.Text = buffer.Text[:buffer.Caret] + buffer.Text[buffer.Caret+width:]
	buffer.ClearSelection()
	buffer.hasWanted = false
}

// DeleteWordBackward removes the selection, or the word before the caret.
func (buffer *EditorBuffer) DeleteWordBackward() {
	if buffer.HasSelection() {
		buffer.Insert("")
		return
	}
	start := buffer.FindWordStart(buffer.Caret)
	if start >= buffer.Caret {
		return
	}
	buffer.rememberBefore(editWhole)
	buffer.Text = buffer.Text[:start] + buffer.Text[buffer.Caret:]
	buffer.Caret = start
	buffer.ClearSelection()
	buffer.hasWanted = false
}

// DeleteWordForward removes the selection, or the word after the caret.
func (buffer *EditorBuffer) DeleteWordForward() {
	if buffer.HasSelection() {
		buffer.Insert("")
		return
	}
	end := buffer.FindWordEnd(buffer.Caret)
	if end <= buffer.Caret {
		return
	}
	buffer.rememberBefore(editWhole)
	buffer.Text = buffer.Text[:buffer.Caret] + buffer.Text[end:]
	buffer.ClearSelection()
	buffer.hasWanted = false
}

// SelectedLineRange returns the offsets of the whole lines the selection touches, and the
// caret's own line where there is no selection.
func (buffer *EditorBuffer) SelectedLineRange() (int, int) {
	start, end := buffer.SelectionRange()
	// A selection that ends on the first cell of a line stops at the line above it, so
	// picking down to the start of a line does not take the line under it as well.
	if end > start && end == buffer.LineStart(end) {
		end--
	}
	return buffer.LineStart(start), buffer.LineEnd(end)
}

// CommentLines writes the mark in front of every line the selection touches, and takes it
// away again where every one of them already carries it. It reports whether anything changed.
func (buffer *EditorBuffer) CommentLines(mark string) bool {
	if mark == "" {
		return false
	}
	start, end := buffer.SelectedLineRange()
	lines := strings.Split(buffer.Text[start:end], "\n")

	// Every line that holds anything has to carry the mark for the press to take it away,
	// so a block half of which is commented is commented whole first.
	commented, written := true, 0
	for _, line := range lines {
		if strings.TrimSpace(line) == "" {
			continue
		}
		written++
		if !strings.HasPrefix(strings.TrimLeft(line, " \t"), mark) {
			commented = false
		}
	}
	if written == 0 {
		return false
	}

	// The mark stands at the indent the shallowest line has, so a block keeps its shape.
	column := -1
	for _, line := range lines {
		if strings.TrimSpace(line) == "" {
			continue
		}
		if at := len(line) - len(strings.TrimLeft(line, " \t")); column < 0 || at < column {
			column = at
		}
	}

	changed := make([]string, 0, len(lines))
	for _, line := range lines {
		if strings.TrimSpace(line) == "" {
			changed = append(changed, line)
			continue
		}
		if commented {
			changed = append(changed, dropCommentMark(line, mark))
			continue
		}
		changed = append(changed, line[:column]+mark+" "+line[column:])
	}
	buffer.replaceLines(start, end, strings.Join(changed, "\n"))
	return true
}

// dropCommentMark takes the mark, and the one blank after it, off the front of the line.
func dropCommentMark(line, mark string) string {
	indent := len(line) - len(strings.TrimLeft(line, " \t"))
	rest := strings.TrimPrefix(line[indent:], mark)
	rest = strings.TrimPrefix(rest, " ")
	return line[:indent] + rest
}

// IndentLines moves every line the selection touches one step to the right.
func (buffer *EditorBuffer) IndentLines(width int) bool {
	if width < 1 {
		return false
	}
	start, end := buffer.SelectedLineRange()
	step := strings.Repeat(" ", width)
	changed := []string{}
	for line := range strings.SplitSeq(buffer.Text[start:end], "\n") {
		if line == "" {
			changed = append(changed, line)
			continue
		}
		changed = append(changed, step+line)
	}
	buffer.replaceLines(start, end, strings.Join(changed, "\n"))
	return true
}

// OutdentLines moves every line the selection touches one step to the left, as far as each
// one can go. It reports whether any of them moved.
func (buffer *EditorBuffer) OutdentLines(width int) bool {
	if width < 1 {
		return false
	}
	start, end := buffer.SelectedLineRange()
	lines := strings.Split(buffer.Text[start:end], "\n")

	changed := make([]string, 0, len(lines))
	moved := false
	for _, line := range lines {
		taken := 0
		for taken < width && taken < len(line) && (line[taken] == ' ' || line[taken] == '\t') {
			taken++
		}
		if taken > 0 {
			moved = true
		}
		changed = append(changed, line[taken:])
	}
	if !moved {
		return false
	}
	buffer.replaceLines(start, end, strings.Join(changed, "\n"))
	return true
}

// replaceLines writes the block of lines again and keeps the selection over it, so a second
// press works on the same lines.
func (buffer *EditorBuffer) replaceLines(start, end int, written string) {
	buffer.rememberBefore(editWhole)
	buffer.Text = buffer.Text[:start] + written + buffer.Text[end:]
	buffer.Anchor, buffer.Caret = start, start+len(written)
	buffer.hasWanted = false
	buffer.lastEdit = editWhole
}

// MoveCaret moves the caret one character, and grows the selection while `selecting`. A move
// that is not selecting and stands on a selection goes to the end of it instead, which is
// what a caret does in every other editor.
func (buffer *EditorBuffer) MoveCaret(step int, selecting bool) {
	if !selecting && buffer.HasSelection() {
		start, end := buffer.SelectionRange()
		if step < 0 {
			buffer.Caret = start
		} else if step > 0 {
			buffer.Caret = end
		}
		buffer.settle(false)
		buffer.hasWanted = false
		return
	}
	if step < 0 && buffer.Caret > 0 {
		_, width := utf8.DecodeLastRuneInString(buffer.Text[:buffer.Caret])
		buffer.Caret -= width
	} else if step > 0 && buffer.Caret < len(buffer.Text) {
		_, width := utf8.DecodeRuneInString(buffer.Text[buffer.Caret:])
		buffer.Caret += width
	}
	buffer.settle(selecting)
	buffer.hasWanted = false
}

// MoveWord moves the caret over one word, and grows the selection while `selecting`.
func (buffer *EditorBuffer) MoveWord(step int, selecting bool) {
	if step < 0 {
		buffer.Caret = buffer.FindWordStart(buffer.Caret)
	} else if step > 0 {
		buffer.Caret = buffer.FindWordEnd(buffer.Caret)
	}
	buffer.settle(selecting)
	buffer.hasWanted = false
}

// LineStart returns where the line holding that offset begins.
func (buffer *EditorBuffer) LineStart(offset int) int {
	broke := strings.LastIndexByte(buffer.Text[:core.ClampWithin(offset, len(buffer.Text))], '\n')
	return broke + 1
}

// LineEnd returns where the line holding that offset ends.
func (buffer *EditorBuffer) LineEnd(offset int) int {
	held := core.ClampWithin(offset, len(buffer.Text))
	broke := strings.IndexByte(buffer.Text[held:], '\n')
	if broke == -1 {
		return len(buffer.Text)
	}
	return held + broke
}

// MoveToStart puts the caret before the first character of the text.
func (buffer *EditorBuffer) MoveToStart(selecting bool) {
	buffer.Caret = 0
	buffer.settle(selecting)
	buffer.hasWanted = false
}

// MoveToEnd puts the caret after the last character of the text.
func (buffer *EditorBuffer) MoveToEnd(selecting bool) {
	buffer.Caret = len(buffer.Text)
	buffer.settle(selecting)
	buffer.hasWanted = false
}

// MoveToLineStart puts the caret before the first word of the line, and before the indent of
// the line on a second press.
func (buffer *EditorBuffer) MoveToLineStart(selecting bool) {
	start := buffer.LineStart(buffer.Caret)
	end := buffer.LineEnd(buffer.Caret)
	first := start
	for first < end && (buffer.Text[first] == ' ' || buffer.Text[first] == '\t') {
		first++
	}
	if buffer.Caret == first {
		buffer.Caret = start
	} else {
		buffer.Caret = first
	}
	buffer.settle(selecting)
	buffer.hasWanted = false
}

// MoveToLineEnd puts the caret after the last character of the line.
func (buffer *EditorBuffer) MoveToLineEnd(selecting bool) {
	buffer.Caret = buffer.LineEnd(buffer.Caret)
	buffer.settle(selecting)
	buffer.hasWanted = false
}

// MoveLine moves the caret a line up or down, and keeps the column it was in over a short
// line.
func (buffer *EditorBuffer) MoveLine(step int, selecting bool) {
	lineStart := buffer.LineStart(buffer.Caret)
	if !buffer.hasWanted {
		buffer.wantedColumn = buffer.Caret - lineStart
		buffer.hasWanted = true
	}

	if step < 0 {
		if lineStart == 0 {
			buffer.Caret = 0
			buffer.settle(selecting)
			return
		}
		previousStart := buffer.LineStart(lineStart - 1)
		buffer.Caret = buffer.placeInLine(previousStart, lineStart-1)
		buffer.settle(selecting)
		return
	}

	lineEnd := buffer.LineEnd(buffer.Caret)
	if lineEnd >= len(buffer.Text) {
		buffer.Caret = len(buffer.Text)
		buffer.settle(selecting)
		return
	}
	nextStart := lineEnd + 1
	buffer.Caret = buffer.placeInLine(nextStart, buffer.LineEnd(nextStart))
	buffer.settle(selecting)
}

// MovePage moves the caret as many lines as the pane shows, and keeps the column it was in.
func (buffer *EditorBuffer) MovePage(step, rows int, selecting bool) {
	if rows < 1 {
		rows = 1
	}
	for at := 0; at < rows; at++ {
		buffer.MoveLine(step, selecting)
	}
}

// PlaceCaret puts the caret at that offset, which a press of the pointer does.
func (buffer *EditorBuffer) PlaceCaret(offset int, selecting bool) {
	buffer.Caret = buffer.snapToRune(core.ClampWithin(offset, len(buffer.Text)))
	buffer.settle(selecting)
	buffer.hasWanted = false
}

// SelectWordAt takes the word that offset stands in, which two presses of the pointer do.
func (buffer *EditorBuffer) SelectWordAt(offset int) {
	text := buffer.Text
	at := buffer.snapToRune(core.ClampWithin(offset, len(text)))
	// Nothing under the pointer takes the word that ends there, so a press past the last
	// word of a line still takes one.
	if at >= len(text) || classifyRune(readRuneAt(text, at)) == blankRune {
		if at > 0 {
			character, width := utf8.DecodeLastRuneInString(text[:at])
			if classifyRune(character) != blankRune {
				at -= width
			}
		}
	}
	if at >= len(text) {
		buffer.Anchor, buffer.Caret = len(text), len(text)
		buffer.hasWanted = false
		return
	}
	group := classifyRune(readRuneAt(text, at))
	start, end := at, at
	for start > 0 {
		character, width := utf8.DecodeLastRuneInString(text[:start])
		if classifyRune(character) != group {
			break
		}
		start -= width
	}
	for end < len(text) {
		character, width := utf8.DecodeRuneInString(text[end:])
		if classifyRune(character) != group {
			break
		}
		end += width
	}
	buffer.Anchor, buffer.Caret = start, end
	buffer.hasWanted = false
}

// SelectLineAt takes the whole line that offset stands in, with the break that ends it.
func (buffer *EditorBuffer) SelectLineAt(offset int) {
	start := buffer.LineStart(offset)
	end := buffer.LineEnd(offset)
	if end < len(buffer.Text) {
		end++
	}
	buffer.Anchor, buffer.Caret = start, end
	buffer.hasWanted = false
}

// FindOffsetAt returns the offset of a line and a column, which a press of the pointer names.
// A column past the end of the line returns its end.
func (buffer *EditorBuffer) FindOffsetAt(line, column int) int {
	start := 0
	for range line {
		broke := strings.IndexByte(buffer.Text[start:], '\n')
		if broke == -1 {
			return len(buffer.Text)
		}
		start += broke + 1
	}
	if column < 1 {
		return start
	}
	end := buffer.LineEnd(start)
	if start+column >= end {
		return end
	}
	return buffer.snapToRune(start + column)
}

// FindWordStart returns where the word before that offset begins.
func (buffer *EditorBuffer) FindWordStart(offset int) int {
	text := buffer.Text
	at := core.ClampWithin(offset, len(text))
	for at > 0 {
		character, width := utf8.DecodeLastRuneInString(text[:at])
		if classifyRune(character) != blankRune {
			break
		}
		at -= width
	}
	if at == 0 {
		return 0
	}
	character, _ := utf8.DecodeLastRuneInString(text[:at])
	group := classifyRune(character)
	for at > 0 {
		character, width := utf8.DecodeLastRuneInString(text[:at])
		if classifyRune(character) != group {
			break
		}
		at -= width
	}
	return at
}

// FindWordEnd returns where the word after that offset ends.
func (buffer *EditorBuffer) FindWordEnd(offset int) int {
	text := buffer.Text
	at := core.ClampWithin(offset, len(text))
	for at < len(text) {
		character, width := utf8.DecodeRuneInString(text[at:])
		if classifyRune(character) != blankRune {
			break
		}
		at += width
	}
	if at >= len(text) {
		return len(text)
	}
	group := classifyRune(readRuneAt(text, at))
	for at < len(text) {
		character, width := utf8.DecodeRuneInString(text[at:])
		if classifyRune(character) != group {
			break
		}
		at += width
	}
	return at
}

// FindMatches returns where the buffer holds this text, read without regard to case.
func (buffer *EditorBuffer) FindMatches(term string) []int {
	if term == "" {
		return nil
	}
	text, wanted := strings.ToLower(buffer.Text), strings.ToLower(term)
	// Lowering a character can change how many bytes it takes, and an offset has to stand
	// for the text as it is, so a text that changes length is searched as it was written.
	if len(text) != len(buffer.Text) || len(wanted) != len(term) {
		text, wanted = buffer.Text, term
	}

	found := []int{}
	for at := 0; at+len(wanted) <= len(text); {
		next := strings.Index(text[at:], wanted)
		if next < 0 {
			break
		}
		found = append(found, at+next)
		at += next + len(wanted)
	}
	return found
}

// SelectRange takes the text between two offsets, which a match of a search does.
func (buffer *EditorBuffer) SelectRange(start, end int) {
	buffer.Anchor = core.ClampWithin(start, len(buffer.Text))
	buffer.Caret = core.ClampWithin(end, len(buffer.Text))
	buffer.hasWanted = false
	buffer.lastEdit = editNone
}

// ReplaceMatches writes the text in place of every match of the term, and returns how many
// it wrote. The whole replace is one step of the undo.
func (buffer *EditorBuffer) ReplaceMatches(term, written string) int {
	found := buffer.FindMatches(term)
	if len(found) == 0 {
		return 0
	}
	buffer.rememberBefore(editWhole)

	built := strings.Builder{}
	built.Grow(len(buffer.Text) + len(found)*(len(written)-len(term)))
	at := 0
	for _, start := range found {
		built.WriteString(buffer.Text[at:start])
		built.WriteString(written)
		at = start + len(term)
	}
	built.WriteString(buffer.Text[at:])

	buffer.Text = built.String()
	buffer.Caret = core.ClampWithin(buffer.Caret, len(buffer.Text))
	buffer.ClearSelection()
	buffer.hasWanted = false
	return len(found)
}

// Undo brings the buffer back to how it stood before the last edit, and reports whether
// there was an edit to take back.
func (buffer *EditorBuffer) Undo() bool {
	if len(buffer.undone) == 0 {
		return false
	}
	step := buffer.undone[len(buffer.undone)-1]
	buffer.undone = buffer.undone[:len(buffer.undone)-1]
	buffer.redone = append(buffer.redone, buffer.readStep())
	buffer.applyStep(step)
	return true
}

// Redo writes back what the last undo took away, and reports whether there was one.
func (buffer *EditorBuffer) Redo() bool {
	if len(buffer.redone) == 0 {
		return false
	}
	step := buffer.redone[len(buffer.redone)-1]
	buffer.redone = buffer.redone[:len(buffer.redone)-1]
	buffer.undone = append(buffer.undone, buffer.readStep())
	buffer.applyStep(step)
	return true
}

// rememberBefore keeps the buffer as it stands, so the edit that follows can be taken back.
// An edit of the same kind as the one before it joins that step, so a run of typing is one
// press of the undo and not one press for every letter.
func (buffer *EditorBuffer) rememberBefore(kind editKind) {
	buffer.redone = nil
	if kind != editWhole && kind == buffer.lastEdit && len(buffer.undone) > 0 {
		return
	}
	buffer.undone = append(buffer.undone, buffer.readStep())
	if len(buffer.undone) > undoDepth {
		buffer.undone = append(buffer.undone[:0], buffer.undone[1:]...)
	}
	buffer.lastEdit = kind
}

func (buffer *EditorBuffer) readStep() editorStep {
	return editorStep{Text: buffer.Text, Caret: buffer.Caret, Anchor: buffer.Anchor}
}

func (buffer *EditorBuffer) applyStep(step editorStep) {
	buffer.Text = step.Text
	buffer.Caret = core.ClampWithin(step.Caret, len(step.Text))
	buffer.Anchor = core.ClampWithin(step.Anchor, len(step.Text))
	buffer.hasWanted = false
	buffer.lastEdit = editNone
}

// snapToRune returns the offset moved back to the first byte of the character it stands in.
func (buffer *EditorBuffer) snapToRune(offset int) int {
	for offset > 0 && offset < len(buffer.Text) && !utf8.RuneStart(buffer.Text[offset]) {
		offset--
	}
	return offset
}

// placeInLine returns where the wanted column falls in a line, or its end.
func (buffer *EditorBuffer) placeInLine(start, end int) int {
	wanted := start + buffer.wantedColumn
	if wanted > end {
		return end
	}
	return buffer.snapToRune(wanted)
}

func (buffer *EditorBuffer) settle(selecting bool) {
	buffer.Caret = core.ClampWithin(buffer.Caret, len(buffer.Text))
	if !selecting {
		buffer.ClearSelection()
	}
	// A move ends the run of edits before it, so what is typed after it is taken back on
	// its own.
	buffer.lastEdit = editNone
}

// Lines returns the buffer split into its lines, which is what the editor draws.
func (buffer *EditorBuffer) Lines() []string {
	return strings.Split(buffer.Text, "\n")
}

// CaretPosition returns the line and the column the caret is in, counted from zero.
func (buffer *EditorBuffer) CaretPosition() (int, int) {
	before := buffer.Text[:core.ClampWithin(buffer.Caret, len(buffer.Text))]
	line := strings.Count(before, "\n")
	return line, buffer.Caret - buffer.LineStart(buffer.Caret)
}

// ReadStatementAtCaret returns the statement the caret stands in.
func (buffer *EditorBuffer) ReadStatementAtCaret(language language.Language) string {
	return language.ReadStatementAtOffset(buffer.Text, buffer.Caret)
}

// IndentAtCaret returns the blank the line under the caret opens with, so a line opened
// after it starts under the same word.
func (buffer *EditorBuffer) IndentAtCaret() string {
	line := buffer.Text[buffer.LineStart(buffer.Caret):buffer.Caret]
	indent := 0
	for indent < len(line) && (line[indent] == ' ' || line[indent] == '\t') {
		indent++
	}
	return line[:indent]
}

// The three groups a character falls in for a move over words: a blank, a character a name
// is made of, and everything else.
const (
	blankRune = iota
	nameRune
	symbolRune
)

// classifyRune returns which of the three groups a character is in.
func classifyRune(character rune) int {
	switch {
	case unicode.IsSpace(character):
		return blankRune
	case character == '_' || unicode.IsLetter(character) || unicode.IsDigit(character):
		return nameRune
	}
	return symbolRune
}

// readRuneAt returns the character that starts at that offset.
func readRuneAt(text string, at int) rune {
	character, _ := utf8.DecodeRuneInString(text[at:])
	return character
}
