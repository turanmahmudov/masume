// Package ui draws the client: the theme, the keys, the screens and the panes.
package ui

import (
	"fmt"
	"image/color"
	"math"

	"charm.land/lipgloss/v2"
)

// The arithmetic for a derived colour: a step between two colours, and whether the result
// can be read on its ground.

// TextContrastFloor is the least contrast text on a filled ground must have.
const TextContrastFloor = 4.5

// How far a colour can move before it stops, and how far one step moves.
const (
	liftSteps  = 8
	liftWeight = 0.12
)

var (
	blackInk = lipgloss.Color("#000000")
	whiteInk = lipgloss.Color("#ffffff")
)

// readChannels returns the three channels of a colour, each from 0 to 255.
func readChannels(held color.Color) (int, int, int) {
	if held == nil {
		return 0, 0, 0
	}
	red, green, blue, alpha := held.RGBA()
	if alpha == 0 {
		return 0, 0, 0
	}
	return int(red >> 8), int(green >> 8), int(blue >> 8)
}

// WriteHex writes a colour as a theme file does.
func WriteHex(held color.Color) string {
	red, green, blue := readChannels(held)
	return fmt.Sprintf("#%02x%02x%02x", red, green, blue)
}

// MixColors returns a colour at weight between the first and the second. It is mixed in
// the space the terminal uses, not a linear one, because a theme picks its own steps that
// way too.
func MixColors(from, to color.Color, weight float64) color.Color {
	fromRed, fromGreen, fromBlue := readChannels(from)
	toRed, toGreen, toBlue := readChannels(to)
	held := math.Min(1, math.Max(0, weight))
	step := func(start, end int) int {
		return int(math.Round(float64(start) + (float64(end)-float64(start))*held))
	}
	return lipgloss.RGBColor{
		R: uint8(step(fromRed, toRed)),
		G: uint8(step(fromGreen, toGreen)),
		B: uint8(step(fromBlue, toBlue)),
	}
}

// expandChannel removes the terminal gamma from one channel, as WCAG defines it.
func expandChannel(value int) float64 {
	share := float64(value) / 255
	if share <= 0.03928 {
		return share / 12.92
	}
	return math.Pow((share+0.055)/1.055, 2.4)
}

// CalculateRelativeLuminance returns how bright the colour is, from 0 to 1.
func CalculateRelativeLuminance(held color.Color) float64 {
	red, green, blue := readChannels(held)
	return 0.2126*expandChannel(red) + 0.7152*expandChannel(green) + 0.0722*expandChannel(blue)
}

// CalculateContrastRatio returns the contrast of two colours, from 1 for two equal colours
// to 21 for black against white.
func CalculateContrastRatio(one, other color.Color) float64 {
	first := CalculateRelativeLuminance(one)
	second := CalculateRelativeLuminance(other)
	return (math.Max(first, second) + 0.05) / (math.Min(first, second) + 0.05)
}

// PickReadableInk returns black or white, whichever reads better on the given ground.
func PickReadableInk(ground color.Color) color.Color {
	if CalculateContrastRatio(ground, blackInk) >= CalculateContrastRatio(ground, whiteInk) {
		return blackInk
	}
	return whiteInk
}

// PickInkFor returns the ink for text on a filled ground. The better of the two theme
// colours is used first, so the text stays in the palette of the theme. Black or white is
// used if neither can be read.
func PickInkFor(ground, quiet, loud color.Color, floor float64) color.Color {
	nearer := loud
	if CalculateContrastRatio(ground, quiet) >= CalculateContrastRatio(ground, loud) {
		nearer = quiet
	}
	if CalculateContrastRatio(ground, nearer) >= floor {
		return nearer
	}
	return PickReadableInk(ground)
}

// CalculateHueAngle returns the angle of the colour on the wheel, from 0 for red to 120
// for green. A grey gives 0.
func CalculateHueAngle(held color.Color) float64 {
	red, green, blue := readChannels(held)
	high := math.Max(float64(red), math.Max(float64(green), float64(blue)))
	low := math.Min(float64(red), math.Min(float64(green), float64(blue)))
	if high == low {
		return 0
	}
	span := high - low

	var sixth float64
	switch high {
	case float64(red):
		sixth = (float64(green) - float64(blue)) / span
	case float64(green):
		sixth = 2 + (float64(blue)-float64(red))/span
	default:
		sixth = 4 + (float64(red)-float64(green))/span
	}
	return math.Mod(math.Mod(sixth*60, 360)+360, 360)
}

// CalculateSaturation returns how far the colour is from a grey, from 0 to 1.
func CalculateSaturation(held color.Color) float64 {
	red, green, blue := readChannels(held)
	high := math.Max(float64(red), math.Max(float64(green), float64(blue)))
	if high == 0 {
		return 0
	}
	low := math.Min(float64(red), math.Min(float64(green), float64(blue)))
	return (high - low) / high
}

// CalculateHueDistance returns the shorter distance round the wheel between two angles,
// from 0 to 180.
func CalculateHueDistance(one, other float64) float64 {
	apart := math.Mod(math.Abs(one-other), 360)
	if apart > 180 {
		return 360 - apart
	}
	return apart
}

// searchSteps is how many times the range is halved to find a step at the wanted contrast.
const searchSteps = 14

// ResolveColorAtContrast returns the step between two colours at the wanted contrast
// against the ground. A mix from the first toward the second moves the contrast one way
// only, so the weight is found by halving the range. The three text steps are spread this
// way, because a terminal with little contrast has no room for a fixed floor.
func ResolveColorAtContrast(from, toward, ground color.Color, target float64) color.Color {
	low := 0.0
	high := 1.0
	for range searchSteps {
		middle := (low + high) / 2
		if CalculateContrastRatio(MixColors(from, toward, middle), ground) > target {
			low = middle
			continue
		}
		high = middle
	}
	return MixColors(from, toward, (low+high)/2)
}

// RaiseContrast moves the colour toward the ink until it reaches the floor against the
// ground. A terminal can set a palette colour close to its own background, and a mark in
// that colour cannot be read.
func RaiseContrast(held, ground, ink color.Color, floor float64) color.Color {
	raised := held
	for range liftSteps {
		if CalculateContrastRatio(raised, ground) >= floor {
			break
		}
		raised = MixColors(raised, ink, liftWeight)
	}
	return raised
}
