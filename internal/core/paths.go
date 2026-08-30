package core

import (
	"os"
	"path/filepath"
	"strings"
)

// stateDirectory is where this client keeps what it wrote itself: the history
// file and the logs.
const stateDirectory = "masume"

// ResolveStatePath returns where a file this client writes belongs.
func ResolveStatePath(fileName string) string {
	stateHome := os.Getenv("XDG_STATE_HOME")
	if stateHome == "" {
		stateHome = filepath.Join(HomeDirectory(), ".local", "state")
	}
	return filepath.Join(stateHome, stateDirectory, fileName)
}

// HomeDirectory returns the home directory of the user, or an empty path.
func HomeDirectory() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return home
}

// ExpandHomePath expands a leading `~`, which a config file and a typed path both use.
func ExpandHomePath(path string) string {
	if path == "~" {
		return HomeDirectory()
	}
	if strings.HasPrefix(path, "~/") {
		return filepath.Join(HomeDirectory(), path[2:])
	}
	return path
}
