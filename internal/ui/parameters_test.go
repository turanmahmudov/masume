package ui

import (
	"testing"

	tea "charm.land/bubbletea/v2"
	uv "github.com/charmbracelet/ultraviolet"

	"github.com/turanmahmudov/masume/internal/app"
)

func TestTakesListKeysAnswersEachCard(t *testing.T) {
	for _, held := range []struct {
		overlay app.Overlay
		wanted  bool
	}{
		{app.Overlay{Kind: app.OverlayPalette}, true},
		{app.Overlay{Kind: app.OverlayHistory}, true},
		{app.Overlay{Kind: app.OverlayParameters}, false},
		{app.Overlay{Kind: app.OverlayExport}, false},
		{app.Overlay{Kind: app.OverlayChoice}, false},
		{app.Overlay{Kind: app.OverlayAiChat}, false},
		{app.Overlay{Kind: app.OverlayCellEdit}, false},
		{app.Overlay{Kind: app.OverlayCellEdit, Cell: app.CellTarget{Choices: []string{"true", "false"}}}, true},
	} {
		if answered := takesListKeys(held.overlay); answered != held.wanted {
			t.Errorf("%s answers %v, wanted %v", held.overlay.Kind, answered, held.wanted)
		}
	}
}

func TestFindChoiceKeyPicksTheAnswerOfALetter(t *testing.T) {
	choices := []app.Choice{
		{Key: "r", ID: "apply"},
		{Key: "d", ID: "discard"},
	}
	press := func(letter rune, mod uv.KeyMod) tea.Key {
		return tea.Key{Code: letter, Text: string(letter), Mod: mod}
	}

	if chosen, found := findChoiceKey(choices, press('d', 0)); !found || chosen != "discard" {
		t.Errorf("d answers %q, %v", chosen, found)
	}
	if _, found := findChoiceKey(choices, press('z', 0)); found {
		t.Error("a letter no answer holds picks one")
	}
	// A chord belongs to the registry, not to the answers.
	if _, found := findChoiceKey(choices, press('d', uv.ModCtrl)); found {
		t.Error("Ctrl+D picks an answer")
	}
	if _, found := findChoiceKey(nil, press('d', 0)); found {
		t.Error("a card with no answers picks one")
	}
}
