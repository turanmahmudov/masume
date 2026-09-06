package ui

import (
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"strings"
	"testing"
	"unicode"

	"github.com/BurntSushi/toml"

	"github.com/turanmahmudov/masume/internal/cfg"
)

func TestDocumentDefaultKeyBindings(t *testing.T) {
	written, err := os.ReadFile("../../docs/keys.md")
	if err != nil {
		t.Fatal(err)
	}
	code := regexp.MustCompile("`([^`]+)`")
	documented := map[string]bool{}
	scope := ""
	for _, line := range strings.Split(string(written), "\n") {
		if strings.HasPrefix(line, "`[keys.") {
			scope = strings.TrimSuffix(strings.TrimPrefix(line, "`[keys."), "]`")
		}
		if scope == "" || !strings.HasPrefix(line, "| `") {
			continue
		}
		entries := code.FindAllStringSubmatch(line, -1)
		actionKey := scope + ":" + entries[0][1]
		defaults, known := DefaultPreset.Chords[actionKey]
		if !known || documented[actionKey] {
			t.Errorf("unknown or repeated action %s", actionKey)
			continue
		}
		documented[actionKey] = true
		if len(entries)-1 != len(defaults) {
			t.Errorf("binding count for %s: %d, want %d", actionKey, len(entries)-1, len(defaults))
			continue
		}
		for at, chord := range defaults {
			wanted, _ := cfg.ParseChordSequence(chord)
			actual, parsed := cfg.ParseChordSequence(entries[at+1][1])
			if !parsed || !reflect.DeepEqual(actual, wanted) {
				t.Errorf("binding for %s: %q, want %q", actionKey, entries[at+1][1], chord)
			}
		}
	}
	for actionKey := range DefaultPreset.Chords {
		if !documented[actionKey] {
			t.Errorf("missing action %s", actionKey)
		}
	}
}

func TestDocumentConfigurationExamples(t *testing.T) {
	paths, err := filepath.Glob("../../docs/*.md")
	if err != nil {
		t.Fatal(err)
	}
	paths = append(paths, "../../README.md", "../../config.example.toml")
	blocks := regexp.MustCompile("(?s)```toml\n(.*?)\n```")
	for _, path := range paths {
		written, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		examples := blocks.FindAllStringSubmatch(string(written), -1)
		if filepath.Ext(path) == ".toml" {
			examples = [][]string{{"", string(written)}}
		}
		for at, example := range examples {
			var document cfg.Table
			if _, err := toml.Decode(example[1], &document); err != nil {
				t.Errorf("%s example %d: %v", path, at+1, err)
				continue
			}
			settings := cfg.ParseKeySettings(document)
			problems := append(settings.Problems,
				NewKeyRegistry().ApplyKeySettings(DefaultPreset, settings.Choices, true)...)
			if len(problems) != 0 {
				t.Errorf("%s example %d: %v", path, at+1, problems)
			}
		}
	}
}

func TestDocumentLocalLinks(t *testing.T) {
	paths, err := filepath.Glob("../../docs/*.md")
	if err != nil {
		t.Fatal(err)
	}
	paths = append(paths, "../../README.md", "../../CONTRIBUTING.md", "../../SECURITY.md")
	links := regexp.MustCompile(`\]\(([^\s)]+)\)|(?:src|href)="([^"\s]+)"`)
	for _, path := range paths {
		written, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		for _, match := range links.FindAllStringSubmatch(string(written), -1) {
			target := match[1] + match[2]
			if strings.Contains(target, "://") {
				continue
			}
			file, fragment, _ := strings.Cut(target, "#")
			resolved := path
			if file != "" {
				resolved = filepath.Join(filepath.Dir(path), file)
			}
			if _, err := os.Stat(resolved); err != nil {
				t.Errorf("%s link %s: %v", path, target, err)
				continue
			}
			if fragment == "" {
				continue
			}
			destination, err := os.ReadFile(resolved)
			if err != nil {
				t.Fatal(err)
			}
			found := false
			for _, line := range strings.Split(string(destination), "\n") {
				if !strings.HasPrefix(line, "#") {
					continue
				}
				heading := strings.TrimSpace(strings.TrimLeft(line, "#"))
				anchor := strings.Map(func(character rune) rune {
					if character == ' ' {
						return '-'
					}
					if unicode.IsLetter(character) || unicode.IsNumber(character) || character == '-' || character == '_' {
						return unicode.ToLower(character)
					}
					return -1
				}, heading)
				found = found || anchor == fragment
			}
			if !found {
				t.Errorf("%s link %s: heading not found", path, target)
			}
		}
	}
}
