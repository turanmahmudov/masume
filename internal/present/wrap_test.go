package present

import (
	"strings"
	"testing"
)

// A word may end on the last cell of a row only where the text ends with it. Each case here
// was read from a drawn frame.
func TestWrapWordsKeepsRoomForTheFollowingSpace(t *testing.T) {
	for _, held := range []struct {
		name  string
		text  string
		width int
		rows  []string
	}{
		{
			name: "a word that would fill the row keeps the space after it",
			text: "the values must be a JSON object · Ctrl+R run · " +
				"Ctrl+F prettify JSON · Esc cancel",
			width: 68,
			rows: []string{
				"the values must be a JSON object · Ctrl+R run · Ctrl+F prettify",
				"JSON · Esc cancel",
			},
		},
		{
			name:  "the last word of a text may end on the last cell",
			text:  "update shipment set note = 'aaa' where code <> 'bbbbbbbbbbbbbbbbbbbbbbb'",
			width: 72,
			rows:  []string{"update shipment set note = 'aaa' where code <> 'bbbbbbbbbbbbbbbbbbbbbbb'"},
		},
		{
			name:  "the same word breaks where more text follows it",
			text:  "update shipment set note = 'aaa' where code <> 'bbbbbbbbbbbbbbbbbbbbbbb' and id > 0",
			width: 72,
			rows: []string{
				"update shipment set note = 'aaa' where code <>",
				"'bbbbbbbbbbbbbbbbbbbbbbb' and id > 0",
			},
		},
		{
			name: "the keys of the cell editor",
			text: "Ctrl+S save · Ctrl+F prettify JSON · Ctrl+L NULL · Ctrl+E empty · " +
				"Ctrl+D default · Esc cancel",
			width: 92,
			rows: []string{
				"Ctrl+S save · Ctrl+F prettify JSON · Ctrl+L NULL · " +
					"Ctrl+E empty · Ctrl+D default · Esc",
				"cancel",
			},
		},
		{
			name:  "a text that fits takes one row",
			text:  "Ctrl+R run · Ctrl+F prettify JSON · Esc cancel",
			width: 68,
			rows:  []string{"Ctrl+R run · Ctrl+F prettify JSON · Esc cancel"},
		},
		{
			name:  "an indented line keeps its indent",
			text:  `   "key1": 1,`,
			width: 92,
			rows:  []string{`   "key1": 1,`},
		},
		{
			name:  "an indented line that wraps keeps it on the first row",
			text:  "  alpha beta gamma",
			width: 12,
			rows:  []string{"  alpha", "beta gamma"},
		},
		{
			name:  "a word longer than the row is broken",
			text:  strings.Repeat("x", 10),
			width: 4,
			rows:  []string{"xxxx", "xxxx", "xx"},
		},
	} {
		answered := WrapWords(held.text, held.width)
		if len(answered) != len(held.rows) {
			t.Errorf("%s: answers %q, wanted %q", held.name, answered, held.rows)
			continue
		}
		for at, row := range held.rows {
			if answered[at] != row {
				t.Errorf("%s: row %d answers %q, wanted %q", held.name, at, answered[at], row)
			}
		}
	}
}
