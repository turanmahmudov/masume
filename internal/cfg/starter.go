package cfg

import (
	_ "embed"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
)

// The config file written on the first run. It is embedded, so the binary contains it and
// no install step has to copy it.
//
//go:embed starter.toml
var starterConfig []byte

// StarterConfig returns the config file written on the first run.
func StarterConfig() []byte { return starterConfig }

// EnsureConfigFile writes the starter config if the file does not exist, so the first run
// has a file to edit and the form has a file to write to. It reports whether it wrote a
// file.
//
// An existing file is never changed, whatever it contains.
func EnsureConfigFile(path string) (bool, error) {
	if _, err := os.Stat(path); err == nil {
		return false, nil
	} else if !errors.Is(err, fs.ErrNotExist) {
		return false, err
	}

	// 0700, because the themes of the user are in this directory next to the config file.
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return false, err
	}

	// O_EXCL, so two clients that start at the same time cannot both write the file.
	// 0600, because a profile can hold a password.
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
