package ui

// The rows a dialog form draws. The export form and the import form are both built from
// these, so a row is stepped through and typed into the same way in each.

// DialogField is one row of a form a dialog draws.
type DialogField struct {
	Key   string
	Label string
	Value string
	// Choices are the values the field steps through. A typed field has none.
	Choices []string
}

// The two answers a yes-or-no field steps through.
var yesOrNo = []string{"yes", "no"}

// describeYesOrNo returns a flag as the word a form shows it with.
func describeYesOrNo(held bool) string {
	if held {
		return "yes"
	}
	return "no"
}
