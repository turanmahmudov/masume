package ui

import (
	"slices"
	"strings"
	"testing"

	"github.com/turanmahmudov/masume/internal/app"
	"github.com/turanmahmudov/masume/internal/cfg"
)

// buildHintModeModel answers a model that ran two statements and draws its hints in that mode.
func buildHintModeModel(t *testing.T, mode cfg.KeyHintsMode) (*Model, *app.Tab) {
	t.Helper()
	model, _, tab := buildBatchModel(t)
	model.settings.KeyHints = mode
	return model, tab
}

// readStatusBar answers the drawn bottom bar of the frame, as text.
func readStatusBar(t *testing.T, model *Model) string {
	t.Helper()
	frame := strings.Split(model.render(), "\n")
	return stripEscapes(frame[model.height-1])
}

// The main mode shows the primary keys of the pane and hides the rest.
func TestTheMainModeShowsThePrimaryKeys(t *testing.T) {
	model, tab := buildHintModeModel(t, cfg.KeyHintsMain)

	bar := readStatusBar(t, model)
	if !strings.Contains(bar, "menu") {
		t.Errorf("the status bar drew %q, wanted the key that opens the menu", bar)
	}
	for _, said := range []string{"search", "where", "freeze", "go to column"} {
		if strings.Contains(bar, said) {
			t.Errorf("the status bar drew the %q key in the main mode", said)
		}
	}
	if !strings.Contains(bar, "autocommit is off") {
		t.Errorf("the status bar dropped its report and drew %q", bar)
	}

	tab.Focus = app.PaneEditor
	if bar := readStatusBar(t, model); !strings.Contains(bar, "run the statement") {
		t.Errorf("the status bar drew %q, wanted the key that runs the statement", bar)
	} else if strings.Contains(bar, "select all") {
		t.Errorf("the status bar drew %q, wanted the primary key alone", bar)
	}

	tab.Focus = app.PaneSidebar
	if bar := readStatusBar(t, model); !strings.Contains(bar, "unfold") {
		t.Errorf("the status bar drew %q, wanted the key that opens the row", bar)
	} else if strings.Contains(bar, "refresh") {
		t.Errorf("the status bar drew %q, wanted the primary key alone", bar)
	}
}

// Each step key of a strip or the tab row stands for a chip a press reaches. The main mode
// leaves them out.
func TestTheMainModeHidesTheStepKeys(t *testing.T) {
	model, _ := buildHintModeModel(t, cfg.KeyHintsMain)

	drawn := stripEscapes(model.render())
	for _, said := range []string{"prev/next", "Alt+N new", "Alt+T name"} {
		if strings.Contains(drawn, said) {
			t.Errorf("the frame drew the %q key in the main mode", said)
		}
	}
	// The border of the editor and the strip of the plan name keys no chip stands for.
	if !strings.Contains(drawn, "full height") {
		t.Error("the border of the editor dropped its key in the main mode")
	}
}

// The off mode hides every key hint. The bars and the borders keep their reports.
func TestTheOffModeHidesEveryKeyHint(t *testing.T) {
	model, _ := buildHintModeModel(t, cfg.KeyHintsOff)

	bar := readStatusBar(t, model)
	if strings.Contains(bar, "menu") {
		t.Errorf("the status bar drew %q with the hints off", bar)
	}
	if !strings.Contains(bar, "autocommit is off") {
		t.Errorf("the status bar dropped its report and drew %q", bar)
	}

	drawn := stripEscapes(model.render())
	for _, said := range []string{
		"palette", "? help", "full height", "ask ai", "ask about this",
	} {
		if strings.Contains(drawn, said) {
			t.Errorf("the frame drew the %q key with the hints off", said)
		}
	}
	// The readouts of the strips are not keys, so they stand.
	if !strings.Contains(drawn, "1 of 2") {
		t.Error("the strip of the statements dropped its count")
	}
}

// An empty pane shows the keys it waits for. The off mode leaves it the title alone.
func TestTheOffModeHidesTheKeysOfAnEmptyPane(t *testing.T) {
	model, _ := buildHintModeModel(t, cfg.KeyHintsOff)
	drawn := strings.Join(model.renderEmptyState(40, 8, "no result yet",
		[]Hint{{Key: "^R", Label: "run the statement"}, {Label: "2 rows"}}), "\n")

	if strings.Contains(stripEscapes(drawn), "run the statement") {
		t.Error("the empty pane drew a key with the hints off")
	}
	if !strings.Contains(stripEscapes(drawn), "2 rows") {
		t.Error("the empty pane dropped what it reports")
	}
}

// The keys that reach the model are the only way to the model. The full mode and the main
// mode both show them.
func TestTheMainModeShowsTheKeysOfTheModel(t *testing.T) {
	for _, mode := range []cfg.KeyHintsMode{cfg.KeyHintsFull, cfg.KeyHintsMain} {
		model, _ := buildHintModeModel(t, mode)
		drawn := stripEscapes(model.render())
		if !strings.Contains(drawn, "ask ai") {
			t.Errorf("the %q mode dropped the key that opens the chat", mode)
		}
		if !strings.Contains(drawn, "ask about this") {
			t.Errorf("the %q mode dropped the key that asks about the statement", mode)
		}
	}
}

// A hidden tree is reached by no other key, so the main mode shows that key.
func TestTheMainModeShowsTheKeyThatOpensTheTree(t *testing.T) {
	model, _ := buildHintModeModel(t, cfg.KeyHintsMain)
	model.Active().SidebarVisible = false

	if bar := readStatusBar(t, model); !strings.Contains(bar, "explorer") {
		t.Errorf("the status bar drew %q, wanted the key that shows the tree", bar)
	}
}

// The count of the staged changes stands on the right of the bar, and names the key that
// opens the review beside it. The keys on the left hold no second copy of that key.
func TestTheStagedCountNamesTheKeyOfTheReview(t *testing.T) {
	model, tab := buildHintModeModel(t, cfg.KeyHintsMain)
	stageCellEdits(tab, 2)

	bar := readStatusBar(t, model)
	if !strings.Contains(bar, "2 staged · p to review") {
		t.Errorf("the status bar drew %q, wanted the key that reviews the changes", bar)
	}
	if strings.Contains(bar, "review 2") {
		t.Errorf("the status bar drew %q, wanted the review key once", bar)
	}
}

// The key answers in the grid, so a tab that reads the editor is left with the count.
func TestTheStagedCountNamesNoKeyInTheEditor(t *testing.T) {
	model, tab := buildHintModeModel(t, cfg.KeyHintsMain)
	stageCellEdits(tab, 2)
	tab.Focus = app.PaneEditor

	if bar := readStatusBar(t, model); !strings.Contains(bar, "2 staged") ||
		strings.Contains(bar, "to review") {
		t.Errorf("the status bar drew %q, wanted the count without the key", bar)
	}
}

// A card shows its own keys, and the mode reaches the card too. The main mode shows the keys
// that answer it. The off mode leaves the card its readouts.
func TestTheKeyHintsModeReachesTheChatCard(t *testing.T) {
	for _, held := range []struct {
		mode   cfg.KeyHintsMode
		wanted []string
		gone   []string
	}{
		{cfg.KeyHintsFull, []string{"ask", "newline", "turn", "chats", "close"}, nil},
		{cfg.KeyHintsMain, []string{"ask", "last reply query to editor", "close"},
			[]string{"newline", "turn", "page", "chats", "to a notebook"}},
		{cfg.KeyHintsOff, nil, []string{"ask", "close", "last reply query to editor"}},
	} {
		model, chat := buildChatModel(t)
		model.settings.KeyHints = held.mode
		said := stripEscapes(model.describeChatKeys(chat).buildText())

		for _, key := range held.wanted {
			if !strings.Contains(said, key) {
				t.Errorf("the %q mode drew %q, wanted the %q key", held.mode, said, key)
			}
		}
		for _, key := range held.gone {
			if strings.Contains(said, key) {
				t.Errorf("the %q mode drew the %q key", held.mode, key)
			}
		}
	}
}

// The bar under a card shows the keys of the card. A key the mode hides on the card is
// hidden on the bar.
func TestTheBarUnderACardShowsTheKeysOfTheCard(t *testing.T) {
	model, _ := buildHintModeModel(t, cfg.KeyHintsMain)
	model.Active().Overlay = app.Overlay{Kind: app.OverlayNotebooks, Title: " notebooks "}
	frame := strings.Split(model.render(), "\n")

	bar := stripEscapes(frame[model.height-1])
	if !strings.Contains(bar, "open") {
		t.Errorf("the status bar under the card drew %q, wanted the key that opens a row", bar)
	}
	if strings.Contains(bar, "rename") {
		t.Errorf("the status bar under the card drew %q, wanted the kept keys alone", bar)
	}
}

// A config that sets no mode draws every hint, as the app did before the setting.
func TestAnUnsetKeyHintsModeShowsEveryKeyHint(t *testing.T) {
	model, _ := buildHintModeModel(t, "")

	if bar := readStatusBar(t, model); !strings.Contains(bar, "freeze") {
		t.Errorf("the status bar drew %q, wanted every key", bar)
	}
}

// mainHintActions is the whole policy of the main mode: every action the bars, the strips
// and the cards show in it. A key that joins or leaves the list is a change to what the mode
// draws, so it is written here as well.
var mainHintActions = []string{
	"dialog/answer-no", "dialog/answer-yes", "dialog/apply-changes", "dialog/apply-step",
	"dialog/close", "dialog/copy-value", "dialog/discard-changes", "dialog/insert-ai-sql",
	"dialog/next-row", "dialog/previous-row", "dialog/run-with-values", "dialog/save-cell",
	"dialog/save-form", "dialog/send-question", "dialog/step-back", "dialog/stop-ai-reply",
	"dialog/stop-session", "dialog/toggle-value", "dialog/write-export",
	"document/open-node", "editor/leave-cell",
	"global/ai-fix-error", "global/cancel-query", "global/next-page", "global/reveal-sql",
	"global/run-at-cursor", "global/run-batch", "global/save-query", "global/send-to-ai",
	"global/show-ai-chat", "global/show-help", "global/show-palette",
	"global/toggle-sidebar",
	"grid/count-rows", "grid/open-menu",
	"list/choose-row",
	"notebook/run-cell", "notebook/run-from-cell",
	"plan/ai-check-plan",
	"tree/object-menu", "tree/open-node",
}

func TestTheMainHintPolicyMatchesTheCatalog(t *testing.T) {
	marked := []string{}
	for _, action := range ActionCatalog {
		if action.MainHint {
			marked = append(marked, string(action.Scope)+"/"+string(action.ID))
		}
	}
	slices.Sort(marked)

	wanted := slices.Clone(mainHintActions)
	slices.Sort(wanted)
	if !slices.Equal(marked, wanted) {
		t.Errorf("the catalog marks\n%v\nand the policy holds\n%v", marked, wanted)
	}
}
