package cfg

import (
	"regexp"
	"strings"

	"github.com/turanmahmudov/masume/internal/core"
)

// KeyScope is the area a key works in. The focus selects the scope. `grid`, `plan` and
// `document` are never displayed together, so each one can use the keys of the others.
// `editor` is the statement under edit. `list` is any list that moves with keys and is not
// a pane. `dialog` takes all key presses while it is open, so it can use the same keys as a
// pane.
type KeyScope string

// The eight scopes a chord can be bound to.
const (
	ScopeGlobal KeyScope = "global"
	ScopeGrid   KeyScope = "grid"
	ScopePlan   KeyScope = "plan"
	// ScopeDocument is the tree that shows the rows of a result as documents. It is a
	// separate scope from the object tree: the two contain different rows and use
	// different keys, and the user configures each one separately.
	ScopeDocument KeyScope = "document"
	ScopeTree     KeyScope = "tree"
	ScopeEditor   KeyScope = "editor"
	ScopeList     KeyScope = "list"
	ScopeDialog   KeyScope = "dialog"
)

// KeyScopes lists the scopes a config file can use.
var KeyScopes = []KeyScope{
	ScopeGlobal, ScopeGrid, ScopePlan, ScopeDocument, ScopeTree, ScopeEditor,
	ScopeList, ScopeDialog,
}

// PresetID is the name of one key preset.
type PresetID string

// PresetDefault is the only preset included in the app.
const PresetDefault PresetID = "default"

// PresetIDs lists the available presets.
var PresetIDs = []PresetID{PresetDefault}

// FindPresetID parses the text as a preset name.
func FindPresetID(written string) (PresetID, bool) {
	return core.FindAllowed(PresetIDs, written)
}

// DigitKey is a chord that matches any digit, so the digit selects a tab or a view.
const DigitKey = "digit"

// Chord is one key press. Ctrl and Meta must match exactly. Shift must match only if the
// chord sets it.
type Chord struct {
	Key   string
	Ctrl  bool
	Meta  bool
	Shift bool
}

// ChordSequence is one or more key presses in sequence. The action runs on the last one.
type ChordSequence []Chord

// ChordChoices holds the chords set in the config file, keyed by action.
type ChordChoices map[string][]ChordSequence

// DescribeChord returns a chord in the form used by the config file and by a message:
// `ctrl+shift+p`.
func DescribeChord(chord Chord) string {
	parts := make([]string, 0, 4)
	if chord.Ctrl {
		parts = append(parts, "ctrl")
	}
	if chord.Meta {
		parts = append(parts, "meta")
	}
	if chord.Shift {
		parts = append(parts, "shift")
	}
	return strings.Join(append(parts, chord.Key), "+")
}

// DescribeSequence returns a sequence in the form `space t c`.
func DescribeSequence(sequence ChordSequence) string {
	written := make([]string, 0, len(sequence))
	for _, chord := range sequence {
		written = append(written, DescribeChord(chord))
	}
	return strings.Join(written, " ")
}

// BuildActionKey returns the key of an action in the `[keys]` table and in the choices
// map.
func BuildActionKey(scope KeyScope, id string) string {
	return string(scope) + ":" + id
}

// SplitActionKey splits an action key back into its scope and its id.
func SplitActionKey(actionKey string) (KeyScope, string) {
	before, after, ok := strings.Cut(actionKey, ":")
	if !ok {
		return ScopeGlobal, actionKey
	}
	return KeyScope(before), after
}

// modifiers give the modifier for each name the user can write. A terminal sends Alt as
// Meta.
var modifiers = map[string]string{
	"ctrl": "ctrl", "control": "ctrl",
	"alt": "meta", "meta": "meta", "option": "meta",
	"shift": "shift",
}

// keyAliases give the internal key name for each name the user can write.
var keyAliases = map[string]string{
	"enter": "return", "cr": "return",
	"esc":  "escape",
	"pgup": "pageup", "pgdn": "pagedown", "pagedn": "pagedown",
	"del": "delete", "ins": "insert", "spc": "space",
}

// namedKeys are the keys that have a name and not a character.
var namedKeys = map[string]bool{
	"up": true, "down": true, "left": true, "right": true,
	"home": true, "end": true, "pageup": true, "pagedown": true,
	"insert": true, "delete": true, "backspace": true,
	"tab": true, "return": true, "escape": true, "space": true,
}

var functionKey = regexp.MustCompile(`^f([1-9]|1[0-9]|2[0-4])$`)

func isKnownKey(key string) bool {
	if len([]rune(key)) == 1 || key == DigitKey || namedKeys[key] {
		return true
	}
	return functionKey.MatchString(key)
}

// ParseChord parses a chord in the form the user writes. A single capital letter includes
// Shift.
func ParseChord(text string) (Chord, bool) {
	parts := []string{}
	for part := range strings.SplitSeq(strings.TrimSpace(text), "+") {
		if part != "" {
			parts = append(parts, part)
		}
	}
	if len(parts) == 0 {
		return Chord{}, false
	}

	written := parts[len(parts)-1]
	chord := Chord{}
	for _, part := range parts[:len(parts)-1] {
		modifier, known := modifiers[strings.ToLower(strings.TrimSpace(part))]
		if !known {
			return Chord{}, false
		}
		switch modifier {
		case "ctrl":
			chord.Ctrl = true
		case "meta":
			chord.Meta = true
		case "shift":
			chord.Shift = true
		}
	}

	key := strings.TrimSpace(written)
	if len([]rune(key)) == 1 && key != strings.ToLower(key) {
		chord.Shift = true
	}
	lowered := strings.ToLower(key)

	// The name an old terminal uses for Ctrl+J. Both spellings give the same chord.
	if lowered == "linefeed" {
		chord.Key = "j"
		chord.Ctrl = true
		return chord, true
	}

	if alias, known := keyAliases[lowered]; known {
		chord.Key = alias
	} else {
		chord.Key = lowered
	}
	return chord, isKnownKey(chord.Key)
}

// ParseChordSequence parses a binding in the form the user writes: one chord, or several
// chords separated by spaces. It fails if any part fails, because a partial sequence would
// run the wrong action.
func ParseChordSequence(text string) (ChordSequence, bool) {
	written := strings.Fields(text)
	if len(written) == 0 {
		return nil, false
	}
	sequence := make(ChordSequence, 0, len(written))
	for _, part := range written {
		chord, readable := ParseChord(part)
		if !readable {
			return nil, false
		}
		sequence = append(sequence, chord)
	}
	return sequence, true
}

// ambiguousChords give the key an old terminal sends in place of a chord. Only the kitty
// protocol reports the chord itself. Ctrl+J is handled separately, because it arrives as a
// line feed.
var ambiguousChords = map[string]string{
	"h": "Backspace", "i": "Tab", "m": "Enter", "[": "Escape",
}

const needsProtocol = "unless the terminal speaks the kitty keyboard protocol"

// carriesOneCode are the keys that Ctrl reduces to one code. An arrow, function or page key
// is sent as a sequence that includes the modifiers, so the modifiers are not lost.
var carriesOneCode = func() map[string]bool {
	held := map[string]bool{}
	for key := range strings.SplitSeq("abcdefghijklmnopqrstuvwxyz0123456789", "") {
		held[key] = true
	}
	for _, key := range []string{"[", "]", "\\", "-", "/", " ", "return", "tab", "escape",
		"backspace", "space"} {
		held[key] = true
	}
	return held
}()

var digitOnly = regexp.MustCompile(`^[0-9]$`)

// FindUndeliverableChord returns the reason a standard terminal cannot send this chord, or
// an empty string if it can.
func FindUndeliverableChord(chord Chord) string {
	if !chord.Ctrl {
		return ""
	}

	// A digit with Ctrl has no code, so a standard terminal sends nothing.
	if chord.Key == DigitKey || digitOnly.MatchString(chord.Key) {
		return "is sent by no terminal but one that speaks the kitty keyboard protocol"
	}

	// Ctrl has one code per character and Shift does not change it, so Ctrl+Shift+P
	// arrives as Ctrl+P.
	if chord.Shift && carriesOneCode[chord.Key] {
		return "cannot be told apart from Ctrl+" + strings.ToUpper(chord.Key) + " " + needsProtocol
	}

	// Enter is a carriage return, and Ctrl does not change it.
	if chord.Key == "return" {
		return "arrives as Enter " + needsProtocol
	}

	sent, ambiguous := ambiguousChords[chord.Key]
	if !ambiguous {
		return ""
	}
	return "cannot be told apart from " + sent + " " + needsProtocol
}
