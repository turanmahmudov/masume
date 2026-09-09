package ui

import (
	"strings"
	"testing"

	"github.com/turanmahmudov/masume/internal/cfg"
)

// focusFormField puts the caret on the field of that key.
func focusFormField(t *testing.T, form *FormState, key string) {
	t.Helper()
	for at, field := range form.Shown() {
		if field.Key == key {
			form.Cursor = at
			form.openField()
			return
		}
	}
	t.Fatalf("the form shows no %s field", key)
}

// The five password sources read alike in the form, so the card names what the chosen one
// does while the caret is on it.
func TestTheFormNamesThePasswordSourceUnderTheCaret(t *testing.T) {
	useNoKeyring(t)
	model := buildOfflineModel(t, 100, 40)
	model.form = NewFormState(buildPromptingProfile("shop"), true, nil)
	model.screen = ScreenEditingConnection

	for _, mode := range cfg.AuthModes {
		model.form.Fields = cfg.ApplyFieldChange(model.form.Fields, "auth", string(mode))
		focusFormField(t, model.form, "auth")

		wanted := cfg.DescribeAuthMode(string(mode))
		if wanted == "" {
			t.Fatalf("%q has no hint", mode)
		}
		if hint := model.describeFormHint(); hint != wanted {
			t.Errorf("%q reads %q, wanted %q", mode, hint, wanted)
		}
		if drawn := stripEscapes(model.renderForm()); !strings.Contains(drawn, wanted) {
			t.Errorf("the card of %q does not name the source:\n%s", mode, drawn)
		}
	}
}

// The hint of the password source takes the row the paste hint had, so a form on another
// field still says how a URL fills it.
func TestTheFormKeepsThePasteHintOnEveryOtherField(t *testing.T) {
	useNoKeyring(t)
	model := buildOfflineModel(t, 100, 40)
	model.form = NewFormState(buildPromptingProfile("shop"), true, nil)
	model.screen = ScreenEditingConnection
	focusFormField(t, model.form, "host")

	if hint := model.describeFormHint(); !strings.Contains(hint, "paste a postgres://") {
		t.Errorf("the hint reads %q, wanted the paste hint", hint)
	}
}
