// Package secret reads and writes database passwords in the system keyring.
package secret

import (
	"errors"
	"fmt"
	"strings"

	"github.com/zalando/go-keyring"
)

// Linux uses Secret Service over D-Bus. macOS uses Keychain. Each profile has one password entry.

// Service is the keyring service name for masume passwords.
const Service = "masume"

// ErrNoKeyring is the error class for a machine that has no keyring masume can reach.
var ErrNoKeyring = errors.New("this machine has no keyring masume can use")

// FindPassword returns the profile password. A missing or empty password returns false without an error.
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

// SavePassword adds or replaces the profile password in the keyring.
func SavePassword(profileName, password string) error {
	if !IsAvailable() {
		return ErrNoKeyring
	}
	if err := keyring.Set(Service, profileName, password); err != nil {
		return describeKeyringFault("write", profileName, err)
	}
	return nil
}

// DeletePassword removes the profile password. Missing passwords and unavailable keyrings return no error.
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

// describeKeyringFault adds the operation and profile to the keyring error text.
func describeKeyringFault(operation, profileName string, err error) error {
	said := strings.TrimSpace(err.Error())
	if said == "" {
		said = "no error details from the keyring"
	}
	return fmt.Errorf("the keyring could not %s the password for %s: %s",
		operation, profileName, said)
}
