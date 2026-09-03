package present_test

import (
	"testing"
	"time"

	"github.com/turanmahmudov/masume/internal/present"
)

func TestFormatWhenShowsTheDateOnlyForAnotherDay(t *testing.T) {
	// A list of today shows the seconds, because two rows can be in the same minute. A
	// row from another day shows the date, so it is never read as today.
	now := time.Date(2026, 3, 14, 15, 30, 0, 0, time.UTC)
	for _, held := range []struct {
		name string
		at   time.Time
		want string
	}{
		{"the same day", time.Date(2026, 3, 14, 9, 5, 7, 0, time.UTC), "09:05:07"},
		{"a moment later the same day", now, "15:30:00"},
		{"the day before", time.Date(2026, 3, 13, 9, 5, 7, 0, time.UTC), "03-13 09:05"},
		{"the same day of another year", time.Date(2025, 3, 14, 9, 5, 0, 0, time.UTC),
			"03-14 09:05"},
	} {
		t.Run(held.name, func(t *testing.T) {
			if got := present.FormatWhen(held.at, now); got != held.want {
				t.Errorf("got %q, want %q", got, held.want)
			}
		})
	}
}

func TestFormatStagedChangesAgreesInNumber(t *testing.T) {
	for _, held := range []struct {
		count int
		want  string
	}{
		{0, "0 changes staged"},
		{1, "1 change staged"},
		{2, "2 changes staged"},
		{1234, "1,234 changes staged"},
	} {
		if got := present.FormatStagedChanges(held.count); got != held.want {
			t.Errorf("FormatStagedChanges(%d) = %q, want %q", held.count, got, held.want)
		}
	}
}

func TestDescribeStagedChangesTakesTheVerbThatAgrees(t *testing.T) {
	for _, held := range []struct {
		count int
		want  string
	}{
		{1, "1 change staged that was never applied"},
		{2, "2 changes staged that were never applied"},
		{0, "0 changes staged that were never applied"},
	} {
		if got := present.DescribeStagedChanges(held.count); got != held.want {
			t.Errorf("DescribeStagedChanges(%d) = %q, want %q", held.count, got, held.want)
		}
	}
}

func TestFormatEstimatedRowsStaysShortEnoughForTheDetailColumn(t *testing.T) {
	for _, held := range []struct {
		count int64
		want  string
	}{
		{0, ""},
		{-1, ""},
		{1, "1"},
		{999, "999"},
		{1_000, "1.0k"},
		{1_500, "1.5k"},
		{999_999, "1000.0k"},
		{1_000_000, "1.0M"},
		{2_500_000, "2.5M"},
	} {
		if got := present.FormatEstimatedRows(held.count); got != held.want {
			t.Errorf("FormatEstimatedRows(%d) = %q, want %q", held.count, got, held.want)
		}
	}
}

func TestFormatAgeWritesHowLongAgoAsOneUnit(t *testing.T) {
	for _, held := range []struct {
		name    string
		elapsed time.Duration
		want    string
	}{
		{"a moment", 0, "just now"},
		{"a time still to come", -5 * time.Second, "just now"},
		{"under a minute", 59 * time.Second, "just now"},
		{"a minute", 60 * time.Second, "1m ago"},
		{"minutes", 45 * time.Minute, "45m ago"},
		{"an hour", 60 * time.Minute, "1h ago"},
		{"hours", 5 * time.Hour, "5h ago"},
		{"a day", 24 * time.Hour, "1d ago"},
		{"days", 72 * time.Hour, "3d ago"},
	} {
		t.Run(held.name, func(t *testing.T) {
			if got := present.FormatAge(held.elapsed); got != held.want {
				t.Errorf("got %q, want %q", got, held.want)
			}
		})
	}
}

func TestMatchesTextReadsTheTypedTextAnywhereInTheCandidate(t *testing.T) {
	for _, held := range []struct {
		name      string
		candidate string
		needle    string
		want      bool
	}{
		{"an empty term matches everything", "orders", "", true},
		{"the start", "orders", "ord", true},
		{"the middle", "orders", "rde", true},
		{"any case", "Orders", "ORD", true},
		{"a subsequence is not enough", "orders", "odr", false},
		{"text that is not there", "orders", "zzz", false},
		{"an empty candidate", "", "a", false},
	} {
		t.Run(held.name, func(t *testing.T) {
			got := present.MatchesText(held.candidate, held.needle)
			if got != held.want {
				t.Errorf("MatchesText(%q, %q) = %v, want %v",
					held.candidate, held.needle, got, held.want)
			}
		})
	}
}
