package notebook

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/turanmahmudov/masume/internal/core"
)

// Origin is where a notebook file is kept.
type Origin string

// The three places a notebook comes from.
const (
	// OriginProject is the directory beside the project file, shared through git.
	OriginProject Origin = "project"
	// OriginPersonal is the state directory, beside the history file.
	OriginPersonal Origin = "personal"
	// OriginExtra is a directory the config file adds.
	OriginExtra Origin = "extra"
)

// projectDirectory is the notebook directory inside a project.
var projectDirectory = filepath.Join(".masume", "notebooks")

// stateDirectory is the notebook directory of one user.
const stateDirectory = "notebooks"

// Entry is one notebook the picker lists.
type Entry struct {
	Name   string
	Path   string
	Origin Origin
	Title  string
	Cells  int
	// The cells whose fence asks for a write confirmation.
	Writes    int
	ChangedAt time.Time
}

// ResolveProjectDirectory returns the notebook directory of the project file.
func ResolveProjectDirectory(projectFile string) string {
	if projectFile == "" {
		return ""
	}
	return filepath.Join(filepath.Dir(projectFile), projectDirectory)
}

// ResolvePersonalDirectory returns the notebook directory of the user.
func ResolvePersonalDirectory() string {
	return core.ResolveStatePath(stateDirectory)
}

// ResolvePath returns the path of a notebook of that name in that directory.
func ResolvePath(directory, name string) string {
	return filepath.Join(directory, BuildSlug(name)+FileSuffix)
}

// ReadName returns the name of the notebook a path holds.
func ReadName(path string) string {
	base := filepath.Base(path)
	if trimmed, held := strings.CutSuffix(base, FileSuffix); held {
		return trimmed
	}
	return strings.TrimSuffix(base, filepath.Ext(base))
}

// List returns every notebook of the project, of the user, and of the extra directories.
func List(projectFile string, extra []string) []Entry {
	entries := []Entry{}
	entries = append(entries, listDirectory(ResolveProjectDirectory(projectFile), OriginProject)...)
	entries = append(entries, listDirectory(ResolvePersonalDirectory(), OriginPersonal)...)
	for _, directory := range extra {
		entries = append(entries, listDirectory(
			core.ExpandHomePath(directory), OriginExtra)...)
	}
	slices.SortStableFunc(entries, func(left, right Entry) int {
		return strings.Compare(left.Name, right.Name)
	})
	return entries
}

// listDirectory returns the notebooks of one directory.
func listDirectory(directory string, origin Origin) []Entry {
	if directory == "" {
		return nil
	}
	found, err := os.ReadDir(directory)
	if err != nil {
		return nil
	}
	entries := make([]Entry, 0, len(found))
	for _, file := range found {
		if file.IsDir() || !strings.HasSuffix(file.Name(), FileSuffix) {
			continue
		}
		path := filepath.Join(directory, file.Name())
		entry := Entry{Name: ReadName(path), Path: path, Origin: origin}
		if info, statErr := file.Info(); statErr == nil {
			entry.ChangedAt = info.ModTime()
		}
		if book, readErr := Read(path); readErr == nil {
			entry.Title = book.Title
			entry.Cells = len(book.Cells)
			entry.Writes = book.CountWriteCells()
		}
		entries = append(entries, entry)
	}
	return entries
}

// Read reads one notebook file.
func Read(path string) (Notebook, error) {
	text, err := os.ReadFile(core.ExpandHomePath(path))
	if err != nil {
		return Notebook{}, err
	}
	return Parse(string(text)), nil
}

// Save writes the notebook, and makes the directory of the file where it is missing.
func Save(path string, book Notebook) error {
	full := core.ExpandHomePath(path)
	if err := os.MkdirAll(filepath.Dir(full), 0o700); err != nil {
		return err
	}
	return os.WriteFile(full, []byte(Write(book)), 0o600)
}
