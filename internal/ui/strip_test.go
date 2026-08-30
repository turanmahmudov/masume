package ui

import "github.com/charmbracelet/x/ansi"

// stripEscapes answers the text of a drawn line, without the escapes that colour it. Only
// the tests read a drawn line back as text.
func stripEscapes(line string) string {
	return ansi.Strip(line)
}
