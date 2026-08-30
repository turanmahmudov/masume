package ui

import (
	"image/color"
	"strings"

	"github.com/turanmahmudov/masume/internal/cfg"
	"github.com/turanmahmudov/masume/internal/present"
)

// keyPart is one part of the line a card names its keys on: the key, what it does, and what
// a press on the words runs. A part with no action behind it is a word the card says.
type keyPart struct {
	// The glyph of what the key acts on, drawn before the chord, or nothing for a key that
	// needs none.
	icon   cfg.IconKind
	chord  string
	label  string
	scope  cfg.KeyScope
	action ActionID
	// second is what the other half of a key of a pair runs, such as the step on beside
	// the step back.
	second ActionID
}

// buildText writes the part as the reader sees it: the key, the glyph of what it acts on, then
// what it does.
func (part keyPart) buildText(icons IconSet) string {
	written := []string{}
	if part.chord != "" {
		written = append(written, part.chord)
	}
	if glyph := icons.Icon(part.icon); part.icon != "" && glyph != "" {
		written = append(written, glyph)
	}
	if part.label != "" {
		written = append(written, part.label)
	}
	return strings.Join(written, " ")
}

// KeyLine is the keys a card names at its foot. It writes them as one line, and keeps what
// each one runs, so a press on the word runs what the key runs.
type KeyLine struct {
	registry *KeyRegistry
	icons    IconSet
	parts    []keyPart
}

// sayKeys starts a line of keys for a card of this model.
func (model *Model) sayKeys() *KeyLine {
	return &KeyLine{registry: model.registry, icons: model.icons}
}

// bind adds the key an action is bound to now, with its label after it. An action with no
// chord is left out, as the status bar leaves it out.
func (line *KeyLine) bind(scope cfg.KeyScope, action ActionID, label string) *KeyLine {
	chord := line.registry.FormatActionChords(scope, action)
	if chord == "" {
		return line
	}
	return line.run(chord, label, scope, action)
}

// bindIcon adds the key an action is bound to now, with the glyph of what it acts on before
// it. A key that names what a model is asked carries the mark of the model.
func (line *KeyLine) bindIcon(
	scope cfg.KeyScope, action ActionID, icon cfg.IconKind, label string,
) *KeyLine {
	chord := line.registry.FormatActionChordCompact(scope, action)
	if chord == "" {
		return line
	}
	line.parts = append(line.parts, keyPart{
		icon: icon, chord: chord, label: label, scope: scope, action: action,
	})
	return line
}

// bindCompact adds a key drawn in the short form the strips of a pane use.
func (line *KeyLine) bindCompact(scope cfg.KeyScope, action ActionID, label string) *KeyLine {
	chord := line.registry.FormatActionChordCompact(scope, action)
	if chord == "" {
		return line
	}
	return line.run(chord, label, scope, action)
}

// bindPair adds one key for a pair of actions, such as the step back and the step on. A press
// on the first half runs the first action.
func (line *KeyLine) bindPair(
	scope cfg.KeyScope, previous, next ActionID, label, separator string,
) *KeyLine {
	chord := line.registry.FormatChordPair(scope, previous, next, separator)
	if chord == "" {
		return line
	}
	line.parts = append(line.parts, keyPart{
		chord: chord, label: label, scope: scope, action: previous, second: next,
	})
	return line
}

// run adds a key drawn as this chord and this label, which runs this action.
func (line *KeyLine) run(chord, label string, scope cfg.KeyScope, action ActionID) *KeyLine {
	if chord == "" && label == "" {
		return line
	}
	line.parts = append(line.parts, keyPart{
		chord: chord, label: label, scope: scope, action: action,
	})
	return line
}

// name adds a key the field or the list returns itself, drawn as every other key is: the
// chord in the ink of a key and what it does in the quiet ink. The registry cannot move such
// a key, so no action stands behind it and no press runs it.
func (line *KeyLine) name(chord, label string) *KeyLine {
	return line.run(chord, label, "", "")
}

// say adds a word the card says, which no press runs.
func (line *KeyLine) say(text string) *KeyLine {
	if text == "" {
		return line
	}
	line.parts = append(line.parts, keyPart{label: text})
	return line
}

// buildText writes the line as one string, with the parts a middle dot apart.
func (line *KeyLine) buildText() string {
	if line == nil {
		return ""
	}
	written := make([]string, 0, len(line.parts))
	for _, part := range line.parts {
		written = append(written, part.buildText(line.icons))
	}
	return strings.Join(written, hintSeparator)
}

// isEmpty is true for a line that names nothing.
func (line *KeyLine) isEmpty() bool {
	return line == nil || len(line.parts) == 0
}

// buildHints returns the line as the keys of the status bar, so the bar under an open card
// names what the card returns rather than what the pane behind it returns.
func (line *KeyLine) buildHints() []Hint {
	if line == nil {
		return nil
	}
	hints := make([]Hint, 0, len(line.parts))
	for _, part := range line.parts {
		if part.chord == "" {
			continue
		}
		hints = append(hints, Hint{
			Key: part.chord, Label: part.label, Scope: part.scope, Action: part.action,
		})
	}
	return hints
}

// appendCardKeyRow puts the row of keys at the foot of a card, under a blank row that holds it
// off the content. The row is appended before it is drawn, because the key line records where
// each key landed and that needs the row it was drawn on.
func (model *Model) appendCardKeyRow(
	lines []string, keys *KeyLine, said string, top, left int,
) []string {
	lines = append(lines, "", "")
	lines[len(lines)-1] = model.renderKeyLine(keys, []string{said},
		top+len(lines)-1, left, model.styles.Theme.Panel)[0]
	return lines
}

// renderKeyLine draws the wrapped lines of a key line and keeps the cells each key covers, so
// a press on the word runs what the key runs and the key reads as one everywhere it is drawn:
// the chord in the ink of a key, the glyph in the colour of what it stands for, and what it
// does in the quiet ink.
//
// The parts are drawn in the order they were named, so each one is looked for after the one
// before it. A part the wrap broke over two rows is drawn quietly and left without a hit box,
// because half a key is not the key.
func (model *Model) renderKeyLine(
	line *KeyLine, wrapped []string, top, left int, ground color.Color,
) []string {
	if line == nil || len(wrapped) == 0 {
		return wrapped
	}
	theme := model.styles.Theme
	drawn := make([]string, len(wrapped))
	written := make([]strings.Builder, len(wrapped))
	row, at := 0, 0

	for _, part := range line.parts {
		text := part.buildText(line.icons)
		found := -1
		for row < len(wrapped) {
			if held := strings.Index(wrapped[row][at:], text); held >= 0 {
				found = at + held
				break
			}
			writeTextOn(&written[row], theme.Faint, ground, wrapped[row][at:])
			row, at = row+1, 0
		}
		if found < 0 {
			break
		}
		writeTextOn(&written[row], theme.Faint, ground, wrapped[row][at:found])
		from := left + present.MeasureText(wrapped[row][:found])
		model.writeKeyPart(&written[row], part, ground)
		if part.action != "" {
			model.recordKeyPart(line, part, top+row, from, present.MeasureText(text))
		}
		at = found + len(text)
	}

	for ; row < len(wrapped); row++ {
		writeTextOn(&written[row], theme.Faint, ground, wrapped[row][at:])
		at = 0
	}
	for index := range wrapped {
		drawn[index] = written[index].String()
	}
	return drawn
}

// writeKeyPart draws one key: the chord, the glyph of what it acts on, and what it does.
func (model *Model) writeKeyPart(
	written *strings.Builder, part keyPart, ground color.Color,
) {
	theme := model.styles.Theme
	wrote := false
	if part.chord != "" {
		writeTextOn(written, theme.Accent, ground, part.chord)
		wrote = true
	}
	if glyph := model.icons.Icon(part.icon); part.icon != "" && glyph != "" {
		writeTextOn(written, model.styles.IconColor(part.icon), ground, blankBefore(wrote)+glyph)
		wrote = true
	}
	if part.label != "" {
		writeTextOn(written, theme.Muted, ground, blankBefore(wrote)+part.label)
	}
}

// blankBefore returns the blank that holds one part of a key apart from the one before it.
func blankBefore(wrote bool) string {
	if wrote {
		return " "
	}
	return ""
}

// measureKeyLine returns how many cells the line takes when it is drawn, so a strip that
// holds it against its right end knows where it starts.
func measureKeyLine(line *KeyLine) int {
	return present.MeasureText(line.buildText())
}

// writeKeyLine draws a line of keys on a strip and records the cells each key covers, so a
// press on the word runs what the key runs. The line is drawn from the column given, on the
// row given, both counted from the screen.
func (model *Model) writeKeyLine(
	line *KeyLine, ground color.Color, row, left int,
) string {
	if line.isEmpty() {
		return ""
	}
	var written strings.Builder
	at := left
	for index, part := range line.parts {
		if index > 0 {
			writeTextOn(&written, model.styles.Theme.Faint, ground, hintSeparator)
			at += present.MeasureText(hintSeparator)
		}
		width := present.MeasureText(part.buildText(line.icons))
		model.writeKeyPart(&written, part, ground)
		if part.action != "" {
			model.recordKeyPart(line, part, row, at, width)
		}
		at += width
	}
	return written.String()
}

// recordKeyPart keeps the cells one key of a line covers. A key of a pair holds both actions,
// and the half of the chord that was pressed decides which one runs.
func (model *Model) recordKeyPart(line *KeyLine, part keyPart, row, from, width int) {
	if width < 1 {
		return
	}
	model.layout.buttons = append(model.layout.buttons, buttonHit{
		row: row, from: from, to: from + width - 1,
		keyTo:  from + max(measureKeyWidth(part), 1) - 1,
		scope:  part.scope,
		action: part.action,
		second: part.second,
	})
}

// measureKeyWidth returns the cells the key itself covers, before the glyph and what it does.
// A press on the first half of a key of a pair runs the first action, and the half is measured
// from the key and not from what it says.
func measureKeyWidth(part keyPart) int {
	return present.MeasureText(part.chord)
}

// rememberCardKeys keeps the keys the card on show names, so the status bar under it names
// the same ones and a press on either runs the card and not the pane behind it.
func (model *Model) rememberCardKeys(line *KeyLine) {
	if line.isEmpty() {
		return
	}
	model.cardKeys = line
}
