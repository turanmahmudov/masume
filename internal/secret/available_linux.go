package secret

import (
	"slices"
	"sync"

	"github.com/godbus/dbus/v5"
)

// secretServiceName is the D-Bus name every keyring of a Linux desktop answers on.
const secretServiceName = "org.freedesktop.secrets"

// IsAvailable reports whether this machine has a keyring masume can reach. The test asks the
// bus who answers, and never reads a password: a read on a locked keyring opens a dialog, and
// the user has asked for nothing at the moment of the test.
var IsAvailable = sync.OnceValue(func() bool {
	bus, err := dbus.SessionBus()
	if err != nil {
		return false
	}

	running := false
	if call := bus.BusObject().Call(
		"org.freedesktop.DBus.NameHasOwner", 0, secretServiceName,
	); call.Err == nil {
		_ = call.Store(&running)
	}
	if running {
		return true
	}

	// A keyring that is not running yet still counts, because the bus starts it on the
	// first call. A desktop that starts it on demand is the usual case.
	names := []string{}
	if call := bus.BusObject().Call(
		"org.freedesktop.DBus.ListActivatableNames", 0,
	); call.Err == nil {
		_ = call.Store(&names)
	}
	return slices.Contains(names, secretServiceName)
})
