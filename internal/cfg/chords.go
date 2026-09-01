package cfg

import (
	"regexp"
	"strings"

	"github.com/turanmahmudov/masume/internal/core"
)

// KeyScope says where a key works, which the focus decides. `grid`, `plan` and
// `document` never draw together, so each may use the keys of the others. `editor`
// is the statement being written. `list` is any list moved by keys that is not a
// pane. `dialog` owns the keyboard while it is open, so it may share the keys of a
// pane.
type KeyScope string

// The eight scopes a chord can be bound in.
const (
	ScopeGlobal KeyScope = "global"
	ScopeGrid   KeyScope = "grid"
	ScopePlan   KeyScope = "plan"
	// ScopeDocument is the tree that opens the rows of a result as documents. It is
	// its own scope and not the scope of the object tree: the two hold different
	// rows and answer different keys, and a reader configures each one on its own.
	ScopeDocument KeyScope = "document"
	ScopeTree     KeyScope = "tree"
	ScopeEditor   KeyScope = "editor"
	ScopeList     KeyScope = "list"
	ScopeDialog   KeyScope = "dialog"
)

// KeyScopes lists the scopes a config file may name.
var KeyScopes = []KeyScope{
	ScopeGlobal, ScopeGrid, ScopePlan, ScopeDocument, ScopeTree, ScopeEditor,
	ScopeList, ScopeDialog,
}

// PresetID names one key preset.
type PresetID string

// PresetDefault is the only preset the app ships.
const PresetDefault PresetID = "default"

// PresetIDs lists the presets the app offers.
var PresetIDs = []PresetID{PresetDefault}

// FindPresetID reads this text as a preset name.
func FindPresetID(written string) (PresetID, bool) {
	return core.FindAllowed(PresetIDs, written)
}

// DigitKey is a chord that matches any digit, so the number chooses a tab or a view.
const DigitKey = "digit"

// Chord is one key press. Ctrl and Meta match exactly. Shift matches only where a
// chord names it.
type Chord struct {
	Key   string
	Ctrl  bool
	Meta  bool
	Shift bool
}

// ChordSequence is one or more presses in a row. The action runs on the last one.
type ChordSequence []Chord

// ChordChoices holds the chords chosen in the config file, keyed by action.
type ChordChoices map[string][]ChordSequence

// DescribeChord writes a chord as the config file and a message do: `ctrl+shift+p`.
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

// DescribeSequence writes a sequence as `space t c`.
func DescribeSequence(sequence ChordSequence) string {
	written := make([]string, 0, len(sequence))
	for _, chord := range sequence {
		written = append(written, DescribeChord(chord))
	}
	return strings.Join(written, " ")
}

// BuildActionKey names an action in the `[keys]` table and in the map of choices.
func BuildActionKey(scope KeyScope, id string) string {
	return string(scope) + ":" + id
}

// SplitActionKey reads an action key back into its scope and its id.
func SplitActionKey(actionKey string) (KeyScope, string) {
	before, after, ok := strings.Cut(actionKey, ":")
	if !ok {
		return ScopeGlobal, actionKey
	}
	return KeyScope(before), after
}

// modifiers names the modifier of each name a person can write. A terminal sends
// Alt as Meta.
var modifiers = map[string]string{
	"ctrl": "ctrl", "control": "ctrl",
	"alt": "meta", "meta": "meta", "option": "meta",
	"shift": "shift",
}

// keyAliases names the key this app uses, for each name a person can write.
var keyAliases = map[string]string{
	"enter": "return", "cr": "return",
	"esc":  "escape",
	"pgup": "pageup", "pgdn": "pagedown", "pagedn": "pagedown",
	"del": "delete", "ins": "insert", "spc": "space",
}

// namedKeys are the keys that have a name instead of a character.
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

// ParseChord reads a chord as a person writes it. A single capital letter includes
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

	// The name an old terminal gives Ctrl+J. Both spellings give one chord.
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

// ParseChordSequence reads a binding as a person writes it: one chord, or several
// separated by spaces. It reads nothing if any part fails, because half a sequence
// would reach the wrong action.
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

// ambiguousChords names the key an old terminal sends instead of a chord. Only the
// kitty protocol reports the chord. Ctrl+J is handled apart, because it arrives as
// a line feed.
var ambiguousChords = map[string]string{
	"h": "Backspace", "i": "Tab", "m": "Enter", "[": "Escape",
}

const needsProtocol = "unless the terminal speaks the kitty keyboard protocol"

// carriesOneCode names the keys Ctrl folds into one code. An arrow, function or
// page key is sent as a sequence that names its modifiers, so those survive.
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

// FindUndeliverableChord returns why an ordinary terminal cannot send this chord,
// or an empty reason if it can.
func FindUndeliverableChord(chord Chord) string {
	if !chord.Ctrl {
		return ""
	}

	// A digit with Ctrl has no code, so an ordinary terminal sends nothing.
	if chord.Key == DigitKey || digitOnly.MatchString(chord.Key) {
		return "is sent by no terminal but one that speaks the kitty keyboard protocol"
	}

	// Ctrl has one code per character, and Shift changes none of them, so
	// Ctrl+Shift+P arrives as Ctrl+P.
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
