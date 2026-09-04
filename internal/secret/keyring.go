// Package secret reads and writes the database passwords that masume keeps outside its
// config file: the keyring of the operating system, and the secret store of the user.
package secret

import (
	"errors"
	"fmt"
	"strings"

	"github.com/zalando/go-keyring"
)

// The keyring of the operating system: secret-service over D-Bus on Linux, the Keychain on
// macOS. masume stores one password per profile, and it stores nothing else there.

// Service is the name masume stores its passwords under. Every entry of the keyring that
// belongs to masume carries it, so the user can find them in the tool of their desktop.
const Service = "masume"

// ErrNoKeyring is the error class for a machine that has no keyring masume can reach.
var ErrNoKeyring = errors.New("this machine has no keyring masume can use")

// FindPassword returns the password the keyring holds for that profile. The second value is
// false where the keyring holds none, which is not an error: it is the first connection.
func FindPassword(profileName string) (string, bool, error) {
	if !IsAvailable() {
		return "", false, ErrNoKeyring
	}
	password, err := keyring.Get(Service, profileName)
	if errors.Is(err, keyring.ErrNotFound) {
		return "", false, nil
	}
	if err != nil {
		return "", false, describeKeyringFault("read", profileName, err)
	}
	return password, password != "", nil
}

// SavePassword stores the password of that profile in the keyring, and replaces the one that
// is there.
func SavePassword(profileName, password string) error {
	if !IsAvailable() {
		return ErrNoKeyring
	}
	if err := keyring.Set(Service, profileName, password); err != nil {
		return describeKeyringFault("write", profileName, err)
	}
	return nil
}

// DeletePassword removes the password of that profile. A profile the keyring does not hold is
// not an error, so the removal of a profile never fails on this.
func DeletePassword(profileName string) error {
	if !IsAvailable() {
		return nil
	}
	err := keyring.Delete(Service, profileName)
	if err == nil || errors.Is(err, keyring.ErrNotFound) {
		return nil
	}
	return describeKeyringFault("remove", profileName, err)
}

// describeKeyringFault returns the reason one keyring operation failed. A locked keyring and
// a keyring the user refused both arrive here, so the reason of the keyring itself is kept.
func describeKeyringFault(operation, profileName string, err error) error {
	said := strings.TrimSpace(err.Error())
	if said == "" {
		said = "the keyring gave no reason"
	}
	return fmt.Errorf("the keyring could not %s the password of %s: %s",
		operation, profileName, said)
}
