package ui

import (
	"testing"

	"github.com/turanmahmudov/masume/internal/cfg"
)

// A chain that stops early once left every colour the child does not name at the missing
// colour, and the whole client drew in magenta.
func TestAThemeWithAMissingParentInheritsTheFallbackTheme(t *testing.T) {
	registry := NewThemeRegistry()
	registry.RegisterDocuments([]cfg.ThemeDocument{{
		Name:    "orphan",
		Extends: "no-such-theme",
		Colors:  map[string]string{"accent": "#00ff00"},
	}})

	resolved, found := registry.FindResolvedTheme("orphan")
	if !found {
		t.Fatal("the theme did not resolve")
	}

	fallback, held := registry.FindResolvedTheme(FallbackThemeName)
	if !held {
		t.Fatalf("the fallback theme %q did not resolve", FallbackThemeName)
	}
	if resolved.Theme.Background != fallback.Theme.Background {
		t.Errorf("the background is %v, wanted the one of %s",
			resolved.Theme.Background, FallbackThemeName)
	}
	if resolved.Theme.Text != fallback.Theme.Text {
		t.Errorf("the text colour is %v, wanted the one of %s",
			resolved.Theme.Text, FallbackThemeName)
	}
	if resolved.Theme.Accent == fallback.Theme.Accent {
		t.Error("the accent the child names was replaced by the one of the parent")
	}
	if len(resolved.Problems) == 0 {
		t.Error("the missing parent was not reported")
	}
}

// An inheritance loop ends the chain the same way a missing parent does, so it falls back
// the same way.
func TestAThemeWithAnInheritanceLoopInheritsTheFallbackTheme(t *testing.T) {
	registry := NewThemeRegistry()
	registry.RegisterDocuments([]cfg.ThemeDocument{
		{Name: "one", Extends: "two"},
		{Name: "two", Extends: "one"},
	})

	resolved, found := registry.FindResolvedTheme("one")
	if !found {
		t.Fatal("the theme did not resolve")
	}
	fallback, _ := registry.FindResolvedTheme(FallbackThemeName)
	if resolved.Theme.Background != fallback.Theme.Background {
		t.Errorf("the background is %v, wanted the one of %s",
			resolved.Theme.Background, FallbackThemeName)
	}
	if len(resolved.Problems) == 0 {
		t.Error("the loop was not reported")
	}
}
