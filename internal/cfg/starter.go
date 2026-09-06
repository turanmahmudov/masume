package cfg

import (
	_ "embed"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
)

// The embedded config file for the first run.
//
//go:embed starter.toml
var starterConfig []byte

// StarterConfig returns the config file written on the first run.
func StarterConfig() []byte { return starterConfig }

// EnsureConfigFile creates a missing config file and reports creation. Existing files remain unchanged.
func EnsureConfigFile(path string) (bool, error) {
	if _, err := os.Stat(path); err == nil {
		return false, nil
	} else if !errors.Is(err, fs.ErrNotExist) {
		return false, err
	}

	// The config directory is private to its owner.
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return false, err
	}

	// O_EXCL permits one creator. File access is limited to the owner.
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		// Another client wrote the file first.
		if errors.Is(err, fs.ErrExist) {
			return false, nil
		}
		return false, err
	}

	if _, err := file.Write(starterConfig); err != nil {
		_ = file.Close()
		return false, err
	}
	return true, file.Close()
}
