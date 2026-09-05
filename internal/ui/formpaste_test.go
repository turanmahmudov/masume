package ui

import (
	"testing"

	"github.com/turanmahmudov/masume/internal/cfg"
)

// The card tells the reader to paste a connection string into the host field, so the paste
// fills the form rather than joining what the field already holds.
func TestPastingAConnectionURLFillsTheForm(t *testing.T) {
	model := buildOfflineModel(t, 120, 40)
	model.screen = ScreenEditingConnection
	model.form = NewFormState(cfg.Profile{}, false, nil)
	for at, field := range model.form.Shown() {
		if field.Key == "host" {
			model.form.Cursor = at
		}
	}
	model.form.openField()

	model.pasteIntoForm("postgres://alice@db.example.com:6543/shop?sslmode=require")

	for _, expected := range [][2]string{
		{"host", "db.example.com"}, {"port", "6543"}, {"database", "shop"},
		{"user", "alice"}, {"sslMode", "require"}, {"name", "shop"},
	} {
		if held := cfg.ReadField(model.form.Fields, expected[0]); held != expected[1] {
			t.Errorf("%s reads %q, wanted %q", expected[0], held, expected[1])
		}
	}
}

// A paste that is no connection string is written where the caret stands.
func TestPastingPlainTextWritesItIntoTheField(t *testing.T) {
	model := buildOfflineModel(t, 120, 40)
	model.screen = ScreenEditingConnection
	model.form = NewFormState(cfg.Profile{}, false, nil)
	for at, field := range model.form.Shown() {
		if field.Key == "database" {
			model.form.Cursor = at
		}
	}
	model.form.openField()

	model.pasteIntoForm("shop")

	if held := cfg.ReadField(model.form.Fields, "database"); held != "shop" {
		t.Errorf("the database reads %q, wanted the pasted text", held)
	}
}
