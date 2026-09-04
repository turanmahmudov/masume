package secret

import (
	"os/exec"
	"sync"
)

// IsAvailable reports whether this machine has a keyring masume can reach. macOS always has
// the Keychain, so the test only looks for the tool that reaches it.
var IsAvailable = sync.OnceValue(func() bool {
	_, err := exec.LookPath("security")
	return err == nil
})
