//go:build !linux && !darwin

package secret

// IsAvailable reports whether this machine has a keyring masume can reach. masume is built
// for Linux and macOS, so any other system has none.
var IsAvailable = func() bool { return false }
