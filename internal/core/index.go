package core

// Index arithmetic shared by every list on screen. A row number stays inside the list,
// and a caret stays inside its line.

// ClampIndex returns a row number inside a list of count rows. An empty list gives zero.
func ClampIndex(index, count int) int {
	if count <= 0 {
		return 0
	}
	return ClampWithin(index, count-1)
}

// WrapIndex returns a row number that wraps. One row after the last row is the first row.
func WrapIndex(index, count int) int {
	if count <= 0 {
		return 0
	}
	return ((index % count) + count) % count
}

// ClampWithin returns a position between zero and highest, both included. A caret can
// stand after the last character.
func ClampWithin(position, highest int) int {
	if position < 0 {
		return 0
	}
	if position > highest {
		return highest
	}
	return position
}
