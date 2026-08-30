package ui

import (
	"image/color"
	"math"

	"charm.land/lipgloss/v2"

	"github.com/turanmahmudov/masume/internal/cfg"
)

// The theme worked out from what the terminal answered: the ground it paints, the ink it
// writes in, and the sixteen slots of its palette. Every surface is mixed from the ground
// and the ink, and each semantic hue is taken from the pair of slots that holds it.

// The share of the way from the ground toward the ink that each surface sits at.
const (
	zebraWeight  = 0.06
	headerWeight = 0.12
	borderWeight = 0.22
)

// The two quieter text steps, as a power of the contrast between the ink and the ground of
// the terminal. A power keeps the steps in order and below the ink, and it shrinks with the
// contrast the terminal has. A floor cannot: Solarized Light writes its body text at 4.1:1,
// so a floor of 3 makes all three steps the same.
const (
	mutedPower = 0.75
	faintPower = 0.55
)

// The least contrast a semantic colour must have on the ground to be read.
const markContrastFloor = 3.0

// Which slots hold each hue, and where that hue sits on the wheel. A slot is judged against
// the angle, because a terminal may put any colour in a bright slot: Solarized puts a slate
// grey where the bright green belongs.
var paletteHues = map[string]struct {
	plain, bright int
	angle         float64
}{
	"red":     {1, 9, 0},
	"yellow":  {3, 11, 60},
	"green":   {2, 10, 120},
	"cyan":    {6, 14, 180},
	"blue":    {4, 12, 240},
	"magenta": {5, 13, 300},
}

// Below this saturation a slot holds a grey, which cannot serve as a hue.
const leastSaturation = 0.2

// Two slots this close in hue are ranked by contrast instead.
const hueTieDegrees = 20.0

// How far the second red moves toward the amber where both reds are the same colour, and
// where the warm accent sits between the red and the amber.
const (
	secondRedMix = 0.25
	warmMix      = 0.5
)

// TerminalColors is what the terminal answered about itself.
type TerminalColors struct {
	Background    color.Color
	HasBackground bool
	Foreground    color.Color
	HasForeground bool
	// The slots of the palette, and which of them the terminal answered for.
	Palette    [paletteSlots]color.Color
	HasPalette [paletteSlots]bool
}

// standardGround and standardInk are used where the terminal returns nothing. They are the
// ground and the ink a terminal has before anything changes them.
var (
	standardGround = lipgloss.Color("#000000")
	standardInk    = lipgloss.Color("#ffffff")
)

// KeepTerminalColors keeps what the terminal reported, so the system theme is built from it.
func (registry *ThemeRegistry) KeepTerminalColors(colors TerminalColors) {
	registry.terminal = colors
}

// systemAppearance returns whether the terminal paints a dark ground or a light one.
func (registry *ThemeRegistry) systemAppearance() cfg.Appearance {
	ground, _ := registry.readTerminalPair()
	if CalculateRelativeLuminance(ground) > 0.5 {
		return cfg.AppearanceLight
	}
	return cfg.AppearanceDark
}

// readTerminalPair returns the two colours every terminal has: the ground it paints and the
// ink it writes in.
func (registry *ThemeRegistry) readTerminalPair() (color.Color, color.Color) {
	ground := color.Color(standardGround)
	ink := color.Color(standardInk)
	if registry.terminal.HasBackground {
		ground = registry.terminal.Background
	}
	if registry.terminal.HasForeground {
		ink = registry.terminal.Foreground
	}
	return ground, ink
}

// readSlot returns the colour of one palette slot, or the standard colour of that slot where
// the terminal answered nothing for it.
func (registry *ThemeRegistry) readSlot(slot int) color.Color {
	if slot < 0 || slot >= paletteSlots {
		return standardPalette[0]
	}
	if registry.terminal.HasPalette[slot] {
		return registry.terminal.Palette[slot]
	}
	return standardPalette[slot]
}

// rankHue returns the two slots of one hue, the better one first. A slot with a grey loses
// to one that holds the hue. Between two hues the nearer angle wins, and two close angles
// are ranked by contrast on this ground. That puts the bright slot first on a dark terminal
// and the plain slot first on a light one.
func (registry *ThemeRegistry) rankHue(name string, ground color.Color) (color.Color, color.Color) {
	held, known := paletteHues[name]
	if !known {
		return standardPalette[0], standardPalette[0]
	}
	plain, bright := registry.readSlot(held.plain), registry.readSlot(held.bright)

	plainIsGrey := CalculateSaturation(plain) < leastSaturation
	brightIsGrey := CalculateSaturation(bright) < leastSaturation
	if plainIsGrey != brightIsGrey {
		if plainIsGrey {
			return bright, plain
		}
		return plain, bright
	}

	plainDistance := CalculateHueDistance(CalculateHueAngle(plain), held.angle)
	brightDistance := CalculateHueDistance(CalculateHueAngle(bright), held.angle)
	if math.Abs(plainDistance-brightDistance) > hueTieDegrees {
		if plainDistance < brightDistance {
			return plain, bright
		}
		return bright, plain
	}

	if CalculateContrastRatio(bright, ground) > CalculateContrastRatio(plain, ground) {
		return bright, plain
	}
	return plain, bright
}

// buildSystemColors works the colours out from what the terminal answered.
func (registry *ThemeRegistry) buildSystemColors() ThemeColors {
	ground, ink := registry.readTerminalPair()
	inkContrast := CalculateContrastRatio(ink, ground)

	// The zebra stripe, the header and the border are the ground mixed toward the ink.
	colors := ThemeColors{
		Background: ground,
		Panel:      ground,
		Zebra:      MixColors(ground, ink, zebraWeight),
		Header:     MixColors(ground, ink, headerWeight),
		Border:     MixColors(ground, ink, borderWeight),
		Text:       ink,

		// The two quieter text steps are the ink brought back toward the ground, each to a
		// power of the contrast the ink of the terminal has, so a terminal that writes at 4:1
		// keeps three steps apart rather than landing all three on the ink.
		Muted: ResolveColorAtContrast(
			ink, ground, ground, math.Pow(inkContrast, mutedPower)),
		Faint: ResolveColorAtContrast(
			ink, ground, ground, math.Pow(inkContrast, faintPower))}

	// Each semantic colour comes from the slot of the terminal that holds its hue, and is
	// pulled toward the ink until it can be read on the ground.
	raise := func(held color.Color) color.Color {
		return RaiseContrast(held, ground, ink, markContrastFloor)
	}
	readHue := func(name string) color.Color {
		first, _ := registry.rankHue(name, ground)
		return raise(first)
	}

	colors.Accent = readHue("blue")
	colors.AccentAlt = readHue("magenta")
	colors.Info = readHue("cyan")
	colors.Success = readHue("green")
	colors.Warning = readHue("yellow")

	firstRed, secondRed := registry.rankHue("red", ground)
	colors.Danger = raise(firstRed)
	// Some terminals put the same colour in both red slots. A failure and a production
	// mark must never read as one thing, so the second one moves toward the amber.
	if sameColor(secondRed, firstRed) {
		secondRed = MixColors(secondRed, colors.Warning, secondRedMix)
	}
	colors.Error = raise(secondRed)
	// A terminal has a yellow where an orange is wanted, so the hue between the amber and
	// the red is mixed.
	colors.AccentWarm = raise(MixColors(colors.Danger, colors.Warning, warmMix))
	colors.BorderFocus = colors.Accent
	return colors
}

// ResolveSystemTheme returns the theme that follows the terminal. Its colours are derived,
// it takes its highlights from the fallback theme, and `[ui]` applies to it as to any theme.
func (registry *ThemeRegistry) ResolveSystemTheme() ResolvedTheme {
	fallbackChain, problems := registry.collectChain(FallbackThemeName)
	fallback, found := registry.mergeChain(fallbackChain)
	if !found {
		return ResolvedTheme{Problems: problems}
	}
	derived := registry.buildSystemColors()

	written := map[string]string{}
	for _, entry := range writtenColors {
		held := derived
		if value := *entry.read(&held); value != nil {
			written[entry.written] = WriteHex(value)
		}
	}

	merged := mergedTheme{
		ThemeTables: mergeTables(registry.overrides, cfg.ThemeTables{
			Palette: fallback.Palette, Colors: written, Syntax: fallback.Syntax,
		}),
		name: SystemThemeName, title: systemThemeTitle,
		appearance: registry.systemAppearance(),
	}

	// The three colours the terminal owns keep its own values, so a transparent terminal
	// stays transparent. A colour from `[ui]` still wins.
	kept := func(colors *ThemeColors) {
		if _, overridden := registry.overrides.Colors["background"]; !overridden {
			colors.Background = derived.Background
		}
		if _, overridden := registry.overrides.Colors["panel"]; !overridden {
			colors.Panel = derived.Panel
		}
		if _, overridden := registry.overrides.Colors["text"]; !overridden {
			colors.Text = derived.Text
		}
	}
	theme, faults := buildTheme(merged, kept)
	return ResolvedTheme{Theme: theme, Problems: append(problems, faults...)}
}
