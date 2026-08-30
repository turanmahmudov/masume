package app

import (
	"testing"

	"github.com/turanmahmudov/masume/internal/db"
)

func TestFindColumnChoicesAnswersTheValuesAColumnTakes(t *testing.T) {
	target := EditTarget{Columns: []db.ColumnDetail{
		{Name: "state", DataType: "shipping_state", Choices: []string{"new", "sent"}},
		{Name: "express", DataType: "bool"},
		{Name: "flagged", DataType: "TinyInt(1)"},
		{Name: "code", DataType: "text"},
		{Name: "total", DataType: "numeric", IsGenerated: true},
	}}

	for _, held := range []struct {
		name   string
		wanted []string
	}{
		{"state", []string{"new", "sent"}},
		{"STATE", []string{"new", "sent"}},
		{"express", []string{"true", "false"}},
		{"flagged", []string{"true", "false"}},
		{"code", nil},
		{"nosuch", nil},
	} {
		answered := target.FindColumnChoices(held.name)
		if len(answered) != len(held.wanted) {
			t.Errorf("%s answers %v, wanted %v", held.name, answered, held.wanted)
			continue
		}
		for at, value := range held.wanted {
			if answered[at] != value {
				t.Errorf("%s answers %v, wanted %v", held.name, answered, held.wanted)
				break
			}
		}
	}
}

func TestFindColumnProblemNamesAColumnTheServerFills(t *testing.T) {
	target := EditTarget{Columns: []db.ColumnDetail{
		{Name: "total", IsGenerated: true},
		{Name: "code"},
	}}

	if problem := target.FindColumnProblem("total"); problem == "" {
		t.Error("a generated column reports no problem")
	}
	if problem := target.FindColumnProblem("TOTAL"); problem == "" {
		t.Error("the name is read with its own case")
	}
	if problem := target.FindColumnProblem("code"); problem != "" {
		t.Errorf("a plain column reports %q", problem)
	}
}
