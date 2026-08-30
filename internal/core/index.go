package core

// The arithmetic every list on screen shares: a row number is held inside the list, and a
// caret is held inside the line it stands on.

// ClampIndex returns a row number inside a list of count rows. An empty list gives zero.
func ClampIndex(index, count int) int {
	if count <= 0 {
		return 0
	}
	return ClampWithin(index, count-1)
}

// WrapIndex returns a row number that wraps: one past the last row is the first row.
func WrapIndex(index, count int) int {
	if count <= 0 {
		return 0
	}
	return ((index % count) + count) % count
}

// ClampWithin returns a position from zero to highest, both allowed, as a caret
// can stand after the last character.
func ClampWithin(position, highest int) int {
	if position < 0 {
		return 0
	}
	if position > highest {
		return highest
	}
	return position
}
