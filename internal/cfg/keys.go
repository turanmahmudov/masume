package cfg

import (
	"fmt"
	"maps"
	"slices"
	"strings"

	"github.com/turanmahmudov/masume/internal/core"
)

// KeySettings holds everything under `[keys]`: the preset, and the chords written
// over it.
type KeySettings struct {
	// The key set the app uses, before the chords of this file are applied.
	Preset  PresetID
	Choices ChordChoices
	// The faults in the file, which are reported and not applied.
	Problems []string
}

// DefaultKeySettings holds the keys the app opens with.
func DefaultKeySettings() KeySettings {
	return KeySettings{Preset: PresetDefault, Choices: ChordChoices{}}
}

// presetSetting is the entry of `[keys]` that names the preset, not a scope.
const presetSetting = "preset"

func listPresetNames() string {
	names := make([]string, 0, len(PresetIDs))
	for _, id := range PresetIDs {
		names = append(names, string(id))
	}
	return strings.Join(names, ", ")
}

func listScopeNames() string {
	names := make([]string, 0, len(KeyScopes))
	for _, scope := range KeyScopes {
		names = append(names, string(scope))
	}
	return strings.Join(names, ", ")
}

// readPreset returns the preset the file asks for. An unknown name keeps the default.
func readPreset(keys Table, problems *[]string) PresetID {
	value, present := keys[presetSetting]
	if !present {
		return PresetDefault
	}
	written, isText := value.(string)
	if !isText {
		*problems = append(*problems, "keys.preset must be one of "+listPresetNames())
		return PresetDefault
	}
	found, known := FindPresetID(strings.ToLower(strings.TrimSpace(written)))
	if known {
		return found
	}
	*problems = append(*problems, fmt.Sprintf(
		"keys.preset %q is not a preset; use %s", written, listPresetNames()))
	return PresetDefault
}

// readChords returns the chords of one entry of the table, or reports that it has none.
func readChords(written any) ([]string, bool) {
	switch held := written.(type) {
	case string:
		if strings.TrimSpace(held) == "" {
			return []string{}, true
		}
		return []string{held}, true
	case []any:
		texts := make([]string, 0, len(held))
		for _, entry := range held {
			text, isText := entry.(string)
			if !isText {
				return nil, false
			}
			if strings.TrimSpace(text) != "" {
				texts = append(texts, text)
			}
		}
		return texts, true
	}
	return nil, false
}

// readActionSequences returns the sequences one action is bound to. A line nobody
// can read is reported and left out, and the action keeps its default key.
func readActionSequences(written string, entry any, problems *[]string) ([]ChordSequence, bool) {
	texts, readable := readChords(entry)
	if !readable {
		*problems = append(*problems, written+" must be a chord or a list of chords")
		return nil, false
	}

	sequences := make([]ChordSequence, 0, len(texts))
	for _, text := range texts {
		sequence, parsed := ParseChordSequence(text)
		if !parsed {
			*problems = append(*problems, fmt.Sprintf("%s cannot read the chord %q", written, text))
			continue
		}
		sequences = append(sequences, sequence)
	}

	// A line with no readable chord still meant to bind something, so the action
	// keeps its default key.
	if len(texts) > 0 && len(sequences) == 0 {
		return nil, false
	}

	for _, sequence := range sequences {
		for _, chord := range sequence {
			if reason := FindUndeliverableChord(chord); reason != "" {
				*problems = append(*problems, written+" "+reason)
			}
		}
	}
	return sequences, true
}

// readScopeChoices returns the chords every action of one scope is bound to, keyed
// as the registry names them.
func readScopeChoices(scope KeyScope, table any, problems *[]string) ChordChoices {
	actions, isTable := FindTable(table)
	if !isTable {
		*problems = append(*problems, fmt.Sprintf("keys.%s must be a table of actions", scope))
		return ChordChoices{}
	}

	choices := ChordChoices{}
	for _, id := range sortedKeys(actions) {
		sequences, bound := readActionSequences(
			fmt.Sprintf("keys.%s.%s", scope, id), actions[id], problems)
		if bound {
			choices[BuildActionKey(scope, id)] = sequences
		}
	}
	return choices
}

func findKeyScope(written string) (KeyScope, bool) {
	return core.FindAllowed(KeyScopes, written)
}

// ParseKeySettings reads `[keys]`. A wrong line is reported and left out.
func ParseKeySettings(document Table) KeySettings {
	keys, present := FindSection(document, "keys")
	if !present {
		return DefaultKeySettings()
	}

	problems := []string{}
	choices := ChordChoices{}
	preset := readPreset(keys, &problems)

	names := make([]string, 0, len(keys))
	for name := range keys {
		names = append(names, name)
	}
	slices.Sort(names)

	for _, name := range names {
		// The preset is the only entry of this section that names no scope.
		if name == presetSetting {
			continue
		}
		scope, known := findKeyScope(name)
		if !known {
			problems = append(problems, fmt.Sprintf(
				"keys.%s is not a scope; use %s", name, listScopeNames()))
			continue
		}
		maps.Copy(choices, readScopeChoices(scope, keys[name], &problems))
	}

	return KeySettings{Preset: preset, Choices: choices, Problems: problems}
}
