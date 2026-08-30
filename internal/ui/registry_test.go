package ui

import (
	"slices"
	"strings"
	"testing"

	"github.com/turanmahmudov/masume/internal/cfg"
)

// The keys the client ships with must not fight each other. Two actions on one chord in one
// pane means one of them cannot be reached at all, and the user has no way to know which.
func TestTheShippedKeysHoldNoConflict(t *testing.T) {
	for _, preset := range ListKeyPresets() {
		t.Run(string(preset.ID), func(t *testing.T) {
			registry := NewKeyRegistry()
			problems := registry.ApplyKeySettings(preset, nil, true)
			if len(problems) != 0 {
				t.Errorf("the %q keys report %d problems:\n  %s",
					preset.ID, len(problems), strings.Join(problems, "\n  "))
			}
		})
	}
}

// Every action the client offers needs a chord in every preset, or the action can only be
// reached from the palette and the hint that names it draws nothing.
func TestEveryPresetBindsEveryAction(t *testing.T) {
	for _, preset := range ListKeyPresets() {
		t.Run(string(preset.ID), func(t *testing.T) {
			registry := NewKeyRegistry()
			registry.ApplyKeySettings(preset, nil, true)

			bound := map[string]bool{}
			for _, binding := range registry.ListActiveBindings() {
				bound[cfg.BuildActionKey(binding.Scope, string(binding.ID))] = true
			}
			for _, action := range ActionCatalog {
				key := cfg.BuildActionKey(action.Scope, string(action.ID))
				if !bound[key] {
					t.Errorf("%q of the %q pane is bound to nothing", action.ID, action.Scope)
				}
			}
		})
	}
}

// A chord the config file names for an action the client has not is a mistake in the file, and
// the user has to be told rather than left wondering why the key does nothing.
func TestApplyKeySettingsReportsAnActionTheClientHasNot(t *testing.T) {
	registry := NewKeyRegistry()
	problems := registry.ApplyKeySettings(FindKeyPreset(""), cfg.ChordChoices{
		cfg.BuildActionKey(cfg.ScopeGlobal, "not-an-action"): {mustSequence(t, "ctrl+j")},
	}, true)

	if len(problems) == 0 {
		t.Fatal("an action the client has not was accepted")
	}
	if !strings.Contains(strings.Join(problems, " "), "not-an-action") {
		t.Errorf("the problems read %v and do not name the action", problems)
	}
}

// A chord the config file gives an action replaces the one it shipped with, so the reader can
// bind what they like.
func TestApplyKeySettingsTakesTheChordOfTheConfigFile(t *testing.T) {
	registry := NewKeyRegistry()
	// A chord nothing else uses, so the change is the only thing under test.
	chosen := mustSequence(t, "ctrl+alt+shift+f9")

	action := firstActionOfScope(t, cfg.ScopeGlobal)
	problems := registry.ApplyKeySettings(FindKeyPreset(""), cfg.ChordChoices{
		cfg.BuildActionKey(cfg.ScopeGlobal, string(action)): {chosen},
	}, true)
	if len(problems) != 0 {
		t.Fatalf("the choice reported %v", problems)
	}

	held := registry.FindActionChords(cfg.ScopeGlobal, action)
	if len(held) != 1 {
		t.Fatalf("the action holds %d chords, wanted the one chosen", len(held))
	}
	if cfg.DescribeSequence(held[0]) != cfg.DescribeSequence(chosen) {
		t.Errorf("the action reads %q, wanted %q",
			cfg.DescribeSequence(held[0]), cfg.DescribeSequence(chosen))
	}
}

// Two actions of one pane on one chord is a conflict, and it has to be reported rather than
// leaving one of them unreachable.
func TestApplyKeySettingsReportsTwoActionsOnOneChord(t *testing.T) {
	registry := NewKeyRegistry()
	chosen := mustSequence(t, "ctrl+alt+shift+f9")

	actions := actionsOfScope(t, cfg.ScopeGlobal, 2)
	problems := registry.ApplyKeySettings(FindKeyPreset(""), cfg.ChordChoices{
		cfg.BuildActionKey(cfg.ScopeGlobal, string(actions[0])): {chosen},
		cfg.BuildActionKey(cfg.ScopeGlobal, string(actions[1])): {chosen},
	}, true)

	if len(problems) == 0 {
		t.Fatal("one chord on two actions of one pane was accepted")
	}
	written := strings.Join(problems, " ")
	if !strings.Contains(written, string(actions[0])) || !strings.Contains(written, string(actions[1])) {
		t.Errorf("the conflict reads %v and does not name both actions", problems)
	}
}

// The chord of an action is written for the hints and the help, and an action with none reads
// as nothing rather than as an empty pair of brackets.
func TestFormatActionChordWritesSomethingForABoundAction(t *testing.T) {
	registry := NewKeyRegistry()
	registry.ApplyKeySettings(FindKeyPreset(""), nil, true)

	action := firstActionOfScope(t, cfg.ScopeGlobal)
	if written := registry.FormatActionChord(cfg.ScopeGlobal, action); written == "" {
		t.Errorf("%q is bound and its chord writes as nothing", action)
	}
	// An action nothing binds writes as nothing, and never as a stray separator.
	if written := registry.FormatActionChord(cfg.ScopeGlobal, ActionID("not-an-action")); written != "" {
		t.Errorf("an action nothing binds writes as %q", written)
	}
}

// mustSequence reads a chord sequence, and stops the test where it cannot.
func mustSequence(t *testing.T, text string) cfg.ChordSequence {
	t.Helper()
	held, is := cfg.ParseChordSequence(text)
	if !is {
		t.Fatalf("%q is not a chord sequence", text)
	}
	return held
}

// firstActionOfScope answers one action of that pane.
func firstActionOfScope(t *testing.T, scope cfg.KeyScope) ActionID {
	t.Helper()
	return actionsOfScope(t, scope, 1)[0]
}

// actionsOfScope answers that many actions of one pane.
func actionsOfScope(t *testing.T, scope cfg.KeyScope, count int) []ActionID {
	t.Helper()
	found := []ActionID{}
	for _, action := range ActionCatalog {
		if action.Scope != scope {
			continue
		}
		found = append(found, action.ID)
		if len(found) == count {
			return found
		}
	}
	t.Fatalf("the client offers fewer than %d actions of the %q pane", count, scope)
	return nil
}

func TestConfirmCardAnswersTheChordsItDraws(t *testing.T) {
	// The card draws "y yes · n no", so both have to be in the group it matches against.
	// It also takes Enter as yes.
	group := FindDialogActions("confirm")
	for _, wanted := range []ActionID{
		ActionAnswerYes, ActionAnswerNo, ActionClose, ActionChooseRow,
	} {
		if !slices.Contains(group, wanted) {
			t.Errorf("the confirm group does not hold %q", wanted)
		}
	}
}

func TestEveryDialogActionTheConfirmCardReadsIsInItsGroup(t *testing.T) {
	// readConfirmKey switches on these. An action outside the group never matches, so the
	// card would draw a key that does nothing.
	group := FindDialogActions("confirm")
	for _, wanted := range []ActionID{ActionAnswerYes, ActionChooseRow, ActionAnswerNo} {
		if !slices.Contains(group, wanted) {
			t.Fatalf("%q is read by the confirm card but is not in its group", wanted)
		}
	}
	if slices.Contains(group, ActionSaveForm) {
		t.Error("the confirm card would swallow the save chord of the form")
	}
}
