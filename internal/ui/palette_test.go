package ui

import "testing"

// Every row the palette offers has to run something. A row whose id names no action is
// drawn like any other, and reports that there is no such action only once the user has
// chosen it, so the fault is found by the person using the client and not by the suite.
func TestEveryPaletteEntryRunsSomething(t *testing.T) {
	// The ids the palette handles itself, before it looks for an action of that name.
	handled := map[string]bool{
		"copy-plan": true, "reload-themes": true,
		"ai-explain-query": true, "ai-optimize-query": true,
		configProblemsAction: true,
	}
	// The rows that move the result pane to one of its views, which the palette resolves
	// against the views themselves and not against an action.
	for _, view := range paletteViews {
		handled["tab-"+string(view)] = true
	}

	for _, entry := range paletteEntries {
		if handled[entry.id] {
			continue
		}
		if _, known := FindActionID(entry.id); !known {
			t.Errorf("the palette row %q names no action", entry.id)
		}
	}
}

// A row that names an action has to name its own. An id that names one action while the row
// runs another would run the wrong thing, which is worse than running nothing.
func TestEveryPaletteEntryNamesItsOwnAction(t *testing.T) {
	for _, entry := range paletteEntries {
		if entry.action == "" {
			continue
		}
		if entry.id != string(entry.action) {
			t.Errorf("the palette row %q runs %q; the id and the action have to be the same",
				entry.id, entry.action)
		}
	}
}
