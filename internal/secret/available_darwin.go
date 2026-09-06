package secret

import (
	"os/exec"
	"sync"
)

// IsAvailable checks for the macOS security tool used to access Keychain.
var IsAvailable = sync.OnceValue(func() bool {
	_, err := exec.LookPath("security")
	return err == nil
})
