package app

import "testing"

func TestResolveDrawnViewFallsBackToTheFirstOffered(t *testing.T) {
	outcome := []ResultView{ViewStatistics, ViewPlan}
	query := []ResultView{ViewData, ViewFields, ViewPlan}

	for _, held := range []struct {
		name    string
		offered []ResultView
		asked   ResultView
		wanted  ResultView
	}{
		{"a view the tab offers is drawn", query, ViewFields, ViewFields},
		{"a statement with no rows draws its statistics", outcome, ViewData, ViewStatistics},
		{"a view the tab offers is kept", outcome, ViewPlan, ViewPlan},
		{"a tab that offers nothing draws its data", nil, ViewPlan, ViewData},
	} {
		if answered := ResolveDrawnView(held.offered, held.asked); answered != held.wanted {
			t.Errorf("%s: answers %s, wanted %s", held.name, answered, held.wanted)
		}
	}
}
