package ui

import (
	"testing"

	tea "charm.land/bubbletea/v2"
	uv "github.com/charmbracelet/ultraviolet"

	"github.com/turanmahmudov/masume/internal/cfg"
)

// pressCtrl builds the press of one letter with Ctrl held.
func pressCtrl(letter rune) tea.Key {
	return tea.Key{Code: letter, Mod: uv.ModCtrl}
}

func TestMatchFirstTakesTheActionTheCardNames(t *testing.T) {
	keymap := NewKeymap(NewKeyRegistry())

	// The dialog scope binds `ctrl+l` to both `new-ai-chat` and `set-null`.
	match, matched := keymap.Match(pressCtrl('l'), cfg.ScopeDialog)
	if !matched || match.Action != ActionNewAiChat {
		t.Fatalf("the plain match answers %+v", match)
	}

	match, matched = keymap.MatchFirst(
		pressCtrl('l'), FindDialogActions("cell-edit"), cfg.ScopeDialog)
	if !matched || match.Action != ActionSetNull {
		t.Errorf("the cell editor answers %+v, wanted set-null", match)
	}

	match, matched = keymap.MatchFirst(
		pressCtrl('l'), FindDialogActions("ai-chat"), cfg.ScopeDialog)
	if !matched || match.Action != ActionNewAiChat {
		t.Errorf("the chat answers %+v, wanted new-ai-chat", match)
	}
}

func TestMatchFirstLeavesAChordTheCardDoesNotName(t *testing.T) {
	keymap := NewKeymap(NewKeyRegistry())

	// `ctrl+o` belongs to the chat, and the cell editor names neither action of it,
	// so the press still reaches what the scope binds it to.
	match, matched := keymap.MatchFirst(
		pressCtrl('o'), FindDialogActions("cell-edit"), cfg.ScopeDialog)
	if !matched || match.Action != ActionShowAiChats {
		t.Errorf("answers %+v, wanted show-ai-chats", match)
	}
}

func TestMatchFirstAnswersTheExportForm(t *testing.T) {
	keymap := NewKeymap(NewKeyRegistry())

	// `ctrl+s` is bound to `save-cell`, `save-form` and `write-export`.
	match, matched := keymap.MatchFirst(
		pressCtrl('s'), FindDialogActions("export"), cfg.ScopeDialog)
	if !matched || match.Action != ActionWriteExport {
		t.Errorf("the export form answers %+v, wanted write-export", match)
	}
	match, matched = keymap.MatchFirst(
		pressCtrl('s'), FindDialogActions("form"), cfg.ScopeDialog)
	if !matched || match.Action != ActionSaveForm {
		t.Errorf("the connection form answers %+v, wanted save-form", match)
	}
}
