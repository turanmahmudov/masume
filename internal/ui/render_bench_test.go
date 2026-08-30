package ui

import (
	"os"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/turanmahmudov/masume/internal/cfg"
	"github.com/turanmahmudov/masume/internal/db/engines"
	"github.com/turanmahmudov/masume/internal/hist"
)

// buildFrameModel opens the config the environment names and reads the first relation, so the
// frame the benchmark draws is the one the user looks at.
func buildFrameModel(reporter testing.TB) *Model {
	reporter.Helper()
	configPath := os.Getenv("MASUME_SMOKE_CONFIG")
	if configPath == "" {
		reporter.Skip("MASUME_SMOKE_CONFIG names the config file to draw from")
	}
	loaded := cfg.LoadConfig(configPath)
	log, err := hist.Open(os.Getenv("MASUME_SMOKE_HISTORY"))
	if err != nil {
		reporter.Fatalf("the history file did not open: %v", err)
	}
	reporter.Cleanup(func() { _ = log.Close() })

	model := NewModel(loaded, engines.CreateAdapters(), log, nil)
	held, _ := model.Update(tea.WindowSizeMsg{Width: 120, Height: 34})
	model = held.(*Model)
	next, command := model.chooseProfile(model.profiles[0])
	model = next.(*Model)
	held, more := model.Update(command())
	model = held.(*Model)
	for _, follow := range drainCommand(more) {
		held, _ = model.Update(follow)
		model = held.(*Model)
	}
	connection := model.Active()
	table := connection.Catalog.Tables[0]
	preview := connection.Session.Composer().ComposeRelationRead(
		table, coreRewrite()).Display
	tab := connection.OpenTable(table, preview)
	held, command = model.runTabRead(connection, tab)
	model = held.(*Model)
	for _, follow := range drainCommand(command) {
		held, _ = model.Update(follow)
		model = held.(*Model)
	}
	return model
}

func BenchmarkViewOfARelation(bench *testing.B) {
	model := buildFrameModel(bench)
	for bench.Loop() {
		_ = model.View()
	}
}

func BenchmarkViewWhileDragging(bench *testing.B) {
	model := buildFrameModel(bench)
	model.selection = screenSelection{
		fromX: 40, fromY: 6, toX: 100, toY: 24, held: true,
		block:   blockRect{fromX: 37, toX: 118, fromY: 3, toY: 31},
		bounded: true,
	}
	for bench.Loop() {
		_ = model.View()
	}
}

func BenchmarkMouseMotion(bench *testing.B) {
	model := buildFrameModel(bench)
	model.selection = screenSelection{
		fromX: 40, fromY: 6, toX: 41, toY: 6, dragging: true,
		block:   blockRect{fromX: 37, toX: 118, fromY: 3, toY: 31},
		bounded: true,
	}
	at := 0
	for bench.Loop() {
		at = (at % 60) + 40
		held, _ := model.Update(tea.MouseMotionMsg{X: at, Y: 20, Button: tea.MouseLeft})
		model = held.(*Model)
		_ = model.View()
	}
}
