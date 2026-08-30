package present_test

import (
	"testing"

	"github.com/turanmahmudov/masume/internal/present"
)

// The report names how many changes went, and the verb agrees with the count: a reader who
// staged one change reads about one.
func TestDescribeDroppedChangesAgreesWithTheCount(t *testing.T) {
	cases := []struct {
		count int
		want  string
	}{
		{count: 1, want: "1 staged change was dropped with the rows it named"},
		{count: 2, want: "2 staged changes were dropped with the rows they named"},
		{count: 1200, want: "1,200 staged changes were dropped with the rows they named"},
	}
	for _, held := range cases {
		if found := present.DescribeDroppedChanges(held.count); found != held.want {
			t.Errorf("%d changes read as %q, wanted %q", held.count, found, held.want)
		}
	}
}
