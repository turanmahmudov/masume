package ui

import (
	"testing"

	tea "charm.land/bubbletea/v2"
	uv "github.com/charmbracelet/ultraviolet"

	"github.com/turanmahmudov/masume/internal/app"
	"github.com/turanmahmudov/masume/internal/cfg"
)

func buildSequenceModel(t *testing.T, focus string) (*Model, *app.Connection, *app.Tab) {
	t.Helper()
	model := buildOrdersModel(t, 20)
	connection := model.Active()
	tab := connection.Active()
	session := connection.Session.(*offlineSession)
	session.capabilities.PlansStatement = true
	session.capabilities.PlansEveryStatement = true
	session.capabilities.HasServerSessions = true
	switch focus {
	case "editor":
		tab.Focus = app.PaneEditor
	case "document":
		tab.View = app.ViewTree
	case "plan":
		tab.View = app.ViewPlan
	case "detail":
		tab.View = app.ViewStatistics
	}
	return model, connection, tab
}

func applySequenceBindings(t *testing.T, model *Model, bindings map[string]string) {
	t.Helper()
	choices := cfg.ChordChoices{}
	for action, sequence := range bindings {
		choices[action] = []cfg.ChordSequence{mustSequence(t, sequence)}
	}
	model.registry.ApplyKeySettings(FindKeyPreset(""), choices, true)
}

func TestRouteWorkspaceGlobalSequences(t *testing.T) {
	for _, focus := range []string{"editor", "document", "plan", "detail"} {
		for _, action := range []string{"focus-sidebar", "show-activity"} {
			t.Run(focus+"/"+action, func(t *testing.T) {
				model, connection, tab := buildSequenceModel(t, focus)
				session := &watchedSession{offlineSession: connection.Session.(*offlineSession)}
				connection.Session = session
				text, caret, pane := tab.Editor.Text, tab.Editor.Caret, tab.Focus
				prefix, suffix := 'p', 's'
				if action == "show-activity" {
					prefix, suffix = 'o', 'a'
				}
				_, command := model.readWorkspaceKey(tea.Key{Code: prefix, Mod: uv.ModAlt})
				if !model.keymap.Pending() || command != nil || tab.Focus != pane {
					t.Fatal("the prefix did not wait")
				}
				_, command = model.readWorkspaceKey(tea.Key{Code: suffix, Text: string(suffix)})
				if model.keymap.Pending() {
					t.Error("the completed sequence is pending")
				}
				if tab.Editor.Text != text || tab.Editor.Caret != caret {
					t.Errorf("the sequence changed the editor to %q at %d", tab.Editor.Text, tab.Editor.Caret)
				}
				if action == "focus-sidebar" {
					if tab.Focus != app.PaneSidebar {
						t.Errorf("focus is %q, want sidebar", tab.Focus)
					}
				} else {
					if command == nil {
						t.Fatal("the activity sequence returned no command")
					}
					model.Update(command())
					if connection.Overlay.Kind != app.OverlayActivity || session.countReads() != 1 {
						t.Fatal("the sequence did not open activity")
					}
				}
			})
		}
	}
}

func TestRouteWorkspaceUserSequences(t *testing.T) {
	for _, focus := range []string{"editor", "document", "plan"} {
		t.Run(focus, func(t *testing.T) {
			model, _, tab := buildSequenceModel(t, focus)
			applySequenceBindings(t, model, map[string]string{"global:focus-sidebar": "alt+f9 x s"})
			text := tab.Editor.Text
			for _, key := range []tea.Key{{Code: tea.KeyF9, Mod: uv.ModAlt}, {Code: 'x', Text: "x"}} {
				_, command := model.readWorkspaceKey(key)
				if !model.keymap.Pending() || command != nil || tab.Editor.Text != text {
					t.Fatal("the incomplete user sequence did not wait")
				}
			}
			model.readWorkspaceKey(tea.Key{Code: 's', Text: "s"})
			if tab.Focus != app.PaneSidebar || model.keymap.Pending() || tab.Editor.Text != text {
				t.Fatal("the user sequence did not focus the sidebar")
			}
		})
	}
}

func TestRouteWorkspaceSequenceMismatch(t *testing.T) {
	for _, focus := range []string{"editor", "document", "plan"} {
		t.Run(focus, func(t *testing.T) {
			model, connection, tab := buildSequenceModel(t, focus)
			tab.Editor.PlaceCaret(len(tab.Editor.Text), false)
			text := tab.Editor.Text
			model.readWorkspaceKey(tea.Key{Code: 'p', Mod: uv.ModAlt})
			model.readWorkspaceKey(tea.Key{Code: '?', Text: "?"})
			if model.keymap.Pending() || connection.Overlay.IsOpen() {
				t.Fatal("the mismatch stayed pending or ran a standalone binding")
			}
			if focus == "editor" {
				text += "?"
			}
			if tab.Editor.Text != text {
				t.Errorf("the editor is %q, want %q", tab.Editor.Text, text)
			}
			model.readWorkspaceKey(tea.Key{Code: 'p', Mod: uv.ModAlt})
			model.readWorkspaceKey(tea.Key{Code: 's', Text: "s"})
			if tab.Focus != app.PaneSidebar {
				t.Fatal("the next sequence did not match")
			}
		})
	}
}

func TestRouteWorkspacePlainTyping(t *testing.T) {
	model, connection, tab := buildEditingModel(t, "", 0)
	applySequenceBindings(t, model, map[string]string{"global:focus-sidebar": "s"})
	for _, letter := range "s?.1[" {
		model.readWorkspaceKey(tea.Key{Code: letter, Text: string(letter)})
	}
	if tab.Editor.Text != "s?.1[" || tab.Focus != app.PaneEditor || connection.Overlay.IsOpen() || model.keymap.Pending() {
		t.Fatal("plain typing ran a binding")
	}
}

func TestRouteWorkspaceCompletionOwnsSequenceKeys(t *testing.T) {
	for _, key := range []tea.Key{{Code: tea.KeyDown}, {Code: tea.KeyTab}} {
		t.Run(key.String(), func(t *testing.T) {
			model, _, tab := buildListingModel(t, "select * from ")
			applySequenceBindings(t, model, map[string]string{"global:focus-sidebar": "alt+p " + key.String()})
			model.readWorkspaceKey(tea.Key{Code: 'p', Mod: uv.ModAlt})
			model.readWorkspaceKey(key)
			if tab.Focus != app.PaneEditor || !model.keymap.Pending() {
				t.Fatal("the sequence took a completion key")
			}
			if key.Code == tea.KeyDown && tab.Completion.Selected != 1 {
				t.Error("Down did not move the completion selection")
			}
			if key.Code == tea.KeyTab && (tab.Completion.IsListing() || tab.Editor.Text == "select * from ") {
				t.Error("Tab did not accept the completion")
			}
			model.readWorkspaceKey(tea.Key{Code: tea.KeyEscape})
			if model.keymap.Pending() || tab.Completion.IsListing() {
				t.Fatal("Escape did not clear the sequence and completion")
			}
		})
	}
}

func TestRouteWorkspaceGlobalPrecedence(t *testing.T) {
	for _, focus := range []string{"editor", "document", "plan"} {
		for _, sequence := range []string{"single", "sequence"} {
			t.Run(focus+"/"+sequence, func(t *testing.T) {
				model, _, tab := buildSequenceModel(t, focus)
				chord := "down"
				if sequence == "sequence" {
					chord = "alt+f9 down"
				}
				bindings := map[string]string{"global:focus-sidebar": chord, "list:cursor-down": chord}
				switch focus {
				case "editor":
					bindings["editor:caret-down"] = chord
				case "document":
					bindings["document:cursor-down"] = chord
				case "plan":
					bindings["plan:toggle-raw-plan"] = chord
				}
				applySequenceBindings(t, model, bindings)
				if sequence == "sequence" {
					model.readWorkspaceKey(tea.Key{Code: tea.KeyF9, Mod: uv.ModAlt})
					if !model.keymap.Pending() {
						t.Fatal("the shared prefix did not wait")
					}
				}
				model.readWorkspaceKey(tea.Key{Code: tea.KeyDown})
				if tab.Focus != app.PaneSidebar || tab.TreeRow != 0 || tab.DetailOffset != 0 || tab.RawPlan {
					t.Fatal("a pane action took the global binding")
				}
			})
		}
	}
}

func TestRouteWorkspaceDocumentAndDetailScrolling(t *testing.T) {
	for _, focus := range []string{"document", "plan"} {
		t.Run(focus, func(t *testing.T) {
			model, _, tab := buildSequenceModel(t, focus)
			model.readWorkspaceKey(tea.Key{Code: tea.KeyDown})
			if focus == "document" {
				if tab.TreeRow != 1 || tab.DetailOffset != 0 {
					t.Fatal("Down did not move the document cursor")
				}
			} else if tab.DetailOffset != 1 {
				t.Fatal("Down did not scroll the plan")
			}
			model.readWorkspaceKey(tea.Key{Code: tea.KeyPgDown})
			if focus == "document" && (tab.TreeRow != min(1+listPage, 19) || tab.DetailOffset != 0) {
				t.Fatal("Page Down did not move the document cursor")
			}
			if focus == "plan" && tab.DetailOffset != 1+listPage {
				t.Fatal("Page Down did not scroll the plan")
			}
			model.readWorkspaceKey(tea.Key{Code: tea.KeyHome})
			if focus == "document" && (tab.TreeRow != 0 || tab.DetailOffset != 0) {
				t.Fatal("Home did not move the document cursor")
			}
			if focus == "plan" && tab.DetailOffset != 0 {
				t.Fatal("Home did not scroll to the start of the plan")
			}
		})
	}
}

func TestRouteWorkspacePaneSequences(t *testing.T) {
	for _, focus := range []string{"document", "plan", "list", "document-list"} {
		t.Run(focus, func(t *testing.T) {
			model, _, tab := buildSequenceModel(t, "plan")
			action := "plan:toggle-raw-plan"
			if focus == "document" {
				tab.View = app.ViewTree
				action = "document:cursor-down"
			} else if focus == "list" || focus == "document-list" {
				action = "list:cursor-down"
				if focus == "document-list" {
					tab.View = app.ViewTree
				}
			}
			applySequenceBindings(t, model, map[string]string{action: "alt+f9 x"})
			model.readWorkspaceKey(tea.Key{Code: tea.KeyF9, Mod: uv.ModAlt})
			if !model.keymap.Pending() || tab.TreeRow != 0 || tab.DetailOffset != 0 || tab.RawPlan {
				t.Fatal("the pane sequence did not wait")
			}
			model.readWorkspaceKey(tea.Key{Code: 'x', Text: "x"})
			if model.keymap.Pending() {
				t.Fatal("the pane sequence is still pending")
			}
			if focus == "document" && tab.TreeRow != 1 || focus == "plan" && !tab.RawPlan {
				t.Fatal("the pane sequence did not run")
			}
			if (focus == "list" || focus == "document-list") && (tab.DetailOffset != 1 || tab.TreeRow != 0) {
				t.Fatal("the pane sequence did not run")
			}
		})
	}
}

func TestRouteWorkspaceIgnoresListChoiceBinding(t *testing.T) {
	model, _, tab := buildSequenceModel(t, "plan")
	applySequenceBindings(t, model, map[string]string{"list:choose-row": "r"})
	model.readWorkspaceKey(tea.Key{Code: 'r', Text: "r"})
	if !tab.RawPlan {
		t.Fatal("the list choice took the plan binding")
	}
}
