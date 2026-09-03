package present

import (
	"strings"
)

// MatchesSubsequence is true if every typed character is in the candidate in the same order,
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

// MatchesText is true if the candidate contains the typed text, without case comparison. A
// filter over a list uses this, and the object tree uses a subsequence.
func MatchesText(candidate, needle string) bool {
	if needle == "" {
		return true
	}
	return strings.Contains(strings.ToLower(candidate), strings.ToLower(needle))
}

// MaskedDisplay is drawn in place of a hidden value.
const MaskedDisplay = "••••••"

// DefaultMaskedNames match at any position in the name, without case comparison. `pin` is
// not in the list, because it is a part of `shipping`.
var DefaultMaskedNames = []string{
	"password", "passwd", "secret", "token", "api_key", "apikey", "private_key",
	"credit_card", "card_number", "cvv", "iban", "ssn",
}

// MaskingRules is the masking setting: the on or off state, and the names that select a
// column.
type MaskingRules struct {
	Enabled bool
	Names   []string
}

// DefaultMasking hides the value of a column whose name indicates a secret. The value is
// still read, and a copy or an export contains it.
func DefaultMasking() MaskingRules {
	return MaskingRules{Enabled: true, Names: DefaultMaskedNames}
}

// FindMaskedColumns runs one time per result and not per cell, because the result is the
// same for every row.
func FindMaskedColumns(columnNames []string, rules MaskingRules) map[int]bool {
	masked := map[int]bool{}
	if !rules.Enabled {
		return masked
	}
	// The names are converted to lower case one time here and not for every column.
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
