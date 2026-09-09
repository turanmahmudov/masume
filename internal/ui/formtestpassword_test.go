package ui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/turanmahmudov/masume/internal/cfg"
)

// The user is the only source of a prompt password, so a test of the form asks for one as
// opening a connection does. A test that reads no password reaches the server without one
// and reports a refusal the connection itself never hits.
func TestATestOfTheFormAsksForThePasswordOnlyTheUserHas(t *testing.T) {
	useNoKeyring(t)
	model := buildOfflineModel(t, 160, 48)
	model.form = NewFormState(buildPromptingProfile("shop"), true, nil)
	model.screen = ScreenEditingConnection

	held, command, ran := model.runFormAction(Match{Action: ActionTestConnection})
	model = held.(*Model)
	if !ran {
		t.Fatal("the form did not take the test action")
	}
	if command != nil {
		t.Fatal("the test reached the server before the password was typed")
	}
	if model.screen != ScreenPromptingPassword {
		t.Fatalf("the screen is %q, wanted the password prompt", model.screen)
	}
	if !model.picker.testsForm {
		t.Error("the prompt opens the connection instead of testing the form")
	}
	drawn := stripEscapes(model.renderPassword())
	if !strings.Contains(drawn, "testing shop") {
		t.Errorf("the card does not name the test:\n%s", drawn)
	}
	if !strings.Contains(drawn, "test") || strings.Contains(drawn, "connect") {
		t.Errorf("the card offers to connect instead of to test:\n%s", drawn)
	}

	model.picker.password.Insert("hunter2")
	next, command := model.readPasswordKey(tea.Key{Code: tea.KeyEnter})
	model = next.(*Model)
	if command == nil {
		t.Fatal("the typed password ran no test")
	}
	if model.screen != ScreenEditingConnection {
		t.Fatalf("the screen is %q, wanted the form", model.screen)
	}
	if model.form.Test != TestRunning {
		t.Errorf("the form reports %q, wanted a running test", model.form.Test)
	}
	if model.picker.testsForm {
		t.Error("the prompt still tests the form after the test started")
	}
}

// A password card the user closes returns to the form it was opened from, not to the list of
// connections, so the values they typed are still on the screen.
func TestClosingThePasswordCardOfATestReturnsToTheForm(t *testing.T) {
	useNoKeyring(t)
	model := buildOfflineModel(t, 160, 48)
	model.form = NewFormState(buildPromptingProfile("shop"), true, nil)
	model.screen = ScreenEditingConnection
	model.runFormAction(Match{Action: ActionTestConnection})

	next, _ := model.readPasswordKey(tea.Key{Code: tea.KeyEscape})
	model = next.(*Model)
	if model.screen != ScreenEditingConnection {
		t.Fatalf("the screen is %q, wanted the form", model.screen)
	}
	if model.picker.testsForm {
		t.Error("the closed prompt still tests the form")
	}
}

// A test keeps no password, so the card that asks for one does not offer the keyring.
func TestThePasswordCardOfATestKeepsNoPassword(t *testing.T) {
	useMockKeyring(t)
	model := buildOfflineModel(t, 160, 48)
	model.picker.askPasswordForFormTest(buildPromptingProfile("shop"))

	if model.picker.offersKeyring() {
		t.Error("the card offers to keep the password of a test")
	}
	if drawn := stripEscapes(model.renderPassword()); strings.Contains(drawn, "keyring") {
		t.Errorf("the card names the keyring:\n%s", drawn)
	}
}

// A profile whose password the client can read is tested without a prompt, so a test of a
// password from the environment reaches the server at once.
func TestATestOfTheFormReadsAPasswordTheClientHolds(t *testing.T) {
	useNoKeyring(t)
	model := buildOfflineModel(t, 160, 48)
	profile := buildPromptingProfile("shop")
	profile.Auth = cfg.AuthPassword
	profile.PasswordEnv = "MASUME_TEST_FORM_PASSWORD"
	t.Setenv(profile.PasswordEnv, "hunter2")
	model.form = NewFormState(profile, true, nil)
	model.screen = ScreenEditingConnection

	held, command, _ := model.runFormAction(Match{Action: ActionTestConnection})
	model = held.(*Model)
	if command == nil {
		t.Fatal("the test ran nothing")
	}
	if model.screen != ScreenEditingConnection {
		t.Fatalf("the screen is %q, wanted the form", model.screen)
	}
}
