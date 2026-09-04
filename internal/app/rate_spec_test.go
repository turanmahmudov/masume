package app_test

import (
	"testing"
	"time"

	"github.com/turanmahmudov/masume/internal/app"
)

// A counter of a server counts up from the moment it started, so a rate is the difference
// between two readings over the time between them.
func TestResolveCounterRateMeasuresBetweenTwoReadings(t *testing.T) {
	rate, held := app.ResolveCounterRate(1000, 1040, 2*time.Second)
	if !held {
		t.Fatal("two readings two seconds apart measured nothing")
	}
	if rate != 20 {
		t.Errorf("forty in two seconds reads %v a second, wanted 20", rate)
	}
}

// A counter that has not moved is a rate of zero, which is a number a reader can act on.
// That is not the same as having nothing to measure.
func TestResolveCounterRateReportsAZeroRate(t *testing.T) {
	rate, held := app.ResolveCounterRate(1000, 1000, 2*time.Second)
	if !held || rate != 0 {
		t.Errorf("a counter that did not move reads %v, %v, wanted a rate of zero", rate, held)
	}
}

// A server that restarted begins its counters again, so the second reading is lower than
// the first. That fall is not a rate and reporting it as one would draw a number that never
// happened.
func TestResolveCounterRateReportsNothingForACounterThatFell(t *testing.T) {
	if _, held := app.ResolveCounterRate(9000, 12, 2*time.Second); held {
		t.Error("a counter that fell was read as a rate")
	}
}

// Two readings at the same moment have no time between them to divide by.
func TestResolveCounterRateReportsNothingWithoutTimeBetween(t *testing.T) {
	for _, span := range []time.Duration{0, -time.Second} {
		if _, held := app.ResolveCounterRate(10, 20, span); held {
			t.Errorf("a span of %v was read as a rate", span)
		}
	}
}
