package ui

import "testing"

func TestCountCardBodyRowsTakesOffWhatSurroundsTheContent(t *testing.T) {
	for _, held := range []struct {
		name         string
		height, hint int
		wanted       int
	}{
		{"a card with no keys keeps its borders and its blank rows", 12, 0, 8},
		{"the keys take their rows and the blank row over them", 12, 1, 6},
		{"keys over two rows take one more", 12, 2, 5},
		{"a card too small for its content keeps one row", 5, 3, 1},
	} {
		if answered := countCardBodyRows(held.height, held.hint); answered != held.wanted {
			t.Errorf("%s: answers %d, wanted %d", held.name, answered, held.wanted)
		}
	}
}
