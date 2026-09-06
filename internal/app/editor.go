package app

import (
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/turanmahmudov/masume/internal/core"
	"github.com/turanmahmudov/masume/internal/query/language"
)

// EditorBuffer is the editable statement and caret state. Offsets count bytes.
type EditorBuffer struct {
	Text  string
	Caret int
	// The other end of a selection. The selection is empty while it equals the caret.
	Anchor int
	// The column the caret keeps while it moves up and down over short lines.
	wantedColumn int
	hasWanted    bool
	// The steps of the undo stack and the steps of the redo stack.
	undone []editorStep
	redone []editorStep
	// The kind of the last edit, so one undo removes a group of typed characters.
	lastEdit editKind
	// The class of the last typed character, so one undo removes one word.
	lastGroup int
}

// editorStep is the state of the buffer before one edit.
type editorStep struct {
	Text   string
	Caret  int
	Anchor int
}

// editKind groups the edits that form one step of the undo.
type editKind int

const (
	// editNone is the state after a move, which ends the group of edits before it.
	editNone editKind = iota
	// editTyping is one character inserted at the caret.
	editTyping
	// editDeleting is one character deleted at the caret.
	editDeleting
	// editWhole is a separate edit, for example a paste or a format.
	editWhole
)

// undoDepth is the number of steps the undo stack holds.
const undoDepth = 500

// NewEditorBuffer returns a buffer with the text and the caret of a restored tab.
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

// HasSelection is true while the caret and the anchor are at different offsets.
func (buffer *EditorBuffer) HasSelection() bool {
	start, end := buffer.SelectionRange()
	return end > start
}

// ClearSelection removes the selection. Every normal edit calls it.
func (buffer *EditorBuffer) ClearSelection() {
	buffer.Anchor = buffer.Caret
}

// SelectAll selects the whole buffer. The editor binds Ctrl+A to it.
func (buffer *EditorBuffer) SelectAll() {
	buffer.Anchor = 0
	buffer.Caret = len(buffer.Text)
	buffer.hasWanted = false
}

// SetText replaces the buffer and puts the caret at the end.
func (buffer *EditorBuffer) SetText(text string) {
	buffer.SetTextWithCaret(text, len(text))
}

// SetTextWithCaret replaces the buffer, sets the caret, and clears the selection.
func (buffer *EditorBuffer) SetTextWithCaret(text string, caret int) {
	buffer.rememberBefore(editWhole)
	buffer.Text = text
	buffer.Caret = core.ClampWithin(caret, len(text))
	buffer.ClearSelection()
	buffer.hasWanted = false
}

// Insert writes the text at the caret, and replaces the selection if there is one.
func (buffer *EditorBuffer) Insert(written string) {
	kind := editTyping
	switch {
	case buffer.HasSelection(), len([]rune(written)) != 1,
		strings.ContainsAny(written, "\n\t"):
		kind = editWhole
	default:
		// Whitespace after a word starts a new undo group.
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

// SelectedLineRange returns the selected line range, or the caret line when no selection exists.
func (buffer *EditorBuffer) SelectedLineRange() (int, int) {
	start, end := buffer.SelectionRange()
	// Exclude a final line whose start is the selection end.
	if end > start && end == buffer.LineStart(end) {
		end--
	}
	return buffer.LineStart(start), buffer.LineEnd(end)
}

// CommentLines toggles comments on selected nonblank lines and reports whether the buffer changed.
func (buffer *EditorBuffer) CommentLines(mark string) bool {
	if mark == "" {
		return false
	}
	start, end := buffer.SelectedLineRange()
	lines := strings.Split(buffer.Text[start:end], "\n")

	// Remove comments only when every nonblank line is commented.
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

	// Align comment marks at the smallest indentation.
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

// dropCommentMark removes the mark and one blank after it from the start of the line.
func dropCommentMark(line, mark string) string {
	indent := len(line) - len(strings.TrimLeft(line, " \t"))
	rest := strings.TrimPrefix(line[indent:], mark)
	rest = strings.TrimPrefix(rest, " ")
	return line[:indent] + rest
}

// IndentLines moves every line of the selection one step to the right.
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

// OutdentLines removes up to width leading whitespace characters from each selected line and reports changes.
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

// replaceLines replaces a line range and selects the replacement.
func (buffer *EditorBuffer) replaceLines(start, end int, written string) {
	buffer.rememberBefore(editWhole)
	buffer.Text = buffer.Text[:start] + written + buffer.Text[end:]
	buffer.Anchor, buffer.Caret = start, start+len(written)
	buffer.hasWanted = false
	buffer.lastEdit = editWhole
}

// MoveCaret moves one character or extends the selection. Without selection mode, an existing selection collapses in the movement direction.
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

// MoveWord moves the caret over one word, and extends the selection if `selecting` is true.
func (buffer *EditorBuffer) MoveWord(step int, selecting bool) {
	if step < 0 {
		buffer.Caret = buffer.FindWordStart(buffer.Caret)
	} else if step > 0 {
		buffer.Caret = buffer.FindWordEnd(buffer.Caret)
	}
	buffer.settle(selecting)
	buffer.hasWanted = false
}

// LineStart returns the start of the line that contains that offset.
func (buffer *EditorBuffer) LineStart(offset int) int {
	broke := strings.LastIndexByte(buffer.Text[:core.ClampWithin(offset, len(buffer.Text))], '\n')
	return broke + 1
}

// LineEnd returns the end of the line that contains that offset.
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

// MoveToLineStart toggles the caret between the first nonblank character and the line start.
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

// MoveLine moves the caret one line up or down and keeps its column over a short line.
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

// MovePage moves the caret by the number of lines of the pane and keeps its column.
func (buffer *EditorBuffer) MovePage(step, rows int, selecting bool) {
	if rows < 1 {
		rows = 1
	}
	for at := 0; at < rows; at++ {
		buffer.MoveLine(step, selecting)
	}
}

// PlaceCaret puts the caret at that offset. A mouse click calls it.
func (buffer *EditorBuffer) PlaceCaret(offset int, selecting bool) {
	buffer.Caret = buffer.snapToRune(core.ClampWithin(offset, len(buffer.Text)))
	buffer.settle(selecting)
	buffer.hasWanted = false
}

// SelectWordAt selects the word at that offset. A double click calls it.
func (buffer *EditorBuffer) SelectWordAt(offset int) {
	text := buffer.Text
	at := buffer.snapToRune(core.ClampWithin(offset, len(text)))
	// At whitespace or EOF, select an adjacent preceding word when present.
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

// SelectLineAt selects the whole line at that offset, with its line break.
func (buffer *EditorBuffer) SelectLineAt(offset int) {
	start := buffer.LineStart(offset)
	end := buffer.LineEnd(offset)
	if end < len(buffer.Text) {
		end++
	}
	buffer.Anchor, buffer.Caret = start, end
	buffer.hasWanted = false
}

// FindOffsetAt converts a line and byte column to an offset, capped at the line end.
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

// FindWordStart returns the start of the word before that offset.
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

// FindWordEnd returns the end of the word after that offset.
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

// FindMatches returns case-insensitive match offsets unless lowercasing changes byte lengths.
func (buffer *EditorBuffer) FindMatches(term string) []int {
	if term == "" {
		return nil
	}
	text, wanted := strings.ToLower(buffer.Text), strings.ToLower(term)
	// Unicode lowercasing can change byte lengths. Use exact matching in that case.
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

// SelectRange selects the text between two offsets. A search match calls it.
func (buffer *EditorBuffer) SelectRange(start, end int) {
	buffer.Anchor = core.ClampWithin(start, len(buffer.Text))
	buffer.Caret = core.ClampWithin(end, len(buffer.Text))
	buffer.hasWanted = false
	buffer.lastEdit = editNone
}

// ReplaceMatches replaces all matches in one undo step and returns the replacement count.
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

// Undo restores the previous buffer state and reports success.
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

// Redo restores what the last undo removed and reports whether there was an undo.
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

// rememberBefore records an undo snapshot unless the edit continues the current group.
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

// snapToRune moves the offset back to the first byte of its character.
func (buffer *EditorBuffer) snapToRune(offset int) int {
	for offset > 0 && offset < len(buffer.Text) && !utf8.RuneStart(buffer.Text[offset]) {
		offset--
	}
	return offset
}

// placeInLine returns the offset of that column in the line, or the end of the line.
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
	// Caret movement ends the current undo group.
	buffer.lastEdit = editNone
}

// Lines returns the buffer split into lines, which is what the editor draws.
func (buffer *EditorBuffer) Lines() []string {
	return strings.Split(buffer.Text, "\n")
}

// CaretPosition returns the line and the column of the caret, counted from zero.
func (buffer *EditorBuffer) CaretPosition() (int, int) {
	before := buffer.Text[:core.ClampWithin(buffer.Caret, len(buffer.Text))]
	line := strings.Count(before, "\n")
	return line, buffer.Caret - buffer.LineStart(buffer.Caret)
}

// ReadStatementAtCaret returns the statement at the caret.
func (buffer *EditorBuffer) ReadStatementAtCaret(language language.Language) string {
	return language.ReadStatementAtOffset(buffer.Text, buffer.Caret)
}

// IndentAtCaret returns the current line indentation before the caret.
func (buffer *EditorBuffer) IndentAtCaret() string {
	line := buffer.Text[buffer.LineStart(buffer.Caret):buffer.Caret]
	indent := 0
	for indent < len(line) && (line[indent] == ' ' || line[indent] == '\t') {
		indent++
	}
	return line[:indent]
}

// Character classes for word movement.
const (
	blankRune = iota
	nameRune
	symbolRune
)

// classifyRune returns the class of a character.
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
