package ui

import (
	"testing"
	"time"

	uv "github.com/charmbracelet/ultraviolet"

	"github.com/turanmahmudov/masume/internal/cfg"
)

func TestReadPaletteAnswer(t *testing.T) {
	cases := []struct {
		written string
		slot    int
		hex     string
		answers bool
	}{
		{"\x1b]4;3;rgb:cdcd/cdcd/0000\a", 3, "#cdcd00", true},
		{"\x1b]4;12;rgb:5c5c/5c5c/ffff\x1b\\", 12, "#5c5cff", true},
		{"\x1b]4;0;#102030\a", 0, "#102030", true},
		{"\x1b]11;rgb:0000/0000/0000\a", 0, "", false},
		{"\x1b]4;99;rgb:0000/0000/0000\a", 0, "", false},
		{"\x1b]4;3\a", 0, "", false},
	}
	for _, held := range cases {
		slot, answered, found := readPaletteAnswer(uv.UnknownOscEvent(held.written))
		if found != held.answers {
			t.Errorf("%q answered %v, wanted %v", held.written, found, held.answers)
			continue
		}
		if !found {
			continue
		}
		if slot != held.slot || WriteHex(answered) != held.hex {
			t.Errorf("%q gave slot %d and %s, wanted slot %d and %s",
				held.written, slot, WriteHex(answered), held.slot, held.hex)
		}
	}
}

func TestRankHueSkipsGrey(t *testing.T) {
	registry := NewThemeRegistry()
	colors := TerminalColors{}
	// Solarized puts a slate grey where the bright green belongs, so the plain slot wins.
	colors.Palette[2], colors.HasPalette[2] = rgb(0x85, 0x99, 0x00), true
	colors.Palette[10], colors.HasPalette[10] = rgb(0x58, 0x6e, 0x75), true
	registry.KeepTerminalColors(colors)

	first, second := registry.rankHue("green", rgb(0xfd, 0xf6, 0xe3))
	if WriteHex(first) != "#859900" {
		t.Errorf("the first green is %s, wanted #859900", WriteHex(first))
	}
	if WriteHex(second) != "#586e75" {
		t.Errorf("the second green is %s, wanted #586e75", WriteHex(second))
	}
}

func TestBuildSystemColorsReadsThePalette(t *testing.T) {
	registry := NewThemeRegistry()
	colors := TerminalColors{
		Background: rgb(0x1f, 0x1f, 0x1f), HasBackground: true,
		Foreground: rgb(0xff, 0xff, 0xff), HasForeground: true,
	}
	// A terminal whose blue is a plain slot with a strong hue.
	colors.Palette[4], colors.HasPalette[4] = rgb(0x3a, 0x3a, 0xff), true
	colors.Palette[12], colors.HasPalette[12] = rgb(0x3a, 0x3a, 0xff), true
	registry.KeepTerminalColors(colors)

	built := registry.buildSystemColors()
	if WriteHex(built.Background) != "#1f1f1f" {
		t.Errorf("the ground is %s, wanted #1f1f1f", WriteHex(built.Background))
	}
	// The accent keeps the hue of the terminal, and is raised until it can be read.
	if angle := CalculateHueAngle(built.Accent); angle < 220 || angle > 260 {
		t.Errorf("the accent sits at %.0f degrees, wanted a blue", angle)
	}
	if contrast := CalculateContrastRatio(built.Accent, built.Background); contrast < markContrastFloor {
		t.Errorf("the accent reads at %.2f, wanted %.1f at least", contrast, markContrastFloor)
	}
	// The two quieter steps stay between the ground and the ink, in order.
	inkContrast := CalculateContrastRatio(built.Text, built.Background)
	mutedContrast := CalculateContrastRatio(built.Muted, built.Background)
	faintContrast := CalculateContrastRatio(built.Faint, built.Background)
	if !(faintContrast < mutedContrast && mutedContrast < inkContrast) {
		t.Errorf("the steps are %.2f, %.2f and %.2f, wanted them in order",
			faintContrast, mutedContrast, inkContrast)
	}
}

// loadedConfigForTest answers a config that names one theme and nothing else.
func loadedConfigForTest(theme string) cfg.LoadedConfig {
	loaded := cfg.LoadedConfig{Ai: cfg.DefaultAiConfig()}
	loaded.Settings.Theme = theme
	return loaded
}

// answerEveryTerminalColor gives the model an answer for the ground, the ink and every slot.
func answerEveryTerminalColor(model *Model) {
	model.terminal.keepGround(rgb(0x2e, 0x34, 0x40))
	model.terminal.keepInk(rgb(0xd8, 0xde, 0xe9))
	for slot := range paletteSlots {
		model.terminal.keepSlot(slot, rgb(0x81, 0xa1, 0xc1))
	}
}

func TestFirstFrameWaitsForTheTerminal(t *testing.T) {
	model := &Model{
		terminal: terminalColorState{waiting: true}, styles: NewStyles(NewThemeRegistry()),
	}
	if drawn := model.View().Content; drawn != "" {
		t.Errorf("the first frame drew %q, wanted nothing while the terminal is unanswered",
			drawn)
	}
	answerEveryTerminalColor(model)
	model.applyTerminalColors()
	if model.terminal.waiting {
		t.Error("the frame still waits after every colour arrived")
	}
}

func TestTheTerminalIsAskedAgainWithADoublingWait(t *testing.T) {
	model := NewModel(loadedConfigForTest("system"), nil, nil, nil)
	if !model.styles.FollowsTerminal() {
		t.Fatal("the system theme is not the applied one")
	}

	// The first ask comes one second in, and each wait after it is twice the last.
	start := time.Unix(0, 0)
	asked := []int{}
	for second := 1; second <= 40; second++ {
		if model.askTerminalAgain(start) != nil {
			asked = append(asked, second)
		}
	}
	wanted := []int{1, 3, 7, 15, 31}
	if len(asked) != len(wanted) {
		t.Fatalf("the terminal was asked at %v, wanted %v", asked, wanted)
	}
	for at, second := range wanted {
		if asked[at] != second {
			t.Errorf("the terminal was asked at %v, wanted %v", asked, wanted)
			break
		}
	}

	// Once every colour has arrived the terminal is read again on a slow beat, because it
	// reports nothing when a slot of its palette is edited.
	answerEveryTerminalColor(model)
	model.terminal.askAgainIn = 1
	model.terminal.watchedAt = start
	if model.askTerminalAgain(start) != nil {
		t.Error("the terminal was read again before the beat came round")
	}
	if model.askTerminalAgain(start.Add(watchTerminalWait)) == nil {
		t.Error("the terminal was not read again once the beat came round")
	}
	if model.askTerminalAgain(start.Add(watchTerminalWait)) != nil {
		t.Error("the terminal was read again twice on one beat")
	}
}

func TestAnExplicitThemeIsNotAskedFor(t *testing.T) {
	model := NewModel(loadedConfigForTest("nord"), nil, nil, nil)
	model.terminal.askAgainIn = 1
	if model.askTerminalAgain(time.Unix(0, 0)) != nil {
		t.Error("a theme that does not follow the terminal asked it for its colours")
	}
	answerEveryTerminalColor(model)
	if model.askTerminalAgain(time.Unix(0, 0).Add(time.Hour)) != nil {
		t.Error("a theme that does not follow the terminal was read again on the beat")
	}
}
