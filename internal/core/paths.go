package core

import (
	"os"
	"path/filepath"
	"strings"
)

// stateDirectory is the directory for the files this client writes: the history file
// and the logs.
const stateDirectory = "masume"

// ResolveStatePath returns the full path of a file this client writes.
func ResolveStatePath(fileName string) string {
	stateHome := os.Getenv("XDG_STATE_HOME")
	if stateHome == "" {
		stateHome = filepath.Join(HomeDirectory(), ".local", "state")
	}
	return filepath.Join(stateHome, stateDirectory, fileName)
}

// HomeDirectory returns the home directory of the user, or an empty string.
func HomeDirectory() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return home
}

// ShortenHomePath replaces the home directory prefix with `~`.
func ShortenHomePath(path string) string {
	home := HomeDirectory()
	if home == "" || !strings.HasPrefix(path, home) {
		return path
	}
	return "~" + path[len(home):]
}

// ExpandHomePath expands a leading `~`. A config file and a path the user types both
// accept it.
func ExpandHomePath(path string) string {
	if path == "~" {
		return HomeDirectory()
	}
	if strings.HasPrefix(path, "~/") {
		return filepath.Join(HomeDirectory(), path[2:])
	}
	return path
}
