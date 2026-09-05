package ui

import (
	"image/color"
	"testing"
)

// markContrastFloorOfTheme is the least a mark of a theme may stand at against a pane. It is
// lower than the floor of ordinary text, because a mark carries its meaning by hue as well
// and is never a whole line of reading.
const markContrastFloorOfTheme = 3.0

// Every theme has to be readable on its own pane: the two steps of text that carry words,
// and every mark drawn in a colour of its own. A theme whose accents were tuned against a
// ground of text rather than against a pane fails here rather than on a user's screen.
func TestEveryThemeReadsOnItsOwnPane(t *testing.T) {
	model := buildOfflineModel(t, 120, 34)
	for _, choice := range model.styles.registry.ListThemeChoices() {
		problems, applied := model.styles.ApplyThemeByName(choice.Name)
		if !applied || len(problems) > 0 {
			t.Errorf("theme %q reports %v", choice.Name, problems)
			continue
		}
		theme := model.styles.Theme
		check := func(label string, ink color.Color, floor float64) {
			t.Helper()
			if stood := CalculateContrastRatio(ink, theme.Panel); stood < floor {
				t.Errorf(
					"the %s of theme %q stands at %.2f against the pane, and %.2f is the least",
					label, choice.Name, stood, floor)
			}
		}
		check("text", theme.Text, TextContrastFloor)
		check("muted text", theme.Muted, TextContrastFloor)
		check("faint text", theme.Faint, markContrastFloorOfTheme)
		for _, mark := range []struct {
			label string
			ink   color.Color
		}{
			{"accent", theme.Accent}, {"second accent", theme.AccentAlt},
			{"warm accent", theme.AccentWarm}, {"info", theme.Info},
			{"success", theme.Success}, {"warning", theme.Warning},
			{"danger", theme.Danger}, {"error", theme.Error},
		} {
			check(mark.label, mark.ink, markContrastFloorOfTheme)
		}
		// The ink of a filled accent is picked against the accent, so it is measured
		// against the accent and not against the pane.
		if stood := CalculateContrastRatio(
			theme.OnAccent, theme.Accent); stood < TextContrastFloor {
			t.Errorf("the ink on the accent of theme %q stands at %.2f", choice.Name, stood)
		}
	}
}
