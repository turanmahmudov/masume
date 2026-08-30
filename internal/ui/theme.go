package ui

import (
	"image/color"

	"charm.land/lipgloss/v2"

	"github.com/turanmahmudov/masume/internal/cfg"
)

// Styles holds the theme applied and the styles built out of it. One is built per frame, so
// a theme change repaints every pane.
type Styles struct {
	Theme    Theme
	registry *ThemeRegistry
	// revision counts how many times a theme was installed, so anything that keeps what
	// a theme drew can tell that it has to draw it again.
	revision int
}

// keepTheme installs a theme. Every path that changes the colours of the client goes through
// here, so the count below it stands for all of them.
func (styles *Styles) keepTheme(theme Theme) {
	styles.Theme = theme
	styles.revision++
}

// Revision returns how many times a theme was installed.
func (styles *Styles) Revision() int {
	return styles.revision
}

// NewStyles applies the fallback theme, so the first frame has colours before the config is
// read.
func NewStyles(registry *ThemeRegistry) *Styles {
	styles := &Styles{registry: registry}
	resolved, found := registry.FindResolvedTheme(FallbackThemeName)
	if found {
		styles.keepTheme(resolved.Theme)
	}
	return styles
}

// ApplyThemeByName applies the theme of that name, and reports the problems in its files. It
// returns false where there is no theme of that name, and the applied theme does not change.
func (styles *Styles) ApplyThemeByName(name string) ([]string, bool) {
	resolved, found := styles.registry.FindResolvedTheme(name)
	if !found {
		return nil, false
	}
	styles.keepTheme(resolved.Theme)
	return resolved.Problems, true
}

// ApplyColorOverrides keeps the colours from `[ui]`, which are laid over the chosen theme.
func (styles *Styles) ApplyColorOverrides(tables cfg.ThemeTables) []string {
	styles.registry.ApplyOverrides(tables)
	resolved, found := styles.registry.FindResolvedTheme(styles.Theme.Name)
	if !found {
		return nil
	}
	styles.keepTheme(resolved.Theme)
	return resolved.Problems
}

// ApplyTerminalColors keeps the colours the terminal reported, and repaints if the system
// theme is applied. The answer arrives after the first frame, so the theme is built from the
// standard palette first and from the terminal colours after.
func (styles *Styles) ApplyTerminalColors(colors TerminalColors) {
	styles.registry.KeepTerminalColors(colors)
	if styles.FollowsTerminal() {
		styles.keepTheme(styles.registry.ResolveSystemTheme().Theme)
	}
}

// FollowsTerminal is true if the applied theme takes its colours from the terminal.
func (styles *Styles) FollowsTerminal() bool {
	return styles.Theme.Name == SystemThemeName
}

// InkOn returns the ink for text on a ground filled with a theme colour. The accent has
// OnAccent for this. Every other fill is resolved here, because one ink cannot be read on
// both a dark blue and a mid amber.
func (styles *Styles) InkOn(ground color.Color) color.Color {
	return PickInkFor(ground, styles.Theme.Panel, styles.Theme.Text, TextContrastFloor)
}

// EnvironmentColor returns the colour of the environment a connection is in.
func (styles *Styles) EnvironmentColor(environment cfg.Environment) color.Color {
	switch environment {
	case cfg.EnvironmentProd:
		return styles.Theme.EnvProd
	case cfg.EnvironmentTest:
		return styles.Theme.EnvTest
	}
	return styles.Theme.EnvDev
}

// Ink returns text in the ordinary colour.
func (styles *Styles) Ink() lipgloss.Style {
	return lipgloss.NewStyle().Foreground(styles.Theme.Text)
}

// Muted returns a label beside a value: one step down from the text.
func (styles *Styles) Muted() lipgloss.Style {
	return lipgloss.NewStyle().Foreground(styles.Theme.Muted)
}

// Faint returns what is there and is not being read: a third step down.
func (styles *Styles) Faint() lipgloss.Style {
	return lipgloss.NewStyle().Foreground(styles.Theme.Faint)
}

// Accent returns where the user is: the row under the cursor, the tab in front.
func (styles *Styles) Accent() lipgloss.Style {
	return lipgloss.NewStyle().Foreground(styles.Theme.Accent)
}

// Error returns a failure, which is kept off the production mark.
func (styles *Styles) Error() lipgloss.Style {
	return lipgloss.NewStyle().Foreground(styles.Theme.Error)
}

// Filled returns text on a ground of that colour, with the ink that can be read on it.
func (styles *Styles) Filled(ground color.Color) lipgloss.Style {
	return lipgloss.NewStyle().Background(ground).Foreground(styles.InkOn(ground))
}

// Bold returns a style that draws its text heavier, on the ground given.
func (styles *Styles) Bold(ink, ground color.Color) lipgloss.Style {
	return lipgloss.NewStyle().Foreground(ink).Background(ground).Bold(true)
}

// resolveSyntaxOpening returns the escapes that open one kind of token on this ground. A
// kind the theme gives a ground of its own keeps it, and every other kind takes the ground
// of the block it is drawn in.
func (styles *Styles) resolveSyntaxOpening(kind HighlightKind, ground color.Color) string {
	held := styles.Theme.Syntax[kind]
	var ink color.Color
	if held.HasForeground {
		ink = held.Foreground
	}
	if held.HasBackground {
		ground = held.Background
	}
	key := paintKey{
		ink: packColor(ink), ground: packColor(ground),
		bold: held.Bold, italic: held.Italic, underline: held.Underline,
	}
	if opening, found := findOpening(key); found {
		return opening
	}
	return keepOpening(key, styles.HighlightStyleOf(kind).Background(ground))
}

// HighlightStyleOf returns the style of one highlight in the applied theme.
func (styles *Styles) HighlightStyleOf(kind HighlightKind) lipgloss.Style {
	held := styles.Theme.Syntax[kind]
	style := lipgloss.NewStyle()
	if held.HasForeground {
		style = style.Foreground(held.Foreground)
	}
	if held.HasBackground {
		style = style.Background(held.Background)
	}
	if held.Bold {
		style = style.Bold(true)
	}
	if held.Italic {
		style = style.Italic(true)
	}
	if held.Underline {
		style = style.Underline(true)
	}
	return style
}

// roundedBorder is the frame every pane and card is drawn in.
var roundedBorder = lipgloss.RoundedBorder()

// Panel returns a pane with a border that shows whether the user is in it.
func (styles *Styles) Panel(focused bool) lipgloss.Style {
	frame := styles.Theme.Border
	if focused {
		frame = styles.Theme.BorderFocus
	}
	return lipgloss.NewStyle().
		Border(roundedBorder).
		BorderForeground(frame).
		BorderBackground(styles.Theme.Panel).
		Background(styles.Theme.Panel).
		Foreground(styles.Theme.Text)
}

// Card returns the card of a dialog or a full screen. A card that asks about something that
// cannot be undone is framed in the error colour, so the kind of question is clear before
// the words are.
func (styles *Styles) Card(destructive bool) lipgloss.Style {
	frame := styles.Theme.BorderFocus
	if destructive {
		frame = styles.Theme.Error
	}
	return lipgloss.NewStyle().
		Border(roundedBorder).
		BorderForeground(frame).
		BorderBackground(styles.Theme.Panel).
		Background(styles.Theme.Panel).
		Foreground(styles.Theme.Text).
		Padding(1, 1)
}
