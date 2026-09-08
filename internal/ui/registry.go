package ui

import (
	"fmt"
	"maps"
	"regexp"
	"slices"
	"strings"
	"unicode"

	"github.com/turanmahmudov/masume/internal/cfg"
)

// The key registry combines a preset with configured key bindings.

// ActionBinding is one chord sequence bound to one action in one scope.
type ActionBinding struct {
	// Plain text, because the config file can name an action this app does not have.
	ID     string
	Scope  cfg.KeyScope
	Chords cfg.ChordSequence
}

// KeyRegistry holds the bindings now applied.
type KeyRegistry struct {
	preset   cfg.PresetID
	bindings []ActionBinding
}

// readChordChoices reads the chords of a preset through the parser a user file goes through,
// so both mean the same.
func readChordChoices(written map[string][]string) (cfg.ChordChoices, []string) {
	choices := cfg.ChordChoices{}
	problems := []string{}

	keys := make([]string, 0, len(written))
	for actionKey := range written {
		keys = append(keys, actionKey)
	}
	slices.Sort(keys)

	for _, actionKey := range keys {
		sequences := make([]cfg.ChordSequence, 0, len(written[actionKey]))
		for _, text := range written[actionKey] {
			sequence, parsed := cfg.ParseChordSequence(text)
			if !parsed {
				problems = append(problems, fmt.Sprintf(
					"%s has an invalid preset chord: %q", actionKey, text))
				continue
			}
			sequences = append(sequences, sequence)
		}
		choices[actionKey] = sequences
	}
	return choices, problems
}

// buildChosenBindings returns only the chords given, and nothing else.
func buildChosenBindings(choices cfg.ChordChoices) []ActionBinding {
	keys := make([]string, 0, len(choices))
	for actionKey := range choices {
		keys = append(keys, actionKey)
	}
	slices.Sort(keys)

	bindings := []ActionBinding{}
	for _, actionKey := range keys {
		scope, id := cfg.SplitActionKey(actionKey)
		for _, chords := range choices[actionKey] {
			bindings = append(bindings, ActionBinding{ID: id, Scope: scope, Chords: chords})
		}
	}
	return bindings
}

// NewKeyRegistry applies the keys of the app, before the config file is read.
func NewKeyRegistry() *KeyRegistry {
	choices, _ := readChordChoices(DefaultPreset.Chords)
	return &KeyRegistry{preset: DefaultPreset.ID, bindings: buildChosenBindings(choices)}
}

// ListActiveBindings returns the bindings now applied.
func (registry *KeyRegistry) ListActiveBindings() []ActionBinding {
	return registry.bindings
}

// ListActionKeys returns every action a key can be chosen for.
func ListActionKeys() []string {
	keys := make([]string, 0, len(DefaultPreset.Chords))
	for actionKey := range DefaultPreset.Chords {
		keys = append(keys, actionKey)
	}
	slices.Sort(keys)
	return keys
}

// findUnknownActions returns every choice that names an action this app does not have.
func keepKnownActions(choices cfg.ChordChoices) cfg.ChordChoices {
	known := map[string]bool{}
	for _, actionKey := range ListActionKeys() {
		known[actionKey] = true
	}
	kept := make(cfg.ChordChoices, len(choices))
	for actionKey, chords := range choices {
		if known[actionKey] {
			kept[actionKey] = chords
		}
	}
	return kept
}

func findUnknownActions(choices cfg.ChordChoices) []string {
	known := map[string]bool{}
	for _, actionKey := range ListActionKeys() {
		known[actionKey] = true
	}

	unknown := []string{}
	for actionKey := range choices {
		if known[actionKey] {
			continue
		}
		scope, id := cfg.SplitActionKey(actionKey)
		unknown = append(unknown, fmt.Sprintf(
			"keys.%s.%s is not an action in this scope", scope, id))
	}
	slices.Sort(unknown)
	return unknown
}

// ApplyKeySettings applies a preset with the chords of the config file over it, and returns
// the faults in those choices.
func (registry *KeyRegistry) ApplyKeySettings(
	preset KeyPreset, choices cfg.ChordChoices, offersAi bool,
) []string {
	base, problems := readChordChoices(preset.Chords)
	// An unknown action is reported below and never dispatched, so its chords stay out of
	// the bindings and out of the conflict check.
	maps.Copy(base, keepKnownActions(choices))
	registry.preset = preset.ID
	registry.bindings = buildChosenBindings(base)
	if !offersAi {
		registry.bindings = dropAiBindings(registry.bindings)
	}

	problems = append(problems, findUnknownActions(choices)...)
	return append(problems, registry.FindChordConflicts()...)
}

// dropAiBindings leaves out every binding of an AI action, so no chord reaches one and no
// hint or help line can find a chord to draw.
func dropAiBindings(bindings []ActionBinding) []ActionBinding {
	kept := make([]ActionBinding, 0, len(bindings))
	for _, binding := range bindings {
		if IsAiAction(ActionID(binding.ID)) {
			continue
		}
		kept = append(kept, binding)
	}
	return kept
}

// FindActionChords returns every chord of this action, in the order they were written.
func (registry *KeyRegistry) FindActionChords(
	scope cfg.KeyScope, id ActionID,
) []cfg.ChordSequence {
	found := []cfg.ChordSequence{}
	for _, binding := range registry.bindings {
		if binding.Scope == scope && binding.ID == string(id) {
			found = append(found, binding.Chords)
		}
	}
	return found
}

// listRooms returns which group a binding shares with another: one scope, or one dialog.
func listRooms(binding ActionBinding) []string {
	if binding.Scope != cfg.ScopeDialog {
		return []string{string(binding.Scope)}
	}
	rooms := []string{}
	for _, dialog := range ListDialogNames() {
		for _, id := range FindDialogActions(dialog) {
			if string(id) == binding.ID {
				rooms = append(rooms, "dialog:"+dialog)
				break
			}
		}
	}
	return rooms
}

// FindChordConflicts returns every chord bound twice where only one can run: two actions in
// one scope, or a pane that takes a chord of the workspace. A dialog owns the keyboard while
// it is open, so it may share a chord with the workspace.
func (registry *KeyRegistry) FindChordConflicts() []string {
	seen := map[string]string{}
	globals := map[string]string{}
	conflicts := []string{}

	for _, binding := range registry.bindings {
		chord := cfg.DescribeSequence(binding.Chords)
		for _, room := range listRooms(binding) {
			taken, held := seen[room+":"+chord]
			if held && taken != binding.ID {
				conflicts = append(conflicts, fmt.Sprintf(
					"%s in %s: %s and %s", chord, room, taken, binding.ID))
				continue
			}
			seen[room+":"+chord] = binding.ID
		}
		if binding.Scope == cfg.ScopeGlobal {
			globals[chord] = binding.ID
		}
	}

	for _, binding := range registry.bindings {
		if binding.Scope == cfg.ScopeGlobal || binding.Scope == cfg.ScopeDialog {
			continue
		}
		chord := cfg.DescribeSequence(binding.Chords)
		shadowed, held := globals[chord]
		if !held {
			continue
		}
		conflicts = append(conflicts, fmt.Sprintf(
			"%s in %s: %s hides %s", chord, binding.Scope, binding.ID, shadowed))
	}

	return append(conflicts, registry.findWaitingChords()...)
}

// findWaitingChords returns a chord that starts a longer binding of the same group. The
// engine holds it for the rest of the sequence, and a key that pauses looks broken.
func (registry *KeyRegistry) findWaitingChords() []string {
	waiting := []string{}

	for _, binding := range registry.bindings {
		if len(binding.Chords) != 1 {
			continue
		}
		opening := cfg.DescribeSequence(binding.Chords)
		rooms := map[string]bool{}
		for _, room := range listRooms(binding) {
			rooms[room] = true
		}

		for _, other := range registry.bindings {
			if len(other.Chords) < 2 {
				continue
			}
			if cfg.DescribeSequence(other.Chords[:1]) != opening {
				continue
			}
			shared := false
			for _, room := range listRooms(other) {
				if rooms[room] {
					shared = true
					break
				}
			}
			if !shared {
				continue
			}
			waiting = append(waiting, fmt.Sprintf(
				"%s in %s: %s waits for %s", opening, binding.Scope, binding.ID, other.ID))
		}
	}
	return waiting
}

// keyGlyphs is how a key is drawn on screen, where its event name differs. One key is drawn
// one way everywhere: a glyph where a common one exists, and a short word where none does.
var keyGlyphs = map[string]string{
	"up": "↑", "down": "↓", "left": "←", "right": "→",
	"pageup": "PgUp", "pagedown": "PgDn", "home": "Home", "end": "End",
	"return": "↵", "escape": "Esc", "space": "␣", "tab": "⇥",
	"insert": "Ins", "delete": "Del", "backspace": "⌫",
}

// buildKeyGlyph returns the key of a chord as the screen draws it.
func buildKeyGlyph(key string) string {
	if key == cfg.DigitKey {
		return "1 … 9"
	}
	if named, known := keyGlyphs[key]; known {
		return named
	}
	if functionKey.MatchString(key) {
		return strings.ToUpper(key)
	}
	return key
}

// formatOneChord writes one chord: ^ for Ctrl, ⇧ for Shift, Alt+ for Alt, and the glyph of
// the key. One chord carries one glyph, so a chord with a modifier spells its key: ↵ alone,
// ⇧ Enter with Shift. A letter is written as a capital where a modifier stands before it.
func formatOneChord(chord cfg.Chord) string {
	modified := chord.Ctrl || chord.Meta || chord.Shift
	if !modified {
		return buildKeyGlyph(chord.Key)
	}

	key := buildKeyGlyph(chord.Key)
	if spelled, known := spelledKeys[chord.Key]; known {
		key = spelled
	}
	runes := []rune(key)
	if len(runes) == 1 && unicode.IsLetter(runes[0]) {
		key = strings.ToUpper(key)
	}

	written := ""
	if chord.Ctrl {
		written += "^"
	}
	if chord.Shift {
		written += "⇧"
	}
	if chord.Meta {
		written += "Alt+"
	}
	// A key spelled as a word stands a blank apart from the marks before it, so ⇧ Tab reads
	// as one key and not as two.
	if len([]rune(key)) > 1 && !strings.HasSuffix(written, "+") {
		written += " "
	}
	return written + key
}

// spelledKeys is how a key is written out in full, for the help. The interface draws a glyph
// where one exists; the help spells the key.
var spelledKeys = map[string]string{
	"up": "Up", "down": "Down", "left": "Left", "right": "Right",
	"pageup": "PgUp", "pagedown": "PgDn", "home": "Home", "end": "End",
	"return": "Enter", "escape": "Esc", "space": "Space", "tab": "Tab",
	"insert": "Ins", "delete": "Del", "backspace": "Backspace",
}

// formatOneChordName writes one chord in full: Ctrl+Shift+F3, Alt+Enter, Tab. A letter is
// written as a capital where a modifier stands before it.
func formatOneChordName(chord cfg.Chord) string {
	parts := []string{}
	if chord.Ctrl {
		parts = append(parts, "Ctrl")
	}
	if chord.Meta {
		parts = append(parts, "Alt")
	}
	if chord.Shift {
		parts = append(parts, "Shift")
	}
	key := chord.Key
	switch {
	case key == cfg.DigitKey:
		key = "1 … 9"
	case spelledKeys[key] != "":
		key = spelledKeys[key]
	case functionKey.MatchString(key):
		key = strings.ToUpper(key)
	case len(parts) > 0 && len([]rune(key)) == 1:
		key = strings.ToUpper(key)
	}
	return strings.Join(append(parts, key), "+")
}

// FormatChordName writes a sequence in full, for the help: every press in order, a space
// apart.
func FormatChordName(sequence cfg.ChordSequence) string {
	written := make([]string, 0, len(sequence))
	for _, chord := range sequence {
		written = append(written, formatOneChordName(chord))
	}
	return strings.Join(written, " ")
}

// FormatFirstActionChordName writes the first chord of an action in full, for a row of the
// help, the palette or a menu. A row has room for one chord.
func (registry *KeyRegistry) FormatFirstActionChordName(
	scope cfg.KeyScope, id ActionID,
) string {
	chords := registry.FindActionChords(scope, id)
	if len(chords) == 0 {
		return ""
	}
	return FormatChordName(chords[0])
}

// FormatChord writes a binding as the interface draws it: every press in order, separated by
// a space.
func FormatChord(sequence cfg.ChordSequence) string {
	written := make([]string, 0, len(sequence))
	for _, chord := range sequence {
		written = append(written, formatOneChord(chord))
	}
	return strings.Join(written, " ")
}

// FormatActionChords writes every chord of an action. Two chords with the same label are
// drawn once, as Ctrl+J is bound twice.
func (registry *KeyRegistry) FormatActionChords(scope cfg.KeyScope, id ActionID) string {
	seen := map[string]bool{}
	written := []string{}
	for _, sequence := range registry.FindActionChords(scope, id) {
		label := FormatChord(sequence)
		if seen[label] {
			continue
		}
		seen[label] = true
		written = append(written, label)
	}
	return strings.Join(written, " / ")
}

// FormatFirstActionChord writes the first chord of an action. A row of a list and a strip
// have room for one chord; only a card and the help write every chord of an action.
func (registry *KeyRegistry) FormatFirstActionChord(
	scope cfg.KeyScope, id ActionID,
) string {
	chords := registry.FindActionChords(scope, id)
	if len(chords) == 0 {
		return ""
	}
	return FormatChord(chords[0])
}

// FormatChordPair joins the first chord of each action of a previous and next pair, for a
// strip that steps through tabs, statements, views or connections. An action without a chord
// is left out.
func (registry *KeyRegistry) FormatChordPair(
	scope cfg.KeyScope, previous, next ActionID, separator string,
) string {
	written := []string{}
	for _, id := range []ActionID{previous, next} {
		chords := registry.FindActionChords(scope, id)
		if len(chords) == 0 {
			continue
		}
		written = append(written, FormatChord(chords[0]))
	}
	return strings.Join(written, separator)
}

// functionKey matches a key named `f1` to `f24`.
var functionKey = regexp.MustCompile(`^f([1-9]|1[0-9]|2[0-4])$`)
