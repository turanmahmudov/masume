package ui

import (
	"strings"

	tea "charm.land/bubbletea/v2"
	uv "github.com/charmbracelet/ultraviolet"

	"github.com/turanmahmudov/masume/internal/cfg"
)

// Reads a key press as a chord, and returns the action a scope binds it to. A chord of
// several presses is held until the next press decides it.

// keyNames name the key each special code is written as in a chord.
var keyNames = map[rune]string{
	tea.KeyUp: "up", tea.KeyDown: "down", tea.KeyLeft: "left", tea.KeyRight: "right",
	tea.KeyHome: "home", tea.KeyEnd: "end",
	tea.KeyPgUp: "pageup", tea.KeyPgDown: "pagedown",
	tea.KeyInsert: "insert", tea.KeyDelete: "delete", tea.KeyBackspace: "backspace",
	tea.KeyTab: "tab", tea.KeyEnter: "return", tea.KeyEscape: "escape", tea.KeySpace: "space",
	tea.KeyF1: "f1", tea.KeyF2: "f2", tea.KeyF3: "f3", tea.KeyF4: "f4",
	tea.KeyF5: "f5", tea.KeyF6: "f6", tea.KeyF7: "f7", tea.KeyF8: "f8",
	tea.KeyF9: "f9", tea.KeyF10: "f10", tea.KeyF11: "f11", tea.KeyF12: "f12",
}

// ReadChord reads a key press as a chord, the way the config file writes one.
func ReadChord(key tea.Key) cfg.Chord {
	chord := cfg.Chord{
		Ctrl: key.Mod.Contains(uv.ModCtrl),
		Meta: key.Mod.Contains(uv.ModAlt),
	}

	if named, known := keyNames[key.Code]; known {
		chord.Key = named
		// A named key carries its Shift, because the terminal sends the modifier with it.
		chord.Shift = key.Mod.Contains(uv.ModShift)
		return chord
	}

	// A printable character is read as itself. A capital letter carries its own Shift, so
	// `S` and `shift+s` are one chord.
	written := string(key.Code)
	if key.Text != "" {
		written = key.Text
	}
	lowered := strings.ToLower(written)
	chord.Key = lowered
	if written != lowered {
		chord.Shift = true
	}
	// With Ctrl the terminal reports no text, so the code carries the letter.
	if chord.Ctrl && key.Text == "" {
		chord.Key = strings.ToLower(string(key.Code))
	}
	return chord
}

// digitOf returns the digit a chord names, from 1 to 9, and whether it names one.
func digitOf(chord cfg.Chord) (int, bool) {
	if len(chord.Key) != 1 {
		return 0, false
	}
	digit := int(chord.Key[0] - '0')
	if digit < 1 || digit > 9 {
		return 0, false
	}
	return digit, true
}

// matchesChord is true where the press returns the bound chord. A bound `digit` matches any
// digit from 1 to 9.
func matchesChord(bound, pressed cfg.Chord) bool {
	if bound.Ctrl != pressed.Ctrl || bound.Meta != pressed.Meta {
		return false
	}
	// Shift matches only where a chord names it, because a capital letter already carries it.
	if bound.Shift != pressed.Shift {
		return false
	}
	if bound.Key == cfg.DigitKey {
		_, isDigit := digitOf(pressed)
		return isDigit
	}
	return bound.Key == pressed.Key
}

// Keymap holds the presses of a chord not yet finished, and returns the action a press runs.
type Keymap struct {
	registry *KeyRegistry
	// The presses of a sequence read so far, which the next press either finishes or drops.
	pending []cfg.Chord
}

// NewKeymap builds the keymap over a registry of bindings.
func NewKeymap(registry *KeyRegistry) *Keymap {
	return &Keymap{registry: registry}
}

// Pending is true while a chord of several presses is half read.
func (keymap *Keymap) Pending() bool {
	return len(keymap.pending) > 0
}

// Match returns the action this press runs in one of the scopes, in the order they are
// given. The first scope that binds the press wins.
//
// The digit a press named is answered too, because one binding covers the nine of them.
type Match struct {
	Action ActionID
	Scope  cfg.KeyScope
	Digit  int
	// The chord the press read as. One action can be bound to a chord and to its shifted
	// twin, so the handler reads which of the two ran.
	Chord cfg.Chord
	// True while the press opened a chord of several presses and the rest is awaited.
	Waiting bool
}

// Match reads one press against the bindings of these scopes.
func (keymap *Keymap) Match(key tea.Key, scopes ...cfg.KeyScope) (Match, bool) {
	return keymap.matchTaken(key, nil, nil, scopes)
}

// MatchOnly reads one press against the bindings of these scopes, and returns only an action
// the caller names. One scope binds a key to more than one action, such as `n` for both
// `answer-no` and `new-connection`, so the screen on show says which of them it takes.
func (keymap *Keymap) MatchOnly(
	key tea.Key, taken []ActionID, scopes ...cfg.KeyScope,
) (Match, bool) {
	return keymap.matchTaken(key, buildActionSet(taken), nil, scopes)
}

// MatchFirst reads one press against the bindings of these scopes, and returns an action the
// caller names before any other the same chord is bound to. A card that names the actions it
// returns therefore wins a chord the scope binds twice, and a chord it does not answer still
// reaches whatever else the scope binds it to.
func (keymap *Keymap) MatchFirst(
	key tea.Key, taken []ActionID, scopes ...cfg.KeyScope,
) (Match, bool) {
	return keymap.matchTaken(key, nil, buildActionSet(taken), scopes)
}

// MatchExcept reads one press against the bindings of these scopes, and returns every action
// but the ones the caller refuses.
func (keymap *Keymap) MatchExcept(
	key tea.Key, refused []ActionID, scopes ...cfg.KeyScope,
) (Match, bool) {
	if len(refused) == 0 {
		return keymap.matchTaken(key, nil, nil, scopes)
	}
	allowed := make(map[ActionID]bool, len(ActionCatalog))
	for _, action := range ActionCatalog {
		allowed[action.ID] = true
	}
	for _, action := range refused {
		delete(allowed, action)
	}
	return keymap.matchTaken(key, allowed, nil, scopes)
}

// buildActionSet returns the named actions as a set, or nothing where none are named.
func buildActionSet(actions []ActionID) map[ActionID]bool {
	if len(actions) == 0 {
		return nil
	}
	set := make(map[ActionID]bool, len(actions))
	for _, action := range actions {
		set[action] = true
	}
	return set
}

func (keymap *Keymap) matchTaken(
	key tea.Key, allowed, first map[ActionID]bool, scopes []cfg.KeyScope,
) (Match, bool) {
	pressed := ReadChord(key)
	sequence := append(append([]cfg.Chord{}, keymap.pending...), pressed)

	waiting := false
	for _, scope := range scopes {
		found, chosen := ActionID(""), ActionID("")
		for _, binding := range keymap.registry.bindings {
			if binding.Scope != scope {
				continue
			}
			action, known := FindActionID(binding.ID)
			if !known {
				continue
			}
			if allowed != nil && !allowed[action] {
				continue
			}
			if len(binding.Chords) < len(sequence) {
				continue
			}
			if !matchesSequence(binding.Chords[:len(sequence)], sequence) {
				continue
			}
			if len(binding.Chords) > len(sequence) {
				// A longer binding starts here, so the press is held for the rest of it.
				waiting = true
				continue
			}
			if found == "" {
				found = action
			}
			if first[action] && chosen == "" {
				chosen = action
			}
		}
		if chosen == "" {
			chosen = found
		}
		if chosen != "" {
			keymap.pending = nil
			digit, _ := digitOf(pressed)
			return Match{Action: chosen, Scope: scope, Digit: digit, Chord: pressed}, true
		}
	}

	if waiting {
		keymap.pending = sequence
		return Match{Waiting: true}, false
	}
	// A press the group has no command for is handed back and acts as itself.
	keymap.pending = nil
	return Match{}, false
}

func matchesSequence(bound, pressed []cfg.Chord) bool {
	for at, chord := range pressed {
		if !matchesChord(bound[at], chord) {
			return false
		}
	}
	return true
}

// Reset drops a chord half read, which Escape does.
func (keymap *Keymap) Reset() {
	keymap.pending = nil
}
