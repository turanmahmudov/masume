package ui

import (
	"fmt"
	"maps"
	"regexp"
	"slices"
	"strings"

	"github.com/turanmahmudov/masume/internal/cfg"
)

// The applied key bindings: the chosen preset with the chords of the config file over it. A
// key that only moves the cursor is not an action, and the list on screen reads it from the
// registry too.

// ActionBinding is one chord sequence bound to one action in one scope.
type ActionBinding struct {
	// Plain text, because the config file can name an action this app does not have.
	ID     string
	Scope  cfg.KeyScope
	Chords cfg.ChordSequence
}

// dialogGroups say which actions each card and each screen of the dialog scope returns. A
// card is named after its overlay, and the two screens after themselves. The matcher reads it,
// because the scope binds one chord to more than one action, such as `ctrl+l` to both
// `set-null` and `new-ai-chat`, and the card on show says which of them it takes. The conflict
// check reads it too: two actions of one card that share a chord clash, and two actions of
// different cards do not.
var dialogGroups = map[string][]ActionID{
	"confirm": {ActionClose, ActionAnswerYes, ActionAnswerNo, ActionChooseRow},
	"picker": {
		ActionClose, ActionNewConnection, ActionEditConnection, ActionDeleteConnection,
	},
	"form": {ActionClose, ActionSaveForm, ActionTestConnection},
	"cell-edit": {
		ActionClose, ActionSaveCell, ActionPrettifyJSON,
		ActionSetNull, ActionSetEmpty, ActionSetDefault,
	},
	"parameters": {ActionClose, ActionRunWithValues, ActionPrettifyJSON},
	// The find field turns into the replace field, so finding and replacing is one key.
	"prompt":       {ActionClose, ActionReplaceInStatement},
	"export":       {ActionClose, ActionWriteExport},
	"cell":         {ActionClose, ActionCopyValue},
	"history":      {ActionClose, ActionOpenInNewTab},
	"saved":        {ActionClose, ActionOpenInNewTab, ActionListSecondary},
	"ai-chats":     {ActionClose, ActionListSecondary},
	"changes":      {ActionClose, ActionApplyChanges, ActionDiscardChanges},
	"value-filter": {ActionClose, ActionToggleValue, ActionKeepAllValues, ActionKeepOnlyValue},
	"ai-chat": {
		ActionClose, ActionInsertAiSQL, ActionStopAiReply, ActionNewAiChat,
		ActionShowAiChats, ActionScrollBack, ActionScrollForward,
		ActionPreviousTurn, ActionNextTurn,
		// The chat asks its own question before it runs a statement.
		ActionAnswerYes, ActionAnswerNo,
	},
}

// FindDialogActions returns the actions the card or the screen of this name returns.
func FindDialogActions(name string) []ActionID {
	return dialogGroups[name]
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
					"%s ships the unreadable chord %q", actionKey, text))
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
			"keys.%s.%s is not an action of this scope", scope, id))
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
	maps.Copy(base, choices)
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
	names := make([]string, 0, len(dialogGroups))
	for dialog := range dialogGroups {
		names = append(names, dialog)
	}
	slices.Sort(names)
	for _, dialog := range names {
		for _, id := range dialogGroups[dialog] {
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

// keyLabels say how a key is written on screen, where its event name differs.
var keyLabels = map[string]string{
	"up": "↑", "down": "↓", "left": "←", "right": "→",
	"pageup": "PgUp", "pagedown": "PgDn", "home": "Home", "end": "End",
	"return": "Enter", "escape": "Esc", "space": "Space", "tab": "Tab",
	"insert": "Ins", "delete": "Del", "backspace": "Backspace",
}

func buildKeyLabel(key string) string {
	if key == cfg.DigitKey {
		return "1 … 9"
	}
	if named, known := keyLabels[key]; known {
		return named
	}
	if functionKey.MatchString(key) {
		return strings.ToUpper(key)
	}
	return key
}

// formatOneChord writes a chord as the help writes it. A letter without a modifier stays
// lower case.
func formatOneChord(chord cfg.Chord) string {
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
	key := buildKeyLabel(chord.Key)
	if len(parts) > 0 && len([]rune(key)) == 1 {
		key = strings.ToUpper(key)
	}
	return strings.Join(append(parts, key), "+")
}

// compactKeys say how a key is drawn where the width is short.
var compactKeys = map[string]string{
	"up": "↑", "down": "↓", "left": "←", "right": "→",
	"return": "↵", "escape": "Esc", "pageup": "PgUp", "pagedown": "PgDn",
	"space": "␣", "tab": "Tab",
}

// formatOneChordCompact writes the shortest readable form, for the one line of the bottom
// bar.
func formatOneChordCompact(chord cfg.Chord) string {
	// Two modifiers are harder to read short: `^Alt+H` is worse than `Ctrl+Alt+H`.
	if chord.Ctrl && chord.Meta {
		return formatOneChord(chord)
	}
	if chord.Key == cfg.DigitKey {
		return formatOneChord(chord)
	}

	base := chord.Key
	if named, known := compactKeys[chord.Key]; known {
		base = named
	} else if functionKey.MatchString(chord.Key) {
		base = strings.ToUpper(chord.Key)
	}

	// A letter with a modifier is written as a capital. Shift alone is that capital. With
	// another modifier it needs a mark, or Alt+Shift+W reads as Alt+W.
	modified := chord.Ctrl || chord.Meta || chord.Shift
	key := base
	if len([]rune(base)) == 1 && modified {
		key = strings.ToUpper(base)
	}
	shift := ""
	if chord.Shift && (chord.Ctrl || chord.Meta) {
		shift = "⇧"
	}
	marked := shift + key
	if chord.Shift && len([]rune(key)) > 1 {
		marked = "⇧" + key
	}

	written := ""
	if chord.Ctrl {
		written += "^"
	}
	if chord.Meta {
		written += "Alt+"
	}
	return written + marked
}

// FormatChord writes a binding as the help writes it: every press in order, separated by a
// space.
func FormatChord(sequence cfg.ChordSequence) string {
	written := make([]string, 0, len(sequence))
	for _, chord := range sequence {
		written = append(written, formatOneChord(chord))
	}
	return strings.Join(written, " ")
}

// FormatChordCompact writes the shortest form of a sequence, for the one line of the bar.
//
// One key must not be written two ways on one screen, so the form depends on what draws it.
// A one-row strip uses the compact form. A card, a list or a sentence uses the full form.
func FormatChordCompact(sequence cfg.ChordSequence) string {
	written := make([]string, 0, len(sequence))
	for _, chord := range sequence {
		written = append(written, formatOneChordCompact(chord))
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
	return strings.Join(written, "  ")
}

// FormatActionChord writes the first chord of an action, in full, for a column of a list.
// Only the help names both chords of an action, because a row has room for one.
func (registry *KeyRegistry) FormatActionChord(scope cfg.KeyScope, id ActionID) string {
	chords := registry.FindActionChords(scope, id)
	if len(chords) == 0 {
		return ""
	}
	return FormatChord(chords[0])
}

// FormatActionChordCompact writes the compact form of the first chord of an action, for a
// strip.
func (registry *KeyRegistry) FormatActionChordCompact(
	scope cfg.KeyScope, id ActionID,
) string {
	chords := registry.FindActionChords(scope, id)
	if len(chords) == 0 {
		return ""
	}
	return FormatChordCompact(chords[0])
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
		written = append(written, FormatChordCompact(chords[0]))
	}
	return strings.Join(written, separator)
}

// functionKey matches a key named `f1` to `f24`.
var functionKey = regexp.MustCompile(`^f([1-9]|1[0-9]|2[0-4])$`)
