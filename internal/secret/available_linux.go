package secret

import (
	"slices"
	"sync"

	"github.com/godbus/dbus/v5"
)

// secretServiceName is the Secret Service D-Bus name.
const secretServiceName = "org.freedesktop.secrets"

// IsAvailable checks for a running or activatable Secret Service without reading passwords or opening an unlock dialog.
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

	// D-Bus can start an activatable keyring on the first call.
	names := []string{}
	if call := bus.BusObject().Call(
		"org.freedesktop.DBus.ListActivatableNames", 0,
	); call.Err == nil {
		_ = call.Store(&names)
	}
	return slices.Contains(names, secretServiceName)
})
