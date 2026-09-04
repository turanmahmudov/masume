package ui

import (
	"os"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/zalando/go-keyring"

	"github.com/turanmahmudov/masume/internal/app"
	"github.com/turanmahmudov/masume/internal/cfg"
	"github.com/turanmahmudov/masume/internal/core"
	"github.com/turanmahmudov/masume/internal/secret"
)

// useMockKeyring points the keyring at one in memory, so a case neither reads nor writes the
// keyring of the person running the tests.
func useMockKeyring(t *testing.T) {
	t.Helper()
	held := secret.IsAvailable
	keyring.MockInit()
	secret.IsAvailable = func() bool { return true }
	t.Cleanup(func() { secret.IsAvailable = held })
}

// useNoKeyring points the client at a machine that has none.
func useNoKeyring(t *testing.T) {
	t.Helper()
	held := secret.IsAvailable
	secret.IsAvailable = func() bool { return false }
	t.Cleanup(func() { secret.IsAvailable = held })
}

// buildPromptingProfile returns a connection whose password only the user can give.
func buildPromptingProfile(name string) cfg.Profile {
	return cfg.Profile{
		Name: name, Engine: core.EnginePostgres, Host: "db.internal", Port: 5432,
		Database: name, User: "reader", Auth: cfg.AuthPrompt,
		Environment: cfg.EnvironmentProd, AccessMode: cfg.AccessWrite,
		PageSize: cfg.DefaultPageSize, InConfigFile: true,
	}
}

func TestThePasswordCardOffersTheKeyring(t *testing.T) {
	useMockKeyring(t)
	model := buildOfflineModel(t, 160, 48)
	model.picker.askPassword(buildPromptingProfile("shop"))

	drawn := stripEscapes(model.renderPassword())
	if !strings.Contains(drawn, "[ ] remember in the keyring") {
		t.Fatalf("the card does not offer the keyring:\n%s", drawn)
	}
	if !strings.Contains(drawn, "Tab") {
		t.Error("the card does not name the key that ticks the box")
	}

	model.readPasswordKey(tea.Key{Code: tea.KeyTab})
	if !model.picker.keepInKeyring {
		t.Fatal("Tab did not tick the box")
	}
	if !strings.Contains(stripEscapes(model.renderPassword()), "[x] remember in the keyring") {
		t.Error("the ticked box is not drawn as ticked")
	}
}

// The box costs a row, so a machine with no keyring is not offered one it cannot use.
func TestThePasswordCardOffersNoKeyringWithoutOne(t *testing.T) {
	useNoKeyring(t)
	model := buildOfflineModel(t, 160, 48)
	model.picker.askPassword(buildPromptingProfile("shop"))

	if strings.Contains(stripEscapes(model.renderPassword()), "keyring") {
		t.Error("the card offers a keyring the machine does not have")
	}
	model.readPasswordKey(tea.Key{Code: tea.KeyTab})
	if model.picker.keepInKeyring {
		t.Error("Tab ticked a box the card does not draw")
	}
}

// A profile that already reads the keyring opens with the box ticked: the keyring is where
// its password belongs, and it holds none yet.
func TestTheKeyringBoxStartsTickedForAKeyringProfile(t *testing.T) {
	useMockKeyring(t)
	model := buildOfflineModel(t, 160, 48)

	profile := buildPromptingProfile("shop")
	profile.Auth = cfg.AuthKeyring
	model.picker.askPassword(profile)
	if !model.picker.keepInKeyring {
		t.Error("a profile that reads the keyring opens with the box clear")
	}

	model.picker.askPassword(buildPromptingProfile("other"))
	if model.picker.keepInKeyring {
		t.Error("a profile that asks the user opens with the box ticked")
	}
}

// The password reaches the keyring only once the server took it, and the profile is written
// back as one that reads the keyring, so the next connection asks for nothing.
func TestATypedPasswordReachesTheKeyringAndTheConfigFile(t *testing.T) {
	useMockKeyring(t)
	configPath := useConfigFile(t)
	model := buildOfflineModel(t, 160, 48)

	profile := buildPromptingProfile("shop")
	model.profiles = []cfg.Profile{profile}
	model.picker.askPassword(profile)
	model.picker.password.SetText("hunter2")
	model.picker.keepInKeyring = true

	model.keepTypedPassword(app.NewConnection(&offlineSession{profile: profile}, nil, true))

	password, found, err := secret.FindPassword("shop")
	if err != nil || !found || password != "hunter2" {
		t.Fatalf("the keyring answered %q, found=%v, err=%v", password, found, err)
	}
	if model.profiles[0].Auth != cfg.AuthKeyring {
		t.Errorf("the profile now reads %q", model.profiles[0].Auth)
	}

	written, readErr := os.ReadFile(configPath)
	if readErr != nil {
		t.Fatalf("the config file could not be read: %v", readErr)
	}
	if !strings.Contains(string(written), `auth = "keyring"`) {
		t.Errorf("the config file reads:\n%s", written)
	}
	// The box is cleared, so the next connection of another profile does not store its
	// password without being asked.
	if model.picker.keepInKeyring {
		t.Error("the box stayed ticked after the password was stored")
	}
}

// A box that was never ticked stores nothing, whatever was typed.
func TestATypedPasswordStaysOutOfTheKeyringWhenTheBoxIsClear(t *testing.T) {
	useMockKeyring(t)
	model := buildOfflineModel(t, 160, 48)

	profile := buildPromptingProfile("shop")
	model.picker.askPassword(profile)
	model.picker.password.SetText("hunter2")
	model.picker.keepInKeyring = false

	model.keepTypedPassword(app.NewConnection(&offlineSession{profile: profile}, nil, true))
	if _, found, _ := secret.FindPassword("shop"); found {
		t.Error("the keyring holds a password the user did not ask it to keep")
	}
}

// A connection that is in no file has nowhere to be written back to, so the keyring is set as
// its source in memory and the question asked on the way out writes it without a password.
func TestATypedPasswordOfAnUnsavedConnectionMarksItForTheKeyring(t *testing.T) {
	useMockKeyring(t)
	model := buildOfflineModel(t, 160, 48)

	profile := buildPromptingProfile("from-the-command-line")
	profile.InConfigFile = false
	model.picker.askPassword(profile)
	model.picker.password.SetText("hunter2")
	model.picker.keepInKeyring = true
	connection := app.NewConnection(&offlineSession{profile: profile}, nil, true)

	model.keepTypedPassword(connection)

	if _, found, _ := secret.FindPassword(profile.Name); !found {
		t.Fatal("the keyring does not hold the password")
	}
	at, held := findProfileIndex(model.unsaved, profile.Name)
	if !held {
		t.Fatal("the connection is not offered for saving")
	}
	if model.unsaved[at].Auth != cfg.AuthKeyring {
		t.Errorf("the connection would be written with auth %q", model.unsaved[at].Auth)
	}
	if model.unsaved[at].Password != "" {
		t.Error("the connection would be written with its password in the file")
	}
}

// The password of a connection goes with it, so the keyring keeps no secret for a connection
// that is gone.
func TestRemovingAConnectionRemovesItsStoredPassword(t *testing.T) {
	useMockKeyring(t)
	useConfigFile(t)
	model := buildOfflineModel(t, 160, 48)

	profile := buildPromptingProfile("shop")
	profile.Auth = cfg.AuthKeyring
	if err := secret.SavePassword(profile.Name, "hunter2"); err != nil {
		t.Fatalf("the password was not stored: %v", err)
	}
	model.profiles = []cfg.Profile{profile}

	model.askDeleteProfile(profile)
	if model.confirm == nil {
		t.Fatal("the client did not ask before the removal")
	}
	model.confirm.Answer(true)

	if _, found, _ := secret.FindPassword(profile.Name); found {
		t.Error("the keyring still holds the password of a connection that was removed")
	}
}
