//go:build !linux && !darwin

package secret

// IsAvailable is false on unsupported operating systems.
var IsAvailable = func() bool { return false }
