package present_test

import (
	"testing"

	"github.com/turanmahmudov/masume/internal/present"
)

func TestMatchesTextIgnoresLetterCase(t *testing.T) {
	if !present.MatchesText("CustomerName", "name") {
		t.Error("a name that holds the typed text was missed")
	}
	if present.MatchesText("CustomerName", "xyz") {
		t.Error("a name that does not hold the typed text matched")
	}
	if !present.MatchesText("anything", "") {
		t.Error("an empty needle missed")
	}
}

func TestFindMaskedColumnsHidesSecretNames(t *testing.T) {
	masked := present.FindMaskedColumns(
		[]string{"id", "password_hash", "shipping_pin", "api_key"},
		present.DefaultMasking())
	if !masked[1] || !masked[3] {
		t.Errorf("secret columns were not masked: %v", masked)
	}
	if masked[0] {
		t.Error("id was masked")
	}
	if masked[2] {
		t.Error("shipping_pin was masked, and pin is left out of the default names")
	}
}

func TestFindMaskedColumnsDoesNothingWhenOff(t *testing.T) {
	masked := present.FindMaskedColumns(
		[]string{"password"},
		present.MaskingRules{Enabled: false, Names: []string{"password"}})
	if len(masked) != 0 {
		t.Errorf("masking that is off still hid columns: %v", masked)
	}
}
