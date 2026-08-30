package ui

import (
	"testing"
	"time"
)

func TestClickCounter(t *testing.T) {
	counter := &clickCounter{}
	start := time.Unix(0, 0)

	if run := counter.count("row-1", start); run != 1 {
		t.Errorf("the first click counted %d, wanted 1", run)
	}
	if run := counter.count("row-1", start.Add(100*time.Millisecond)); run != 2 {
		t.Errorf("a second click counted %d, wanted 2", run)
	}
	if run := counter.count("row-2", start.Add(150*time.Millisecond)); run != 1 {
		t.Errorf("another target counted %d, wanted 1", run)
	}
	if run := counter.count("row-2", start.Add(2*time.Second)); run != 1 {
		t.Errorf("a late click counted %d, wanted 1", run)
	}
}
