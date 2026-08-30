package ui

import (
	"testing"

	"github.com/turanmahmudov/masume/internal/cfg"
)

func TestThePickerStepsRoundTheEndsOfTheList(t *testing.T) {
	picker := pickerState{cursor: 2}
	picker.step(1, 3)
	if picker.cursor != 0 {
		t.Errorf("a step past the last row left the cursor on %d", picker.cursor)
	}
	picker.step(-1, 3)
	if picker.cursor != 2 {
		t.Errorf("a step before the first row left the cursor on %d", picker.cursor)
	}
}

func TestThePickerPagesStopAtTheEnds(t *testing.T) {
	picker := pickerState{cursor: 1}
	picker.page(10, 3)
	if picker.cursor != 2 {
		t.Errorf("a page past the last row left the cursor on %d", picker.cursor)
	}
	picker.page(-10, 3)
	if picker.cursor != 0 {
		t.Errorf("a page before the first row left the cursor on %d", picker.cursor)
	}
}

func TestThePickerStaysPutOnAnEmptyList(t *testing.T) {
	picker := pickerState{cursor: 4}
	picker.step(1, 0)
	if picker.cursor != 0 {
		t.Errorf("a step on no rows left the cursor on %d", picker.cursor)
	}
	picker.page(10, 0)
	if picker.cursor != 0 {
		t.Errorf("a page on no rows left the cursor on %d", picker.cursor)
	}
	picker.focus(3, 0)
	if picker.cursor != 0 {
		t.Errorf("a focus on no rows left the cursor on %d", picker.cursor)
	}
	if _, found := picker.pick(nil); found {
		t.Error("an empty list picked a profile")
	}
}

func TestThePickerPicksTheProfileTheCursorStandsOn(t *testing.T) {
	profiles := []cfg.Profile{{Name: "alpha"}, {Name: "beta"}}
	picker := pickerState{cursor: 1}
	held, found := picker.pick(profiles)
	if !found || held.Name != "beta" {
		t.Errorf("the cursor on 1 picked %+v, found=%v", held, found)
	}
	picker.cursor = 2
	if _, found := picker.pick(profiles); found {
		t.Error("a cursor off the list picked a profile")
	}
}

func TestThePickerAsksForAPasswordOnAFreshField(t *testing.T) {
	picker := pickerState{}
	picker.askPassword(cfg.Profile{Name: "first"})
	picker.password.SetText("secret")
	picker.askPassword(cfg.Profile{Name: "second"})
	if picker.pending.Name != "second" {
		t.Errorf("the pending profile is %q", picker.pending.Name)
	}
	if picker.password == nil || picker.password.Text != "" {
		t.Error("the password of the first profile was left in the field")
	}
	if !picker.waitsFor("second") || picker.waitsFor("first") {
		t.Error("the picker waits for a profile it is not opening")
	}
}

func TestThePickerKeysMoveTheCursor(t *testing.T) {
	model := NewModel(loadedConfigForTest("tokyonight"), nil, nil, nil)
	model.screen = ScreenPickingProfile
	model.profiles = []cfg.Profile{{Name: "alpha"}, {Name: "beta"}, {Name: "gamma"}}

	model.runPickerAction(Match{Action: ActionCursorDown})
	if model.picker.cursor != 1 {
		t.Errorf("down left the cursor on %d", model.picker.cursor)
	}
	model.runPickerAction(Match{Action: ActionCursorLastRow})
	if model.picker.cursor != 2 {
		t.Errorf("end left the cursor on %d", model.picker.cursor)
	}
	model.runPickerAction(Match{Action: ActionCursorFirstRow})
	if model.picker.cursor != 0 {
		t.Errorf("home left the cursor on %d", model.picker.cursor)
	}
	model.runPickerAction(Match{Action: ActionCursorPageDown})
	if model.picker.cursor != 2 {
		t.Errorf("page down left the cursor on %d", model.picker.cursor)
	}
}

func TestChooseProfileAsksForAPasswordTheClientCannotFind(t *testing.T) {
	model := NewModel(loadedConfigForTest("tokyonight"), nil, nil, nil)
	profile := cfg.Profile{
		Name: "shop", Engine: "postgres", User: "ada", Auth: cfg.AuthPrompt,
	}

	model.chooseProfile(profile)
	if model.screen != ScreenPromptingPassword {
		t.Errorf("the screen is %q, wanted the password prompt", model.screen)
	}
	if !model.picker.waitsFor("shop") {
		t.Error("the picker does not wait for the profile it asked a password for")
	}
	if model.picker.password == nil || model.picker.password.Text != "" {
		t.Error("the password field was not opened empty")
	}
}
