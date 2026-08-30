package cfg

import (
	_ "embed"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
)

// The config file a first run writes. Embedded, so a build carries it and no install step
// has to put it anywhere.
//
//go:embed starter.toml
var starterConfig []byte

// StarterConfig returns the config file a first run writes.
func StarterConfig() []byte { return starterConfig }

// EnsureConfigFile writes the starter config if the file is missing, so a first run has a
// file to edit and the form has a file to write into. It reports whether it wrote one.
//
// An existing file is never touched, whatever it holds.
func EnsureConfigFile(path string) (bool, error) {
	if _, err := os.Stat(path); err == nil {
		return false, nil
	} else if !errors.Is(err, fs.ErrNotExist) {
		return false, err
	}

	// 0700, because the themes of the user sit in this directory beside the config file.
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return false, err
	}

	// O_EXCL, so two clients starting at once cannot both write it. 0600, because a profile
	// may hold a password.
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		// Another client won the race and wrote the file first.
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
