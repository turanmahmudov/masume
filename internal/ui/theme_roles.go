package ui

import (
	"image/color"

	"github.com/turanmahmudov/masume/internal/cfg"
	"github.com/turanmahmudov/masume/internal/query/syntax"
)

// The words of a theme: every colour it names and every highlight it can style. A theme
// file uses these words, and no other.

// ThemeColors holds the colours of one theme, each named for its use, not for its hue.
type ThemeColors struct {
	Background  color.Color
	Panel       color.Color
	Header      color.Color
	Zebra       color.Color
	Border      color.Color
	BorderFocus color.Color
	// The ground of selected text, which the editor draws under the colours of the tokens.
	Selection color.Color
	Text      color.Color
	Muted     color.Color
	// One step below Muted, kept at 3:1 against the panel.
	Faint  color.Color
	Accent color.Color
	// A second accent: a schema, a primary key, a type.
	AccentAlt color.Color
	// The third accent hue, warmer than the other two.
	AccentWarm color.Color
	// The ink on a ground filled with the accent.
	OnAccent color.Color
	Info     color.Color
	Success  color.Color
	Warning  color.Color
	Danger   color.Color
	// Kept apart from Danger, so a failure and a production mark never look the same.
	Error   color.Color
	EnvDev  color.Color
	EnvTest color.Color
	EnvProd color.Color
}

// HighlightKind names a highlight a theme can style: every token kind, the fault mark, the
// bracket pair and the indent guide.
type HighlightKind string

// The three highlights that are not a token kind. Each is drawn over the colour the text
// already has.
const (
	// ProblemStyle marks what the scanner or the server objected to.
	ProblemStyle HighlightKind = "problem"
	// BracketStyle marks the bracket at the caret, and the one that closes it.
	BracketStyle HighlightKind = "bracket"
	// GuideStyle marks the indent step of a line, drawn on the space that is already there.
	GuideStyle HighlightKind = "guide"
	// MatchStyle marks what a search of the statement found.
	MatchStyle HighlightKind = "match"
)

// HighlightKinds lists every highlight a theme file may write under `[syntax]`.
var HighlightKinds = []HighlightKind{
	HighlightKind(syntax.TokenKeyword),
	HighlightKind(syntax.TokenType),
	HighlightKind(syntax.TokenString),
	HighlightKind(syntax.TokenComment),
	HighlightKind(syntax.TokenNumber),
	HighlightKind(syntax.TokenIdentifier),
	HighlightKind(syntax.TokenQuoted),
	HighlightKind(syntax.TokenOperator),
	HighlightKind(syntax.TokenParameter),
	ProblemStyle, BracketStyle, GuideStyle, MatchStyle,
}

// HighlightStyle is one highlight, resolved to the colours it is drawn with.
type HighlightStyle struct {
	Foreground    color.Color
	HasForeground bool
	Background    color.Color
	HasBackground bool
	Bold          bool
	Italic        bool
	Underline     bool
}

// Theme is a whole theme: its name, its colours, and how it colours SQL.
type Theme struct {
	ThemeColors
	Name string
	// The title a person reads, such as "Tokyo Night".
	Title      string
	Appearance cfg.Appearance
	Syntax     map[HighlightKind]HighlightStyle
}

// colorsDraft holds the colours as they are written, before the derived ones are added.
type colorsDraft struct {
	ThemeColors
	hasOnAccent  bool
	hasSelection bool
	hasEnvDev    bool
	hasEnvTest   bool
	hasEnvProd   bool
}

// buildThemeColors fills in the colours a theme need not write. The accent ink is
// whichever of the pane ground and the text can be read on the accent, and an environment
// colour follows the colour of the same meaning. A theme that names its own keeps it.
func buildThemeColors(draft colorsDraft) ThemeColors {
	colors := draft.ThemeColors
	if !draft.hasSelection {
		colors.Selection = MixColors(colors.Panel, colors.Text, selectionWeight)
	}
	if !draft.hasOnAccent {
		colors.OnAccent = PickInkFor(
			colors.Accent, colors.Panel, colors.Text, TextContrastFloor)
	}
	if !draft.hasEnvDev {
		colors.EnvDev = colors.Success
	}
	if !draft.hasEnvTest {
		colors.EnvTest = colors.Warning
	}
	if !draft.hasEnvProd {
		colors.EnvProd = colors.Danger
	}
	return colors
}

// writtenColors names what each colour is written as in a theme file.
var writtenColors = []struct {
	written string
	read    func(*ThemeColors) *color.Color
}{
	{"background", func(colors *ThemeColors) *color.Color { return &colors.Background }},
	{"panel", func(colors *ThemeColors) *color.Color { return &colors.Panel }},
	{"header", func(colors *ThemeColors) *color.Color { return &colors.Header }},
	{"zebra", func(colors *ThemeColors) *color.Color { return &colors.Zebra }},
	{"border", func(colors *ThemeColors) *color.Color { return &colors.Border }},
	{"border_focus", func(colors *ThemeColors) *color.Color { return &colors.BorderFocus }},
	{"selection", func(colors *ThemeColors) *color.Color { return &colors.Selection }},
	{"text", func(colors *ThemeColors) *color.Color { return &colors.Text }},
	{"muted", func(colors *ThemeColors) *color.Color { return &colors.Muted }},
	{"faint", func(colors *ThemeColors) *color.Color { return &colors.Faint }},
	{"accent", func(colors *ThemeColors) *color.Color { return &colors.Accent }},
	{"accent_alt", func(colors *ThemeColors) *color.Color { return &colors.AccentAlt }},
	{"accent_warm", func(colors *ThemeColors) *color.Color { return &colors.AccentWarm }},
	{"on_accent", func(colors *ThemeColors) *color.Color { return &colors.OnAccent }},
	{"info", func(colors *ThemeColors) *color.Color { return &colors.Info }},
	{"success", func(colors *ThemeColors) *color.Color { return &colors.Success }},
	{"warning", func(colors *ThemeColors) *color.Color { return &colors.Warning }},
	{"danger", func(colors *ThemeColors) *color.Color { return &colors.Danger }},
	{"error", func(colors *ThemeColors) *color.Color { return &colors.Error }},
	{"env_dev", func(colors *ThemeColors) *color.Color { return &colors.EnvDev }},
	{"env_test", func(colors *ThemeColors) *color.Color { return &colors.EnvTest }},
	{"env_prod", func(colors *ThemeColors) *color.Color { return &colors.EnvProd }},
}

// The colours a theme need not write, because they are derived from the others.
var derivedColors = map[string]bool{
	"on_accent": true, "env_dev": true, "env_test": true, "env_prod": true,
	"selection": true,
}

// selectionWeight is how far the ground of a selection stands from the ground of the pane,
// toward the text. It reads the same way in a light theme and a dark one, and it is a small
// enough step that the colour of every token stays readable on it.
const selectionWeight = 0.25
