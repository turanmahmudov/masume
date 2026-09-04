package secret_test

import (
	"errors"
	"testing"

	"github.com/zalando/go-keyring"

	"github.com/turanmahmudov/masume/internal/secret"
)

// useMockKeyring points the package at a keyring in memory, so a case neither reads nor
// writes the keyring of the person running the tests.
func useMockKeyring(t *testing.T) {
	t.Helper()
	held := secret.IsAvailable
	keyring.MockInit()
	secret.IsAvailable = func() bool { return true }
	t.Cleanup(func() { secret.IsAvailable = held })
}

func TestThePasswordOfAProfileGoesInAndComesBack(t *testing.T) {
	useMockKeyring(t)

	if err := secret.SavePassword("shop", "hunter2"); err != nil {
		t.Fatalf("the password was not stored: %v", err)
	}
	password, found, err := secret.FindPassword("shop")
	if err != nil || !found || password != "hunter2" {
		t.Fatalf("the keyring answered %q, found=%v, err=%v", password, found, err)
	}

	// A second write replaces the first, so a changed password is not kept twice.
	if err := secret.SavePassword("shop", "hunter3"); err != nil {
		t.Fatalf("the password was not replaced: %v", err)
	}
	if password, _, _ := secret.FindPassword("shop"); password != "hunter3" {
		t.Errorf("the keyring answered %q after the replacement", password)
	}
}

// A profile the keyring does not hold is the first connection of that profile, which is not
// an error: the user is asked instead.
func TestAProfileTheKeyringDoesNotHoldIsNoError(t *testing.T) {
	useMockKeyring(t)

	password, found, err := secret.FindPassword("never-stored")
	if err != nil {
		t.Fatalf("a profile with no password reported %v", err)
	}
	if found || password != "" {
		t.Errorf("a profile with no password answered %q, found=%v", password, found)
	}
}

func TestRemovingThePasswordOfAProfile(t *testing.T) {
	useMockKeyring(t)

	if err := secret.SavePassword("shop", "hunter2"); err != nil {
		t.Fatalf("the password was not stored: %v", err)
	}
	if err := secret.DeletePassword("shop"); err != nil {
		t.Fatalf("the password was not removed: %v", err)
	}
	if _, found, _ := secret.FindPassword("shop"); found {
		t.Error("the keyring still holds the password of a profile that was removed")
	}
	// A profile the keyring does not hold is removed without an error, so the removal of a
	// connection never fails on this.
	if err := secret.DeletePassword("shop"); err != nil {
		t.Errorf("a second removal reported %v", err)
	}
}

// A machine with no keyring says so, rather than failing with the message of a bus.
func TestAMachineWithoutAKeyringSaysSo(t *testing.T) {
	held := secret.IsAvailable
	secret.IsAvailable = func() bool { return false }
	t.Cleanup(func() { secret.IsAvailable = held })

	if err := secret.SavePassword("shop", "hunter2"); !errors.Is(err, secret.ErrNoKeyring) {
		t.Errorf("the write reported %v", err)
	}
	if _, _, err := secret.FindPassword("shop"); !errors.Is(err, secret.ErrNoKeyring) {
		t.Errorf("the read reported %v", err)
	}
	// A removal on a machine with no keyring is not an error: there is nothing to remove.
	if err := secret.DeletePassword("shop"); err != nil {
		t.Errorf("the removal reported %v", err)
	}
}
