package ui

import (
	"github.com/turanmahmudov/masume/internal/app"
	"github.com/turanmahmudov/masume/internal/cfg"
	"github.com/turanmahmudov/masume/internal/secret"
)

type pickerState struct {
	// The row of the picker the cursor is on, and why the last attempt failed.
	cursor  int
	problem string
	// The profile a password is being typed for, or a connection is being opened on.
	pending cfg.Profile
	// The field a password is typed into.
	password *app.EditorBuffer
	// True where the user asked to keep the typed password in the keyring of the
	// operating system.
	keepInKeyring bool
}

func (picker *pickerState) step(by, count int) {
	picker.cursor = wrap(picker.cursor+by, count)
}

func (picker *pickerState) page(by, count int) {
	picker.cursor = clamp(picker.cursor+by, count)
}

func (picker *pickerState) focus(index, count int) {
	picker.cursor = clamp(index, count)
}

func (picker *pickerState) pick(profiles []cfg.Profile) (cfg.Profile, bool) {
	if picker.cursor < 0 || picker.cursor >= len(profiles) {
		return cfg.Profile{}, false
	}
	return profiles[picker.cursor], true
}

// askPassword opens the field for a profile. A profile that already reads the keyring keeps
// the box ticked, because the keyring is where its password belongs.
func (picker *pickerState) askPassword(profile cfg.Profile) {
	picker.pending, picker.password = profile, app.NewEditorBuffer("", 0)
	picker.keepInKeyring = profile.Auth == cfg.AuthKeyring && secret.IsAvailable()
}

// offersKeyring is true where the card draws the box that keeps the password.
func (picker *pickerState) offersKeyring() bool {
	return secret.IsAvailable()
}

func (picker *pickerState) waitsFor(name string) bool {
	return picker.pending.Name == name
}
