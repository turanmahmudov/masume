package cfg_test

import (
	"strings"
	"testing"

	"github.com/turanmahmudov/masume/internal/cfg"
)

// readSettings reads the interface settings from a config file.
func readSettings(t *testing.T, body string) cfg.UISettings {
	t.Helper()
	document, err := cfg.DecodeDocument(body)
	if err != nil {
		t.Fatalf("the config text does not read: %v", err)
	}
	return cfg.ParseUISettings(document)
}

// A colour is written as a hex value. Any other form must be reported and not ignored,
// because a theme with a missing colour gives an interface the user cannot read.
func TestIsHexColorReadsWhatATerminalCanDraw(t *testing.T) {
	for _, held := range []struct {
		value string
		want  bool
	}{
		{"#000000", true},
		{"#ffffff", true},
		{"#FFFFFF", true},
		{"#abc", true},
		{"#ABC", true},

		{"", false},
		{"000000", false},
		{"#12345", false},
		{"#1234567", false},
		{"#gggggg", false},
		{"red", false},
		{"#", false},
	} {
		if answered := cfg.IsHexColor(held.value); answered != held.want {
			t.Errorf("%q reads as a colour = %v, wanted %v", held.value, answered, held.want)
		}
	}
}

// A setting the file does not set keeps its default, so a file with one line changes one
// setting.
func TestParseUiSettingsKeepsTheDefaultsForWhatIsNotNamed(t *testing.T) {
	defaults := cfg.DefaultUISettings()
	held := readSettings(t, "[ui]\ntheme = \"nord\"\n")

	if held.Theme != "nord" {
		t.Errorf("the theme reads %q", held.Theme)
	}
	if held.IconSet != defaults.IconSet {
		t.Errorf("the icons read %q, wanted the default %q", held.IconSet, defaults.IconSet)
	}
	if held.HideSystemSchemas != defaults.HideSystemSchemas {
		t.Errorf("hiding the system schemas reads %v, wanted the default %v",
			held.HideSystemSchemas, defaults.HideSystemSchemas)
	}
}

func TestParseUiSettingsReadsEverySetting(t *testing.T) {
	held := readSettings(t, `
[ui]
theme = "gruvbox-dark"
icons = "ascii"
hide_system_schemas = true
`)

	if held.Theme != "gruvbox-dark" {
		t.Errorf("the theme reads %q", held.Theme)
	}
	if held.IconSet != cfg.IconsASCII {
		t.Errorf("the icons read %q, wanted the plain set", held.IconSet)
	}
	if !held.HideSystemSchemas {
		t.Error("hiding the system schemas was not read")
	}
}

// A colour set under `[ui]` is applied over the theme, so the user can change one colour
// without a theme file.
func TestParseUiSettingsReadsAColourLaidOverTheTheme(t *testing.T) {
	held := readSettings(t, `
[ui.colors]
accent = "#ff8800"
`)
	if len(held.ColorProblems) != 0 {
		t.Fatalf("a good colour reported %v", held.ColorProblems)
	}
	if held.Colors.Colors["accent"] != "#ff8800" {
		t.Errorf("the colour reads %q, wanted the one set", held.Colors.Colors["accent"])
	}
}

// A colour can name a palette entry instead of a hex value, so a theme defines a colour one
// time and uses it in several places. The name is resolved later, so it is kept unchanged
// here.
func TestParseUiSettingsTakesAColourThatNamesAPaletteEntry(t *testing.T) {
	held := readSettings(t, `
[ui.palette]
orange = "#ff8800"

[ui.colors]
accent = "orange"
`)
	if len(held.ColorProblems) != 0 {
		t.Fatalf("a colour naming a palette entry reported %v", held.ColorProblems)
	}
	if held.Colors.Colors["accent"] != "orange" {
		t.Errorf("the colour reads %q, wanted the name it was given",
			held.Colors.Colors["accent"])
	}
}

// A palette entry cannot be a palette name, because the names refer to this table. An entry
// that is not a hex value is reported and removed, so nothing resolves to it later.
func TestParseUiSettingsDropsAPaletteEntryThatIsNotHex(t *testing.T) {
	held := readSettings(t, `
[ui.palette]
orange = "not-a-colour"
`)
	if len(held.ColorProblems) == 0 {
		t.Fatal("a palette entry that is not hex was accepted")
	}
	if !strings.Contains(strings.Join(held.ColorProblems, " "), "orange") {
		t.Errorf("the problems read %v and do not name the entry", held.ColorProblems)
	}
	if _, there := held.Colors.Palette["orange"]; there {
		t.Error("the entry was reported and kept, so a colour could still resolve to it")
	}
}

// A colour that is not a string cannot be read, in any of the tables.
func TestParseUiSettingsReportsAColourNotWrittenAsText(t *testing.T) {
	held := readSettings(t, `
[ui.colors]
accent = 16711680
`)
	if len(held.ColorProblems) == 0 {
		t.Fatal("a colour written as a number was accepted")
	}
}

// A file without an `[ui]` section is the usual case, and every default applies.
func TestParseUiSettingsAnswersTheDefaultsForAFileWithNoSection(t *testing.T) {
	held := readSettings(t, "[profile.shop]\nengine = \"sqlite\"\n")
	defaults := cfg.DefaultUISettings()

	if held.IconSet != defaults.IconSet || held.Theme != defaults.Theme {
		t.Errorf("a file with no ui section read %+v", held)
	}
	if len(held.ColorProblems) != 0 {
		t.Errorf("a file with no ui section reported %v", held.ColorProblems)
	}
}

// An icon set name must match a set of the client, or the user gets the default set and no
// message about the reason.
func TestFindIconSetNameReadsTheNamesThereAre(t *testing.T) {
	held, is := cfg.FindIconSetName("ascii")
	if !is || held != cfg.IconsASCII {
		t.Errorf("the plain set reads as %q and %v", held, is)
	}
	// A set from an older version and a name that never existed are both invalid. Both
	// use the default and are reported.
	for _, written := range []string{"", "  ", "off", "nerdfont", "emoji"} {
		if _, is := cfg.FindIconSetName(written); is {
			t.Errorf("%q was read as a set of icons", written)
		}
	}
}

// A theme takes its name from its file name, so two files cannot have the same theme name.
// The errors in a file are reported to the user.
func TestParseThemeDocumentNamesTheThemeAndReportsWhatItCannotRead(t *testing.T) {
	document, err := cfg.DecodeDocument(`
title = "Held"

[palette]
orange = "not-a-colour"

[colors]
accent = "#ff8800"
`)
	if err != nil {
		t.Fatalf("the theme text does not read: %v", err)
	}

	parsed, problems := cfg.ParseThemeDocument(document, "held")
	if parsed.Name != "held" {
		t.Errorf("the theme is named %q, wanted the file name", parsed.Name)
	}
	if parsed.Title != "Held" {
		t.Errorf("the title reads %q", parsed.Title)
	}
	if len(problems) == 0 {
		t.Fatal("a palette entry that is not hex was accepted")
	}
	if !strings.Contains(strings.Join(problems, " "), "orange") {
		t.Errorf("the problems read %v and do not name the entry", problems)
	}
}

// A theme without a title uses its file name, so the picker still shows a name.
func TestParseThemeDocumentFallsBackToTheFileNameForATitle(t *testing.T) {
	document, err := cfg.DecodeDocument("[colors]\naccent = \"#ff8800\"\n")
	if err != nil {
		t.Fatalf("the theme text does not read: %v", err)
	}
	parsed, _ := cfg.ParseThemeDocument(document, "held")
	if parsed.Title != "held" {
		t.Errorf("the title reads %q, wanted the file name", parsed.Title)
	}
}

// A theme without errors reports no problem.
func TestParseThemeDocumentIsSilentForAGoodTheme(t *testing.T) {
	document, err := cfg.DecodeDocument("[colors]\naccent = \"#ff8800\"\n")
	if err != nil {
		t.Fatalf("the theme text does not read: %v", err)
	}
	if _, problems := cfg.ParseThemeDocument(document, "held"); len(problems) != 0 {
		t.Errorf("a good theme reported %v", problems)
	}
}

// An icon set in the config that the client does not have is reported, so the user sees a
// spelling error. The client continues with the set it started with.
func TestASetOfIconsThatIsNotThereIsReported(t *testing.T) {
	held := cfg.ParseUISettings(cfg.Table{
		"ui": map[string]any{"icons": "nerdfont"},
	})
	if held.IconSet != cfg.DefaultUISettings().IconSet {
		t.Errorf("the icons read %q, and a set that is not there changes nothing",
			held.IconSet)
	}
	if len(held.Problems) != 1 {
		t.Fatalf("the settings reported %d problems", len(held.Problems))
	}
	for _, said := range []string{"nerdfont", "plain", "ascii"} {
		if !strings.Contains(held.Problems[0], said) {
			t.Errorf("the report reads %q and does not name %q", held.Problems[0], said)
		}
	}
}

// An icon kind in the config that the client does not have is also reported, and the known
// kinds are still read.
func TestAKindOfIconThatIsNotThereIsReported(t *testing.T) {
	held := cfg.ParseUISettings(cfg.Table{
		"ui": map[string]any{
			"icon_glyphs": map[string]any{"table": "T", "tabel": "X"},
		},
	})
	if held.IconGlyphs[cfg.IconTable] != "T" {
		t.Errorf("the glyph of a table reads %q", held.IconGlyphs[cfg.IconTable])
	}
	if len(held.Problems) != 1 {
		t.Fatalf("the settings reported %d problems: %v", len(held.Problems), held.Problems)
	}
	if !strings.Contains(held.Problems[0], "tabel") {
		t.Errorf("the report reads %q", held.Problems[0])
	}
}

// Every kind the client draws can be set in the config file, so `[ui.icon_glyphs]` covers
// every icon.
func TestEveryKindTheConfigMayNameIsAKindThereIs(t *testing.T) {
	for _, kind := range cfg.IconKinds {
		if !cfg.IsIconKind(string(kind)) {
			t.Errorf("the kind %q is drawn but cannot be named in the config", kind)
		}
	}
}
