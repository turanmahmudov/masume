package ui

import (
	"os"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/turanmahmudov/masume/internal/app"
	"github.com/turanmahmudov/masume/internal/cfg"
	"github.com/turanmahmudov/masume/internal/core"
	"github.com/turanmahmudov/masume/internal/db/engines"
	"github.com/turanmahmudov/masume/internal/hist"
)

// TestFrameDraws opens the config the environment names, walks into the first profile, and
// draws a frame of every screen it reaches. It reports what it drew, so a reader can compare
// the frames with the client this one was ported from.
func TestFrameDraws(t *testing.T) {
	configPath := os.Getenv("MASUME_SMOKE_CONFIG")
	if configPath == "" {
		t.Skip("MASUME_SMOKE_CONFIG names the config file to draw from")
	}
	loaded := cfg.LoadConfig(configPath)

	log, err := hist.Open(os.Getenv("MASUME_SMOKE_HISTORY"))
	if err != nil {
		t.Fatalf("the history file did not open: %v", err)
	}
	defer func() { _ = log.Close() }()

	model := NewModel(loaded, engines.CreateAdapters(), log, nil)
	held, _ := model.Update(tea.WindowSizeMsg{Width: 120, Height: 34})
	model = held.(*Model)

	t.Log("\n" + model.View().Content)

	// The first profile of the file is opened, so the workspace draws with real rows.
	if len(model.profiles) == 0 {
		t.Fatal("the config file named no profile")
	}
	next, command := model.chooseProfile(model.profiles[0])
	model = next.(*Model)
	if command == nil {
		t.Fatal("choosing a profile asked for no work")
	}

	answered := command()
	held, more := model.Update(answered)
	model = held.(*Model)
	if model.screen != ScreenWorking {
		t.Fatalf("the connection did not open: %s", model.picker.problem)
	}

	// The catalog read and the marks read both answer, so the tree draws its rows.
	for _, follow := range drainCommand(more) {
		held, _ = model.Update(follow)
		model = held.(*Model)
	}
	t.Log("\n" + model.View().Content)

	// The tree is opened on its first schema, and the first relation of it is read.
	connection := model.Active()
	if connection == nil {
		t.Fatal("no connection is on screen")
	}
	if len(connection.Catalog.Tables) == 0 {
		t.Fatal("the catalog read no relation")
	}

	table := connection.Catalog.Tables[0]
	preview := connection.Session.Composer().ComposeRelationRead(table, coreRewrite()).Display
	tab := connection.OpenTable(table, preview)
	held, command = model.runTabRead(connection, tab)
	model = held.(*Model)
	for _, follow := range drainCommand(command) {
		held, _ = model.Update(follow)
		model = held.(*Model)
	}

	frame := model.View().Content
	t.Log("\n" + frame)
	if !strings.Contains(frame, table.Name) {
		t.Errorf("the frame does not name the relation it opened: %s", table.Name)
	}

	// A word typed in the editor offers the names the catalog holds.
	query := connection.OpenQueryTab("")
	query.Focus = app.PaneEditor
	for _, letter := range "select * from al" {
		held, _ = model.Update(tea.KeyPressMsg{Code: letter, Text: string(letter)})
		model = held.(*Model)
	}
	t.Log("\n" + model.View().Content)
	if !query.Completion.IsListing() {
		t.Error("the editor offered no completion for a word it knows")
	}
}

// drainCommand runs a command and every command it batched, so the frame is drawn with every
// answer already in.
func drainCommand(command tea.Cmd) []tea.Msg {
	if command == nil {
		return nil
	}
	answered := command()
	switch held := answered.(type) {
	case nil:
		return nil
	case tea.BatchMsg:
		messages := []tea.Msg{}
		for _, one := range held {
			messages = append(messages, drainCommand(one)...)
		}
		return messages
	}
	return []tea.Msg{answered}
}

// coreRewrite answers a read with no sort and no filter.
func coreRewrite() core.ReadRewrite { return core.ReadRewrite{} }
