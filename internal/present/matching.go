package present

import (
	"strings"
)

// MatchesSubsequence is true if every typed character appears in the candidate in order,
// so `t88` finds `tenant_88231`.
func MatchesSubsequence(candidate, needle string) bool {
	if needle == "" {
		return true
	}
	haystack := strings.ToLower(candidate)
	wanted := strings.ToLower(needle)

	at := 0
	for _, character := range wanted {
		found := strings.IndexRune(haystack[at:], character)
		if found == -1 {
			return false
		}
		at += found + len(string(character))
	}
	return true
}

// MatchesText is true where the candidate holds the typed text, letter case aside. A filter
// over a list reads this, where the object tree reads a subsequence.
func MatchesText(candidate, needle string) bool {
	if needle == "" {
		return true
	}
	return strings.Contains(strings.ToLower(candidate), strings.ToLower(needle))
}

// MaskedDisplay is drawn in place of a hidden value.
const MaskedDisplay = "••••••"

// DefaultMaskedNames are matched anywhere in the name, in any case. `pin` is left out,
// because it is inside `shipping`.
var DefaultMaskedNames = []string{
	"password", "passwd", "secret", "token", "api_key", "apikey", "private_key",
	"credit_card", "card_number", "cvv", "iban", "ssn",
}

// MaskingRules is the masking setting: whether it is on, and the names that trigger it.
type MaskingRules struct {
	Enabled bool
	Names   []string
}

// DefaultMasking hides the value of a column whose name suggests a secret. The value is
// still read, and a copy or an export writes it out.
func DefaultMasking() MaskingRules {
	return MaskingRules{Enabled: true, Names: DefaultMaskedNames}
}

// FindMaskedColumns is read once per result, not per cell, because it is the same for
// every row.
func FindMaskedColumns(columnNames []string, rules MaskingRules) map[int]bool {
	masked := map[int]bool{}
	if !rules.Enabled {
		return masked
	}
	// The markers are put in lower case once here, and not once for every column.
	markers := make([]string, 0, len(rules.Names))
	for _, marker := range rules.Names {
		if marker != "" {
			markers = append(markers, strings.ToLower(marker))
		}
	}

	for index, name := range columnNames {
		lowered := strings.ToLower(name)
		for _, marker := range markers {
			if strings.Contains(lowered, marker) {
				masked[index] = true
				break
			}
		}
	}
	return masked
}
